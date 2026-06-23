// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_height_guard_test.go — round-2 safety tests for the per-height
// single-finalize guard (CRITICAL-1) and the BFT parameter floor (CRITICAL-2).
//
// CRITICAL-1 was a PROVEN fork: AcceptViaCert finalized unconditionally and the
// consensus tracked blocks by ID only, so two VALID α-certs for two DIFFERENT
// blocks at the SAME height (each over a different, attacker-chosen Round) both
// Verified and both VM.Accepted — two blocks at one height. These tests prove
// the guard now finalizes the FIRST and REJECTS the SECOND (no second VM.Accept)
// and surfaces the conflict as equivocation evidence.
package chain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/slashing"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// params4 is the MINIMAL Byzantine-fault-tolerant config: K=4, α=3 (3-of-4),
// f=1. Overlap 2·3−4=2 ≥ ⌊3/3⌋+1=2 — a real quorum (unlike 3-of-5, which the
// BFT floor correctly rejects). Used for the multi-validator fork tests.
func params4() config.Parameters {
	return config.Parameters{K: 4, Alpha: 0.75, AlphaPreference: 3, AlphaConfidence: 3, Beta: 2}
}

// buildCertAtRound assembles a real (Ed25519-signed) α-of-K cert for blockID at
// (height, round) signed by the first `n` validators of vs. The Round is folded
// into every signature, so two certs at the same height with DIFFERENT rounds
// are BOTH internally valid — exactly the attacker's two-valid-certs primitive.
func buildCertAtRound(t *testing.T, vs *testValidatorSet, chainID, blockID, parentID ids.ID, height uint64, round uint32, n int) []byte {
	t.Helper()
	pos := VotePosition{ChainID: chainID, Height: height, Round: round, BlockID: blockID, ParentID: parentID}
	votes := make([]SignedVote, 0, n)
	for i := 0; i < n; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	cert, err := AssembleQuorumCert(pos, uint32(n), votes)
	if err != nil {
		t.Fatalf("assemble cert (round %d): %v", round, err)
	}
	b, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal cert (round %d): %v", round, err)
	}
	return b
}

// trackVerifiedBlock simulates HandleIncomingBlock: the follower has locally
// verified+tracked a block (added to consensus + pendingBlocks) so a cert for it
// can finalize.
func trackVerifiedBlock(rt *Runtime, blk *verifyOnceBlock, round uint32) {
	cb := &Block{id: blk.id, parentID: blk.parentID, height: blk.height, timestamp: blk.timestamp.Unix(), data: blk.bytes}
	_ = rt.Transitive.consensus.AddBlock(context.Background(), cb)
	rt.Transitive.mu.Lock()
	rt.Transitive.pendingBlocks[blk.id] = &PendingBlock{ConsensusBlock: cb, VMBlock: blk, ProposedAt: time.Now(), Round: round}
	rt.Transitive.mu.Unlock()
}

// TestCriticalFork_TwoCertsOneHeightAcrossRounds is the round-1 PoC, now a
// regression: two valid α-of-K certs for DIFFERENT blocks at the SAME height
// (different rounds). The first finalizes; the SECOND must be REJECTED — no
// second VM.Accept, no second finalized block at that height.
func TestCriticalFork_TwoCertsOneHeightAcrossRounds(t *testing.T) {
	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()
	db := slashing.NewDB(time.Hour)

	follower := NewWithConfig(Config{Params: params4()},
		WithQuorumCert(chainID, vs.nodeID(3), vs, &recordingGossiper{}, vs.signerFor(3)),
		WithSlashing(slashing.NewDetector(64, 0.5), db))
	if err := follower.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = follower.Stop(context.Background()) })
	rt := &Runtime{Transitive: follower, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	// Two conflicting blocks at the SAME height 1 (genesis parent), distinct IDs.
	blkA := newTestBlock(1, ids.Empty, "height1-A")
	blkB := newTestBlock(1, ids.Empty, "height1-B")
	trackVerifiedBlock(rt, blkA, 0)
	trackVerifiedBlock(rt, blkB, 7) // attacker-chosen non-zero round

	// Cert A over round 0; cert B over round 7 — BOTH valid 3-of-4 quorums.
	certA := buildCertAtRound(t, vs, chainID, blkA.id, ids.Empty, 1, 0, 3)
	certB := buildCertAtRound(t, vs, chainID, blkB.id, ids.Empty, 1, 7, 3)

	// First cert finalizes A.
	if !rt.HandleIncomingCert(certA) {
		t.Fatal("first cert must finalize block A")
	}
	if !follower.IsAccepted(blkA.id) || blkA.AcceptCalled() != 1 {
		t.Fatalf("A must be accepted exactly once (accepted=%v calls=%d)", follower.IsAccepted(blkA.id), blkA.AcceptCalled())
	}

	// SECOND cert at the SAME height — MUST be rejected. THE FORK STOPS HERE.
	if rt.HandleIncomingCert(certB) {
		t.Fatal("CRITICAL FORK: second cert at an already-finalized height was accepted")
	}
	if follower.IsAccepted(blkB.id) {
		t.Fatal("CRITICAL FORK: block B finalized at an already-finalized height")
	}
	if blkB.AcceptCalled() != 0 {
		t.Fatalf("CRITICAL FORK: VM.Accept called %d times on the conflicting block B (must be 0)", blkB.AcceptCalled())
	}

	// Exactly one block finalized at height 1, and it is A.
	if fin, ok := follower.consensus.FinalizedBlockAtHeight(1); !ok || fin != blkA.id {
		t.Fatalf("height 1 must be finalized to A, got (%s, ok=%v)", fin, ok)
	}

	// The conflict must have produced equivocation evidence for B's voters.
	if len(db.GetAllRecords()) == 0 {
		t.Fatal("equivocation evidence must be recorded for the conflicting cert's voters")
	}
}

