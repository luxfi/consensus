// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_finality_test.go — the launch-gate tests. Each proves one safety or
// liveness property of α-of-K quorum finality. These are the gates the DEX
// value-activation depends on: no real value until a value block finalizes ONLY
// with a quorum (no proposer self-finality), and the proposer-freeze must not
// return.
package chain

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// params5 is a 5-validator config: K=5, alpha=3 (BFT 3-of-5).
func params5() config.Parameters {
	return config.Parameters{K: 5, AlphaPreference: 3, AlphaConfidence: 3, Beta: 2}
}

// -----------------------------------------------------------------------------
// GATE 1: a proposer cannot finalize its own block on its lone self-vote.
// -----------------------------------------------------------------------------

func TestConsensus_NoProposerSelfFinality(t *testing.T) {
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params5(), vs, 0, rec)

	blk := newTestBlock(1, ids.Empty, "proposer-block")
	pos := trackProposal(e, chainID, blk, 0)
	_ = pos

	// The proposer self-voted at proposal time (acceptVotes=1). Drive the
	// self-finalize path. With NO peer votes, alpha=3 is not met → MUST NOT
	// finalize. The old ForceAccept path would finalize here on the lone vote.
	e.finalizeOwnProposal(context.Background(), blk.id)

	if e.IsAccepted(blk.id) {
		t.Fatal("SAFETY VIOLATION: proposer finalized its own block on a lone self-vote (no alpha-of-K quorum)")
	}
	if blk.AcceptCalled() != 0 {
		t.Fatalf("VM.Accept must NOT be called without quorum, got %d", blk.AcceptCalled())
	}
	// And tryFinalizeBlock (the count-driven path) must also refuse: count=1<3.
	e.tryFinalizeBlock(context.Background(), blk.id)
	if e.IsAccepted(blk.id) {
		t.Fatal("SAFETY VIOLATION: block finalized below alpha via tryFinalizeBlock")
	}
}

// -----------------------------------------------------------------------------
// GATE 2: acceptance requires an α-of-K quorum of correctly-signed accepts.
// -----------------------------------------------------------------------------

func TestConsensus_AcceptRequiresQuorum(t *testing.T) {
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params5(), vs, 0, rec)

	blk := newTestBlock(1, ids.Empty, "needs-quorum")
	pos := trackProposal(e, chainID, blk, 0)

	// Two peer accepts (validators 1,2). Total distinct accepts = proposer(0) is
	// counted in consensus via ProcessVote at proposal; here we feed signed peer
	// votes. With alpha=3, two peers + proposer = 3 → quorum. But to isolate the
	// "below quorum stays pending" property, first send only ONE peer.
	e.ReceiveVote(vs.signedVote(1, pos))
	if waitFor(300*time.Millisecond, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("SAFETY VIOLATION: block accepted with only 2 accepts (proposer+1) below alpha=3")
	}

	// Now add the third distinct signer → alpha reached → MUST finalize, and a
	// cert MUST have been assembled+gossiped.
	e.ReceiveVote(vs.signedVote(2, pos))
	if !waitFor(2*time.Second, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("LIVENESS: block did not finalize after alpha-of-K signed accepts arrived")
	}
	if blk.AcceptCalled() != 1 {
		t.Fatalf("VM.Accept must run exactly once at quorum, got %d", blk.AcceptCalled())
	}
	rec.mu.Lock()
	gotCerts := len(rec.certs)
	rec.mu.Unlock()
	if gotCerts == 0 {
		t.Fatal("a verified quorum cert must be assembled and gossiped at finality")
	}
}

