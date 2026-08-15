// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// zz_red_bootstrap_test.go — RED re-review probes for v1.36.45. DELETE AFTER REVIEW.
//
// The commit adds a SECOND replay lane with the same shape in
// AcceptBootstrapBlock. Two of the four fixes the author made to the catch-up lane
// were NOT carried across:
//
//   - HIGH-1 (parent binding before Accept): the catch-up lane now requires
//     blk.ParentID() == appliedHeadID. The bootstrap band checks height only.
//   - HIGH-2 (floor collapse when applied == 0): settledHeight guards it; the
//     bootstrap lane's inline floor does not.
package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// TestRED_BootstrapBandHasNoParentBinding — bootstrap_accept.go:139 gates the
// replay band on `h != floor+1` and nothing else. A sibling at the right height
// with an unrelated parent is Verified and Accepted onto the VM. The catch-up lane
// refuses exactly this shape (catchup_accept.go:245).
func TestRED_BootstrapBandHasNoParentBinding(t *testing.T) {
	const N = uint64(1_000_000)
	const k = 5

	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	tip := newTestBlock(N, ids.Empty, "applied@N")
	base.register(tip)
	if err := vm.SetPreference(context.Background(), tip.id); err != nil {
		t.Fatalf("seed applied head: %v", err)
	}
	gap := buildGap(base, tip, k)
	// Boot-seed shape: the ledger knows ONLY the top, so FinalizedAt is silent for
	// the band and the negative check cannot fire.
	top := gap[k-1]
	if _, err := rt.Transitive.consensus.FinalizeBranch(top.id, top.height, top.parentID); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}
	if _, _, known := rt.Transitive.consensus.FinalizedAt(N + 1); known {
		t.Fatalf("precondition: the ledger must not know height %d", N+1)
	}

	// A block at the right height whose parent is NOT our applied head.
	orphan := newTestBlock(N+1, ids.GenerateTestID(), "ORPHAN@N+1")
	base.register(orphan)

	err := rt.AcceptBootstrapBlock(context.Background(), orphan.bytes)
	if got := orphan.AcceptCalled(); got != 0 {
		t.Fatalf("EXPLOITED (HIGH-1 not carried to the bootstrap lane): a block at height %d "+
			"whose parent (%s) is not the applied head (%s) was Accepted onto the VM. "+
			"Accept=%d err=%v. bootstrap_accept.go:139 checks the HEIGHT only.",
			orphan.height, orphan.parentID, tip.id, got, err)
	}
	t.Logf("refused: %v", err)
}

// TestRED_BootstrapFloorCollapsesOnUnreadableHead — localLastAccepted returns
// (id, 0, nil) whenever GetBlock on the last-accepted id fails (pruned index /
// partial import — the author documents both as live conditions at
// catchup_accept.go:203-206 and guards `applied == 0` there). bootstrap_accept.go:128
// has no such guard, so the floor collapses to 0 and the whole band becomes
// out-of-order: the bootstrap loop can never advance.
func TestRED_BootstrapFloorCollapsesOnUnreadableHead(t *testing.T) {
	const N = uint64(1_000_000)
	const k = 5

	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	// A VM that NAMES a last-accepted block it cannot return.
	if err := vm.SetPreference(context.Background(), ids.GenerateTestID()); err != nil {
		t.Fatalf("seed unreadable head: %v", err)
	}
	tip := newTestBlock(N, ids.Empty, "ledger@N")
	base.register(tip)
	gap := buildGap(base, tip, k)
	for _, blk := range gap {
		if _, err := rt.Transitive.consensus.FinalizeBranch(blk.id, blk.height, blk.parentID); err != nil {
			t.Fatalf("fold ledger over %d: %v", blk.height, err)
		}
	}

	id, applied, aerr := rt.AppliedHead(context.Background())
	t.Logf("AppliedHead = (%s, %d, %v)", id, applied, aerr)
	if id == ids.Empty || applied != 0 {
		t.Fatalf("precondition: need a NON-empty head with an unreadable height, got (%s,%d)", id, applied)
	}

	// The bootstrap fetch loop delivers the band oldest-first. Every entry is
	// rejected because floor collapsed to 0 and h != 1.
	rejected := 0
	for _, blk := range gap {
		if err := rt.AcceptBootstrapBlock(context.Background(), blk.bytes); err != nil {
			rejected++
		}
	}
	settled, _ := rt.settledHeight(context.Background())
	if rejected == len(gap) {
		t.Fatalf("WEDGED (HIGH-2 not carried to the bootstrap lane): all %d band blocks "+
			"rejected. bootstrap_accept.go:128 lowered the floor to applied==0, so the band "+
			"is permanently out of order and the bootstrap loop cannot advance. The catch-up "+
			"lane guards this (settledHeight reports %d).", rejected, settled)
	}
	t.Logf("rejected %d of %d", rejected, len(gap))
}

