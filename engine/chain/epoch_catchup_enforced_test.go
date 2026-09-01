// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// epoch_catchup_enforced_test.go — the catch-up epoch bound, driven through the
// door itself.
//
// epoch_catchup_door_test.go asserts the PREDICATE (rt.epochRegresses) and hands
// AcceptCatchupBlock a three-byte cert. Those bytes fail UnmarshalQuorumCert, so
// the call returns before the epoch is ever read: the refusal it observes is a
// codec error wearing the shape of a safety proof. Deleting the bound from
// AcceptCatchupBlock leaves that file green.
//
// A door is only guarded by a test that walks through it. These drive a REAL
// 4-of-5 cert — the same one the liveness path accepts — so the only difference
// between the refusal case and the admission case is the epoch on the block.
package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// catchupCertAt assembles a genuine cert for an arbitrary (height, id, parent),
// so a test can certify a block type other than verifyOnceBlock. Same assembly
// as catchupCertFor — a peer ahead of us would have stored these exact bytes.
func catchupCertAt(t *testing.T, vs *testValidatorSet, chainID ids.ID, height uint64, blkID, parentID ids.ID, voters []int, threshold uint32) []byte {
	t.Helper()
	pos := VotePosition{ChainID: chainID, Height: height, Round: 0, BlockID: blkID, ParentID: parentID}
	votes := make([]SignedVote, 0, len(voters))
	for _, i := range voters {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	qc, err := AssembleQuorumCert(pos, Quasar, threshold, votes)
	if err != nil {
		t.Fatalf("assemble cert: %v", err)
	}
	b, err := qc.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal cert: %v", err)
	}
	return b
}

// behindAtEpoch strands a runtime at height N on a tracked parent carrying a
// recorded epoch, which is the state a node fetching history is in: finalized at
// N, holding the parent, asking peers for N+1.
func behindAtEpoch(t *testing.T, rt *Runtime, vm *epochGateVM, parent *pChainBlock) {
	t.Helper()
	vm.register(parent)
	trackParentEpoch(t, rt, parent)
	if _, err := rt.Transitive.consensus.FinalizeBranch(parent.id, parent.height, ids.Empty); err != nil {
		t.Fatalf("seed behind at height %d: %v", parent.height, err)
	}
	if fh, set := rt.Transitive.consensus.GetFinalizedHeight(); !set || fh != parent.height {
		t.Fatalf("precondition: behind node must be finalized at %d, got (%d,%v)", parent.height, fh, set)
	}
}

// TestCatchupAcceptRefusesFarPastEpoch is the safety half, at the door.
//
// The parent is tracked at epoch 100. A peer serves height N+1 stamped at epoch
// 5 — a past P-chain epoch whose coalition has since departed but still holds,
// at that height, the weight to attest. Admitting it resolves the validator set
// at epoch 5 and checks the cert against a set the current set never approved.
// The gossip door has refused this since the far-past epoch work; catch-up is
// the second door onto the same field.
func TestCatchupAcceptRefusesFarPastEpoch(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newEpochGateVM()
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(1_000_000)
	parent := newEpochBlock(N, 100, ids.GenerateTestID(), "tip@N-epoch100")
	behindAtEpoch(t, rt, vm, parent)

	attack := newEpochBlock(N+1, 5, parent.id, "catchup-attack-epoch5")
	vm.register(attack)
	cert := catchupCertAt(t, vs, chainID, attack.height, attack.id, attack.parentID, []int{0, 1, 2, 3}, 4)

	err := rt.AcceptCatchupBlock(context.Background(), attack.Bytes(), cert)
	if err == nil {
		t.Fatal("SAFETY BREAK: AcceptCatchupBlock admitted a block whose epoch (5) regresses " +
			"below its parent's (100) — a peer serving history can pin a block to a departed " +
			"validator set, which the gossip door has refused since the far-past epoch work")
	}
	if rt.IsAccepted(attack.id) {
		t.Fatal("a refused far-past-epoch block was finalized anyway")
	}
	if got := attack.AcceptCalled(); got != 0 {
		t.Fatalf("a refused block reached VM.Accept %d times", got)
	}
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != N {
		t.Fatalf("finalized height moved on a refused block: %d, want %d", fh, N)
	}
}

// TestCatchupAcceptAdmitsForwardEpoch is the positive control, and it is what
// makes the refusal above mean anything.
//
// Identical machinery — same runtime, same validator set, same 4-of-5 cert, same
// block shape at the same height — differing ONLY in the epoch. If this is
// admitted and the far-past case is refused, the epoch is what decided it. Any
// weakening that refuses here (freezing the epoch rather than bounding its
// direction) breaks catch-up itself, which exists to serve exactly the history
// across which an honest chain's epoch advances.
func TestCatchupAcceptAdmitsForwardEpoch(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newEpochGateVM()
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(1_000_000)
	parent := newEpochBlock(N, 100, ids.GenerateTestID(), "tip@N-epoch100")
	behindAtEpoch(t, rt, vm, parent)

	forward := newEpochBlock(N+1, 101, parent.id, "catchup-forward-epoch101")
	vm.register(forward)
	cert := catchupCertAt(t, vs, chainID, forward.height, forward.id, forward.parentID, []int{0, 1, 2, 3}, 4)

	if err := rt.AcceptCatchupBlock(context.Background(), forward.Bytes(), cert); err != nil {
		t.Fatalf("a block whose epoch advances past its parent's must be admitted: %v", err)
	}
	if !rt.IsAccepted(forward.id) {
		t.Fatal("forward-epoch block was not finalized via the cert path")
	}
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != N+1 {
		t.Fatalf("finalized height = %d, want %d", fh, N+1)
	}
}

// TestCatchupAcceptRefusesStrippedEpoch pins the boundary the gossip door pins.
// Epoch 0 is the genesis-set fallback, so stamping it under a live parent drops
// the chain back to the set it launched with — the same attack with the cheapest
// possible payload.
func TestCatchupAcceptRefusesStrippedEpoch(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newEpochGateVM()
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(2_000_000)
	parent := newEpochBlock(N, 42, ids.GenerateTestID(), "tip@N-epoch42")
	behindAtEpoch(t, rt, vm, parent)

	stripped := newEpochBlock(N+1, 0, parent.id, "catchup-stripped-epoch0")
	vm.register(stripped)
	cert := catchupCertAt(t, vs, chainID, stripped.height, stripped.id, stripped.parentID, []int{0, 1, 2, 3}, 4)

	if err := rt.AcceptCatchupBlock(context.Background(), stripped.Bytes(), cert); err == nil {
		t.Fatal("epoch 0 under a parent at 42 is a regression to the genesis set and must be refused")
	}
	if rt.IsAccepted(stripped.id) {
		t.Fatal("a stripped-epoch block was finalized")
	}
}