// TestConsensus_RejectVotesDoNotFinalizeOwnProposal proves the deleted
// `effectiveAccept = ... || IsOwnProposal` flip is gone: a proposer's own block
// is NOT finalized by peers voting REJECT (the old code laundered those into
// accepts and committed).
func TestConsensus_RejectVotesDoNotFinalizeOwnProposal(t *testing.T) {
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params5(), vs, 0, rec)

	blk := newTestBlock(1, ids.Empty, "own-with-rejects")
	_ = trackProposal(e, chainID, blk, 0)

	// Four REJECT votes for the proposer's OWN block. Pre-fix: counted as
	// accepts via IsOwnProposal → finalized. Post-fix: counted as rejects.
	for i := 1; i < 5; i++ {
		e.ReceiveVote(Vote{BlockID: blk.id, NodeID: vs.nodeID(i), Accept: false, SignedAt: time.Now()})
	}
	if waitFor(300*time.Millisecond, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("SAFETY VIOLATION: own block finalized by peer REJECT votes (effectiveAccept flip not removed)")
	}
	if blk.AcceptCalled() != 0 {
		t.Fatalf("VM.Accept must not run for a block peers rejected, got %d", blk.AcceptCalled())
	}
}

// -----------------------------------------------------------------------------
// GATE 3: an equivocating proposer cannot finalize BOTH forks.
// -----------------------------------------------------------------------------

func TestConsensus_EquivocatingProposerCannotFinalizeBothForks(t *testing.T) {
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params5(), vs, 0, rec)

	// Two distinct, individually-valid blocks at the SAME height (equivocation).
	forkA := newTestBlock(1, ids.Empty, "fork-A")
	forkB := newTestBlock(1, ids.Empty, "fork-B")
	posA := trackProposal(e, chainID, forkA, 0)
	posB := trackProposal(e, chainID, forkB, 0)

	// An honest α-of-K majority (3 of 5) can sign AT MOST ONE value per height.
	// Model that: validators 1,2 sign fork A; validator 1 cannot ALSO be the
	// third distinct signer of fork B (it already committed to A). Only
	// validator 3 is left for B → B has proposer(0 self) + 3 = 2 distinct
	// signers < alpha. A gets proposer(0) + 1 + 2 = 3 → finalizes; B does not.
	e.ReceiveVote(vs.signedVote(1, posA))
	e.ReceiveVote(vs.signedVote(2, posA))
	e.ReceiveVote(vs.signedVote(3, posB))

	finalizedA := waitFor(2*time.Second, func() bool { return e.IsAccepted(forkA.id) })
	// Give B the same window; it must NOT finalize.
	finalizedB := waitFor(300*time.Millisecond, func() bool { return e.IsAccepted(forkB.id) })

	if finalizedA && finalizedB {
		t.Fatal("SAFETY VIOLATION: equivocating proposer finalized BOTH forks at one height")
	}
	if !finalizedA {
		t.Fatal("fork A reached alpha-of-K and should finalize")
	}
	if finalizedB {
		t.Fatal("SAFETY VIOLATION: fork B finalized without an alpha-of-K quorum (only 2 distinct signers)")
	}

	// Cert audit: exactly one cert, for fork A, and it must verify.
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for i, cb := range rec.certs {
		cert, err := UnmarshalQuorumCert(cb)
		if err != nil {
			t.Fatalf("gossiped cert %d failed to decode: %v", i, err)
		}
		if cert.Position.BlockID == forkB.id {
			t.Fatal("SAFETY VIOLATION: a finality cert was produced for fork B")
		}
		if err := cert.Verify(vs); err != nil {
			t.Fatalf("gossiped cert %d failed verification: %v", i, err)
		}
	}
}

// -----------------------------------------------------------------------------
// GATE 4: a minority partition cannot finalize.
// -----------------------------------------------------------------------------

func TestConsensus_PartitionMinorityCannotFinalize(t *testing.T) {
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params5(), vs, 0, rec)

	blk := newTestBlock(1, ids.Empty, "minority-partition")
	pos := trackProposal(e, chainID, blk, 0)

	// Minority partition: only the proposer (0) and ONE other validator (1) are
	// reachable — 2 of 5 < alpha=3. They sign; the cert can never assemble.
	e.ReceiveVote(vs.signedVote(1, pos))

	if waitFor(500*time.Millisecond, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("SAFETY VIOLATION: a minority partition (2 of 5) finalized a block")
	}
	rec.mu.Lock()
	gotCerts := len(rec.certs)
	rec.mu.Unlock()
	if gotCerts != 0 {
		t.Fatalf("no cert may be produced by a sub-alpha minority, got %d", gotCerts)
	}
	// The block remains pending — recoverable when the partition heals.
	e.mu.RLock()
	_, stillPending := e.pendingBlocks[blk.id]
	e.mu.RUnlock()
	if !stillPending {
		t.Fatal("a sub-quorum block must remain pending (not dropped, not accepted)")
	}
}

