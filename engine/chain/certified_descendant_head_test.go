// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// certified_descendant_head_test.go — the head = certified+1 false-positive halt.
//
// THE INCIDENT (devnet v1.36.33, all five validators). Each node hit, exactly once:
//
//	error VM accepted head is CONSENSUS-CERTIFIED and conflicts with the newly finalized
//	      block — refusing to orphan it  orphanedHeight=1364
//	fatal SetPreference would orphan a CONSENSUS-CERTIFIED block — refusing (fail-closed)
//	      error="cannot orphan finalized block at height: 1364 to common block at height: 1363"
//
// at three distinct height pairs (1258/1259, 1363/1364, 1364/1365), ALWAYS with
// certified = head − 1, and with every surviving node holding byte-identical blocks at
// every one of those heights: no fork anywhere. os.Exit(1) on a benign state.
//
// TWO defects, one crash:
//
//  1. PRODUCER. acceptWithCertCore releases t.mu across every VM call-out, then steers the
//     VM with its STALE local blockID. A finalize that completed in that window has already
//     advanced the ledger AND the EVM to blockID+1, so the steer is BACKWARDS and the EVM's
//     accepted-irreversibility guard (evm/core/blockchain.go: commonBlock < lastAccepted)
//     correctly refuses it. Fix: steer at the LIVE build anchor (PreferredBuildTip), which
//     is never below the VM's accepted head.
//
//  2. CLASSIFIER. reconcileVMToCertified asked only "is the head the ledger's certified
//     canonical at ITS OWN height?" — trivially TRUE for every healthy node whose head is
//     certified — and reported a double-finalization. It never established that `certified`
//     was at that same height. Fix: a certified head is only orphaned when `certified` is
//     NOT itself the ledger's certified canonical at its own height.
//
// The safety property is UNCHANGED and still fail-closed: a certified head is dropped only
// for a block our own ledger certifies on the same one chain. See
// TestReconcile_CertifiedHead_HaltsFailClosed (orphan_reconcile_test.go), which now proves
// the halt with BOTH blocks readable — the ledger, not an unreadable id, decides.
package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// twoHeightLedger builds a certified ledger holding a CONTIGUOUS two-height chain:
// byHeight[h] = lower and byHeight[h+1] = upper, tip = upper. This is exactly the state a
// node is in after a second finalize completes while an earlier one is still between its
// VM call-outs.
func twoHeightLedger(lower ids.ID, h uint64, upper ids.ID) FinalityLedger {
	led := seedLedger(lower, lower, h)
	led.byHeight[h+1] = finalizedEntry{canonical: upper, envelope: upper}
	led.tip, led.canonical, led.height = upper, upper, h+1
	return led
}

// TestReconcile_CertifiedDescendantHead_NoHalt is the incident, reduced. The VM's accepted
// head is the CERTIFIED block at 1364; the steer target is the CERTIFIED block at 1363 —
// its own parent on the one certified chain. SetPreference refuses (backwards steer), but
// nothing would be orphaned: the VM already CONTAINS 1363. The engine must not halt, and
// must not touch the VM head.
//
// Pre-fix this returned false ⇒ os.Exit(1) ⇒ five dead validators.
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
		t.Fatal("a VM head that is the CERTIFIED DESCENDANT of the finalized block is benign " +
			"(head = certified+1 on the one certified chain, nothing to orphan) — the engine must " +
			"NOT halt fail-closed")
	}
	if vm.reconcileCalls != 0 {
		t.Fatalf("the certified head must be LEFT ALONE (it already contains the certified block), "+
			"got %d ReconcilePreference calls", vm.reconcileCalls)
	}
	if vm.head != head {
		t.Fatalf("VM head must be untouched, got %s want %s", vm.head, head)
	}
}

// TestReconcile_CertifiedAncestorHead_NoHalt is the mirror: the head is certified BELOW the
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
		t.Fatal("a certified head BELOW the steer target (both on the one certified chain) is benign — must not halt")
	}
	if vm.reconcileCalls != 0 {
		t.Fatalf("nothing to reconcile, got %d calls", vm.reconcileCalls)
	}
}

// TestAcceptWithCert_StaleSteerIsNotIssued is the PRODUCER fix, end to end through the sole
// finalizer. The VM block's Accept advances the ledger to H+1 — a deterministic injection of
// the exact race window acceptWithCertCore documents (t.mu released across the VM call-outs).
// The engine must then steer at the LIVE anchor (H+1), never at its stale local H.
//
// Pre-fix the engine issued SetPreference(H) — backwards, into the EVM's orphan refusal and
// (with the pre-fix classifier) into os.Exit(1).
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
		t.Fatalf("STALE BACKWARDS STEER: SetPreference(%s) at height 1 while the ledger and the VM "+
			"are already at height 2 (%s) — this is the refusal the EVM turns into "+
			"\"cannot orphan finalized block\"", lower, upper)
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
