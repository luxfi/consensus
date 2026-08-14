// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/ids"
)

// TestSyncState_RefusesBackwardRegression pins the SyncState monotonic-height
// rule. SyncState bypasses markFinalizedLocked by design (an import is an
// out-of-band reconcile, not an α-of-K finalize), so the monotonic invariant must
// be re-asserted there explicitly: a backward import (height below the
// already-finalized height) is refused and leaves the finalized head untouched.
// Otherwise a shorter or older imported chain silently regresses finalizedHeight
// and un-finalizes finalized blocks, re-opening the fork window the per-height
// guard closes.
func TestSyncState_RefusesBackwardRegression(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	// Seed a finalized head at height 100 through the real finalize path so the
	// per-height ledger is in the exact shape SyncState must respect.
	h100 := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(h100, 100, ids.Empty); err != nil {
		t.Fatalf("seed finalize at height 100: %v", err)
	}

	// A backward import at height 99 must be refused with ErrSyncStateRegression…
	older := ids.GenerateTestID()
	if err := c.SyncState(older, 99); !errors.Is(err, ErrSyncStateRegression) {
		t.Fatalf("backward SyncState(h=99) should return ErrSyncStateRegression, got: %v", err)
	}
	// …and must not have regressed any finalized state (fail-closed no-op).
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

// TestSyncState_DifferentBlockIsHintNotEquivocation pins what an import may and may
// not conclude. Seeding finality from vm.LastAccepted makes a re-import at an
// already-finalized height with a different block look like equivocation, which is
// fatal — but a local import is not a finality source, so it cannot witness a
// conflict. SyncState is a non-authoritative recovery hint: a different block at a
// certified height does not equivocate, does not touch the certified frontier, and
// returns cleanly. Equivocation is decided exclusively by conflicting certs over
// canonical commitments.
func TestSyncState_DifferentBlockIsHintNotEquivocation(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	h100 := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(h100, 100, ids.Empty); err != nil {
		t.Fatalf("seed certified finalize: %v", err)
	}

	// Same block, same height → still fine (a benign reconcile hint).
	if err := c.SyncState(h100, 100); err != nil {
		t.Fatalf("SyncState(same block @100) should succeed, got: %v", err)
	}

	// A different block at the same certified height is a hint, not equivocation:
	// accepted as non-authoritative, certified finality untouched, no
	// ErrHeightAlreadyFinalized.
	other := ids.GenerateTestID()
	if err := c.SyncState(other, 100); err != nil {
		t.Fatalf("SyncState(different block @100) must be a clean hint, got: %v", err)
	}
	// The certified frontier is untouched: height stays 100, the canonical tip is
	// still h100's canonical (== h100 here, a non-wrapped block).
	if h, set := c.GetFinalizedHeight(); !set || h != 100 {
		t.Fatalf("certified height moved on a hint import: got (%d,%v) want (100,true)", h, set)
	}
	if got := c.GetNovaTip(); got != h100 {
		t.Fatalf("certified tip changed on a hint import: got %s want %s", got, h100)
	}
}

// TestSyncState_EmptyResetYieldsCleanGenesis pins the reset as a whole value: an empty
// import head (ids.Empty, height 0) on a ledger already finalized above genesis resets
// finality to a clean genesis — tip Empty, height 0, unset. A half reset that nulls the
// tip while leaving height and byHeight stale desyncs tip from height, and pathFromTip
// then seeks a tip that is not there, wedging every future cert. So the test finalizes a
// fresh cert after the reset: a usable ledger is the observable proof of a whole reset.
func TestSyncState_EmptyResetYieldsCleanGenesis(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	h100 := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(h100, 100, ids.Empty); err != nil {
		t.Fatalf("seed finalize: %v", err)
	}

	// Empty reset of a live ledger → clean genesis value, no error.
	if err := c.SyncState(ids.Empty, 0); err != nil {
		t.Fatalf("empty reset SyncState(Empty,0) should succeed, got: %v", err)
	}
	if got := c.GetFinalizedTip(); got != ids.Empty {
		t.Fatalf("empty reset should clear the tip, got %s", got)
	}
	// No desync: height/set are reset too (not left stale at 100/true).
	if h, set := c.GetFinalizedHeight(); set || h != 0 {
		t.Fatalf("empty reset left a tip-vs-height desync: got (%d,%v) want (0,false)", h, set)
	}
	// No wedge: a fresh cert finalizes cleanly (first-finalize seeds), proving the
	// post-reset ledger is usable rather than permanently stuck seeking an Empty tip.
	next := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(next, 7, ids.Empty); err != nil {
		t.Fatalf("post-reset finalize wedged on a tip-vs-height desync: %v", err)
	}
	if got := c.GetFinalizedTip(); got != next {
		t.Fatalf("post-reset finalize did not advance: got %s want %s", got, next)
	}
}

// TestSyncState_ForwardImportAdvancesBuildAnchorNotFinality splits the two things a
// forward import moves. It advances the build anchor (where the VM builds —
// GetFinalizedTip) so the node builds on the imported head, and it does not advance
// certified finality: vm.LastAccepted is a recovery hint, so the certified height
// stays at the last QC-backed height and byHeight gains no entry at the imported
// height, which only a cert may write. Finality at that height arrives later through
// the bootstrap/cert path.
func TestSyncState_ForwardImportAdvancesBuildAnchorNotFinality(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	h100 := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(h100, 100, ids.Empty); err != nil {
		t.Fatalf("seed certified finalize: %v", err)
	}

	// Forward import (hint) to a mature height.
	h5000 := ids.GenerateTestID()
	if err := c.SyncState(h5000, 5000); err != nil {
		t.Fatalf("forward SyncState(h=5000) should succeed, got: %v", err)
	}
	// The build anchor advances to the imported head (the forward hint outranks the
	// lower certified tip) so the VM builds where the node has state.
	if got := c.GetFinalizedTip(); got != h5000 {
		t.Fatalf("forward import did not advance the build anchor: got %s want %s", got, h5000)
	}
	// Certified finality does not advance — an import is a hint, never a finality
	// source. The certified height stays 100 (the last QC-backed finalize).
	if h, set := c.GetFinalizedHeight(); !set || h != 100 {
		t.Fatalf("forward import wrongly advanced the certified height: got (%d,%v) want (100,true)", h, set)
	}
	// No certified ledger entry was written at the imported height (only a cert can).
	if _, ok := c.FinalizedBlockAtHeight(5000); ok {
		t.Fatalf("forward import wrote a certified ledger entry at 5000 — a hint must not finalize")
	}
}

// TestSyncState_EmptyResetAllowed verifies that an explicit genesis/empty reset
// (lastAcceptedID == ids.Empty) is not treated as a regression — it is a
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

// TestSyncState_EmptyHeadWithPositiveHeightRefused pins the one import shape that
// contradicts itself. An empty head is the genesis/teardown reset and is meaningful
// only at height 0; paired with a positive height it would set finalizedTip=Empty
// while skipping the seed branch, so finalizedHeight stays stale, and it would prune
// every block below that height — the same finalizedTip-vs-finalizedHeight desync
// ForcePreference is hardened against. SyncState refuses it fail-closed, leaving
// finalized state and the block pool untouched.
func TestSyncState_EmptyHeadWithPositiveHeightRefused(t *testing.T) {
	c := NewChainConsensus(4, 3, 2)

	// Finalized head at height 100.
	h100 := ids.GenerateTestID()
	if _, err := c.FinalizeBranch(h100, 100, ids.Empty); err != nil {
		t.Fatalf("seed finalize at height 100: %v", err)
	}
	// Seed a low-height block in the pool so we can prove the refused import does
	// not prune it (the desync path deletes every block below the height).
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
	// The low-height block must still be present — the refused import pruned nothing.
	if _, ok := c.GetBlock(low.id); !ok {
		t.Fatal("refused empty-at-height import pruned a live block below the height (the desync the guard prevents)")
	}

	// Sanity: the legitimate empty reset (Empty, 0) is still allowed (the guard is
	// precise — it gates only height>0, not all empty heads).
	if err := c.SyncState(ids.Empty, 0); err != nil {
		t.Fatalf("empty reset SyncState(Empty,0) must still succeed after the guard, got: %v", err)
	}
}
