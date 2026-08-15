// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// zz_red_replay_order_test.go — RED probes, delete after review.
package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// The replay branch sits ABOVE the `blk.Height() > fh+1` contiguity reject in
// AcceptCatchupBlock (catchup_accept.go:351 vs :354), so any height in
// (settled, ledgerHeight] is applied directly with no parent check at all.
func TestRED_ReplayAppliesOutOfOrderWithNoParentCheck(t *testing.T) {
	const N = uint64(1_000_000)
	const k = 12

	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)

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

	// Serve the TOP of the gap first — the responder walks DOWN from a requested id,
	// so this is the natural arrival order, not a contrived one.
	last := gap[k-1]
	cert := catchupCertFor(t, vs, chainID, last, []int{0, 1, 2, 3}, 3)
	err := rt.AcceptCatchupBlock(context.Background(), last.bytes, cert)

	if got := last.AcceptCalled(); got != 0 {
		t.Fatalf("EXPLOITED: height %d applied to the VM while heights %d..%d were never "+
			"applied (applied head was %d). Accept=%d err=%v",
			last.height, N+1, last.height-1, N, got, err)
	}
	for i := 0; i < k-1; i++ {
		if gap[i].AcceptCalled() != 0 {
			t.Fatalf("unexpected: intermediate %d applied", gap[i].height)
		}
	}
}

// settledHeight's absence test is `id == ids.Empty`, but localLastAccepted
// (bootstrap_accept.go:198) returns (id, 0, nil) — a NON-Empty id with height 0 —
// whenever GetBlock on the last-accepted id fails. The floor then collapses to 0 and
// every height up to the ledger becomes replay-eligible.
func TestRED_SettledHeightCollapsesToZeroOnUnreadableHead(t *testing.T) {
	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(1_000_000)
	// The VM names a last-accepted id whose block it cannot return — a real state after
	// an unclean stop / partial import / pruned index.
	orphanHead := ids.GenerateTestID()
	if err := vm.SetPreference(context.Background(), orphanHead); err != nil {
		t.Fatalf("seed head: %v", err)
	}
	ledgerTip := newTestBlock(N, ids.Empty, "ledger@N")
	base.register(ledgerTip)
	if _, err := rt.Transitive.consensus.FinalizeBranch(ledgerTip.id, N, ids.Empty); err != nil {
		t.Fatalf("fold ledger: %v", err)
	}

	id, applied, err := rt.AppliedHead(context.Background())
	t.Logf("AppliedHead = (%s, %d, %v)", id, applied, err)

	got, set := rt.settledHeight(context.Background())
	if set && got == 0 {
		t.Fatalf("EXPLOITED: settledHeight collapsed to 0 with ledger at %d — every height "+
			"1..%d is now replay-eligible, which the guard at catchup_accept.go:188-193 "+
			"claims to prevent", N, N)
	}
	t.Logf("settledHeight = (%d,%v)", got, set)
}
