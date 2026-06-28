// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// reorg_test.go — the SIBLING-TOLERANT finality regression.
//
// THE BUG (fresh-net deadlock). The engine forked avalanchego's Snowman model by
// adding a hard per-height gate (the old markFinalizedLocked invariant (c): a
// finalized block's parent MUST equal the current finalized tip) AND never reorged.
// When a cert finalized one child (A) of a parent while production/preference had
// explored a SIBLING (B) and built on it, A finalized but B and its descendants were
// NEVER pruned and preference never reorged to A — so production ran away on the
// losing B branch and every cert for B's descendants hit "parent != finalized tip"
// forever (devnet symptom: finalized stuck at height 1 while blocks produced past 10,
// every cert REFUSED).
//
// THE FIX (this regression proves it). Finalizing a cert-selected block now REORGS
// exactly like avalanchego topological.go's acceptPreferredChild + rejectTransitively:
// it prunes the losing-sibling subtree (VM.Reject + drop from tracking) and moves the
// finalized tip / preference to the certified branch. Siblings coexist BEFORE the cert
// decides; the cert's reorg IS the recovery — there is no permanent refusal.
package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// addTracked adds a block to the consensus DAG (mirrors what the engine does for a
// verified, tracked block) so FinalizeBranch's ancestry walk can reach it.
func addTracked(c *ChainConsensus, id, parent ids.ID, height uint64) {
	_ = c.AddBlock(context.Background(), &Block{id: id, parentID: parent, height: height})
}

// TestFinalizeBranch_MultiStepFinalizesPathAndPrunes proves a cert that certifies a
// DESCENDANT several heights above the finalized tip (a catch-up jump) finalizes the
// WHOLE contiguous path in one call AND prunes the losing sibling subtree branching
// off the path. This is avalanchego acceptPreferredChild applied along a path +
// rejectTransitively, generalized to a multi-step cert.
func TestFinalizeBranch_MultiStepFinalizesPathAndPrunes(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)
	g0 := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(g0, 0, ids.Empty); err != nil {
		t.Fatalf("seed finalize at height 0: %v", err)
	}

	// Winning path g0→A1→A2→A3, with a losing fork B1→B2 off g0.
	a1, a2, a3 := ids.GenerateTestID(), ids.GenerateTestID(), ids.GenerateTestID()
	b1, b2 := ids.GenerateTestID(), ids.GenerateTestID()
	addTracked(c, a1, g0, 1)
	addTracked(c, a2, a1, 2)
	addTracked(c, a3, a2, 3)
	addTracked(c, b1, g0, 1)
	addTracked(c, b2, b1, 2)

	// Cert certifies A3 at height 3 (a 3-step jump above the tip g0).
	plan, err := c.FinalizeBranch(a3, 3, a2)
	if err != nil {
		t.Fatalf("multi-step finalize must succeed: %v", err)
	}
	// The whole path is finalized in ascending order.
	want := []ids.ID{a1, a2, a3}
	if len(plan.Accept) != 3 || plan.Accept[0] != a1 || plan.Accept[1] != a2 || plan.Accept[2] != a3 {
		t.Fatalf("path must be finalized ascending %v, got %v", want, plan.Accept)
	}
	// The losing fork off the path is pruned (B1 + descendant B2).
	prunedSet := map[ids.ID]bool{}
	for _, id := range plan.Reject {
		prunedSet[id] = true
	}
	if !prunedSet[b1] || !prunedSet[b2] || len(plan.Reject) != 2 {
		t.Fatalf("losing fork {B1,B2} must be pruned, got %v", plan.Reject)
	}
	// Ledger: tip is A3 at height 3, each height finalized to its path block.
	if fh, set := c.GetFinalizedHeight(); !set || fh != 3 || c.GetFinalizedTip() != a3 {
		t.Fatalf("tip must be A3@3, got tip=%s height=(%d,%v)", c.GetFinalizedTip(), fh, set)
	}
	for h, id := range map[uint64]ids.ID{1: a1, 2: a2, 3: a3} {
		if fin, ok := c.FinalizedBlockAtHeight(h); !ok || fin != id {
			t.Fatalf("height %d must finalize %s, got (%s,%v)", h, id, fin, ok)
		}
	}
	// Pruned blocks are gone from the live DAG.
	if _, ok := c.GetBlock(b1); ok {
		t.Fatal("pruned B1 must be removed from the live DAG")
	}
}

// TestFinalizeBranch_RefusesConflictingBranch proves a cert for a block that descends
// from a LOSING/pruned branch (its ancestry reaches a non-tip block at/below the
// finalized height) is refused with ErrConflictsWithFinalizedBranch — never finalized.
// This is the KEPT safety the old gate (c) provided, now applied as a branch check
// rather than a blanket "parent != tip" admission refusal.
func TestFinalizeBranch_RefusesConflictingBranch(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)
	g0 := ids.GenerateTestID()
	a1 := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(g0, 0, ids.Empty); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := c.FinalizeBranch(a1, 1, g0); err != nil {
		t.Fatalf("finalize A1@1: %v", err)
	}
	// A losing sibling B1@1 (parent g0) and its child B2@2 appear after A1 won.
	b1, b2 := ids.GenerateTestID(), ids.GenerateTestID()
	addTracked(c, b1, g0, 1)
	addTracked(c, b2, b1, 2)
	// A cert for B2 (parent B1, a losing sibling) must be refused as a conflict.
	if _, err := c.FinalizeBranch(b2, 2, b1); !errors.Is(err, ErrConflictsWithFinalizedBranch) {
		t.Fatalf("cert on a losing branch must conflict, got %v", err)
	}
	// Finalized history is unchanged: tip still A1@1.
	if c.GetFinalizedTip() != a1 {
		t.Fatalf("tip must stay A1 after refusing the conflicting cert, got %s", c.GetFinalizedTip())
	}
}