// TestHeightGuard_SecondCertSameBlockIsIdempotent proves a re-delivered cert for
// the SAME block at an already-finalized height is a harmless no-op (idempotent),
// NOT an equivocation — only a DIFFERENT block triggers rejection/evidence.
func TestHeightGuard_SecondCertSameBlockIsIdempotent(t *testing.T) {
	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()
	db := slashing.NewDB(time.Hour)
	follower := NewWithConfig(Config{Params: params4()},
		WithQuorumCert(chainID, vs.nodeID(3), vs, &recordingGossiper{}, vs.signerFor(3)),
		WithSlashing(slashing.NewDetector(64, 0.5), db))
	_ = follower.Start(context.Background(), true)
	t.Cleanup(func() { _ = follower.Stop(context.Background()) })
	rt := &Runtime{Transitive: follower, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	blk := newTestBlock(1, ids.Empty, "idem")
	trackVerifiedBlock(rt, blk, 0)
	cert := buildCertAtRound(t, vs, chainID, blk.id, ids.Empty, 1, 0, 3)

	if !rt.HandleIncomingCert(cert) {
		t.Fatal("first delivery must finalize")
	}
	// Re-delivery: block already Decided → returns false (no re-Accept) but is NOT
	// an equivocation (same block).
	rt.HandleIncomingCert(cert)
	if blk.AcceptCalled() != 1 {
		t.Fatalf("VM.Accept must be exactly once across re-delivery, got %d", blk.AcceptCalled())
	}
	if len(db.GetAllRecords()) != 0 {
		t.Fatal("a re-delivered SAME-block cert must NOT be flagged as equivocation")
	}
}

// TestHeightGuard_MonotonicAndParent unit-tests markFinalizedLocked's invariants
// directly on ChainConsensus: one-per-height, monotonic, parent==tip.
func TestHeightGuard_MonotonicAndParent(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	g := ids.GenerateTestID() // height 1 (genesis-ish, empty parent)
	h2 := ids.GenerateTestID()
	// Finalize height 1.
	if err := c.AcceptViaCert(g, 1, ids.Empty); err != nil {
		t.Fatalf("finalize height 1: %v", err)
	}
	// (a) different block at the SAME height → ErrHeightAlreadyFinalized.
	if err := c.AcceptViaCert(ids.GenerateTestID(), 1, ids.Empty); !errors.Is(err, ErrHeightAlreadyFinalized) {
		t.Fatalf("expected ErrHeightAlreadyFinalized, got %v", err)
	}
	// (c) child whose parent != tip → ErrParentNotFinalizedTip.
	if err := c.AcceptViaCert(h2, 2, ids.GenerateTestID()); !errors.Is(err, ErrParentNotFinalizedTip) {
		t.Fatalf("expected ErrParentNotFinalizedTip, got %v", err)
	}
	// Correct child (parent == tip g, height 2) → ok.
	if err := c.AcceptViaCert(h2, 2, g); err != nil {
		t.Fatalf("valid child must finalize: %v", err)
	}
	// (b) re-finalize an OLD height with a new block → non-monotonic.
	if err := c.AcceptViaCert(ids.GenerateTestID(), 1, ids.Empty); !errors.Is(err, ErrHeightAlreadyFinalized) {
		// height 1 is already occupied → caught as already-finalized (a) which
		// precedes the monotonic check; either is a correct rejection.
		if !errors.Is(err, ErrNonMonotonicFinalizedHeight) {
			t.Fatalf("expected a rejection re-finalizing height 1, got %v", err)
		}
	}
	// Same block, same height → idempotent nil.
	if err := c.AcceptViaCert(h2, 2, g); err != nil {
		t.Fatalf("idempotent re-finalize must be nil, got %v", err)
	}
	if fh, set := c.GetFinalizedHeight(); !set || fh != 2 {
		t.Fatalf("finalized height must be 2, got (%d, %v)", fh, set)
	}
}

// TestHeightGuard_RejectsHeightGap proves finality is CONTIGUOUS: a cert that
// would jump the finalized height by more than one (leaving a gap) is rejected
// even though its parent links to the tip. Defense-in-depth — a valid α-cert
// cannot carry a height inconsistent with its block without >f Byzantine voters,
// but the guard refuses the gap regardless.
func TestHeightGuard_RejectsHeightGap(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)
	g := ids.GenerateTestID()
	if err := c.AcceptViaCert(g, 1, ids.Empty); err != nil {
		t.Fatalf("finalize height 1: %v", err)
	}
	// height 3 with parent==tip(g) — a GAP over height 2 → rejected.
	if err := c.AcceptViaCert(ids.GenerateTestID(), 3, g); !errors.Is(err, ErrNonMonotonicFinalizedHeight) {
		t.Fatalf("a height gap must be rejected, got %v", err)
	}
	// The strict successor (height 2, parent g) is accepted.
	if err := c.AcceptViaCert(ids.GenerateTestID(), 2, g); err != nil {
		t.Fatalf("contiguous successor must finalize: %v", err)
	}
}

