// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// certified_steer_unreadable_test.go — an UNREADABLE steer target is not a safety violation.
//
// THE INCIDENT (lux-testnet luxd-0, node v1.36.58). The pod crash-looped on:
//
//	error VM accepted head is CONSENSUS-CERTIFIED and the finalized block is NOT on the
//	      certified chain — refusing to orphan it  orphanedHeight=12781
//	fatal SetPreference would orphan a CONSENSUS-CERTIFIED block — refusing (fail-closed)
//	      error="context canceled"
//
// Every neighbouring VM call-out failed with the same `context canceled`: the engine was
// SHUTTING DOWN. reconcileVMToCertified declines to crash for exactly that reason on both of
// its other read paths (LastAccepted, GetBlock(head)) — but certifiedAtItsHeight folded its
// GetBlock error into a bare `false`, and `false` was the ONE value routed to os.Exit(1). So
// an unreadable block was indistinguishable from a proven double-finalization, and an orderly
// stop became a fatal on a node whose finality ledger was never in question.
//
// The fix makes the predicate three-valued — (true,nil) / (false,nil) / (_,err) — so only a
// block we READ and the ledger PROVES uncertified may halt. The safety property is UNCHANGED:
// see TestReconcile_CertifiedHead_HaltsFailClosed (orphan_reconcile_test.go), which halts with
// BOTH blocks readable, and TestCertifiedAtItsHeight_ThreeValued below, which pins all three
// outcomes so a future refactor cannot quietly re-collapse them.
package chain

import (
	"context"
	"testing"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// ctxHonoringOrphanVM fails GetBlock with the context's error once the context is cancelled —
// the real shutdown shape. Blocks already in the map are served from "cache" regardless, so a
// test can make the HEAD readable while the STEER TARGET requires a live read that fails.
type ctxHonoringOrphanVM struct {
	*orphanVMBase
	cached         map[ids.ID]*mockBlock // served even after cancellation
	reconcileCalls int
}

func (m *ctxHonoringOrphanVM) GetBlock(ctx context.Context, id ids.ID) (block.Block, error) {
	if b, ok := m.cached[id]; ok {
		return b, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return m.orphanVMBase.GetBlock(ctx, id)
}

func (m *ctxHonoringOrphanVM) ReconcilePreference(_ context.Context, id ids.ID) error {
	m.reconcileCalls++
	m.head = id
	return nil
}

// TestReconcile_CertifiedHead_UnreadableSteerTarget_NoHalt: the VM's head IS the ledger's
// certified canonical at its height, and the steer target cannot be read at all. Nothing is
// proven about the target, so nothing may be orphaned and the node must NOT halt.
//
// Pre-fix: GetBlock error ⇒ false ⇒ "NOT on the certified chain" ⇒ os.Exit(1).
func TestReconcile_CertifiedHead_UnreadableSteerTarget_NoHalt(t *testing.T) {
	e := newTestEngine()

	head := ids.GenerateTestID()      // certified at 12781 — the VM's accepted head
	certified := ids.GenerateTestID() // the steer target — UNREADABLE
	e.consensus.ledger = seedLedger(head, head, 12781)

	vm := &reconcilingOrphanVM{orphanVMBase: &orphanVMBase{
		head: head,
		// `certified` is deliberately ABSENT ⇒ GetBlock returns "block not found".
		blocks: map[ids.ID]*mockBlock{head: mb(head, 12781)},
	}}

	if !e.reconcileVMToCertified(context.Background(), vm, certified, errOrphan) {
		t.Fatal("an UNREADABLE steer target proves no safety violation — the engine must NOT " +
			"halt fail-closed (only a block we read and the ledger proves uncertified may halt)")
	}
	if vm.reconcileCalls != 0 {
		t.Fatalf("an unclassifiable state must leave the VM head ALONE, got %d ReconcilePreference calls",
			vm.reconcileCalls)
	}
	if vm.head != head {
		t.Fatalf("VM head must be untouched, got %s want %s", vm.head, head)
	}
}

// TestReconcile_CertifiedHead_CancelledContext_NoHalt is the incident verbatim: the engine is
// shutting down, so the steer target's read fails with context.Canceled while the head is
// still served from cache. This is the state that killed luxd-0 on every restart.
func TestReconcile_CertifiedHead_CancelledContext_NoHalt(t *testing.T) {
	e := newTestEngine()

	head := ids.GenerateTestID()
	certified := ids.GenerateTestID()
	e.consensus.ledger = seedLedger(head, head, 12781)

	vm := &ctxHonoringOrphanVM{
		orphanVMBase: &orphanVMBase{
			head:   head,
			blocks: map[ids.ID]*mockBlock{head: mb(head, 12781), certified: mb(certified, 12780)},
		},
		cached: map[ids.ID]*mockBlock{head: mb(head, 12781)},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the engine is stopping — exactly what failBootstrapChain does

	if !e.reconcileVMToCertified(ctx, vm, certified, errOrphan) {
		t.Fatal("a cancelled context is a SHUTDOWN, not a double-finalization — the engine must " +
			"NOT os.Exit(1) while stopping")
	}
	if vm.reconcileCalls != 0 {
		t.Fatalf("nothing may be reconciled on an unreadable classification, got %d calls", vm.reconcileCalls)
	}
	if vm.head != head {
		t.Fatalf("VM head must be untouched, got %s want %s", vm.head, head)
	}
}

// TestCertifiedAtItsHeight_ThreeValued pins the predicate's contract: the error is a THIRD
// outcome and is never folded into the bool. A future refactor that collapses it back into
// `false` fails here rather than in production at 3am.
func TestCertifiedAtItsHeight_ThreeValued(t *testing.T) {
	onChain := ids.GenerateTestID()
	offChain := ids.GenerateTestID()
	missing := ids.GenerateTestID()

	e := newTestEngine()
	e.consensus.ledger = seedLedger(onChain, onChain, 7)

	vm := &orphanVMBase{
		head: onChain,
		blocks: map[ids.ID]*mockBlock{
			onChain:  mb(onChain, 7),
			offChain: mb(offChain, 7), // same height, NOT the ledger's canonical there
		},
	}
	ctx := context.Background()

	if ok, err := e.certifiedAtItsHeight(ctx, vm, onChain); err != nil || !ok {
		t.Fatalf("a block the ledger certifies at its own height ⇒ (true, nil), got (%v, %v)", ok, err)
	}
	if ok, err := e.certifiedAtItsHeight(ctx, vm, offChain); err != nil || ok {
		t.Fatalf("a READ block the ledger does not certify at its height ⇒ (false, nil) — the "+
			"halt-authorising answer — got (%v, %v)", ok, err)
	}
	ok, err := e.certifiedAtItsHeight(ctx, vm, missing)
	if err == nil {
		t.Fatal("an UNREADABLE block must report an ERROR, not (false, nil) — conflating the two " +
			"is what turned a shutdown into a fatal")
	}
	if ok {
		t.Fatalf("an unreadable block must never report certified=true, got %v", ok)
	}
}
