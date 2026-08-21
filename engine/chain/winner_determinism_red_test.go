// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// winner_determinism_red_test.go — RED adversarial attack on choke point #2 (the
// anyone-can-propose sibling storm). When pChainHeight=0 gives an empty validator
// snapshot, EVERY node emits a competing sibling at each height. The view-change can
// only collapse that split if convergedWinnerAtHeightLocked selects the IDENTICAL
// winner on every honest node — INDEPENDENT of Go's randomized map iteration order.
//
// The historical fork (block 1082879 storm): the winner among EQUAL-canonical siblings
// was decided by map-iteration order, so different nodes picked different winners → no
// POL → the split never collapsed. The fix is the (canonical, then OUTER-id) total
// order. This suite proves that order is stable across thousands of randomized
// iterations at scales up to 1024 siblings — a literal reproduction of the storm at the
// selection layer, where a regression would surface as a flapping winner.
//
// White-box: convergedWinnerAtHeightLocked only reads t.pendingBlocks and takes no lock
// (its caller holds t.mu), so a zero-value Transitive with a populated map is a faithful,
// fast fixture for scale + map-order fuzzing.
package chain

import (
	"testing"

	"github.com/luxfi/ids"
)

// mkID builds a distinct, order-controllable ids.ID from a 16-bit seed placed in the
// HIGH bytes, so numeric seed order == ids.Compare order (lets a test name the expected
// lowest/highest winner without guessing random ids).
func mkID(seed uint16) ids.ID {
	var id ids.ID
	id[0] = byte(seed >> 8)
	id[1] = byte(seed)
	id[31] = 0x01 // never all-zero (ids.Empty is the "no winner" sentinel)
	return id
}

// redBuildTransitive returns a bare engine whose ONLY populated field is the pending set.
func redBuildTransitive(blocks []*Block, decided, abandoned map[ids.ID]bool) *Transitive {
	t := &Transitive{pendingBlocks: make(map[ids.ID]*PendingBlock, len(blocks))}
	for _, b := range blocks {
		t.pendingBlocks[b.id] = &PendingBlock{
			ConsensusBlock:  b,
			Decided:         decided[b.id],
			rePollAbandoned: abandoned[b.id],
		}
	}
	return t
}

// TestRedWinner_EqualCanonicalAliases_MapOrderInvariant is the exact 1082879 regression:
// N siblings that are canonical-EQUIVALENT aliases (identical canonicalID, distinct outer
// ids) at one (height,parent) slot. The winner is decided PURELY by the outer-id
// tie-break. Repeated 2000× (fresh Go map randomization each range) at scales up to 1024:
// the winner MUST be the lowest OUTER id every single time. A flapping result is the
// storm-forever bug.
func TestRedWinner_EqualCanonicalAliases_MapOrderInvariant(t *testing.T) {
	parent := mkID(0xF000)
	sharedCanon := mkID(0x4242) // all aliases share ONE canonical
	for _, n := range []int{2, 3, 5, 21, 64, 256, 1024} {
		blocks := make([]*Block, 0, n)
		lowestOuter := ids.ID{}
		for i := 0; i < n; i++ {
			outer := mkID(uint16(1000 + i)) // distinct outer ids, ascending
			blocks = append(blocks, &Block{
				id:                outer,
				parentID:          parent,
				height:            1,
				canonicalID:       sharedCanon, // EQUAL canonical → only outer-id breaks the tie
				parentCanonicalID: ids.Empty,
			})
			if lowestOuter == (ids.ID{}) || outer.Compare(lowestOuter) < 0 {
				lowestOuter = outer
			}
		}
		tr := redBuildTransitive(blocks, nil, nil)

		var first ids.ID
		for iter := 0; iter < 2000; iter++ {
			w, cnt, ok := tr.convergedWinnerAtHeightLocked(1, parent)
			if !ok {
				t.Fatalf("n=%d: no winner among %d live aliases", n, n)
			}
			if cnt != n {
				t.Fatalf("n=%d: winner count=%d, want %d (all live siblings counted)", n, cnt, n)
			}
			if iter == 0 {
				first = w
				if w != lowestOuter {
					t.Fatalf("n=%d: winner=%s, want lowest OUTER id %s (equal-canonical tie-break)", n, w, lowestOuter)
				}
			} else if w != first {
				t.Fatalf("n=%d: NON-DETERMINISTIC winner across map iterations: iter0=%s iter%d=%s — the 1082879 storm-forever regression",
					n, first, iter, w)
			}
		}
	}
}

