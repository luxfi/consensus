package chain

// Pre-build gate — the AvalancheGo-aligned rule that a mempool event is
// PERMISSION to build later, not a mandate to rebuild the in-flight candidate.
//
// buildBlocksLocked consults ownUndecidedCandidateOnParentLocked(preference)
// BEFORE calling the VM's BuildBlock: if our own undecided candidate already
// occupies the tip, the build is skipped (the VM would only re-wrap the same
// (parent,height) into a fresh envelope — the mainnet-1082880 churn) and the
// EXISTING candidate is re-solicited. These tests pin the predicate that gate
// branches on. Its safety property is conservatism: it returns a candidate ONLY
// when we are certain a build would duplicate the slot; in every other case it
// returns nil and buildBlocksLocked falls through to build normally (the
// post-build guards remain the correctness backstop).

import (
	"testing"

	"github.com/luxfi/ids"
)

// newGateTestEngine builds a bare Transitive with only the pendingBlocks map
// wired — enough to exercise the candidate-lookup predicate directly.
func newGateTestEngine() *Transitive {
	return &Transitive{pendingBlocks: map[ids.ID]*PendingBlock{}}
}

func ownUndecided(id, parent ids.ID, height uint64) *PendingBlock {
	return &PendingBlock{
		ConsensusBlock: &Block{id: id, parentID: parent, height: height},
		IsOwnProposal:  true,
		Decided:        false,
	}
}

// Gate: our own undecided candidate on the preference IS found — so a mempool
// tick skips the rebuild and re-solicits it (no sibling, votes stay on one ID).
func TestPreBuildGate_OwnCandidateOnPreferenceFound(t *testing.T) {
	e := newGateTestEngine()
	parent := ids.GenerateTestID()
	cand := ownUndecided(ids.GenerateTestID(), parent, 10)
	e.pendingBlocks[cand.ConsensusBlock.id] = cand

	got := e.ownUndecidedCandidateOnParentLocked(parent)
	if got != cand {
		t.Fatalf("expected the own undecided candidate on the preferred parent, got %v", got)
	}
}

// Conservative fall-through #1: preference MOVED. The candidate sits on the old
// parent; a lookup on the new preference returns nil so buildBlocksLocked builds
// on the new tip normally (Gate 2 — no false collapse across parent).
func TestPreBuildGate_PreferenceMovedFallsThrough(t *testing.T) {
	e := newGateTestEngine()
	oldParent := ids.GenerateTestID()
	cand := ownUndecided(ids.GenerateTestID(), oldParent, 10)
	e.pendingBlocks[cand.ConsensusBlock.id] = cand

	newPreference := ids.GenerateTestID()
	if got := e.ownUndecidedCandidateOnParentLocked(newPreference); got != nil {
		t.Fatalf("preference moved — must return nil (build on new parent), got %v", got)
	}
}

// Conservative fall-through #2: the candidate DECIDED. A decided block must
// never be re-solicited (Gate 3 — no false collapse after decision).
func TestPreBuildGate_DecidedCandidateNotReused(t *testing.T) {
	e := newGateTestEngine()
	parent := ids.GenerateTestID()
	cand := ownUndecided(ids.GenerateTestID(), parent, 10)
	cand.Decided = true
	e.pendingBlocks[cand.ConsensusBlock.id] = cand

	if got := e.ownUndecidedCandidateOnParentLocked(parent); got != nil {
		t.Fatalf("decided candidate must not be reused, got %v", got)
	}
}

// Conservative fall-through #3: a GOSSIPED (remote) block on the preference is
// not ours to stabilize (Gate 4 — no false collapse of a remote proposal).
func TestPreBuildGate_RemoteCandidateNotReused(t *testing.T) {
	e := newGateTestEngine()
	parent := ids.GenerateTestID()
	remote := ownUndecided(ids.GenerateTestID(), parent, 10)
	remote.IsOwnProposal = false
	e.pendingBlocks[remote.ConsensusBlock.id] = remote

	if got := e.ownUndecidedCandidateOnParentLocked(parent); got != nil {
		t.Fatalf("remote block must not be reused as our candidate, got %v", got)
	}
}

// Churn model: N mempool ticks against a stable preference must resolve to the
// SAME single candidate every time (one blkID, never a sibling per tick — the
// 1082880 freeze condition). The predicate is idempotent across ticks.
func TestPreBuildGate_ChurnResolvesToSingleCandidate(t *testing.T) {
	e := newGateTestEngine()
	parent := ids.GenerateTestID()
	cand := ownUndecided(ids.GenerateTestID(), parent, 1082880)
	e.pendingBlocks[cand.ConsensusBlock.id] = cand

	for tick := 0; tick < 20; tick++ {
		got := e.ownUndecidedCandidateOnParentLocked(parent)
		if got != cand {
			t.Fatalf("tick %d: churn produced a different candidate %v, want the stable one", tick, got)
		}
	}
	if len(e.pendingBlocks) != 1 {
		t.Fatalf("candidate set grew to %d under churn — sibling leak", len(e.pendingBlocks))
	}
}
