// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_epoch_receive_gate_test.go — the RECEIVE-SIDE epoch gate proof
// (HIGH-1, predicate a: monotonicity). The build side stamps a block's P-chain
// epoch height H = max(currentH, parentH); that is PROPOSER-ONLY. A Byzantine
// proposer skips it and stamps a STALE H_old — a past epoch where its departed
// coalition held ≥⅔ — to bind a fresh block to a validator set the current set
// never approved (a safety break). followVerifiedBlock re-asserts monotonicity
// against the parent's RECORDED epoch (ChainConsensus.EpochHeightOf) before the
// block is ever tracked or voted, so a chain's epoch can only move forward.
//
// These tests drive followVerifiedBlock — the exact receive path that records
// `pChainHeight: pChainHeightOf(blk)` — through a real Runtime, asserting the
// tracking DECISION via consensus.GetBlock:
//
//	(1) a far-PAST child epoch (below the tracked parent's) is REFUSED (not
//	    tracked) — the attack.
//	(2) an HONEST monotone-increasing epoch sequence is TRACKED — including a
//	    legitimate P-chain skew (epoch advances by a staking change) within the
//	    forward direction.
//	(3) an EQUAL epoch (a child in the same epoch as its parent — the common case
//	    when no staking change occurred) is TRACKED (monotone is ≥, not >).
//	(4) a child whose parent is NOT yet tracked is admitted (nothing to regress
//	    against; an orphan cannot extend finalized history regardless).
package chain

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// epochGateVM is a BlockBuilder whose ParseBlock returns a pre-registered
// pChainBlock (carrying a chosen PChainHeight), so a test can gossip bytes that
// decode to a block with a SPECIFIC epoch. It is the receive-side analogue of the
// node's inner VM: the engine calls ParseBlock(bytes) and gets a block exposing
// PChainHeight via the pChainHeightOf boundary.
type epochGateVM struct {
	byBytes map[string]*pChainBlock
	byID    map[ids.ID]*pChainBlock
}

func newEpochGateVM() *epochGateVM {
	return &epochGateVM{
		byBytes: make(map[string]*pChainBlock),
		byID:    make(map[ids.ID]*pChainBlock),
	}
}

func (vm *epochGateVM) register(b *pChainBlock) {
	vm.byBytes[string(b.bytes)] = b
	vm.byID[b.id] = b
}

func (vm *epochGateVM) BuildBlock(context.Context) (block.Block, error) {
	return nil, errEpochGateNoBuild
}

func (vm *epochGateVM) ParseBlock(_ context.Context, b []byte) (block.Block, error) {
	if blk, ok := vm.byBytes[string(b)]; ok {
		return blk, nil
	}
	return nil, errEpochGateUnknown
}

func (vm *epochGateVM) GetBlock(_ context.Context, id ids.ID) (block.Block, error) {
	if blk, ok := vm.byID[id]; ok {
		return blk, nil
	}
	return nil, errEpochGateUnknown
}

func (vm *epochGateVM) LastAccepted(context.Context) (ids.ID, error) { return ids.Empty, nil }
func (vm *epochGateVM) SetPreference(context.Context, ids.ID) error  { return nil }

type epochGateErr string

func (e epochGateErr) Error() string { return string(e) }

const (
	errEpochGateNoBuild = epochGateErr("epochGateVM: BuildBlock not used in receive-gate test")
	errEpochGateUnknown = epochGateErr("epochGateVM: unknown block")
)

// newEpochBlock builds a pChainBlock at value height `h` with epoch height
// `epoch` and a unique opaque encoding (tag-derived, so ParseBlock keys never
// collide).
func newEpochBlock(h, epoch uint64, parent ids.ID, tag string) *pChainBlock {
	return &pChainBlock{
		id:           ids.GenerateTestID(),
		parentID:     parent,
		height:       h,
		pChainHeight: epoch,
		timestamp:    time.Now(),
		bytes:        []byte("epoch-gate:" + tag),
	}
}

// trackParentEpoch inserts a parent block into the engine's consensus ledger with
// a RECORDED epoch height, exactly as buildBlocksLocked / followVerifiedBlock do
// (the consensus Block carries pChainHeight). The receive gate reads this back via
// EpochHeightOf when deciding a child.
func trackParentEpoch(t *testing.T, rt *Runtime, parent *pChainBlock) {
	t.Helper()
	cb := &Block{
		id:           parent.id,
		parentID:     parent.parentID,
		height:       parent.height,
		timestamp:    parent.timestamp.Unix(),
		data:         parent.bytes,
		pChainHeight: parent.pChainHeight,
	}
	if err := rt.Transitive.consensus.AddBlock(context.Background(), cb); err != nil {
		t.Fatalf("AddBlock(parent): %v", err)
	}
	// Confirm the parent's epoch is the authoritative read the gate will use.
	got, ok := rt.Transitive.consensus.EpochHeightOf(parent.id)
	if !ok || got != parent.pChainHeight {
		t.Fatalf("parent epoch not recorded: EpochHeightOf(%s)=(%d,%v), want %d", parent.id, got, ok, parent.pChainHeight)
	}
}

