// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// certified_descendant_head_test.go — a certified VM head that already contains the steer
// target is benign, not a double-finalization.
//
// A node whose accepted head is certified at height h+1 while the engine steers it at the
// certified block at height h is on one chain, holding both blocks. Nothing would be
// orphaned. Two separate properties have to hold for the engine to see that, one on each
// side of the steer:
//
//   - The producer. acceptWithCertCore releases t.mu across every VM call-out, so a second
//     finalize can complete inside that window and advance both the ledger and the VM to
//     blockID+1. Steering afterwards at the stale local blockID is a backwards steer, which
//     the VM's accepted-irreversibility guard (evm/core/blockchain.go: commonBlock <
//     lastAccepted) correctly refuses. The steer belongs at the live build anchor, which is
//     never below the VM's accepted head — and, when this node does not hold that anchor, at
//     the block VM.Accept has just applied.
//
//   - The classifier. "The head is the ledger's certified canonical at its own height" is
//     trivially true for every healthy node whose head is certified, so on its own it says
//     nothing about a second finalization. A certified head may only be orphaned once the
//     steer target is established as not itself the ledger's certified canonical at its own
//     height — that is, once the two blocks are known to be certified on different chains.
//
// The safety property is unchanged and still fail-closed: a certified head is dropped only
// for a block our own ledger certifies on the same one chain. See
// TestReconcile_CertifiedHead_HaltsFailClosed (orphan_reconcile_test.go), which proves the
// halt with both blocks readable — the ledger, not an unreadable id, decides.
package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// twoHeightLedger builds a certified ledger holding a contiguous two-height chain:
// byHeight[h] = lower and byHeight[h+1] = upper, tip = upper. This is exactly the state a
// node is in after a second finalize completes while an earlier one is still between its
// VM call-outs.
func twoHeightLedger(lower ids.ID, h uint64, upper ids.ID) FinalityLedger {
	led := seedLedger(lower, lower, h)
	led.byHeight[h+1] = finalizedEntry{canonical: upper, envelope: upper}
	led.tip, led.canonical, led.height = upper, upper, h+1
	return led
}

// TestReconcile_CertifiedDescendantHead_NoHalt: the VM's accepted head is the certified
// block at h+1 and the steer target is the certified block at h — its own parent on the one
// certified chain. SetPreference refuses the backwards steer, but nothing would be orphaned:
// the VM already contains h. The engine must neither halt nor touch the VM head.
func TestReconcile_CertifiedDescendantHead_NoHalt(t *testing.T) {
	e := newTestEngine()

	certified := ids.GenerateTestID() // certified at 1363 — the stale steer target
	head := ids.GenerateTestID()      // certified at 1364 — the VM's accepted head
	e.consensus.ledger = twoHeightLedger(certified, 1363, head)

	vm := &reconcilingOrphanVM{orphanVMBase: &orphanVMBase{
		head: head,
		blocks: map[ids.ID]*mockBlock{
			head:      mb(head, 1364),
			certified: mb(certified, 1363),
		},
	}}

	if !e.reconcileVMToCertified(context.Background(), vm, certified, errOrphan) {
		t.Fatal("a VM head that is the certified descendant of the finalized block is benign " +
			"(head = certified+1 on the one certified chain, nothing to orphan) — the engine may " +
			"not halt fail-closed here")
	}
	if vm.reconcileCalls != 0 {
		t.Fatalf("the certified head must be left alone; it already contains the certified block, "+
			"got %d ReconcilePreference calls", vm.reconcileCalls)
	}
	if vm.head != head {
		t.Fatalf("VM head must be untouched, got %s want %s", vm.head, head)
	}
}

// TestReconcile_CertifiedAncestorHead_NoHalt is the mirror: the head is certified below the
// steer target, both on the one certified chain. Steering forward orphans nothing.
func TestReconcile_CertifiedAncestorHead_NoHalt(t *testing.T) {
	e := newTestEngine()

	head := ids.GenerateTestID()      // certified at 1363 — the VM's accepted head
	certified := ids.GenerateTestID() // certified at 1364 — the steer target
	e.consensus.ledger = twoHeightLedger(head, 1363, certified)

	vm := &reconcilingOrphanVM{orphanVMBase: &orphanVMBase{
		head: head,
		blocks: map[ids.ID]*mockBlock{
			head:      mb(head, 1363),
			certified: mb(certified, 1364),
		},
	}}

	if !e.reconcileVMToCertified(context.Background(), vm, certified, errOrphan) {
		t.Fatal("a certified head below the steer target (both on the one certified chain) is benign — must not halt")
	}
	if vm.reconcileCalls != 0 {
		t.Fatalf("nothing to reconcile, got %d calls", vm.reconcileCalls)
	}
}