// TestForcePreference_CannotDesyncFinalizedLedger proves the recovery hatch
// ForcePreference can never move finalizedTip off the finalized head — a
// preference-recovery convenience must not corrupt the per-height guard's source
// of truth (invariant (c): parent == finalized tip). A misuse with a NON-finalized
// block must leave finalizedTip/finalizedHeight/finalizedByHeight intact, so a
// subsequent finalize still checks the parent against the TRUE finalized block.
func TestForcePreference_CannotDesyncFinalizedLedger(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	g := ids.GenerateTestID() // finalized head at height 1
	if err := c.AcceptViaCert(g, 1, ids.Empty); err != nil {
		t.Fatalf("finalize height 1: %v", err)
	}

	// Legit recovery: ForcePreference on the just-finalized head is a reaffirming
	// no-op — finalizedTip stays g.
	c.ForcePreference(g)
	if c.GetFinalizedTip() != g {
		t.Fatalf("ForcePreference(head) must reaffirm finalized tip g, got %s", c.GetFinalizedTip())
	}

	// MISUSE: force preference to an UNRELATED, non-finalized block. This must NOT
	// overwrite finalizedTip (doing so would blind invariant (c)).
	rogue := ids.GenerateTestID()
	c.ForcePreference(rogue)
	if c.GetFinalizedTip() != g {
		t.Fatalf("CRITICAL: ForcePreference(non-finalized) desynced finalized tip: got %s want %s (g)", c.GetFinalizedTip(), g)
	}
	if fh, set := c.GetFinalizedHeight(); !set || fh != 1 {
		t.Fatalf("finalized height must be unchanged at 1, got (%d,%v)", fh, set)
	}

	// PROOF the guard still works against the TRUE tip: a child at height 2 whose
	// parent is the rogue (forced-preference) block must be REJECTED — the guard
	// checks against g, not rogue. If the desync had happened, this would wrongly
	// pass.
	if err := c.AcceptViaCert(ids.GenerateTestID(), 2, rogue); !errors.Is(err, ErrParentNotFinalizedTip) {
		t.Fatalf("child parented on the rogue forced-pref block must be rejected by the guard, got %v", err)
	}
	// The correct child (parent == true tip g) finalizes.
	h2 := ids.GenerateTestID()
	if err := c.AcceptViaCert(h2, 2, g); err != nil {
		t.Fatalf("correct child (parent g) must finalize: %v", err)
	}
}

// TestHeightGuard_LocalVotePathAlsoGuarded proves the LOCAL α-count path
// (ProcessVote) is guarded too: two conflicting blocks at one height, each
// reaching α via local votes, finalize only ONE.
func TestHeightGuard_LocalVotePathAlsoGuarded(t *testing.T) {
	c := NewChainConsensus(4, 3, 2) // α=3 (count)
	a := &Block{id: ids.GenerateTestID(), parentID: ids.Empty, height: 1}
	b := &Block{id: ids.GenerateTestID(), parentID: ids.Empty, height: 1}
	_ = c.AddBlock(context.Background(), a)
	_ = c.AddBlock(context.Background(), b)

	// Drive A to the α=3 accept count.
	for i := 0; i < 3; i++ {
		_ = c.ProcessVote(context.Background(), a.id, true)
	}
	if !c.IsAccepted(a.id) {
		t.Fatal("A must be accepted at the local α count")
	}
	// Drive B to α at the SAME height — must NOT be accepted (guard refuses).
	for i := 0; i < 3; i++ {
		_ = c.ProcessVote(context.Background(), b.id, true)
	}
	if c.IsAccepted(b.id) {
		t.Fatal("FORK via local vote path: B finalized at an already-finalized height")
	}
	if fin, _ := c.FinalizedBlockAtHeight(1); fin != a.id {
		t.Fatalf("height 1 must remain finalized to A, got %s", fin)
	}
}
