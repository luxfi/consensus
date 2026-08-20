// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// epoch_catchup_door_test.go — the far-past epoch bound has two doors.
//
// Every existing test of this invariant drives followVerifiedBlock, the gossip
// door. Catch-up read the same peer-supplied epoch and enforced nothing, so a
// node fetching history could be steered onto a validator set the live path
// would have refused. One invariant with two doors is one guarded invariant.
package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// TestCatchupDoor_RefusesFarPastEpoch is the gossip-door proof, run against the
// door nothing was driving.
//
// A parent is tracked at epoch 100. A block arrives over CATCH-UP stamped at a
// stale epoch 5 — a past P-chain epoch where a departed coalition still held the
// weight to attest. Accepting it would resolve the validator set at epoch 5 and
// verify the cert against a set the current set never approved.
func TestCatchupDoor_RefusesFarPastEpoch(t *testing.T) {
	vm := newEpochGateVM()
	rt := newReceiveGateRuntime(vm)

	parent := newEpochBlock(1_000, 100, ids.GenerateTestID(), "parent-epoch100")
	vm.register(parent)
	trackParentEpoch(t, rt, parent)

	attack := newEpochBlock(1_001, 5, parent.id, "catchup-attack-epoch5")
	vm.register(attack)

	_, _, regressed := rt.epochRegresses(attack)
	if !regressed {
		t.Fatal("SAFETY BREAK: a block whose epoch (5) regresses below its parent's (100) " +
			"passed the catch-up door — a peer serving history could pin a block to a stale " +
			"validator set, which the gossip door has refused since the far-past epoch work")
	}

	if err := rt.AcceptCatchupBlock(context.Background(), attack.Bytes(), []byte{1, 2, 3}); err == nil {
		t.Fatal("AcceptCatchupBlock admitted a far-past-epoch block")
	}
}

// TestCatchupDoor_RefusesStrippedEpoch pins the boundary the gossip door pins:
// epoch 0 is the genesis-set fallback, so stamping it under a live parent drops
// the whole chain back to the genesis set.
func TestCatchupDoor_RefusesStrippedEpoch(t *testing.T) {
	vm := newEpochGateVM()
	rt := newReceiveGateRuntime(vm)

	parent := newEpochBlock(2_000, 42, ids.GenerateTestID(), "parent-epoch42")
	vm.register(parent)
	trackParentEpoch(t, rt, parent)

	stripped := newEpochBlock(2_001, 0, parent.id, "catchup-stripped-epoch0")
	vm.register(stripped)

	if _, _, regressed := rt.epochRegresses(stripped); !regressed {
		t.Fatal("epoch 0 under a parent at 42 is a regression to the genesis set and must be refused")
	}
}

// TestCatchupDoor_AdmitsForwardMotion is the liveness half: the bound is a
// regression bound, not a freeze. An honest chain's epoch advances, and catch-up
// exists to serve exactly that history.
func TestCatchupDoor_AdmitsForwardMotion(t *testing.T) {
	vm := newEpochGateVM()
	rt := newReceiveGateRuntime(vm)

	parent := newEpochBlock(3_000, 10, ids.GenerateTestID(), "parent-epoch10")
	vm.register(parent)
	trackParentEpoch(t, rt, parent)

	for _, e := range []uint64{10, 11, 500} {
		child := newEpochBlock(3_001, e, parent.id, "forward")
		vm.register(child)
		if _, _, regressed := rt.epochRegresses(child); regressed {
			t.Fatalf("epoch %d under a parent at 10 is forward motion and must be admitted", e)
		}
	}
}

// TestCatchupDoor_UntrackedParentIsAdmitted: with no tracked parent there is
// nothing to regress against, and an orphan cannot extend finalized history
// anyway. Refusing here would break catch-up itself, which fetches blocks whose
// parents this node has not yet seen.
func TestCatchupDoor_UntrackedParentIsAdmitted(t *testing.T) {
	vm := newEpochGateVM()
	rt := newReceiveGateRuntime(vm)

	orphan := newEpochBlock(4_001, 1, ids.GenerateTestID(), "orphan")
	vm.register(orphan)

	if _, _, regressed := rt.epochRegresses(orphan); regressed {
		t.Fatal("an untracked parent leaves nothing to regress against; refusing here " +
			"would break the very path that fetches unseen ancestors")
	}
}
