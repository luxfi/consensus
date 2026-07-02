// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/luxfi/ids"
)

// reconcile_test.go — the teeth of the BOUNDED PHANTOM-FLOOR RECONCILE (reconcile.go).
//
// The scenario mirrors the frozen testnet C-Chain exactly:
//   - durable decided-floor = 429 (phantom, uniform across the fleet),
//   - fleet-max VM-applied height = 421 (target),
//   - a behind node's own VM at 415, a caught-up node's VM at 421.
// The abandoned range is (421, 429] — heights no VM applied and no observer saw. The
// committed window (<=421) is retained for cert-carrying catch-up, never abandoned.

// seedPhantomEngine builds an engine whose durable vote-guard already holds a phantom floor
// and a set of (height->canonical, lockRound) bindings, so the engine boots with
// decidedFloor == floor and the bindings seeded (exactly WithVoteGuard's production path).
func seedPhantomEngine(t *testing.T, floor uint64, bindings map[SlotKey]ids.ID, lockRounds map[uint64]uint32) (*Transitive, VoteGuardStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "vote-guard")
	store, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard: %v", err)
	}
	if err := store.Persist(bindings, lockRounds, floor); err != nil {
		t.Fatalf("seed Persist: %v", err)
	}
	// Reopen so the engine seeds from the fsync'd file (the real boot path).
	store2, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	p := params5Prod()
	p.ViewChange = true
	e, _ := newQuorumEngineOpts(t, p, newTestValidatorSet(5), 0, &recordingGossiper{}, WithVoteGuard(store2))
	e.slotMu.Lock()
	got := e.decidedFloor
	e.slotMu.Unlock()
	if got != floor {
		t.Fatalf("precondition: seeded engine decidedFloor=%d, want %d", got, floor)
	}
	return e, store2, path
}

// TestReconcile_LowersFloor_DropsPhantom_KeepsCommitted is the core: the floor drops to the
// fleet-max (421), phantom bindings/locks ABOVE it are dropped, the committed window
// AT/BELOW it is retained (for catch-up).
func TestReconcile_LowersFloor_DropsPhantom_KeepsCommitted(t *testing.T) {
	b417, b421, b425 := ids.GenerateTestID(), ids.GenerateTestID(), ids.GenerateTestID()
	b429 := ids.GenerateTestID()
	e, _, _ := seedPhantomEngine(t, 429,
		map[SlotKey]ids.ID{{Height: 417}: b417, {Height: 421}: b421, {Height: 425}: b425, {Height: 429}: b429},
		map[uint64]uint32{417: 2, 421: 3, 425: 5}, // 429 has NO lock round → seeds a recoveredLock
	)

	got, err := e.reconcilePhantomFloor(421 /*target*/, 421 /*localVM*/)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got != 421 {
		t.Fatalf("reconciled floor = %d, want 421", got)
	}

	e.slotMu.Lock()
	defer e.slotMu.Unlock()
	if e.decidedFloor != 421 {
		t.Fatalf("decidedFloor = %d, want 421 (must be LOWERED from 429)", e.decidedFloor)
	}
	// Committed window (<=421) retained.
	if _, ok := e.committedSlot[SlotKey{Height: 417}]; !ok {
		t.Fatal("binding 417 (<=safeFloor) must be RETAINED — it is committed, recovered via catch-up")
	}
	if _, ok := e.committedSlot[SlotKey{Height: 421}]; !ok {
		t.Fatal("binding 421 (==safeFloor) must be RETAINED")
	}
	// Phantom range (>421) dropped — bindings, lock rounds, and recovered hard-locks.
	if _, ok := e.committedSlot[SlotKey{Height: 425}]; ok {
		t.Fatal("phantom binding 425 (>safeFloor) must be DROPPED")
	}
	if _, ok := e.committedSlot[SlotKey{Height: 429}]; ok {
		t.Fatal("phantom binding 429 (>safeFloor) must be DROPPED")
	}
	if _, ok := e.lockRounds[425]; ok {
		t.Fatal("phantom lockRound 425 must be DROPPED")
	}
	if _, ok := e.recoveredLocks[429]; ok {
		t.Fatal("phantom recoveredLock 429 must be DROPPED")
	}
	if e.lockRounds[421] != 3 {
		t.Fatalf("committed lockRound 421 must be RETAINED, got %d", e.lockRounds[421])
	}
}

