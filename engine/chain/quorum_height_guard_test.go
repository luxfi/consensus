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

// TestHeightGuard_EquivocationAndMonotonic unit-tests FinalizeBranch's KEPT safety
// invariants directly on ChainConsensus: ONE finalized block per height (equivocation
// detection) and strictly-forward, contiguous finalized height. The old
// parent==finalizedTip ADMISSION refusal is GONE — siblings are admitted and resolved
// by the cert's reorg (TestFinalizeBranch_* and TestSiblingReorg_* cover that).
func TestHeightGuard_EquivocationAndMonotonic(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	g := ids.GenerateTestID() // height 1 (genesis-ish, empty parent)
	h2 := ids.GenerateTestID()
	// Finalize height 1 (first finalize seeds the ledger).
	if _, err := c.FinalizeBranch(g, 1, ids.Empty); err != nil {
		t.Fatalf("finalize height 1: %v", err)
	}
	// (a) a DIFFERENT block at the SAME height → ErrHeightAlreadyFinalized (equivocation).
	if _, err := c.FinalizeBranch(ids.GenerateTestID(), 1, ids.Empty); !errors.Is(err, ErrHeightAlreadyFinalized) {
		t.Fatalf("expected ErrHeightAlreadyFinalized, got %v", err)
	}
	// Correct child (parent == tip g, height 2) → single-step finalize, no prune.
	if plan, err := c.FinalizeBranch(h2, 2, g); err != nil {
		t.Fatalf("valid child must finalize: %v", err)
	} else if len(plan.Accept) != 1 || plan.Accept[0] != h2 || len(plan.Reject) != 0 {
		t.Fatalf("single-step plan wrong: accepted=%v pruned=%v", plan.Accept, plan.Reject)
	}
	// Re-finalize an OLD height with a new block → rejected (equivocation at height 1).
	if _, err := c.FinalizeBranch(ids.GenerateTestID(), 1, ids.Empty); !errors.Is(err, ErrHeightAlreadyFinalized) {
		t.Fatalf("expected a rejection re-finalizing height 1, got %v", err)
	}
	// Same block, same height → idempotent: empty plan, nil error.
	if plan, err := c.FinalizeBranch(h2, 2, g); err != nil || len(plan.Accept) != 0 {
		t.Fatalf("idempotent re-finalize must be a nil-error no-op, got plan=%+v err=%v", plan, err)
	}
	if fh, set := c.GetFinalizedHeight(); !set || fh != 2 {
		t.Fatalf("finalized height must be 2, got (%d, %v)", fh, set)
	}
}

// TestHeightGuard_RejectsHeightGap proves finality stays CONTIGUOUS: a cert whose
// block sits more than one height above the tip with the tip as its DIRECT parent (a
// skipped height) is rejected. Defense-in-depth — an honest block's height is its
// parent's +1, so an honest single-step always passes; only a malformed cert fails.
func TestHeightGuard_RejectsHeightGap(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)
	g := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(g, 1, ids.Empty); err != nil {
		t.Fatalf("finalize height 1: %v", err)
	}
	// height 3 with parent==tip(g, height 1) — a GAP over height 2 → rejected.
	if _, err := c.FinalizeBranch(ids.GenerateTestID(), 3, g); !errors.Is(err, ErrNonMonotonicFinalizedHeight) {
		t.Fatalf("a height gap must be rejected, got %v", err)
	}
	// The strict successor (height 2, parent g) finalizes.
	if _, err := c.FinalizeBranch(ids.GenerateTestID(), 2, g); err != nil {
		t.Fatalf("contiguous successor must finalize: %v", err)
	}
}

// TestForcePreference_DoesNotMoveFinalizedTip proves the recovery hatch never moves
// finalizedTip off the recorded head. With the per-height ADMISSION gate gone there is
// no invariant for a stray preference to corrupt, but ForcePreference must still never
// rewrite finalized history — only (re)assert a build tip.
func TestForcePreference_DoesNotMoveFinalizedTip(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	g := ids.GenerateTestID() // finalized head at height 1
	if _, err := c.FinalizeBranch(g, 1, ids.Empty); err != nil {
		t.Fatalf("finalize height 1: %v", err)
	}

	// Reaffirm the head — finalizedTip stays g.
	c.ForcePreference(g)
	if c.GetFinalizedTip() != g {
		t.Fatalf("ForcePreference(head) must keep finalized tip g, got %s", c.GetFinalizedTip())
	}

	// MISUSE: force preference to an UNRELATED block. This must NOT move finalizedTip.
	rogue := ids.GenerateTestID()
	c.ForcePreference(rogue)
	if c.GetFinalizedTip() != g {
		t.Fatalf("ForcePreference(non-head) moved finalized tip: got %s want g", c.GetFinalizedTip())
	}
	if fh, set := c.GetFinalizedHeight(); !set || fh != 1 {
		t.Fatalf("finalized height must stay 1, got (%d,%v)", fh, set)
	}

	// The correct child (parent == true tip g) finalizes.
	if _, err := c.FinalizeBranch(ids.GenerateTestID(), 2, g); err != nil {
		t.Fatalf("correct child (parent g) must finalize: %v", err)
	}
}

// TestLocalVotePath_IsLivenessOnly proves the α-count path (ProcessVote/Poll) is now
// pure LIVENESS, decomplected from finality: reaching α marks a block worth a finalize
// ATTEMPT (IsAccepted, the engine's DrainAccepted trigger) but advances NO finalized
// history. Finality — and the single-block-per-height guarantee — is the cert path's
// job (FinalizeBranch). Two conflicting blocks can both reach α-count; neither is
// finalized without a cert, and a cert then finalizes exactly one.
func TestLocalVotePath_IsLivenessOnly(t *testing.T) {
	c := NewChainConsensus(4, 3, 2) // α=3 (count)
	a := &Block{id: ids.GenerateTestID(), parentID: ids.Empty, height: 1}
	b := &Block{id: ids.GenerateTestID(), parentID: ids.Empty, height: 1}
	_ = c.AddBlock(context.Background(), a)
	_ = c.AddBlock(context.Background(), b)

	// Drive BOTH siblings to the α=3 accept count.
	for i := 0; i < 3; i++ {
		_ = c.ProcessVote(context.Background(), a.id, true)
		_ = c.ProcessVote(context.Background(), b.id, true)
	}
	// Both reach the α-count LIVENESS flag...
	if !c.IsAccepted(a.id) || !c.IsAccepted(b.id) {
		t.Fatal("both siblings must reach the α-count liveness flag")
	}
	// ...but the count path finalizes NOTHING: the per-height ledger is untouched.
	if _, set := c.GetFinalizedHeight(); set {
		t.Fatal("the count path must NOT advance finalized history — finality is cert-only")
	}
	if _, ok := c.FinalizedBlockAtHeight(1); ok {
		t.Fatal("no block may be finalized at height 1 without a cert")
	}
	// A cert finalizes exactly ONE; the conflicting one is then refused by gate (a).
	if _, err := c.FinalizeBranch(a.id, 1, ids.Empty); err != nil {
		t.Fatalf("cert for a must finalize: %v", err)
	}
	if _, err := c.FinalizeBranch(b.id, 1, ids.Empty); !errors.Is(err, ErrHeightAlreadyFinalized) {
		t.Fatalf("a SECOND finalize at height 1 must be refused as equivocation, got %v", err)
	}
	if fin, _ := c.FinalizedBlockAtHeight(1); fin != a.id {
		t.Fatalf("height 1 must be finalized to a, got %s", fin)
	}
}