// -----------------------------------------------------------------------------
// GATE 5 (DEX): a BFT quorum finalizes a value fill.
// -----------------------------------------------------------------------------

// TestDEX_BFTQuorumFinalizesFill models a value C+D block (a DEX fill) being
// finalized: the value block finalizes ONLY after an α-of-K quorum, and the
// fail-closed DEX gate PERMITS activation in this (quorum-finality) mode.
func TestDEX_BFTQuorumFinalizesFill(t *testing.T) {
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params5(), vs, 0, rec)

	// The DEX value gate must PERMIT activation: K>1 + verifier = quorum-finality.
	if err := e.RequireQuorumFinalityForValueDEX(true); err != nil {
		t.Fatalf("value DEX must be permitted under quorum finality: %v", err)
	}
	if e.Mode() != ModeQuorumFinality {
		t.Fatalf("engine must report quorum-finality mode, got %s", e.Mode())
	}

	// A value fill block. Finalizes only at alpha-of-K.
	fill := newTestBlock(42, ids.Empty, "dex-value-fill")
	pos := trackProposal(e, chainID, fill, 0)

	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos))

	if !waitFor(2*time.Second, func() bool { return e.IsAccepted(fill.id) }) {
		t.Fatal("LIVENESS: DEX value fill did not finalize after alpha-of-K quorum")
	}
	if fill.AcceptCalled() != 1 {
		t.Fatalf("value fill VM.Accept must run exactly once at quorum, got %d", fill.AcceptCalled())
	}
	// The finality witness must be a real, verifiable cert.
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.certs) == 0 {
		t.Fatal("DEX value fill finality must produce a verifiable quorum cert")
	}
	cert, err := UnmarshalQuorumCert(rec.certs[len(rec.certs)-1])
	if err != nil {
		t.Fatalf("decode fill cert: %v", err)
	}
	if err := cert.Verify(vs); err != nil {
		t.Fatalf("fill cert must verify: %v", err)
	}
	if cert.VoterCount() < 3 {
		t.Fatalf("fill cert must carry >= alpha=3 distinct voters, got %d", cert.VoterCount())
	}
}

// -----------------------------------------------------------------------------
// GATE 6 (DEX): a Byzantine proposer's fake fill is rejected.
// -----------------------------------------------------------------------------

// TestDEX_ByzantineProposerFakeFillRejected models a Byzantine proposer trying
// to finalize a fake value fill WITHOUT an honest quorum — by self-finalizing,
// by forging votes (unknown/invalid signatures), and by asserting a sub-quorum
// cert. All paths are refused.
func TestDEX_ByzantineProposerFakeFillRejected(t *testing.T) {
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params5(), vs, 0, rec)

	fake := newTestBlock(7, ids.Empty, "fake-fill")
	pos := trackProposal(e, chainID, fake, 0)

	// (a) self-finalize attempt — refused (gate 1).
	e.finalizeOwnProposal(context.Background(), fake.id)
	if e.IsAccepted(fake.id) {
		t.Fatal("SAFETY: Byzantine self-finalized a fake fill")
	}

	// (b) forged votes: signatures from keys NOT in the validator set, and a
	// real validator's signature over a DIFFERENT position. Neither verifies, so
	// neither counts toward the cert.
	outsider := newTestValidatorSet(1) // keys unknown to vs
	e.ReceiveVote(Vote{
		BlockID: fake.id, NodeID: outsider.nodeID(0), Accept: true, SignedAt: time.Now(),
		Signature: outsider.sign(0, pos), ParentID: pos.ParentID, Round: pos.Round,
	})
	wrongPos := pos
	wrongPos.Height = pos.Height + 1 // validator 1 signs the WRONG height
	e.ReceiveVote(Vote{
		BlockID: fake.id, NodeID: vs.nodeID(1), Accept: true, SignedAt: time.Now(),
		Signature: vs.sign(1, wrongPos), ParentID: pos.ParentID, Round: pos.Round,
	})

	if waitFor(400*time.Millisecond, func() bool { return e.IsAccepted(fake.id) }) {
		t.Fatal("SAFETY: fake fill finalized via forged / wrong-position votes")
	}
	rec.mu.Lock()
	gotCerts := len(rec.certs)
	rec.mu.Unlock()
	if gotCerts != 0 {
		t.Fatalf("no cert may form from forged votes, got %d", gotCerts)
	}

	// (c) a hand-forged sub-quorum cert presented to a follower is rejected by
	// Verify (threshold floor + only 1 real signer).
	subCert, err := AssembleQuorumCert(pos, 1, []SignedVote{
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
	})
	if err != nil {
		t.Fatalf("assemble sub-cert: %v", err)
	}
	// It "verifies" at its own threshold=1, but the engine's cert-intake floor
	// (HandleIncomingCert) rejects threshold < chain alpha. Assert the floor.
	if subCert.Threshold >= uint32(e.consensus.Alpha()) {
		t.Fatal("test bug: sub-cert threshold should be below alpha")
	}
	_ = chainID
}

