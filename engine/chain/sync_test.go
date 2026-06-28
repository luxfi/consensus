// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/ids"
)

// TestSyncState_RefusesBackwardRegression is the regression test for the
// SyncState monotonic-height guard. SyncState bypasses markFinalizedLocked by
// design (an import is an out-of-band reconcile, not an α-of-K finalize), so the
// monotonic invariant must be re-asserted there explicitly: a backward import
// (height below the already-finalized height) MUST be refused and MUST leave the
// finalized head untouched. Without the guard a shorter/older imported chain
// would silently regress finalizedHeight and un-finalize already-finalized
// blocks — re-opening the fork window the per-height guard closes.
func TestSyncState_RefusesBackwardRegression(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	// Seed a finalized head at height 100 through the real finalize path so the
	// per-height ledger is in the exact shape SyncState must respect.
	h100 := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(h100, 100, ids.Empty); err != nil {
		t.Fatalf("seed finalize at height 100: %v", err)
	}

	// A backward import at height 99 must be REFUSED with ErrSyncStateRegression…
	older := ids.GenerateTestID()
	if err := c.SyncState(older, 99); !errors.Is(err, ErrSyncStateRegression) {
		t.Fatalf("backward SyncState(h=99) should return ErrSyncStateRegression, got: %v", err)
	}
	// …and must NOT have regressed any finalized state (fail-closed no-op).
	if got := c.GetFinalizedTip(); got != h100 {
		t.Fatalf("finalized tip regressed: got %s want %s (height-100 head must be untouched)", got, h100)
	}
	if h, set := c.GetFinalizedHeight(); h != 100 || !set {
		t.Fatalf("finalizedHeight regressed: got %d set=%v want 100/true", h, set)
	}
	if existing, ok := c.FinalizedBlockAtHeight(100); !ok || existing != h100 {
		t.Fatalf("per-height ledger at 100 corrupted: ok=%v existing=%s", ok, existing)
	}
	if _, ok := c.FinalizedBlockAtHeight(99); ok {
		t.Fatalf("backward import leaked a ledger entry at the rejected height 99")
	}
}

// TestSyncState_EqualHeightDifferentBlockRefused verifies that a re-import at the
// already-finalized height with a DIFFERENT block is refused as equivocation
// (two distinct blocks at one finalized height), while a re-import with the SAME
// block at the SAME height is an idempotent success.
func TestSyncState_EqualHeightDifferentBlockRefused(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	h100 := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(h100, 100, ids.Empty); err != nil {
		t.Fatalf("seed finalize: %v", err)
	}

	// Same block, same height → idempotent OK (a benign reconcile).
	if err := c.SyncState(h100, 100); err != nil {
		t.Fatalf("idempotent SyncState(same block @100) should succeed, got: %v", err)
	}

	// Different block at the same finalized height → equivocation, refused.
	other := ids.GenerateTestID()
	if err := c.SyncState(other, 100); !errors.Is(err, ErrHeightAlreadyFinalized) {
		t.Fatalf("SyncState(different block @100) should return ErrHeightAlreadyFinalized, got: %v", err)
	}
	if got := c.GetFinalizedTip(); got != h100 {
		t.Fatalf("finalized tip changed on a refused equal-height import: got %s want %s", got, h100)
	}
}

// TestSyncState_ForwardImportAdvances verifies the guard does NOT impede the
// legitimate forward case: an import at a height above the current finalized
// head advances finalizedTip/Height (this is the normal admin_importChain /
// state-sync path and must remain a clean success).
func TestSyncState_ForwardImportAdvances(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	h100 := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(h100, 100, ids.Empty); err != nil {
		t.Fatalf("seed finalize: %v", err)
	}

	// Forward import to a mature height.
	h5000 := ids.GenerateTestID()
	if err := c.SyncState(h5000, 5000); err != nil {
		t.Fatalf("forward SyncState(h=5000) should succeed, got: %v", err)
	}
	if got := c.GetFinalizedTip(); got != h5000 {
		t.Fatalf("forward import did not advance tip: got %s want %s", got, h5000)
	}
	if h, _ := c.GetFinalizedHeight(); h != 5000 {
		t.Fatalf("forward import did not advance height: got %d want 5000", h)
	}
	// The import re-seeds the per-height ledger to the imported head; the stale
	// height-100 entry is no longer the source of truth.
	if _, ok := c.FinalizedBlockAtHeight(5000); !ok {
		t.Fatalf("forward import did not seed the ledger at the imported height 5000")
	}
}

