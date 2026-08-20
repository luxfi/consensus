// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// frontier_alias_test.go — the walk must recognise the finalized frontier when the
// network names it under a DIFFERENT envelope than the one this node finalized.
//
// Everywhere else in this package the subject of finality is (height, inner
// canonical): the cert binds the canonical block and canonical parent (cert.go),
// and attestKey deliberately EXCLUDES the outer envelope (attestation.go). Only the
// ancestry walk compared outer ids at the frontier — so a node whose tip is a
// sibling wrapper of the network's refused every cert forever while holding
// byte-identical execution state.
package chain

import (
	"errors"
	"testing"

	"github.com/luxfi/ids"
)

// TestFrontierAlias_TrackedPeerEnvelope_Finalizes is the production topology.
//
// This node finalized height 5 under its own envelope; the network finalized the
// SAME inner block under another. The peer's envelope is TRACKED here (this node
// holds it), so the walk reaches it, sees height 5 <= finalized height 5, and — on
// outer identity alone — calls it a losing branch. It is not a losing branch: it is
// the same decision, wearing a different envelope.
func TestFrontierAlias_TrackedPeerEnvelope_Finalizes(t *testing.T) {
	tipOuter := ids.GenerateTestID() // this node's wrapper of the height-5 execution
	tipCanon := ids.GenerateTestID() // the height-5 execution itself
	led := seedLedger(tipOuter, tipCanon, 5)

	peerTip := ids.GenerateTestID() // the network's wrapper of the SAME execution
	canon4 := ids.GenerateTestID()
	inner6 := ids.GenerateTestID()
	blk6 := ids.GenerateTestID()

	dag := wrappedAncestry{
		peerTip: {parent: ids.GenerateTestID(), height: 5, canonical: tipCanon, parentCanonical: canon4},
		blk6:    {parent: peerTip, height: 6, canonical: inner6, parentCanonical: tipCanon},
	}
	cert := Cert{Block: blk6, Parent: peerTip, ParentCanonical: tipCanon, Height: 6, Canonical: inner6}

	out, plan, err := Finalize(led, cert, dag)
	if err != nil {
		t.Fatalf("a cert extending the finalized execution under a sibling envelope must "+
			"finalize; got %v", err)
	}
	if len(plan.Accept) != 1 || plan.Accept[0] != blk6 {
		t.Fatalf("expected accept [blk6], got %v", plan.Accept)
	}
	if h, _ := out.Height(); h != 6 {
		t.Fatalf("finalized height = %d, want 6", h)
	}
}

// TestFrontierAlias_UntrackedPeerEnvelope_Finalizes pins the asymmetry that makes
// the defect perverse: with the peer's envelope ABSENT, the existing
// WrapperByCanonical arm already substitutes the local wrapper and finality
// advances. Holding MORE data was what made the node refuse. Both topologies must
// reach the same verdict.
func TestFrontierAlias_UntrackedPeerEnvelope_Finalizes(t *testing.T) {
	tipOuter := ids.GenerateTestID()
	tipCanon := ids.GenerateTestID()
	led := seedLedger(tipOuter, tipCanon, 5)

	peerTip := ids.GenerateTestID() // deliberately NOT in the dag
	inner6 := ids.GenerateTestID()
	blk6 := ids.GenerateTestID()

	dag := wrappedAncestry{
		tipOuter: {parent: ids.GenerateTestID(), height: 5, canonical: tipCanon},
		blk6:     {parent: peerTip, height: 6, canonical: inner6, parentCanonical: tipCanon},
	}
	cert := Cert{Block: blk6, Parent: peerTip, ParentCanonical: tipCanon, Height: 6, Canonical: inner6}

	if _, _, err := Finalize(led, cert, dag); err != nil {
		t.Fatalf("untracked-peer-envelope arm regressed: %v", err)
	}
}

// TestFrontierAlias_DifferentExecution_StillConflicts is the safety half. Relaxing
// the frontier comparison must relax it for ALIASES ONLY. A genuinely divergent
// execution at the finalized height is a real fork and must still be refused —
// otherwise the fix would let a losing branch overwrite finalized history.
func TestFrontierAlias_DifferentExecution_StillConflicts(t *testing.T) {
	tipOuter := ids.GenerateTestID()
	tipCanon := ids.GenerateTestID()
	led := seedLedger(tipOuter, tipCanon, 5)

	forkTip := ids.GenerateTestID()
	forkCanon := ids.GenerateTestID() // a DIFFERENT execution at height 5
	inner6 := ids.GenerateTestID()
	blk6 := ids.GenerateTestID()

	dag := wrappedAncestry{
		forkTip: {parent: ids.GenerateTestID(), height: 5, canonical: forkCanon},
		blk6:    {parent: forkTip, height: 6, canonical: inner6, parentCanonical: forkCanon},
	}
	cert := Cert{Block: blk6, Parent: forkTip, ParentCanonical: forkCanon, Height: 6, Canonical: inner6}

	if _, _, err := Finalize(led, cert, dag); !errors.Is(err, ErrConflictsWithFinalizedBranch) {
		t.Fatalf("a divergent execution at the finalized height must still conflict, got %v", err)
	}
}

// TestFrontierAlias_BelowFrontier_StillConflicts: the relaxation is keyed to the
// finalized height exactly. An ancestor BELOW it can never be the frontier's alias,
// so it must conflict regardless of what canonical it carries.
func TestFrontierAlias_BelowFrontier_StillConflicts(t *testing.T) {
	tipOuter := ids.GenerateTestID()
	tipCanon := ids.GenerateTestID()
	led := seedLedger(tipOuter, tipCanon, 5)

	low := ids.GenerateTestID()
	inner6 := ids.GenerateTestID()
	blk6 := ids.GenerateTestID()

	dag := wrappedAncestry{
		// height 4 but carrying the tip's canonical — must NOT be mistaken for the frontier.
		low:  {parent: ids.GenerateTestID(), height: 4, canonical: tipCanon},
		blk6: {parent: low, height: 6, canonical: inner6, parentCanonical: tipCanon},
	}
	cert := Cert{Block: blk6, Parent: low, ParentCanonical: tipCanon, Height: 6, Canonical: inner6}

	if _, _, err := Finalize(led, cert, dag); err == nil {
		t.Fatal("an ancestor below the finalized height must never satisfy the frontier")
	}
}
