// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// vote_guard_test.go — Blue's hardening gates for the v1.33.3 vote-once residuals:
//
//	HIGH-1 durability  : TestBlue_VoteGuard_CrashRestart_RefusesSiblingAfterRestart,
//	                     TestBlue_VoteGuard_PersistFailure_FailsClosed,
//	                     TestBlue_VoteGuard_FileRoundTrip,
//	                     TestBlue_VoteGuard_CorruptFileFailsClosed.
//	HIGH-2 epoch keying: TestBlue_CrossEpochLiveness_NoStall.
package chain

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/luxfi/ids"
)

// failingGuard is a VoteGuardStore whose durable write always fails — proves the
// engine FAILS CLOSED (refuses the signature) and rolls back the in-memory binding.
type failingGuard struct{}

func (failingGuard) Persist(map[SlotKey]ids.ID) error {
	return errors.New("simulated durable-write failure")
}
func (failingGuard) Snapshot() map[SlotKey]ids.ID { return map[SlotKey]ids.ID{} }
func (failingGuard) Close() error                 { return nil }

// TestBlue_VoteGuard_CrashRestart_RefusesSiblingAfterRestart is the HIGH-1 teeth: a
// node that signs canonical A at a height, then CRASHES before that height finalizes,
// must — on restart from the SAME durable store — REFUSE to sign a conflicting sibling
// B at that height. With the in-memory-only guard (v1.33.2) the restart forgot the
// binding and B was freely signable → a cross-node fork with zero Byzantine intent.
func TestBlue_VoteGuard_CrashRestart_RefusesSiblingAfterRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vote-guard")
	vs := newTestValidatorSet(5)

	// --- pre-crash: engine 1 signs its OWN proposal A at height 7 via the real sign
	// path (trackProposal -> recordOwnVoteLocked -> reserveSlotForSign -> Persist+fsync).
	store1, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(store1): %v", err)
	}
	e1, chainID := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store1))
	A := newTestBlock(7, ids.Empty, "restart-A")
	_ = trackProposal(e1, chainID, A, 0)

	e1.slotMu.Lock()
	_, boundMem := e1.committedSlot[SlotKey{Height: 7}]
	e1.slotMu.Unlock()
	if !boundMem {
		t.Fatal("engine 1 must have bound height 7 to A after signing its own proposal")
	}
	_ = e1.Stop(context.Background()) // simulate crash/shutdown BEFORE height 7 finalizes

	// --- the SAME durable store, reopened, must still carry the binding. ---
	store2, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(store2): %v", err)
	}
	if got, ok := store2.Snapshot()[SlotKey{Height: 7}]; !ok || got != A.id {
		t.Fatalf("DURABILITY LOST: reopened store missing (7->A); snapshot=%v", store2.Snapshot())
	}

	// --- post-crash: engine 2 built on the reopened store REFUSES a conflicting sibling.
	e2, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store2))
	B := newTestBlock(7, ids.Empty, "restart-B")
	if e2.reserveSlotForSign(7, B.id) {
		t.Fatalf("POST-CRASH FORK: engine 2 signed a conflicting sibling B at height 7 after restart — the "+
			"durable guard failed to carry the binding across the crash (A=%s B=%s)", A.id, B.id)
	}
	// It still accepts the SAME canonical A (idempotent — a legitimate re-solicit post-restart).
	if !e2.reserveSlotForSign(7, A.id) {
		t.Fatal("engine 2 must still accept the SAME canonical A at height 7 (idempotent re-solicit)")
	}
}

// TestBlue_VoteGuard_PersistFailure_FailsClosed proves the fail-closed contract: when
// the durable write fails, reserveSlotForSign returns false (no signature) AND the
// in-memory map is rolled back (no un-persisted binding left behind).
func TestBlue_VoteGuard_PersistFailure_FailsClosed(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(failingGuard{}))

	if e.reserveSlotForSign(9, ids.GenerateTestID()) {
		t.Fatal("reserveSlotForSign MUST fail closed (return false) when the durable write fails")
	}
	e.slotMu.Lock()
	_, bound := e.committedSlot[SlotKey{Height: 9}]
	e.slotMu.Unlock()
	if bound {
		t.Fatal("a failed durable write must ROLL BACK the in-memory binding (no un-persisted slot lingers)")
	}
}

// TestBlue_VoteGuard_FileRoundTrip proves the file store persists and reloads a binding
// set identically, and leaves no temp file after the atomic replace.
func TestBlue_VoteGuard_FileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vote-guard")
	s, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard: %v", err)
	}
	if len(s.Snapshot()) != 0 {
		t.Fatal("a fresh store (no file) must reload an empty snapshot")
	}
	want := map[SlotKey]ids.ID{
		{Height: 1}:         ids.GenerateTestID(),
		{Height: 2}:         ids.GenerateTestID(),
		{Height: 1_000_000}: ids.GenerateTestID(),
		{Height: 42}:        ids.GenerateTestID(),
	}
	if err := s.Persist(want); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("the temp file must NOT linger after the atomic rename")
	}
	s2, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := s2.Snapshot()
	if len(got) != len(want) {
		t.Fatalf("reloaded %d bindings, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("binding %v: reloaded %s, want %s", k, got[k], v)
		}
	}
}