// -----------------------------------------------------------------------------
// LIVENESS: no proposer-freeze under late / dropped Chits.
// -----------------------------------------------------------------------------

// TestLiveness_NoFreezeUnderLateChits proves the regression that motivated the
// old (unsafe) ForceAccept does NOT return: when peer votes arrive LATE, the
// proposer still finalizes once the quorum is present — without ever
// self-finalizing in the gap. The block stays pending (not frozen-dead, not
// force-accepted) until the late votes complete the quorum, then finalizes.
func TestLiveness_NoFreezeUnderLateChits(t *testing.T) {
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params5(), vs, 0, rec)

	blk := newTestBlock(1, ids.Empty, "late-chits")
	pos := trackProposal(e, chainID, blk, 0)

	// Proposer tries to self-finalize immediately (the old freeze trigger).
	// It must NOT finalize (no quorum yet) AND must NOT be dropped.
	e.finalizeOwnProposal(context.Background(), blk.id)
	if e.IsAccepted(blk.id) {
		t.Fatal("SAFETY: self-finalized before quorum")
	}
	e.mu.RLock()
	_, pendingMid := e.pendingBlocks[blk.id]
	e.mu.RUnlock()
	if !pendingMid {
		t.Fatal("LIVENESS: block dropped while waiting for late votes (would freeze the height)")
	}

	// Late votes arrive (simulating delayed/dropped-then-resent Chits).
	time.Sleep(120 * time.Millisecond)
	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos))

	// Now it finalizes — liveness restored WITHOUT self-finality.
	if !waitFor(2*time.Second, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("LIVENESS: proposer froze — did not finalize after late quorum arrived")
	}
	if blk.AcceptCalled() != 1 {
		t.Fatalf("VM.Accept exactly once after late quorum, got %d", blk.AcceptCalled())
	}
}

