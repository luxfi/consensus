// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// bootstrap_floor_test.go — the bootstrap floor is what we already HOLD, and the VM is an
// authority on that.
//
// THE INCIDENT (lux-testnet luxd-0, node v1.36.58). The pod could not sync. Forever:
//
//	error bootstrap: VM.Accept failed after Verify (sync will halt at next block)
//	      error="expected accepted block to have parent 0xc0166ce9…:12781 but got 0x…:8875"
//
// with the right-hand height climbing one at a time from ~8266 while the left stayed at the
// VM's head. Both halves of the VM agreed at boot ("initialized proposervm forkHeight=1
// lastAcceptedHeight=12828", inner ZAP init height=12828), so there was no index skew and no
// fork — every block hash matched the healthy fleet.
//
// The cause was the FLOOR. luxd-0 had lost its signing journal ("vote-once: booting with NO
// durable signing memory — no certified floor, no vote bindings, no export frontier"), so its
// in-memory ledger booted UNSET. The first cert it then processed SEEDED certified history at
// that cert's height — correctly, with no contiguity requirement, since there is no prior tip
// to extend — and an ancient cert therefore placed the certified tip thousands of blocks below
// what the VM had already accepted. From then on the set-branch trusted that stale seed and
// stopped consulting the VM, feeding it its own ancient history. The VM can never accept any
// of it: the accept invariant is parent-hash equality against its accepted head, so each block
// cost a full re-execution and was refused, until the no-progress watchdog stopped the chain.
//
// The un-seeded branch already binds to the VM's real head to stop a peer steering the start
// height (M2 FIRST-BLOCK ANCHOR, TestBootstrap_M2_FirstBlockAnchorsToLocalLastAccepted). That
// protection was silently bypassed the instant a cert seeded the ledger. The floor is now the
// MAX of the two authorities in both branches.
package chain

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/luxfi/ids"
)

// TestBootstrap_FloorIsTheVMHead_NotAStaleLedgerSeed: certified history seeded far BELOW the
// VM's accepted head must not turn bootstrap into a re-execution treadmill. A block the VM
// already holds is skipped cleanly — never Verified, never handed to VM.Accept.
func TestBootstrap_FloorIsTheVMHead_NotAStaleLedgerSeed(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)
	ctx := context.Background()

	// The VM holds the chain through height 100 — this node's real STATE.
	head := newTestBlock(100, ids.GenerateTestID(), "vm-head@100")
	vm.register(head)
	if err := vm.SetPreference(ctx, head.id); err != nil {
		t.Fatalf("precondition: set VM head: %v", err)
	}
	if _, vmH, err := rt.localLastAccepted(ctx); err != nil || vmH != 100 {
		t.Fatalf("precondition: VM last-accepted must be 100, got (%d, %v)", vmH, err)
	}

	// An ancient cert seeds certified history at 10 — legitimate, and 90 blocks below the
	// VM. This is the state a node boots into having lost its signing journal.
	ancient := newTestBlock(10, ids.GenerateTestID(), "ancient-cert@10")
	seedBehindAt(t, rt, vm, ancient)

	// A peer serves height 11: contiguous per the LEDGER (fh+1), but a block the VM accepted
	// long ago. Pre-fix this was re-executed and handed to VM.Accept, which refused it.
	stale := newTestBlock(11, ancient.id, "stale@11")
	vm.register(stale)
	if err := rt.AcceptBootstrapBlock(ctx, stale.bytes); err != nil {
		t.Fatalf("a block at or below the VM's accepted head must be SKIPPED cleanly "+
			"(the VM already holds it), got: %v", err)
	}
	if got := atomic.LoadInt64(&stale.verifyCount); got != 0 {
		t.Fatalf("a block the VM already holds must NOT be re-executed — Verify ran %d×; "+
			"this is the treadmill that burned luxd-0's CPU for hours", got)
	}
	if got := stale.AcceptCalled(); got != 0 {
		t.Fatalf("a block the VM already holds must never reach VM.Accept — ran %d×", got)
	}

	// The floor did not become a new ceiling: everything up to the VM head is skipped, not
	// rejected, so an ordered feed walks through the already-held range without error.
	for h := uint64(12); h <= 100; h += 22 {
		b := newTestBlock(h, ids.GenerateTestID(), "already-held")
		vm.register(b)
		if err := rt.AcceptBootstrapBlock(ctx, b.bytes); err != nil {
			t.Fatalf("height %d is at or below the VM head and must be skipped, got: %v", h, err)
		}
	}
}

// TestBootstrap_FloorDoesNotManufactureFinality is the OTHER half, deliberately NOT "fixed":
// once the already-held range is skipped, the first genuinely new block (VM head + 1) is
// attempted and the ledger REFUSES to finalize it, because certified history is still parked
// at the ancient seed and FinalizeBranch requires contiguity from its own tip.
//
// That refusal is correct and must stay. A bootstrap fetch must never manufacture finality
// from local VM state — that is exactly what incident-1082814 PART-A forbade, and seeding the
// ledger from vm.LastAccepted is how a locally-held sibling once fabricated a finalized entry
// that a peer's legitimate cert then "conflicted" with. So this fix stops the futile
// re-execution and nothing more: a node that lost its certified history cannot re-derive PROOF
// for heights whose certs no peer retains, and that gap is a real state, not a bug to paper
// over here.
func TestBootstrap_FloorDoesNotManufactureFinality(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)
	ctx := context.Background()

	head := newTestBlock(100, ids.GenerateTestID(), "vm-head@100")
	vm.register(head)
	if err := vm.SetPreference(ctx, head.id); err != nil {
		t.Fatalf("precondition: set VM head: %v", err)
	}
	ancient := newTestBlock(10, ids.GenerateTestID(), "ancient-cert@10")
	seedBehindAt(t, rt, vm, ancient)

	next := newTestBlock(101, head.id, "new@101")
	vm.register(next)
	err := rt.AcceptBootstrapBlock(ctx, next.bytes)
	if err == nil {
		t.Fatal("SAFETY: a block above the VM head must NOT finalize while certified history " +
			"is parked below it — the ledger's contiguity is the authority, and bootstrap may " +
			"not seed finality from local VM state (incident-1082814 PART-A)")
	}
	if !errors.Is(err, ErrBootstrapBlockRejected) {
		t.Fatalf("the refusal must be a clean ErrBootstrapBlockRejected, got: %v", err)
	}
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != 10 {
		t.Fatalf("certified history must be UNCHANGED at the ancient seed, got %d", fh)
	}
}