// TestReconcile_DurableBypassesMonotonicGuard proves the store write LOWERED the fsync'd
// floor — the whole point of Reconcile vs Persist. A reopened file reports 421, not 429.
// (A Persist call would have monotonic-clamped it back up to 429.)
func TestReconcile_DurableBypassesMonotonicGuard(t *testing.T) {
	e, _, path := seedPhantomEngine(t, 429,
		map[SlotKey]ids.ID{{Height: 429}: ids.GenerateTestID()}, nil)

	if _, err := e.reconcilePhantomFloor(421, 421); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	reopened, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if f := reopened.FinalizedThrough(); f != 421 {
		t.Fatalf("DURABLE FLOOR NOT LOWERED: reopened FinalizedThrough=%d, want 421 (Reconcile must "+
			"bypass the monotonic clamp — a phantom 429 would re-seed forever otherwise)", f)
	}
	// And the reopened floor is now a LOWER monotonic base: a subsequent Persist below it clamps
	// to 421 (never resurrects 429), while above it advances normally.
	if err := reopened.Persist(map[SlotKey]ids.ID{}, nil, 400); err != nil {
		t.Fatalf("Persist(400): %v", err)
	}
	if f := reopened.FinalizedThrough(); f != 421 {
		t.Fatalf("post-reconcile Persist(400) floor=%d, want 421 (monotonic clamp base is now 421, not 429)", f)
	}
	if err := reopened.Persist(map[SlotKey]ids.ID{}, nil, 430); err != nil {
		t.Fatalf("Persist(430): %v", err)
	}
	if f := reopened.FinalizedThrough(); f != 430 {
		t.Fatalf("post-reconcile Persist(430) floor=%d, want 430 (forward advance still works)", f)
	}
}

// TestReconcile_IntrinsicGuard_NeverAbandonsOwnCommittedBlocks proves the intrinsic guard:
// a WRONG-LOW target can never abandon a block THIS node itself applied. localVM=421 with a
// bogus target=415 still reconciles only to 421 (keeps 416..421).
func TestReconcile_IntrinsicGuard_NeverAbandonsOwnCommittedBlocks(t *testing.T) {
	b418, b429 := ids.GenerateTestID(), ids.GenerateTestID()
	e, _, _ := seedPhantomEngine(t, 429,
		map[SlotKey]ids.ID{{Height: 418}: b418, {Height: 429}: b429}, nil)

	// Operator error: target below this node's own applied head.
	got, err := e.reconcilePhantomFloor(415 /*WRONG target*/, 421 /*localVM — node applied through 421*/)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got != 421 {
		t.Fatalf("intrinsic guard failed: reconciled to %d, want 421 (max(target=415, localVM=421))", got)
	}
	e.slotMu.Lock()
	defer e.slotMu.Unlock()
	if e.decidedFloor != 421 {
		t.Fatalf("decidedFloor=%d, want 421 — a wrong-low target must NOT abandon own committed blocks", e.decidedFloor)
	}
	if _, ok := e.committedSlot[SlotKey{Height: 418}]; !ok {
		t.Fatal("binding 418 (<=own VM head 421) must be RETAINED despite the wrong-low target")
	}
}

// TestReconcile_BehindNode_TargetGovernsAboveLocalVM: a behind node (localVM=415) with the
// correct target=421 reconciles to 421 — so the committed-divergent range 416..421 stays
// decided (to be caught up), and only the phantom 422..429 is abandoned.
func TestReconcile_BehindNode_TargetGovernsAboveLocalVM(t *testing.T) {
	e, _, _ := seedPhantomEngine(t, 429,
		map[SlotKey]ids.ID{{Height: 420}: ids.GenerateTestID(), {Height: 429}: ids.GenerateTestID()}, nil)

	got, err := e.reconcilePhantomFloor(421 /*fleet-max target*/, 415 /*behind node's VM*/)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got != 421 {
		t.Fatalf("reconciled to %d, want 421 (target governs above localVM=415)", got)
	}
	e.slotMu.Lock()
	defer e.slotMu.Unlock()
	if _, ok := e.committedSlot[SlotKey{Height: 420}]; !ok {
		t.Fatal("binding 420 (committed-divergent, <=safeFloor 421) must be RETAINED for catch-up, NOT abandoned")
	}
}

// TestReconcile_PhantomBecomesSignable_CommittedStaysDecided proves the sign gate is
// un-stuck exactly at the boundary: after reconcile 429->421, a fresh height 422 is signable
// (view-change can re-finalize it), while 421 and 416 remain decided (unsignable).
func TestReconcile_PhantomBecomesSignable_CommittedStaysDecided(t *testing.T) {
	e, _, _ := seedPhantomEngine(t, 429,
		map[SlotKey]ids.ID{{Height: 429}: ids.GenerateTestID()}, nil)

	// Before reconcile: 422 is decided (<=429) → unsignable.
	if e.reserveSlotForSign(422, ids.GenerateTestID()) {
		t.Fatal("precondition: 422 must be UNsignable while the phantom floor is 429")
	}

	if _, err := e.reconcilePhantomFloor(421, 421); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if !e.reserveSlotForSign(422, ids.GenerateTestID()) {
		t.Fatal("after reconcile to 421, a FRESH 422 must be SIGNABLE (the un-stick — view-change re-finalizes 422+)")
	}
	if e.reserveSlotForSign(421, ids.GenerateTestID()) {
		t.Fatal("421 (==safeFloor) must remain DECIDED/unsignable (adopted via catch-up, never re-finalized)")
	}
	if e.reserveSlotForSign(416, ids.GenerateTestID()) {
		t.Fatal("416 (<safeFloor, committed) must remain DECIDED/unsignable")
	}
}

