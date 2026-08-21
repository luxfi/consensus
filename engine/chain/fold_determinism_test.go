// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// fold_determinism_test.go — Finalize is documented as a PURE FOLD: same ledger,
// same cert, same DAG, same result. Two nodes holding the same blocks must decide
// the same accepts and the same rejects, and one node asked twice must answer the
// same way.
//
// The walk resolves an untracked peer envelope through blocksAncestry, whose
// WrapperByCanonical and Children both range over the live block map. Go randomises
// that order, so where a node holds more than one wrapper of an execution the
// alias the walk lands on is drawn fresh on every call — and that alias is written
// into finalized history as the envelope at its height and used as the pivot for
// the losing-subtree sweep. The reject set and the recorded envelope are then
// draws, not functions of the input.
package chain

import (
	"testing"

	"github.com/luxfi/ids"
)

// twoWrapperDAG is a node holding TWO local wrappers of one height-6 execution,
// both children of the finalized tip, neither accepted yet — the ordinary shape
// under anyone-can-propose, where each validator envelopes the same inner block.
// A cert names a THIRD wrapper this node does not hold, so the walk must stand a
// local alias in its place.
func twoWrapperDAG(tip, tipCanon, canon6 ids.ID) (blocksAncestry, ids.ID, ids.ID) {
	wrapA := ids.GenerateTestID()
	wrapB := ids.GenerateTestID()
	if wrapA.Compare(wrapB) > 0 {
		wrapA, wrapB = wrapB, wrapA // name them in a stable order for the report
	}
	blocks := map[ids.ID]*Block{
		tip:   {id: tip, parentID: ids.GenerateTestID(), height: 5, canonicalID: tipCanon},
		wrapA: {id: wrapA, parentID: tip, height: 6, canonicalID: canon6, parentCanonicalID: tipCanon},
		wrapB: {id: wrapB, parentID: tip, height: 6, canonicalID: canon6, parentCanonicalID: tipCanon},
	}
	return blocksAncestry{blocks: blocks}, wrapA, wrapB
}

// TestAncestry_WrapperByCanonical_IsAFunction: the alias resolver is a lookup, and
// a lookup returns one answer. Whichever alias the rule picks, it must pick the
// same one every time — otherwise every caller downstream inherits a coin flip.
func TestAncestry_WrapperByCanonical_IsAFunction(t *testing.T) {
	tip, tipCanon, canon6 := ids.GenerateTestID(), ids.GenerateTestID(), ids.GenerateTestID()
	dag, wrapA, wrapB := twoWrapperDAG(tip, tipCanon, canon6)

	seen := map[ids.ID]int{}
	const draws = 512
	for i := 0; i < draws; i++ {
		got, ok := dag.WrapperByCanonical(canon6, 6)
		if !ok {
			t.Fatal("control broke: the node holds two wrappers of this execution at this height")
		}
		seen[got]++
	}
	if len(seen) != 1 {
		t.Fatalf("WrapperByCanonical is not a function: %d draws over an UNCHANGED block set "+
			"returned %s %d times and %s %d times. It ranges over the live map, so with more than "+
			"one non-accepted alias the winner is whichever entry the iteration happens to visit "+
			"last. The walk writes that id into finalized history as the envelope at its height "+
			"and pivots the losing-subtree sweep on it.",
			draws, wrapA, seen[wrapA], wrapB, seen[wrapB])
	}
}

// TestFinalize_SameInputsSamePlan is the fold's own contract, stated over the
// production ancestry. The cert, the ledger and the block set are fixed; only the
// call is repeated.
func TestFinalize_SameInputsSamePlan(t *testing.T) {
	tip, tipCanon, canon6 := ids.GenerateTestID(), ids.GenerateTestID(), ids.GenerateTestID()
	dag, wrapA, wrapB := twoWrapperDAG(tip, tipCanon, canon6)
	led := seedLedger(tip, tipCanon, 5)

	peerWrapper := ids.GenerateTestID() // the cert's envelope, not held here
	inner7 := ids.GenerateTestID()
	blk7 := ids.GenerateTestID()
	dag.blocks[blk7] = &Block{id: blk7, parentID: peerWrapper, height: 7, canonicalID: inner7, parentCanonicalID: canon6}
	cert := Cert{Block: blk7, Parent: peerWrapper, ParentCanonical: canon6, Height: 7, Canonical: inner7}

	type outcome struct {
		tip      ids.ID
		envelope ids.ID
		accepts  string
		rejects  string
	}
	seen := map[outcome]int{}
	const draws = 512
	for i := 0; i < draws; i++ {
		next, plan, err := Finalize(led, cert, dag)
		if err != nil {
			t.Fatalf("control broke: the node holds the whole certified execution chain, so the "+
				"fold must advance; got %v", err)
		}
		env, _ := next.EnvelopeAt(6)
		seen[outcome{tip: next.Tip(), envelope: env, accepts: idsKey(plan.Accept), rejects: idsKey(plan.Reject)}]++
	}
	if len(seen) != 1 {
		t.Fatalf("Finalize is not a function of its inputs: %d folds of ONE (ledger, cert, DAG) "+
			"produced %d distinct outcomes. The two height-6 wrappers %s and %s are aliases of one "+
			"execution, and the fold accepts whichever the map handed it and REJECTS the other — so "+
			"two nodes holding identical blocks record different envelopes at height 6, hand the VM "+
			"opposite Accept/Reject calls for the same pair, and answer a bootstrapping peer's "+
			"outer-envelope identity check differently.",
			draws, len(seen), wrapA, wrapB)
	}
}

// idsKey renders a plan slice as a stable comparison key, ORDER INCLUDED: the
// engine walks Accept in order and hands Reject to the VM in order, so a permuted
// slice is a different instruction sequence, not the same one.
func idsKey(in []ids.ID) string {
	out := make([]byte, 0, len(in)*33)
	for _, id := range in {
		out = append(out, id[:]...)
		out = append(out, '|')
	}
	return string(out)
}

// TestAncestry_Children_IsOrdered: losingSubtrees seeds its sweep from Children and
// emits in that order, so Plan.Reject is a permutation drawn per call unless
// Children is ordered. The engine rejects transitively in the order it is given.
func TestAncestry_Children_IsOrdered(t *testing.T) {
	parent := ids.GenerateTestID()
	blocks := map[ids.ID]*Block{parent: {id: parent, height: 1}}
	for i := 0; i < 6; i++ {
		id := ids.GenerateTestID()
		blocks[id] = &Block{id: id, parentID: parent, height: 2}
	}
	dag := blocksAncestry{blocks: blocks}

	first := idsKey(dag.Children(parent))
	for i := 0; i < 512; i++ {
		if got := idsKey(dag.Children(parent)); got != first {
			t.Fatalf("Children returns a permutation, not a sequence: two reads of an UNCHANGED " +
				"block set differ. losingSubtrees seeds its queue from this and appends in the order " +
				"it receives, so Plan.Reject — the order the VM is told to reject in — is redrawn on " +
				"every fold and differs between nodes holding identical state.")
		}
	}
}