// TestAcceptWithCert_StaleSteerIsNotIssued pins the producer property end to end through the
// sole finalizer. The VM block's Accept advances the ledger to H+1 — a deterministic
// injection of the race window acceptWithCertCore documents, where t.mu is released across
// the VM call-outs. The engine must then steer at the live anchor H+1, never at its stale
// local H, which is backwards and lands in the VM's orphan refusal.
func TestAcceptWithCert_StaleSteerIsNotIssued(t *testing.T) {
	e := newTestEngine()
	ctx := context.Background()

	lower := ids.GenerateTestID() // finalized here at height 1 (the stale local)
	upper := ids.GenerateTestID() // the concurrent finalize's block at height 2

	cb := &Block{id: lower, parentID: ids.Empty, height: 1}
	if err := e.consensus.AddBlock(ctx, cb); err != nil {
		t.Fatalf("AddBlock: %v", err)
	}

	vm := &steerRecordingVM{orphanVMBase: &orphanVMBase{
		head: upper,
		blocks: map[ids.ID]*mockBlock{
			lower: mb(lower, 1),
			upper: mb(upper, 2),
		},
	}}
	e.SetVM(vm)

	// The VM block whose Accept simulates the concurrent finalize completing: the ledger
	// (and the EVM head) reach height 2 while this call is still inside applyBranchFinalization.
	accepting := &advanceOnAcceptBlock{
		mockBlock: mb(lower, 1),
		onAccept: func() {
			e.consensus.mu.Lock()
			e.consensus.ledger = twoHeightLedger(lower, 1, upper)
			e.consensus.mu.Unlock()
		},
	}
	e.mu.Lock()
	e.pendingBlocks[lower] = &PendingBlock{ConsensusBlock: cb, VMBlock: accepting}
	e.mu.Unlock()

	cert := VerifiedQuorumCert{qc: &QuorumCert{
		Version:   QuorumCertVersion,
		Type:      QCFinality,
		Tier:      Nova,
		Position:  VotePosition{Height: 1, Round: 0, BlockID: lower, ParentID: ids.Empty, CanonicalID: lower},
		Threshold: 1,
	}}

	if err := e.acceptWithCertCore(ctx, lower, cert, false); err != nil {
		t.Fatalf("acceptWithCertCore: %v", err)
	}

	if len(vm.steers) != 1 {
		t.Fatalf("expected exactly one SetPreference, got %d (%v)", len(vm.steers), vm.steers)
	}
	if vm.steers[0] == lower {
		t.Fatalf("stale backwards steer: SetPreference(%s) at height 1 while the ledger and the VM "+
			"are already at height 2 (%s) — the VM refuses this as orphaning a finalized block",
			lower, upper)
	}
	if vm.steers[0] != upper {
		t.Fatalf("expected the steer at the live build anchor %s, got %s", upper, vm.steers[0])
	}
}

// advanceOnAcceptBlock is a VM block whose Accept runs onAccept — used to land the
// concurrent finalize deterministically inside the documented race window.
type advanceOnAcceptBlock struct {
	*mockBlock
	onAccept func()
}

func (b *advanceOnAcceptBlock) Accept(context.Context) error {
	if b.onAccept != nil {
		b.onAccept()
	}
	return nil
}

// steerRecordingVM records every SetPreference target and accepts them all (the VM is
// already at/above the anchor, so a forward steer never refuses).
type steerRecordingVM struct {
	*orphanVMBase
	steers []ids.ID
}

func (m *steerRecordingVM) SetPreference(_ context.Context, id ids.ID) error {
	m.steers = append(m.steers, id)
	return nil
}

