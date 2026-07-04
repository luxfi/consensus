package chain

// Canonical-equivalent siblings are ALIASES, not forks. Every fork-choice / winner /
// parent-selection path must collapse them to ONE deterministic representative before
// making progress decisions. These pin that invariant against the block-1082879 storm:
// 600+ outer wrappers of ONE inner block that never converged because the winner among
// equal canonicals (and the parent group they hung under) was chosen by Go map order.

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
)

func pend(b *Block) *PendingBlock { return &PendingBlock{ConsensusBlock: b, ProposedAt: time.Now()} }

// lowestID returns the id that sorts lowest by ids.ID.Compare — the deterministic
// representative the fixed winner selection MUST return regardless of map order.
func lowestID(ids0 []ids.ID) ids.ID {
	w := ids0[0]
	for _, id := range ids0[1:] {
		if id.Compare(w) < 0 {
			w = id
		}
	}
	return w
}

// Test 1 — EQUAL-CANONICAL REPRESENTATIVE DETERMINISM.
// 600 outer wrappers of ONE inner block (identical canonicalID), inserted in randomized
// map order. The winner MUST be the lowest OUTER id on EVERY call — never a map-order
// artifact. Pre-fix (`canon.Compare(winnerCanon) < 0` only) the equal canonicals never
// update the winner, so it stayed whatever the map yielded first → different per call.
func TestCanonicalRep_EqualCanonicalWinnerIsDeterministic(t *testing.T) {
	const n = 600
	parent := ids.GenerateTestID()
	canon := ids.GenerateTestID() // ONE shared inner canonical for all wrappers
	outerIDs := make([]ids.ID, 0, n)

	// 400 independent Transitive instances, each with the SAME sibling set inserted in a
	// fresh (randomized) map — every one must pick the identical winner.
	base := make([]*Block, 0, n)
	for i := 0; i < n; i++ {
		oid := ids.GenerateTestID()
		outerIDs = append(outerIDs, oid)
		base = append(base, &Block{id: oid, parentID: parent, height: 1082882, canonicalID: canon, parentCanonicalID: parent})
	}
	want := lowestID(outerIDs)

	for trial := 0; trial < 400; trial++ {
		e := &Transitive{pendingBlocks: map[ids.ID]*PendingBlock{}}
		for _, b := range base {
			e.pendingBlocks[b.id] = pend(b)
		}
		got, count, ok := e.convergedWinnerAtHeightLocked(1082882, parent, true)
		if !ok || count != n {
			t.Fatalf("trial %d: ok=%v count=%d (want %d)", trial, ok, count, n)
		}
		if got != want {
			t.Fatalf("trial %d: winner=%s NOT the deterministic lowest-outer-id %s — map-order nondeterminism", trial, got, want)
		}
	}
}

// Test 2 — FORKED-PARENT GROUPING (the actual storm mechanism).
// The parent block exists in TWO wrappers (same inner canonical, different outer id).
// Children built on each wrapper carry different outer parentIDs but the SAME parent
// canonical. They MUST converge as ONE group: convergedWinner called with EITHER parent
// wrapper returns the same winner and counts ALL children. Pre-fix (`cb.parentID !=
// parentID`) each call saw only the children under one wrapper → split vote → stall.
func TestCanonicalRep_ForkedParentChildrenConvergeAsOneGroup(t *testing.T) {
	grandparent := ids.GenerateTestID() // accepted tip's role for the parents; not in pending
	parentCanon := ids.GenerateTestID()
	pw1 := &Block{id: ids.GenerateTestID(), parentID: grandparent, height: 1082881, canonicalID: parentCanon, parentCanonicalID: grandparent}
	pw2 := &Block{id: ids.GenerateTestID(), parentID: grandparent, height: 1082881, canonicalID: parentCanon, parentCanonicalID: grandparent}

	childCanon := ids.GenerateTestID() // ONE inner child block, wrapped many times under both parents
	var childIDs []ids.ID
	e := &Transitive{pendingBlocks: map[ids.ID]*PendingBlock{}}
	e.pendingBlocks[pw1.id] = pend(pw1)
	e.pendingBlocks[pw2.id] = pend(pw2)
	// 5 children under pw1, 5 under pw2 — different outer parentIDs, same parent canonical.
	for i := 0; i < 5; i++ {
		c1 := &Block{id: ids.GenerateTestID(), parentID: pw1.id, height: 1082882, canonicalID: childCanon, parentCanonicalID: parentCanon}
		c2 := &Block{id: ids.GenerateTestID(), parentID: pw2.id, height: 1082882, canonicalID: childCanon, parentCanonicalID: parentCanon}
		e.pendingBlocks[c1.id] = pend(c1)
		e.pendingBlocks[c2.id] = pend(c2)
		childIDs = append(childIDs, c1.id, c2.id)
	}
	want := lowestID(childIDs)

	// Calling with EITHER parent wrapper must yield the SAME winner over ALL 10 children.
	for _, p := range []ids.ID{pw1.id, pw2.id} {
		got, count, ok := e.convergedWinnerAtHeightLocked(1082882, p, true)
		if !ok || count != 10 {
			t.Fatalf("parent %s: ok=%v count=%d (want 10 — forked-parent children not merged)", p, ok, count)
		}
		if got != want {
			t.Fatalf("parent %s: winner=%s want %s — group/winner not deterministic across parent wrappers", p, got, want)
		}
	}
}