// TestBlue_VoteGuard_CorruptFileFailsClosed proves a tampered/torn snapshot is a HARD
// error at open — a signing node must not start with equivocation memory it cannot
// verify (better a loud refusal to start than a silent empty guard that permits a fork).
func TestBlue_VoteGuard_CorruptFileFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vote-guard")
	s, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard: %v", err)
	}
	if err := s.Persist(map[SlotKey]ids.ID{{Height: 5}: ids.GenerateTestID()}); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	data[len(data)/2] ^= 0xFF // flip a record byte → CRC mismatch
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err = OpenVoteGuard(path)
	if err == nil {
		t.Fatal("a corrupt guard file MUST fail OpenVoteGuard (fail-closed)")
	}
	if !errors.Is(err, errVoteGuardCorrupt) {
		t.Fatalf("expected errVoteGuardCorrupt, got: %v", err)
	}
}

// TestVoteOnce_EpochBlind_OneSignaturePerHeight is the REGRESSION for the fresh-net
// double-finalization fatal (two α-of-K certs at one height). The slot is keyed on
// HEIGHT ALONE: once this node has signed ANY canonical at height H, EVERY different
// canonical at H is refused — no matter what validator-set epoch it claims.
//
// This reproduces the production trigger and proves it closed. In the field two honest
// sibling blocks at one consensus height pinned DIFFERENT proposervm P-chain heights (a
// bare/pre-fork block reports 0, a wrapped block reports P), so ValidatorSetRoot(0)=Empty
// ≠ ValidatorSetRoot(P). Under the earlier (height,epoch) keying those landed in
// DIFFERENT slots, so one honest validator signed BOTH → two certs → the exit(1)
// equivocation crash at height 7. With the epoch removed from the slot, the second
// sibling is refused REGARDLESS of epoch, so a validator contributes its stake to AT
// MOST ONE block per height and two conflicting certs can never both form (quorum
// intersection: two ⅔ certs need ≥2α−n honest double-signers = f≥n/3).
//
// The prior TestBlue_CrossEpochLiveness_NoStall asserted the OPPOSITE (that a
// different-epoch sibling at H is a distinct, independently-signable slot) — it encoded
// the bug as the invariant. The "cross-epoch stall" it feared is not a stall: refusing a
// conflicting sibling at H is exactly the non-equivocation we require; convergence on the
// single decided block is a liveness concern resolved by the preferred-sibling vote path,
// NEVER by letting a validator sign twice at one height.
func TestVoteOnce_EpochBlind_OneSignaturePerHeight(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, _ := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})

	const H = uint64(42)
	A := ids.GenerateTestID()      // wrapped sibling (its own P-chain-height epoch R)
	Aprime := ids.GenerateTestID() // same-epoch conflicting sibling
	B := ids.GenerateTestID()      // bare sibling (P-chain height 0 → epoch Empty)

	// Bind A at height H.
	if !e.reserveSlotForSign(H, A) {
		t.Fatal("first bind at height H must be permitted")
	}
	// Same-epoch conflicting sibling A' — refused (basic non-equivocation).
	if e.reserveSlotForSign(H, Aprime) {
		t.Fatal("conflicting sibling at height H MUST be refused")
	}
	// THE FIX: a DIFFERENT-epoch sibling B at the SAME height is ALSO refused. Under the
	// old (height,epoch) keying this returned TRUE (a new slot) — the double-vote that
	// finalized two blocks at height 7 on the fresh devnet.
	if e.reserveSlotForSign(H, B) {
		t.Fatal("DOUBLE-VOTE REGRESSION: a different-validator-set-epoch sibling at an " +
			"already-signed height was admitted — the slot must be epoch-BLIND (height-only), " +
			"else a bare/pre-fork sibling (epoch Empty) and a wrapped sibling (epoch R) each " +
			"gather an α-of-K cert at one height → the exit(1) equivocation fatal")
	}
	// A stays idempotent; every other canonical at H stays refused.
	if !e.reserveSlotForSign(H, A) {
		t.Fatal("H->A must remain idempotent (safe re-solicit of the SAME block)")
	}
	if e.reserveSlotForSign(H, Aprime) || e.reserveSlotForSign(H, B) {
		t.Fatal("all non-A canonicals at H must stay refused")
	}
	// A DIFFERENT height is an independent slot (progress is never blocked across heights).
	if !e.reserveSlotForSign(H+1, B) {
		t.Fatal("a different height is an independent slot and must be permitted")
	}
}