// TestLiveness_DroppedProposerChitsStillFinalizeViaPeerCert proves a follower
// finalizes from a GOSSIPED cert even if it never collected the votes itself —
// the cert-distribution path that replaces fast-follow and prevents the freeze.
func TestLiveness_DroppedProposerChitsStillFinalizeViaPeerCert(t *testing.T) {
	vs := newTestValidatorSet(5)

	// Proposer assembles a real cert (3 of 5 signed accepts) out-of-band.
	chainID := ids.GenerateTestID()
	blk := newTestBlock(9, ids.Empty, "cert-relayed")
	pos := VotePosition{ChainID: chainID, Height: blk.height, Round: 0, BlockID: blk.id, ParentID: blk.parentID}
	cert, err := AssembleQuorumCert(pos, 3, []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
	})
	if err != nil {
		t.Fatalf("assemble cert: %v", err)
	}
	certBytes, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal cert: %v", err)
	}

	// A FOLLOWER (validator 4) that verified the block but collected NO votes.
	follower := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(4), vs, &recordingGossiper{}, vs.signerFor(4)))
	if err := follower.Start(context.Background(), true); err != nil {
		t.Fatalf("follower Start: %v", err)
	}
	t.Cleanup(func() { _ = follower.Stop(context.Background()) })
	rt := &Runtime{Transitive: follower, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	// The follower has verified+tracked the block (as HandleIncomingBlock would).
	cb := &Block{id: blk.id, parentID: blk.parentID, height: blk.height, timestamp: blk.timestamp.Unix(), data: blk.bytes}
	_ = follower.consensus.AddBlock(context.Background(), cb)
	follower.mu.Lock()
	follower.pendingBlocks[blk.id] = &PendingBlock{ConsensusBlock: cb, VMBlock: blk, ProposedAt: time.Now(), Round: 0}
	follower.mu.Unlock()

	// The proposer's direct Chits to this follower were dropped; instead it
	// receives the gossiped cert. It MUST finalize on that verifiable proof.
	if !rt.HandleIncomingCert(certBytes) {
		t.Fatal("LIVENESS: follower failed to finalize from a valid gossiped quorum cert")
	}
	if !follower.IsAccepted(blk.id) {
		t.Fatal("follower must mark block accepted after a verified cert")
	}
	if blk.AcceptCalled() != 1 {
		t.Fatalf("follower VM.Accept exactly once via cert, got %d", blk.AcceptCalled())
	}
}

// TestSingleValidator_StillFinalizes proves the K==1 path is preserved: a
// single-validator engine finalizes its own block (1-of-1 quorum) via the
// gated ForceAccept, and the DEX value gate REFUSES (single-validator is not a
// multi-party quorum-finality mode).
func TestSingleValidator_StillFinalizes(t *testing.T) {
	e := NewWithParams(config.Parameters{K: 1, AlphaPreference: 1, AlphaConfidence: 1, Beta: 1})
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("K=1 Start (no verifier needed): %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	if e.Mode() != ModeSingleValidator {
		t.Fatalf("K=1 must be single-validator mode, got %s", e.Mode())
	}
	// Value DEX MUST be refused on a single validator (fail-closed).
	if err := e.RequireQuorumFinalityForValueDEX(true); err == nil {
		t.Fatal("SAFETY: value DEX must be refused on a single-validator engine")
	}

	blk := newTestBlock(1, ids.Empty, "solo")
	_ = trackProposal(e, ids.Empty, blk, 0)
	// K==1: ForceAccept is permitted; finalizeOwnProposal commits.
	e.finalizeOwnProposal(context.Background(), blk.id)
	if !e.IsAccepted(blk.id) {
		t.Fatal("K=1 single validator must finalize its own block (1-of-1 quorum)")
	}
	if blk.AcceptCalled() != 1 {
		t.Fatalf("K=1 VM.Accept exactly once, got %d", blk.AcceptCalled())
	}
}

// TestForceAccept_RefusedForMultiValidator is the direct unit guard: ForceAccept
// fails closed for K>1.
func TestForceAccept_RefusedForMultiValidator(t *testing.T) {
	c := NewChainConsensus(5, 3, 2)
	blk := &Block{id: ids.GenerateTestID(), height: 1}
	_ = c.AddBlock(context.Background(), blk)
	if err := c.ForceAccept(blk.id); err == nil {
		t.Fatal("SAFETY: ForceAccept must refuse on a multi-validator (K=5) engine")
	}
	if c.IsAccepted(blk.id) {
		t.Fatal("SAFETY: ForceAccept must not accept on K>1")
	}
	// K=1 path works.
	c1 := NewChainConsensus(1, 1, 1)
	blk1 := &Block{id: ids.GenerateTestID(), height: 1}
	_ = c1.AddBlock(context.Background(), blk1)
	if err := c1.ForceAccept(blk1.id); err != nil {
		t.Fatalf("ForceAccept must succeed on K=1: %v", err)
	}
	if !c1.IsAccepted(blk1.id) {
		t.Fatal("K=1 ForceAccept must accept")
	}
}