// TestRED_BootstrapBandExecutesWithNoAuthorityWhenTheLedgerIsSilent — the polarity
// question. The catch-up lane's `!known` arm demands a verified cert
// (catchup_accept.go:270). The bootstrap lane's `!known` arm demands NOTHING: the
// negative check is skipped and the block is executed on frontier trust alone,
// inside a band the node reached by being BEHIND rather than by being empty.
func TestRED_BootstrapBandExecutesWithNoAuthorityWhenTheLedgerIsSilent(t *testing.T) {
	const N = uint64(1_000_000)
	const k = 5

	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	tip := newTestBlock(N, ids.Empty, "applied@N")
	base.register(tip)
	if err := vm.SetPreference(context.Background(), tip.id); err != nil {
		t.Fatalf("seed applied head: %v", err)
	}
	gap := buildGap(base, tip, k)
	top := gap[k-1]
	if _, err := rt.Transitive.consensus.FinalizeBranch(top.id, top.height, top.parentID); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}

	// A block the ledger has NOT recorded, at the contiguous next height, from an
	// unauthenticated peer. No cert accompanies it — the lane takes none.
	impostor := newTestBlock(N+1, tip.id, "IMPOSTOR@N+1")
	base.register(impostor)

	err := rt.AcceptBootstrapBlock(context.Background(), impostor.bytes)
	if got := impostor.AcceptCalled(); got != 0 {
		t.Logf("bootstrap band executed a block with NO ledger entry and NO cert "+
			"(Accept=%d err=%v) — frontier trust is the only authority here. "+
			"Contrast catchup_accept.go:270, which requires a verified Quasar cert "+
			"for the identical state.", got, err)
	} else {
		t.Fatalf("unexpected: refused (%v)", err)
	}
}

// TestRED_BootstrapBandVerifiesTheHeldCopyAnyway — the band's comment says
// "Prefer the copy the VM already holds ... re-verifying our own stored block asks
// the VM to insert it a second time", then calls held.Verify(ctx) regardless. The
// catch-up lane SKIPS Verify for a held block (catchup_accept.go:276). By the
// author's own model, the bootstrap band therefore fails on exactly the blocks it
// exists to serve: ones the VM already stores but never executed.
func TestRED_BootstrapBandVerifiesTheHeldCopyAnyway(t *testing.T) {
	const N = uint64(1_000_000)
	const k = 5

	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	tip := newTestBlock(N, ids.Empty, "applied@N")
	base.register(tip)
	if err := vm.SetPreference(context.Background(), tip.id); err != nil {
		t.Fatalf("seed applied head: %v", err)
	}
	gap := buildGap(base, tip, k)
	for _, blk := range gap {
		if _, err := rt.Transitive.consensus.FinalizeBranch(blk.id, blk.height, blk.parentID); err != nil {
			t.Fatalf("fold ledger over %d: %v", blk.height, err)
		}
	}

	next := gap[0]
	// The VM already holds AND already verified it (gossiped ahead into the store) —
	// the state the band's own comment names. A real VM refuses the second insert.
	if err := next.Verify(context.Background()); err != nil {
		t.Fatalf("priming verify: %v", err)
	}

	err := rt.AcceptBootstrapBlock(context.Background(), next.bytes)
	t.Logf("Accept=%d verifyCalls=%d err=%v", next.AcceptCalled(), next.VerifyCalls(), err)
	if next.AcceptCalled() == 0 {
		t.Fatalf("WEDGE: the bootstrap replay band re-Verified a block the VM already holds "+
			"and refused it (err=%v). The comment at bootstrap_accept.go:142-143 states the "+
			"rationale for skipping; the code does not skip.", err)
	}
}