// TestSyncState_EmptyResetAllowed verifies that an explicit genesis/empty reset
// (lastAcceptedID == ids.Empty) is NOT treated as a regression — it is a
// deliberate teardown, distinct from a backward import of a concrete head.
func TestSyncState_EmptyResetAllowed(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	h100 := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(h100, 100, ids.Empty); err != nil {
		t.Fatalf("seed finalize: %v", err)
	}

	if err := c.SyncState(ids.Empty, 0); err != nil {
		t.Fatalf("empty reset SyncState(Empty,0) should succeed, got: %v", err)
	}
	if got := c.GetFinalizedTip(); got != ids.Empty {
		t.Fatalf("empty reset should clear the finalized tip, got %s", got)
	}
}

// TestSyncState_EmptyHeadWithPositiveHeightRefused is the INFO-6 regression. An
// EMPTY import head paired with a POSITIVE height is contradictory (an empty head
// is the genesis/teardown reset, valid only at height 0). If allowed it would set
// finalizedTip=Empty while the seed branch is skipped (finalizedHeight stays
// stale) AND prune blocks below the positive height — the exact
// finalizedTip-vs-finalizedHeight desync ForcePreference was hardened against.
// SyncState must REFUSE it fail-closed, leaving finalized state AND the block pool
// untouched (no pruning).
func TestSyncState_EmptyHeadWithPositiveHeightRefused(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	// Finalized head at height 100.
	h100 := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(h100, 100, ids.Empty); err != nil {
		t.Fatalf("seed finalize at height 100: %v", err)
	}
	// Seed a low-height block in the pool so we can prove the refused import does
	// NOT prune it (the desync path would delete every block below the height).
	low := &Block{id: ids.GenerateTestID(), height: 50}
	if err := c.AddBlock(context.Background(), low); err != nil {
		t.Fatalf("seed low block @50: %v", err)
	}

	// Empty head at a positive height → refused with ErrSyncStateEmptyWithHeight.
	if err := c.SyncState(ids.Empty, 5000); !errors.Is(err, ErrSyncStateEmptyWithHeight) {
		t.Fatalf("SyncState(Empty, 5000) should return ErrSyncStateEmptyWithHeight, got: %v", err)
	}

	// Finalized state is untouched (the desync is prevented).
	if got := c.GetFinalizedTip(); got != h100 {
		t.Fatalf("refused empty-at-height import desynced the tip: got %s want %s", got, h100)
	}
	if h, set := c.GetFinalizedHeight(); h != 100 || !set {
		t.Fatalf("refused import corrupted finalizedHeight: got %d set=%v want 100/true", h, set)
	}
	if existing, ok := c.FinalizedBlockAtHeight(100); !ok || existing != h100 {
		t.Fatalf("refused import corrupted the per-height ledger at 100: ok=%v existing=%s", ok, existing)
	}
	// The low-height block MUST still be present — the refused import pruned nothing.
	if _, ok := c.GetBlock(low.id); !ok {
		t.Fatal("refused empty-at-height import pruned a live block below the height (the desync the guard prevents)")
	}

	// Sanity: the legitimate empty reset (Empty, 0) is still allowed (the guard is
	// precise — it gates only height>0, not all empty heads).
	if err := c.SyncState(ids.Empty, 0); err != nil {
		t.Fatalf("empty reset SyncState(Empty,0) must still succeed after the guard, got: %v", err)
	}
}