// TestReconcile_NoOps: a zero target, or a floor already at/below the safe height, changes
// nothing (idempotent + fail-safe-against-accidental-invocation).
func TestReconcile_NoOps(t *testing.T) {
	// (a) zero target — never derive an abandonment bound implicitly.
	e, _, _ := seedPhantomEngine(t, 429, map[SlotKey]ids.ID{{Height: 429}: ids.GenerateTestID()}, nil)
	if got, err := e.reconcilePhantomFloor(0, 421); err != nil || got != 429 {
		t.Fatalf("zero-target must be a no-op: got=%d err=%v (want 429,nil)", got, err)
	}
	// (b) floor already at/below safe height.
	e2, _, _ := seedPhantomEngine(t, 421, map[SlotKey]ids.ID{{Height: 421}: ids.GenerateTestID()}, nil)
	if got, err := e2.reconcilePhantomFloor(421, 421); err != nil || got != 421 {
		t.Fatalf("no-phantom must be a no-op: got=%d err=%v (want 421,nil)", got, err)
	}
}

// TestReconcile_Idempotent: reconciling twice with the same target is stable — the second
// call is a no-op (decidedFloor already == safeFloor).
func TestReconcile_Idempotent(t *testing.T) {
	e, _, _ := seedPhantomEngine(t, 429, map[SlotKey]ids.ID{{Height: 429}: ids.GenerateTestID()}, nil)
	if _, err := e.reconcilePhantomFloor(421, 421); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	got, err := e.reconcilePhantomFloor(421, 421)
	if err != nil || got != 421 {
		t.Fatalf("second reconcile must be a stable no-op: got=%d err=%v (want 421,nil)", got, err)
	}
}

// TestReconcile_FailClosed_StoreWriteFails proves fail-closed durability: if the store write
// fails, NOTHING in memory changes — the node stays safely at the (un-recovered) phantom floor,
// bindings intact, and the operator retries.
func TestReconcile_FailClosed_StoreWriteFails(t *testing.T) {
	p := params5Prod()
	p.ViewChange = true
	e, _ := newQuorumEngineOpts(t, p, newTestValidatorSet(5), 0, &recordingGossiper{}, WithVoteGuard(failingGuard{}))
	// Seed the phantom state directly (failingGuard.FinalizedThrough()=0, so set it here).
	phantom := ids.GenerateTestID()
	e.slotMu.Lock()
	e.decidedFloor = 429
	e.committedSlot[SlotKey{Height: 429}] = phantom
	e.slotMu.Unlock()

	got, err := e.reconcilePhantomFloor(421, 421)
	if err == nil {
		t.Fatal("reconcile MUST fail closed when the durable write fails")
	}
	if got != 429 {
		t.Fatalf("failed reconcile must return the UNCHANGED floor 429, got %d", got)
	}
	e.slotMu.Lock()
	defer e.slotMu.Unlock()
	if e.decidedFloor != 429 {
		t.Fatalf("FAIL-CLOSED VIOLATED: decidedFloor moved to %d despite the durable write failing", e.decidedFloor)
	}
	if _, ok := e.committedSlot[SlotKey{Height: 429}]; !ok {
		t.Fatal("FAIL-CLOSED VIOLATED: phantom binding dropped despite the durable write failing")
	}
}

// TestReconcile_Runtime_UsesVMHeightAsIntrinsicGuard exercises the Runtime entry point end to
// end (it derives localVM from the VM), confirming the same bound holds through the public API.
func TestReconcile_Runtime_UsesVMHeightAsIntrinsicGuard(t *testing.T) {
	e, _, _ := seedPhantomEngine(t, 429, map[SlotKey]ids.ID{{Height: 429}: ids.GenerateTestID()}, nil)
	rt := &Runtime{Transitive: e, config: NetworkConfig{VM: nil}} // nil VM → localVM=0, target governs
	got, err := rt.ReconcilePhantomFloor(context.Background(), 421)
	if err != nil {
		t.Fatalf("Runtime.ReconcilePhantomFloor: %v", err)
	}
	if got != 421 {
		t.Fatalf("Runtime reconcile floor=%d, want 421", got)
	}
}