// TestFinalizeBranch_DefersOnUntrackedAncestor proves a multi-step finalize whose path
// to the tip passes through a NOT-yet-tracked ancestor returns ErrAncestorNotTracked —
// a behind-node DEFER, NOT a finalize and NOT a conflict. The node fetches the gap and
// retries; it must never finalize on a path it cannot prove.
func TestFinalizeBranch_DefersOnUntrackedAncestor(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)
	g0 := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(g0, 0, ids.Empty); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// A2@2 is tracked but its parent A1@1 is NOT (the gossip race / behind node).
	a1, a2 := ids.GenerateTestID(), ids.GenerateTestID()
	addTracked(c, a2, a1, 2)
	if _, err := c.FinalizeBranch(a2, 2, a1); !errors.Is(err, ErrAncestorNotTracked) {
		t.Fatalf("a missing ancestor must DEFER, got %v", err)
	}
	if _, set := c.GetFinalizedHeight(); set && c.GetFinalizedTip() != g0 {
		t.Fatalf("nothing above g0 may finalize on a deferred path, tip=%s", c.GetFinalizedTip())
	}
}

// TestSiblingReorg_CertPrunesLoserBranchAndContinues is the owner-mandated
// regression. TWO children at the same height from the same finalized parent; a cert
// selects ONE; the winner's descendants are accepted, the loser's are rejected, there
// is NO permanent cert refusal, and the chain continues past the fork.
func TestSiblingReorg_CertPrunesLoserBranchAndContinues(t *testing.T) {
	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()
	follower := NewWithConfig(Config{Params: params4()},
		WithQuorumCert(chainID, vs.nodeID(3), vs, &recordingGossiper{}, vs.signerFor(3)))
	if err := follower.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = follower.Stop(context.Background()) })
	rt := &Runtime{Transitive: follower, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	// Finalize the common parent P at height 1 (the "finalized parent" of the fork).
	parent := newTestBlock(1, ids.Empty, "P")
	trackVerifiedBlock(rt, parent, 0)
	if !rt.HandleIncomingCert(buildCertAtRound(t, vs, chainID, parent.id, ids.Empty, 1, 0, 3)) {
		t.Fatal("parent P must finalize at height 1")
	}

	// Two children at height 2 with the SAME parent P — the sibling fork. Both are
	// tracked and live above the finalized frontier; this is NORMAL (Avalanche admits
	// competing children of a parent and resolves by preference).
	win := newTestBlock(2, parent.id, "win")
	lose := newTestBlock(2, parent.id, "lose")
	trackVerifiedBlock(rt, win, 0)
	trackVerifiedBlock(rt, lose, 0)

	// Production explored BOTH branches: a descendant on each at height 3.
	winChild := newTestBlock(3, win.id, "winChild")
	loseChild := newTestBlock(3, lose.id, "loseChild")
	trackVerifiedBlock(rt, winChild, 0)
	trackVerifiedBlock(rt, loseChild, 0)

	// The cert selects `win` at height 2. THIS is where the reorg must happen.
	if !rt.HandleIncomingCert(buildCertAtRound(t, vs, chainID, win.id, parent.id, 2, 0, 3)) {
		t.Fatal("cert for the winning child must finalize it")
	}

	// Winner accepted exactly once.
	if !follower.IsAccepted(win.id) || win.AcceptCalled() != 1 {
		t.Fatalf("winner must be accepted once (accepted=%v calls=%d)", follower.IsAccepted(win.id), win.AcceptCalled())
	}
	// Losing sibling PRUNED: VM.Reject called once. THIS is the reorg the old engine
	// never performed (it left the loser in pendingBlocks forever).
	if lose.RejectCalled() != 1 {
		t.Fatalf("REORG MISSING: losing sibling must be VM.Reject'd once, got %d", lose.RejectCalled())
	}
	// Loser's descendant PRUNED transitively (rejectTransitively).
	if loseChild.RejectCalled() != 1 {
		t.Fatalf("REORG MISSING: losing-branch descendant must be rejected once, got %d", loseChild.RejectCalled())
	}
	// Winner's descendant NOT pruned — it descends from the finalized branch.
	if winChild.RejectCalled() != 0 {
		t.Fatalf("winner descendant must NOT be rejected, got %d", winChild.RejectCalled())
	}
	// Finalized tip / preference reorged to the winner.
	if follower.consensus.GetFinalizedTip() != win.id {
		t.Fatalf("finalized tip must reorg to the winner, got %s", follower.consensus.GetFinalizedTip())
	}

	// CHAIN CONTINUES past the fork: cert the winner's descendant at height 3. Its
	// parent IS the (new) finalized tip, so it finalizes cleanly — NO permanent
	// ErrParentNotFinalizedTip refusal, which is the whole point.
	if !rt.HandleIncomingCert(buildCertAtRound(t, vs, chainID, winChild.id, win.id, 3, 0, 3)) {
		t.Fatal("chain must continue: winner's descendant must finalize (no parent-not-tip deadlock)")
	}
	if fh, set := follower.consensus.GetFinalizedHeight(); !set || fh != 3 {
		t.Fatalf("finalized height must advance to 3, got (%d,%v)", fh, set)
	}
	// Exactly one block finalized at the forked height 2, and it is the winner.
	if fin, ok := follower.consensus.FinalizedBlockAtHeight(2); !ok || fin != win.id {
		t.Fatalf("height 2 must be finalized to the winner, got (%s, ok=%v)", fin, ok)
	}
}
