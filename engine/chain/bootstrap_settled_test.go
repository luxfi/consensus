// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// bootstrap_settled_test.go — the bootstrap lane must execute what the ledger decided
// and the VM never ran, exactly as the runtime catch-up lane must.
//
// Both lanes had the same discard, keyed on the same wrong reading: a block at or below
// the LEDGER height was skipped as already-synced, so a node whose ledger ran ahead of
// its VM dropped every block in the gap with a nil error and re-fetched the same batch
// forever. The runtime lane was fixed first, and a live node stayed wedged anyway —
// because it was in BOOTSTRAP (isBootstrapped=false), running this lane.
package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// TestBootstrap_ExecutesTheBandTheLedgerDecided strands a bootstrapping node in the
// live shape: ledger folded to N+k, VM applied only N. The fetched blocks N+1..N+k are
// all at or below the ledger height, and a gate reading the ledger alone skips every
// one of them. The assertion is VM.Accept — the finalized height is already N+k before
// a single block is served, so it proves nothing.
func TestBootstrap_ExecutesTheBandTheLedgerDecided(t *testing.T) {
	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, _, rec := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(1_000_000)
	const k = 9

	tip := newTestBlock(N, ids.Empty, "applied@N")
	base.register(tip)
	if err := vm.SetPreference(context.Background(), tip.id); err != nil {
		t.Fatalf("seed applied head: %v", err)
	}
	gap := buildGap(base, tip, k)

	// Fold the ledger across the whole band with no VM execution — what
	// applyBranchFinalization does on a pendingBlocks miss.
	for _, blk := range gap {
		if _, err := rt.Transitive.consensus.FinalizeBranch(blk.id, blk.height, blk.parentID); err != nil {
			t.Fatalf("fold ledger over height %d: %v", blk.height, err)
		}
	}
	if fh, set := rt.Transitive.consensus.GetFinalizedHeight(); !set || fh != N+uint64(k) {
		t.Fatalf("precondition: ledger at (%d,%v), want %d", fh, set, N+uint64(k))
	}
	if _, applied, err := rt.localLastAccepted(context.Background()); err != nil || applied != N {
		t.Fatalf("precondition: applied head (%d,%v), want %d", applied, err, N)
	}

	// The bootstrap fetch loop delivers the band oldest-first.
	for i, blk := range gap {
		if err := rt.AcceptBootstrapBlock(context.Background(), blk.bytes); err != nil {
			t.Fatalf("band[%d] (height %d): %v — the bootstrap lane still discards what "+
				"the ledger decided and the VM never ran", i, blk.height, err)
		}
		if got := blk.AcceptCalled(); got != 1 {
			t.Fatalf("band[%d] (height %d) must reach the VM exactly once, got %d", i, blk.height, got)
		}
	}

	// The load-bearing assertion: the applied head closed the whole gap.
	if _, applied, err := rt.localLastAccepted(context.Background()); err != nil || applied != N+uint64(k) {
		t.Fatalf("applied head = %d (err %v), want %d — the node is still wedged", applied, err, N+uint64(k))
	}

	// Replay executes decided blocks; it must not vote or gossip certs.
	rec.mu.Lock()
	votes, certs := len(rec.votes), len(rec.certs)
	rec.mu.Unlock()
	if votes != 0 || certs != 0 {
		t.Fatalf("replay must not vote (%d) or gossip certs (%d)", votes, certs)
	}
}

// TestBootstrap_RefusesABandBlockTheLedgerContradicts is the negative check: within the
// band, a block at a height the ledger finalized DIFFERENTLY is refused and never
// reaches the VM — frontier trust does not outrank the node's own decided state.
func TestBootstrap_RefusesABandBlockTheLedgerContradicts(t *testing.T) {
	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(1_000_000)
	tip := newTestBlock(N, ids.Empty, "applied@N")
	base.register(tip)
	if err := vm.SetPreference(context.Background(), tip.id); err != nil {
		t.Fatalf("seed applied head: %v", err)
	}
	gap := buildGap(base, tip, 3)
	for _, blk := range gap {
		if _, err := rt.Transitive.consensus.FinalizeBranch(blk.id, blk.height, blk.parentID); err != nil {
			t.Fatalf("fold ledger over height %d: %v", blk.height, err)
		}
	}

	impostor := newTestBlock(N+1, tip.id, "impostor@N+1")
	base.register(impostor)
	if err := rt.AcceptBootstrapBlock(context.Background(), impostor.bytes); err == nil {
		t.Fatal("the bootstrap lane executed a block the ledger finalized differently")
	}
	if got := impostor.AcceptCalled(); got != 0 {
		t.Fatalf("the impostor reached the VM: Accept called %d times", got)
	}
}