// newReceiveGateRuntime builds a Runtime over the epochGateVM with NO signer (the
// gate decision — track vs refuse — is observable independent of the vote/cert
// machinery, and runs BEFORE the signer==nil early-return in followVerifiedBlock).
func newReceiveGateRuntime(vm *epochGateVM) *Runtime {
	return NewRuntime(NetworkConfig{
		ChainID:   ids.GenerateTestID(),
		NetworkID: ids.GenerateTestID(),
		VM:        vm,
	})
}

func isTracked(rt *Runtime, id ids.ID) bool {
	_, ok := rt.Transitive.consensus.GetBlock(id)
	return ok
}

// --- (1) the attack: a far-PAST child epoch is REFUSED ------------------------

// TestReceiveGate_RefusesFarPastEpoch is the HIGH-1 safety proof. A parent is
// tracked at epoch 100. A Byzantine proposer gossips a CHILD stamped at a stale
// epoch 5 (a past P-chain epoch where its departed coalition held ≥⅔). Without the
// gate the follower would adopt epoch 5 and resolve the validator set at the stale
// epoch — finalizing a fresh block against a set the current set never approved.
// The gate REFUSES it: the child is never tracked.
func TestReceiveGate_RefusesFarPastEpoch(t *testing.T) {
	vm := newEpochGateVM()
	rt := newReceiveGateRuntime(vm)

	parent := newEpochBlock(1_000, 100, ids.GenerateTestID(), "parent-epoch100")
	vm.register(parent)
	trackParentEpoch(t, rt, parent)

	// The attack block: value height advances (a fresh block) but epoch REGRESSES
	// far below the parent's recorded 100.
	attack := newEpochBlock(1_001, 5, parent.id, "attack-epoch5-far-past")
	vm.register(attack)

	rt.fastFollowMu.Lock()
	rt.followVerifiedBlock(context.Background(), attack, ids.GenerateTestNodeID())
	rt.fastFollowMu.Unlock()

	if isTracked(rt, attack.id) {
		t.Fatal("SAFETY BREAK: a child whose P-chain epoch (5) regresses below its parent's recorded epoch (100) " +
			"was TRACKED — a Byzantine proposer could pin a fresh block to a stale validator set (far-past attack). " +
			"The receive-side epoch gate must refuse it.")
	}
}

// TestReceiveGate_RefusesEpochZeroUnderTrackedParent pins the boundary case: a
// child stamped epoch 0 (the genesis-set fallback value) under a parent already at
// a positive epoch is a REGRESSION and must be refused — otherwise an attacker
// strips the epoch (stamps 0) to drop back to the genesis set under a live chain.
func TestReceiveGate_RefusesEpochZeroUnderTrackedParent(t *testing.T) {
	vm := newEpochGateVM()
	rt := newReceiveGateRuntime(vm)

	parent := newEpochBlock(2_000, 42, ids.GenerateTestID(), "parent-epoch42")
	vm.register(parent)
	trackParentEpoch(t, rt, parent)

	stripped := newEpochBlock(2_001, 0, parent.id, "stripped-epoch0")
	vm.register(stripped)

	rt.fastFollowMu.Lock()
	rt.followVerifiedBlock(context.Background(), stripped, ids.GenerateTestNodeID())
	rt.fastFollowMu.Unlock()

	if isTracked(rt, stripped.id) {
		t.Fatal("a child stamped epoch 0 under a parent at epoch 42 is a regression to the genesis set and must be refused")
	}
}

// --- (2) honest forward motion is TRACKED -------------------------------------