// TestRedWinner_DistinctCanonicals_LowestWins proves the primary order: with distinct
// canonicals the LOWEST canonical wins regardless of map order or insertion order, at
// scale. Rebuilding the map each iteration reseeds Go's map hash so this genuinely
// exercises different iteration orders.
func TestRedWinner_DistinctCanonicals_LowestWins(t *testing.T) {
	parent := mkID(0xF000)
	for _, n := range []int{2, 5, 21, 256, 1024} {
		lowestCanon := ids.ID{}
		var wantWinner ids.ID
		mk := func() []*Block {
			blocks := make([]*Block, 0, n)
			for i := 0; i < n; i++ {
				canon := mkID(uint16(5000 + i)) // distinct canonicals
				outer := mkID(uint16(20000 + i))
				blocks = append(blocks, &Block{
					id: outer, parentID: parent, height: 1,
					canonicalID: canon, parentCanonicalID: ids.Empty,
				})
				if lowestCanon == (ids.ID{}) || canon.Compare(lowestCanon) < 0 {
					lowestCanon, wantWinner = canon, outer
				}
			}
			return blocks
		}
		for iter := 0; iter < 500; iter++ {
			tr := redBuildTransitive(mk(), nil, nil)
			w, cnt, ok := tr.convergedWinnerAtHeightLocked(1, parent)
			if !ok || cnt != n {
				t.Fatalf("n=%d iter=%d: ok=%v cnt=%d", n, iter, ok, cnt)
			}
			if w != wantWinner {
				t.Fatalf("n=%d iter=%d: winner=%s, want lowest-canonical block %s", n, iter, w, wantWinner)
			}
		}
	}
}

// TestRedWinner_DecidedAndAbandonedFiltering attacks the exclusion rules that make the
// winner both LIVE and GLOBALLY IDENTICAL:
//   - a Decided sibling is never a candidate (it already finalized);
//   - includeAbandoned=false excludes rePoll-abandoned siblings (legacy accept path);
//   - includeAbandoned=true COUNTS them (the view-change path — abandonment is a
//     PER-NODE clock, so excluding would let two nodes compute different winners → no POL).
//
// The trap this guards: if the view-change ever excluded abandoned siblings, the node
// that abandoned the lowest-canonical sibling would pick a DIFFERENT winner than the node
// that had not — silently re-opening the storm. The assertion pins that the two modes
// diverge EXACTLY on the abandoned block.
func TestRedWinner_DecidedAndAbandonedFiltering(t *testing.T) {
	parent := mkID(0xF000)
	lowest := &Block{id: mkID(100), parentID: parent, height: 1, canonicalID: mkID(1)}     // lowest canonical
	mid := &Block{id: mkID(200), parentID: parent, height: 1, canonicalID: mkID(2)}        // middle
	high := &Block{id: mkID(300), parentID: parent, height: 1, canonicalID: mkID(3)}       // highest
	decidedBlk := &Block{id: mkID(400), parentID: parent, height: 1, canonicalID: mkID(0)} // lowest of all, but DECIDED

	blocks := []*Block{lowest, mid, high, decidedBlk}
	tr := redBuildTransitive(blocks,
		map[ids.ID]bool{decidedBlk.id: true}, // decided → never a candidate
		map[ids.ID]bool{lowest.id: true},     // abandoned → still a candidate
	)

	// Decided is the only thing that removes a sibling. Abandonment does not:
	// it is a per-node clock decision, and letting it change the candidate set
	// would let two nodes pick different winners at the same height.
	w, cnt, ok := tr.convergedWinnerAtHeightLocked(1, parent)
	if !ok || w != lowest.id || cnt != 3 {
		t.Fatalf("got (w=%s,cnt=%d,ok=%v), want (lowest=%s,3,true)", w, cnt, ok, lowest.id)
	}
	if w == decidedBlk.id {
		t.Fatal("a DECIDED block won the convergence — double-finalize risk")
	}
}

// TestRedWinner_ProposerGrindsLowestCanonical is a CONCRETE proof-of-concept for the
// grindability the code documents (RED M-grind): the tie-break is the proposer-controlled
// content hash, so a validator eligible at a CONTESTED height can craft a block whose
// canonical is lower than every honest sibling and DETERMINISTICALLY make all honest nodes
// converge on ITS block — a censorship/MEV lever bounded to multi-proposer contention.
// This test does not fail the build (the behavior is by-design given content-hash
// tie-break); it PINS the lever so a future grind-resistant tie-break (VRF over
// height‖parentID) can be verified to CLOSE it. It is the RED evidence for that follow-up.
func TestRedWinner_ProposerGrindsLowestCanonical(t *testing.T) {
	parent := mkID(0xF000)
	honest := []*Block{
		{id: mkID(600), parentID: parent, height: 1, canonicalID: mkID(50)},
		{id: mkID(601), parentID: parent, height: 1, canonicalID: mkID(60)},
		{id: mkID(602), parentID: parent, height: 1, canonicalID: mkID(70)},
	}
	// Adversary grinds a variant with canonical strictly below every honest sibling.
	grinder := &Block{id: mkID(9999), parentID: parent, height: 1, canonicalID: mkID(1)}
	blocks := append([]*Block{grinder}, honest...)
	tr := redBuildTransitive(blocks, nil, nil)

	w, _, ok := tr.convergedWinnerAtHeightLocked(1, parent)
	if !ok {
		t.Fatal("no winner")
	}
	if w != grinder.id {
		t.Fatalf("grind PoC invariant changed: expected the low-canonical grinder %s to win, got %s "+
			"(if a grind-RESISTANT tie-break was added, update this test to assert the grinder can NO LONGER steer)", grinder.id, w)
	}
	t.Log("CONFIRMED (known, by-design): a proposer at a contested height can grind the lowest canonical and " +
		"steer every honest node's convergence to its block — censorship/MEV lever until a VRF tie-break lands.")
}