// TestAcceptWithCert_UnheldAnchorFallsBackToAccepted pins the single-store invariant at the
// finalize steer: the engine may only name a block its own VM provably holds. An engine that
// steers at a preference it obtained solely by verifying into the VM gets this for free; one
// that steers at a value derived from the consensus DAG does not.
//
// PreferredBuildTip is such a DAG value, so a node that fell behind can name a tip its own VM
// does not hold. Steering there is silently lossy rather than loud: the proposervm keeps its
// prior preference and returns nil (node/vms/proposervm/vm.go SetPreference), so the engine's
// own just-accepted blockID never gets steered either and the VM keeps building on a head
// older than the block consensus just finalized. The fallback is blockID, which VM.Accept has
// just applied and the VM therefore provably holds.
func TestAcceptWithCert_UnheldAnchorFallsBackToAccepted(t *testing.T) {
	e := newTestEngine()
	ctx := context.Background()

	lower := ids.GenerateTestID() // finalized here at height 1 — the VM holds this
	upper := ids.GenerateTestID() // the DAG anchor at height 2 — this node does not hold it

	cb := &Block{id: lower, parentID: ids.Empty, height: 1}
	if err := e.consensus.AddBlock(ctx, cb); err != nil {
		t.Fatalf("AddBlock: %v", err)
	}

	// The fallen-behind node: `upper` is absent from the VM's store entirely.
	vm := &steerRecordingVM{orphanVMBase: &orphanVMBase{
		head: lower,
		blocks: map[ids.ID]*mockBlock{
			lower: mb(lower, 1),
		},
	}}
	e.SetVM(vm)

	accepting := &advanceOnAcceptBlock{
		mockBlock: mb(lower, 1),
		onAccept: func() {
			e.consensus.mu.Lock()
			e.consensus.ledger = twoHeightLedger(lower, 1, upper)
			e.consensus.mu.Unlock()
		},
	}
	e.mu.Lock()
	e.pendingBlocks[lower] = &PendingBlock{ConsensusBlock: cb, VMBlock: accepting}
	e.mu.Unlock()

	cert := VerifiedQuorumCert{qc: &QuorumCert{
		Version:   QuorumCertVersion,
		Type:      QCFinality,
		Tier:      Nova,
		Position:  VotePosition{Height: 1, Round: 0, BlockID: lower, ParentID: ids.Empty, CanonicalID: lower},
		Threshold: 1,
	}}

	if err := e.acceptWithCertCore(ctx, lower, cert, false); err != nil {
		t.Fatalf("acceptWithCertCore: %v", err)
	}

	if len(vm.steers) != 1 {
		t.Fatalf("expected exactly one SetPreference, got %d (%v)", len(vm.steers), vm.steers)
	}
	if vm.steers[0] == upper {
		t.Fatalf("steered at an unheld block: SetPreference(%s) names a block this VM does not "+
			"hold; the proposervm answers with a warn and nil, so the just-accepted %s is never "+
			"steered either and the VM keeps building on its old head", upper, lower)
	}
	if vm.steers[0] != lower {
		t.Fatalf("expected the fallback steer at the held, just-accepted %s, got %s", lower, vm.steers[0])
	}
}

// TestHeldBuildTip_ResolvesOncePerSite pins the helper's three answers directly, so every
// steer site inherits one semantics: unheld -> fallback, held -> tip, no VM -> fallback.
func TestHeldBuildTip_ResolvesOncePerSite(t *testing.T) {
	e := newTestEngine()
	ctx := context.Background()

	held := ids.GenerateTestID()
	fallback := ids.GenerateTestID()

	cb := &Block{id: held, parentID: ids.Empty, height: 1}
	if err := e.consensus.AddBlock(ctx, cb); err != nil {
		t.Fatalf("AddBlock: %v", err)
	}
	if tip := e.PreferredBuildTip(); tip != held {
		t.Fatalf("precondition: PreferredBuildTip = %s, want %s", tip, held)
	}

	if got := e.HeldBuildTip(ctx, nil, fallback); got != fallback {
		t.Fatalf("nil VM: got %s, want fallback %s", got, fallback)
	}

	holds := &orphanVMBase{head: held, blocks: map[ids.ID]*mockBlock{held: mb(held, 1)}}
	if got := e.HeldBuildTip(ctx, holds, fallback); got != held {
		t.Fatalf("VM holds the tip: got %s, want %s", got, held)
	}

	empty := &orphanVMBase{head: fallback, blocks: map[ids.ID]*mockBlock{}}
	if got := e.HeldBuildTip(ctx, empty, fallback); got != fallback {
		t.Fatalf("VM does not hold the tip: got %s, want fallback %s", got, fallback)
	}
}