// Test 3 — GENUINE-FORK SAFETY. Two parents with DIFFERENT canonical at the same
// height/grandparent are NOT aliases; their children MUST NOT be merged into one group.
func TestCanonicalRep_GenuineForkNotMerged(t *testing.T) {
	grandparent := ids.GenerateTestID()
	pcA := ids.GenerateTestID()
	pcB := ids.GenerateTestID()
	pA := &Block{id: ids.GenerateTestID(), parentID: grandparent, height: 1082881, canonicalID: pcA, parentCanonicalID: grandparent}
	pB := &Block{id: ids.GenerateTestID(), parentID: grandparent, height: 1082881, canonicalID: pcB, parentCanonicalID: grandparent}
	e := &Transitive{pendingBlocks: map[ids.ID]*PendingBlock{}}
	e.pendingBlocks[pA.id] = pend(pA)
	e.pendingBlocks[pB.id] = pend(pB)
	// 3 children under A (canon caA), 3 under B (canon caB).
	caA, caB := ids.GenerateTestID(), ids.GenerateTestID()
	for i := 0; i < 3; i++ {
		cA := &Block{id: ids.GenerateTestID(), parentID: pA.id, height: 1082882, canonicalID: caA, parentCanonicalID: pcA}
		cB := &Block{id: ids.GenerateTestID(), parentID: pB.id, height: 1082882, canonicalID: caB, parentCanonicalID: pcB}
		e.pendingBlocks[cA.id] = pend(cA)
		e.pendingBlocks[cB.id] = pend(cB)
	}
	// Group under A must count ONLY A's 3 children — B's must not leak in.
	_, countA, okA := e.convergedWinnerAtHeightLocked(1082882, pA.id, true)
	if !okA || countA != 3 {
		t.Fatalf("group A: count=%d want 3 — genuine fork wrongly merged", countA)
	}
	_, countB, okB := e.convergedWinnerAtHeightLocked(1082882, pB.id, true)
	if !okB || countB != 3 {
		t.Fatalf("group B: count=%d want 3 — genuine fork wrongly merged", countB)
	}
}

// Test 3b — ACCEPTED-TIP CHILDREN (height floor+1) are matched even though the accepted
// parent is NOT in pendingBlocks and its outer id != its canonical. Guards the edge case
// where resolving parentCanon from pendingBlocks fails and we must match by outer parentID.
func TestCanonicalRep_AcceptedTipChildrenMatched(t *testing.T) {
	tipOuter := ids.GenerateTestID() // accepted tip: NOT inserted into pendingBlocks
	tipCanon := ids.GenerateTestID() // its inner canonical (children carry this as parentCanonicalID)
	childCanon := ids.GenerateTestID()
	var childIDs []ids.ID
	e := &Transitive{pendingBlocks: map[ids.ID]*PendingBlock{}}
	for i := 0; i < 4; i++ {
		c := &Block{id: ids.GenerateTestID(), parentID: tipOuter, height: 1082880, canonicalID: childCanon, parentCanonicalID: tipCanon}
		e.pendingBlocks[c.id] = pend(c)
		childIDs = append(childIDs, c.id)
	}
	got, count, ok := e.convergedWinnerAtHeightLocked(1082880, tipOuter, true)
	if !ok || count != 4 {
		t.Fatalf("accepted-tip children: ok=%v count=%d want 4 — children of the finalized tip were filtered out", ok, count)
	}
	if got != lowestID(childIDs) {
		t.Fatalf("accepted-tip winner=%s want lowest %s", got, lowestID(childIDs))
	}
}