// TestReceiveGate_AcceptsMonotoneIncrease proves the gate does NOT break the
// legitimate case: a child whose epoch ADVANCES (a real staking change moved the
// P-chain epoch forward) is tracked. This is the honest skew the gate must admit.
func TestReceiveGate_AcceptsMonotoneIncrease(t *testing.T) {
	vm := newEpochGateVM()
	rt := newReceiveGateRuntime(vm)

	parent := newEpochBlock(3_000, 100, ids.GenerateTestID(), "parent-epoch100-fwd")
	vm.register(parent)
	trackParentEpoch(t, rt, parent)

	// Epoch advances 100 -> 137 (a staking change landed) — strictly forward.
	child := newEpochBlock(3_001, 137, parent.id, "child-epoch137-forward")
	vm.register(child)

	rt.fastFollowMu.Lock()
	rt.followVerifiedBlock(context.Background(), child, ids.GenerateTestNodeID())
	rt.fastFollowMu.Unlock()

	if !isTracked(rt, child.id) {
		t.Fatal("a child whose epoch advances (100 -> 137) is honest forward motion and MUST be tracked — " +
			"the gate must not reject legitimate P-chain epoch advance during a staking change")
	}
	// And the engine recorded the child's REAL forward epoch (137), not the parent's.
	if got, ok := rt.Transitive.consensus.EpochHeightOf(child.id); !ok || got != 137 {
		t.Fatalf("tracked child epoch = (%d,%v), want 137 — the gate must record the child's own forward epoch", got, ok)
	}
}

// TestReceiveGate_AcceptsEqualEpoch proves the common no-staking-change case: a
// child in the SAME epoch as its parent is tracked (monotone is ≥, not strict >).
func TestReceiveGate_AcceptsEqualEpoch(t *testing.T) {
	vm := newEpochGateVM()
	rt := newReceiveGateRuntime(vm)

	parent := newEpochBlock(4_000, 77, ids.GenerateTestID(), "parent-epoch77")
	vm.register(parent)
	trackParentEpoch(t, rt, parent)

	child := newEpochBlock(4_001, 77, parent.id, "child-epoch77-equal")
	vm.register(child)

	rt.fastFollowMu.Lock()
	rt.followVerifiedBlock(context.Background(), child, ids.GenerateTestNodeID())
	rt.fastFollowMu.Unlock()

	if !isTracked(rt, child.id) {
		t.Fatal("a child in the SAME epoch as its parent (no staking change) must be tracked — monotone is ≥, not strict >")
	}
}

// --- (3) honest multi-block monotone sequence ---------------------------------

// TestReceiveGate_AcceptsHonestSequence drives a realistic chain: epochs
// 10,10,11,11,12 across five blocks (no regression anywhere, with both equal and
// forward steps, including a staking change at the 10->11 and 11->12 boundaries).
// Every block is tracked. This is the liveness proof: the gate never drops an
// honest block.
func TestReceiveGate_AcceptsHonestSequence(t *testing.T) {
	vm := newEpochGateVM()
	rt := newReceiveGateRuntime(vm)

	epochs := []uint64{10, 10, 11, 11, 12}
	parent := ids.GenerateTestID()
	for i, e := range epochs {
		blk := newEpochBlock(uint64(5_000+i), e, parent, "seq-"+string(rune('a'+i)))
		vm.register(blk)
		if i == 0 {
			// Seed the first block's parent epoch lower so the first block is forward.
			seed := newEpochBlock(4_999, 9, parent, "seq-seed")
			seed.id = parent
			vm.register(seed)
			trackParentEpoch(t, rt, seed)
		}
		rt.fastFollowMu.Lock()
		rt.followVerifiedBlock(context.Background(), blk, ids.GenerateTestNodeID())
		rt.fastFollowMu.Unlock()
		if !isTracked(rt, blk.id) {
			t.Fatalf("honest monotone sequence block %d (epoch %d) was dropped — the gate must admit every non-regressing block", i, e)
		}
		parent = blk.id
	}
}

// --- (4) untracked parent is admitted (fail-open is safe here) ----------------

// TestReceiveGate_AdmitsWhenParentUntracked proves the gate does NOT block a
// block whose parent the engine has not yet tracked: there is no recorded parent
// epoch to regress against, and an orphan cannot extend finalized history anyway
// (markFinalizedLocked enforces parent == finalized tip). The far-past attack is
// only meaningful relative to a tracked parent; admitting the orphan keeps
// liveness (out-of-order gossip) without weakening safety.
func TestReceiveGate_AdmitsWhenParentUntracked(t *testing.T) {
	vm := newEpochGateVM()
	rt := newReceiveGateRuntime(vm)

	// No parent tracked. A child with ANY epoch (even a small one) is admitted —
	// the gate has nothing to compare against.
	orphan := newEpochBlock(6_000, 3, ids.GenerateTestID(), "orphan-untracked-parent")
	vm.register(orphan)

	rt.fastFollowMu.Lock()
	rt.followVerifiedBlock(context.Background(), orphan, ids.GenerateTestNodeID())
	rt.fastFollowMu.Unlock()

	if !isTracked(rt, orphan.id) {
		t.Fatal("a block whose parent is not yet tracked has no recorded parent epoch to regress against and must be admitted " +
			"(liveness for out-of-order gossip; an orphan cannot extend finalized history regardless)")
	}
}
