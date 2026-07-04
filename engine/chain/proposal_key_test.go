package chain

// One own proposal per ProposalKey: outer IDs are transport aliases, the
// (parent,height) slot is consensus identity. These pin that invariant.

import (
	"testing"

	"github.com/luxfi/ids"
)

func newProposalTestEngine() *Transitive {
	return &Transitive{
		pendingBlocks:       map[ids.ID]*PendingBlock{},
		pendingOwnProposals: map[ProposalKey]*PendingBlock{},
	}
}

func ownBlock(id, parent ids.ID, height uint64) *Block {
	return &Block{id: id, parentID: parent, height: height, data: id[:]}
}

// Gate 1 — Churn repro. Same parent+height, a NEW envelope ID each build tick.
// One pendingOwnProposals entry, one stable vote target, no sibling growth.
func TestProposalKey_ChurnCollapsesToOneCandidate(t *testing.T) {
	e := newProposalTestEngine()
	parent := ids.GenerateTestID()
	first, _ := e.registerOrReuseOwnProposalLocked(ownBlock(ids.GenerateTestID(), parent, 1082880), nil)

	for tick := 0; tick < 20; tick++ {
		// proposervm re-wraps: same (parent,height), fresh outer ID.
		rewrap := ownBlock(ids.GenerateTestID(), parent, 1082880)
		got, reused := e.registerOrReuseOwnProposalLocked(rewrap, nil)
		if !reused {
			t.Fatalf("tick %d: re-wrap was NOT reused — sibling created", tick)
		}
		if got != first {
			t.Fatalf("tick %d: reuse returned a different candidate", tick)
		}
	}
	if len(e.pendingOwnProposals) != 1 {
		t.Fatalf("own-proposal index grew to %d under churn (sibling leak)", len(e.pendingOwnProposals))
	}
	if len(e.pendingBlocks) != 1 {
		t.Fatalf("transport index grew to %d — a re-wrap was registered", len(e.pendingBlocks))
	}
}

// Gate 2 — Same parent, DIFFERENT height: a new slot, a legitimately new proposal.
func TestProposalKey_DifferentHeightIsNewProposal(t *testing.T) {
	e := newProposalTestEngine()
	parent := ids.GenerateTestID()
	e.registerOrReuseOwnProposalLocked(ownBlock(ids.GenerateTestID(), parent, 10), nil)
	_, reused := e.registerOrReuseOwnProposalLocked(ownBlock(ids.GenerateTestID(), parent, 11), nil)
	if reused {
		t.Fatal("height+1 must be a new ProposalKey, not a reuse")
	}
	if len(e.pendingOwnProposals) != 2 {
		t.Fatalf("expected 2 distinct proposals, got %d", len(e.pendingOwnProposals))
	}
}

// Gate 3 — Same height, DIFFERENT parent: no collapse across forks.
func TestProposalKey_DifferentParentNotCollapsed(t *testing.T) {
	e := newProposalTestEngine()
	e.registerOrReuseOwnProposalLocked(ownBlock(ids.GenerateTestID(), ids.GenerateTestID(), 10), nil)
	_, reused := e.registerOrReuseOwnProposalLocked(ownBlock(ids.GenerateTestID(), ids.GenerateTestID(), 10), nil)
	if reused {
		t.Fatal("different parent at same height must NOT collapse onto the other fork")
	}
}

// Gate 4 — Transport lookup by exact outer ID still works after registration.
func TestProposalKey_TransportLookupByOuterID(t *testing.T) {
	e := newProposalTestEngine()
	id := ids.GenerateTestID()
	pb, _ := e.registerOrReuseOwnProposalLocked(ownBlock(id, ids.GenerateTestID(), 10), nil)
	if e.pendingBlocks[id] != pb {
		t.Fatal("exact-ID transport lookup broken after registration")
	}
}

// Gate 5 — A remote (gossiped) block populates ONLY the transport index, never
// the own-proposal index, so it can never be reused as our candidate.
func TestProposalKey_RemoteDoesNotPopulateOwnIndex(t *testing.T) {
	e := newProposalTestEngine()
	parent := ids.GenerateTestID()
	remoteID := ids.GenerateTestID()
	e.pendingBlocks[remoteID] = &PendingBlock{
		ConsensusBlock: ownBlock(remoteID, parent, 10),
		IsOwnProposal:  false,
	}
	// Our own build at the SAME slot must NOT see the remote as a reusable own proposal.
	_, reused := e.registerOrReuseOwnProposalLocked(ownBlock(ids.GenerateTestID(), parent, 10), nil)
	if reused {
		t.Fatal("a remote block must not be reused as our own proposal")
	}
}

// Gate 6 — A DECIDED slot is pruned by dropPendingBlockLocked; a later build at
// the same key is a fresh proposal, never a reuse of the decided one. Also proves
// the two indices stay in lockstep through the ONE unwriter.
func TestProposalKey_DecidedSlotPrunedAndNotReused(t *testing.T) {
	e := newProposalTestEngine()
	parent := ids.GenerateTestID()
	pb, _ := e.registerOrReuseOwnProposalLocked(ownBlock(ids.GenerateTestID(), parent, 10), nil)

	pb.Decided = true
	e.dropPendingBlockLocked(pb.ConsensusBlock.id)
	if len(e.pendingBlocks) != 0 || len(e.pendingOwnProposals) != 0 {
		t.Fatalf("dropPendingBlockLocked left drift: blocks=%d own=%d", len(e.pendingBlocks), len(e.pendingOwnProposals))
	}

	_, reused := e.registerOrReuseOwnProposalLocked(ownBlock(ids.GenerateTestID(), parent, 10), nil)
	if reused {
		t.Fatal("a pruned/decided slot must not be reused")
	}
}

// Gate 7 — A sibling at the same slot never evicts the live owner. dropPendingBlockLocked
// removes a sibling's transport entry but leaves the owner's identity slot intact.
func TestProposalKey_SiblingDropDoesNotEvictOwner(t *testing.T) {
	e := newProposalTestEngine()
	parent := ids.GenerateTestID()
	owner, _ := e.registerOrReuseOwnProposalLocked(ownBlock(ids.GenerateTestID(), parent, 10), nil)

	// A stray transport entry at the same slot (e.g. a gossiped sibling) — not the owner.
	siblingID := ids.GenerateTestID()
	e.pendingBlocks[siblingID] = &PendingBlock{
		ConsensusBlock: ownBlock(siblingID, parent, 10),
		IsOwnProposal:  true,
	}
	e.dropPendingBlockLocked(siblingID)

	key := ProposalKey{ParentID: parent, Height: 10}
	if e.pendingOwnProposals[key] != owner {
		t.Fatal("dropping a same-slot sibling evicted the live owner from the identity index")
	}
}
