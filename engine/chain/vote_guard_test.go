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
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/luxfi/ids"
)

// TestVoteGuard_LegacyMultiEpochFold proves the durable guard's UPGRADE path (N3): a
// snapshot written by the OLD (height,epoch)-keyed build can hold MULTIPLE records at one
// height (different epochs). The height-only decoder must FOLD them into the single height
// slot and keep the LOWEST canonical — the fail-safe direction (refuse every sibling above
// the min at that recovered contested height). This exercises the exact collision branch
// in decodeVoteGuard that no test covered.
func TestVoteGuard_LegacyMultiEpochFold(t *testing.T) {
	// Two records at the SAME height 7 with DIFFERENT epochs and DIFFERENT canonicals —
	// exactly what the buggy epoch-keyed build could persist.
	epochA := ids.GenerateTestID()
	epochB := ids.GenerateTestID()
	hi := ids.ID{0xff} // a HIGH canonical
	lo := ids.ID{0x01} // a LOW canonical (< hi) — the fold must keep THIS one
	// Also a distinct height 9 (single record) to prove non-collision heights survive.
	epochC := ids.GenerateTestID()
	other := ids.GenerateTestID()

	rec := func(h uint64, epoch, canon ids.ID) []byte {
		var b []byte
		var u64 [8]byte
		binary.BigEndian.PutUint64(u64[:], h)
		b = append(b, u64[:]...)
		b = append(b, epoch[:]...)
		b = append(b, canon[:]...)
		return b
	}
	body := []byte(voteGuardMagic)
	body = append(body, voteGuardVersionV1) // a LEGACY v1 snapshot (no finalizedThrough field)
	var cnt [4]byte
	binary.BigEndian.PutUint32(cnt[:], 3)
	body = append(body, cnt[:]...)
	body = append(body, rec(7, epochA, hi)...) // higher canonical FIRST (order must not matter)
	body = append(body, rec(7, epochB, lo)...) // lower canonical — must win the fold
	body = append(body, rec(9, epochC, other)...)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.Checksum(body, voteGuardCRC))
	raw := append(body, crc[:]...)

	path := filepath.Join(t.TempDir(), "vote-guard")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard on a legacy multi-epoch snapshot must succeed (fold, not fail): %v", err)
	}
	snap := s.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 folded height slots (7 and 9), got %d: %v", len(snap), snap)
	}
	if got := snap[SlotKey{Height: 7}]; got != lo {
		t.Fatalf("fold collision at height 7 must keep the LOWEST canonical %s (fail-safe), got %s", lo, got)
	}
	if got := snap[SlotKey{Height: 9}]; got != other {
		t.Fatalf("non-colliding height 9 must survive intact, got %s want %s", got, other)
	}
}

// failingGuard is a VoteGuardStore whose durable write always fails — proves the
// engine FAILS CLOSED (refuses the signature) and rolls back the in-memory binding.
type failingGuard struct{}

func (failingGuard) Persist(map[SlotKey]ids.ID, uint64) error {
	return errors.New("simulated durable-write failure")
}
func (failingGuard) Snapshot() map[SlotKey]ids.ID { return map[SlotKey]ids.ID{} }
func (failingGuard) FinalizedThrough() uint64     { return 0 }
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

// TestRed_CrossRestart_DecidedHeightUnsignable is the RED CRITICAL-1 regression: the
// decided-height gate must survive a RESTART. It reproduces the cross-restart
// prune-then-resign fork and proves the DURABLE floor closes it.
//
// Sequence (mirrors acceptWithCertCore's ApplyCert-then-prune): a node signs winner A at
// height H, finalizes H, then finalizes H+1. The strictly-below prune deletes slot{H} from
// memory AND persists that deletion to the fsync'd vote-guard file — so slot{H} is GONE on
// disk. On the pre-v1.35.4 build, a restart then had: no slot{H} (pruned) AND a certified
// ledger.Height() of (0,false) (a boot HINT is non-authoritative — incident-1082814
// PART-A) ⇒ the decided-height gate was DEAD and a re-gossiped sibling B at the decided
// height H collected this node's SECOND signature. On a correlated rolling restart of a
// storming fresh net that assembled a second α-of-K cert → os.Exit(1) fleet-wide.
//
// The fix persists the decided-through FLOOR in the vote-guard file (fsync'd atomically
// with the pruned map) and seeds decidedFloor from it on boot, so a below-tip decided
// height stays unsignable across the restart with NO reliance on the certified frontier.
// This test fails on v1.35.3 (B is admitted) and passes on v1.35.4.
func TestRed_CrossRestart_DecidedHeightUnsignable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vote-guard")
	vs := newTestValidatorSet(5)

	const H = uint64(42)
	A := ids.GenerateTestID()  // winner at height H
	A2 := ids.GenerateTestID() // winner at height H+1, child of A
	B := ids.GenerateTestID()  // a losing sibling at height H (different outer parent — escapes losingSubtrees)

	// --- pre-crash: engine 1 signs A@H, then finalizes H and H+1 (ApplyCert then prune,
	// exactly as acceptWithCertCore does). The prune of H+1 deletes slot{H} from memory AND
	// the durable file, while advancing the persisted floor to H+1.
	store1, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(store1): %v", err)
	}
	e1, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store1))
	if !e1.reserveSlotForSign(H, A) {
		t.Fatal("engine 1 must bind height H to A")
	}
	if _, err := e1.consensus.FinalizeBranch(A, H, ids.Empty); err != nil {
		t.Fatalf("FinalizeBranch(H): %v", err)
	}
	e1.pruneCommittedSlotsBelow(H) // retains slot{H}, floor→H
	if _, err := e1.consensus.FinalizeBranch(A2, H+1, A); err != nil {
		t.Fatalf("FinalizeBranch(H+1): %v", err)
	}
	e1.pruneCommittedSlotsBelow(H + 1) // drops slot{H} from memory + file, floor→H+1
	_ = e1.Stop(context.Background())   // simulate crash/shutdown

	// --- the reopened durable store must have FORGOTTEN slot{H} (pruned) but REMEMBERED the
	// decided-through floor H+1 (fsync'd atomically with the prune).
	store2, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(store2): %v", err)
	}
	if _, ok := store2.Snapshot()[SlotKey{Height: H}]; ok {
		t.Fatal("precondition: the strictly-below prune must have removed slot{H} from the durable file")
	}
	if got := store2.FinalizedThrough(); got != H+1 {
		t.Fatalf("durable floor lost across restart: FinalizedThrough=%d want %d "+
			"(this is the field that keeps a decided height unsignable when its slot is gone)", got, H+1)
	}

	// --- post-crash engine 2, built ONLY on the reopened store (NO SyncState hint), must
	// still REFUSE a sibling at the decided-below-tip height H. This isolates the durable
	// vote-guard FLOOR as the sole protection: no slot{H}, no certified frontier, no hint.
	e2, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store2))
	if fh, ok := e2.consensus.GetFinalizedHeight(); ok {
		t.Fatalf("precondition: a freshly-booted engine's certified frontier must be (unset), got (%d,%v) — "+
			"the whole point is that the gate cannot rely on it after a restart", fh, ok)
	}
	if e2.reserveSlotForSign(H, B) {
		t.Fatalf("CROSS-RESTART PRUNE-THEN-RESIGN FORK: engine 2 signed sibling B at the decided height %d "+
			"after a restart. slot{%d} was pruned+persisted-away and the certified frontier is a boot hint, so "+
			"only the durable vote-guard floor (=%d) can refuse it — and it did not. (A=%s B=%s)",
			H, H, store2.FinalizedThrough(), A, B)
	}
	// The just-below tip and the tip itself are also refused; a height ABOVE the floor is signable.
	if e2.reserveSlotForSign(H+1, ids.GenerateTestID()) {
		t.Fatal("the decided tip height H+1 must also be unsignable across the restart (floor covers it)")
	}
	if !e2.reserveSlotForSign(H+2, ids.GenerateTestID()) {
		t.Fatal("a height ABOVE the decided floor must remain signable — the durable floor must not stall progress")
	}

	// --- and the complementary path: a boot that ALSO seeds the SyncState hint (vm.LastAccepted)
	// refuses just the same (belt), even if the vote-guard file were absent/fresh.
	store3, err := OpenVoteGuard(filepath.Join(dir, "vote-guard-fresh"))
	if err != nil {
		t.Fatalf("OpenVoteGuard(store3): %v", err)
	}
	e3, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store3))
	if err := e3.consensus.SyncState(A2, H+1); err != nil { // the boot hint from vm.LastAccepted
		t.Fatalf("SyncState hint: %v", err)
	}
	if e3.reserveSlotForSign(H, B) {
		t.Fatal("with a fresh vote-guard, the vm.LastAccepted boot HINT floor must still refuse a decided height H")
	}
	if !e3.reserveSlotForSign(H+2, ids.GenerateTestID()) {
		t.Fatal("hint floor must not refuse a height above the hint (no false stall)")
	}
}

// TestRed_V1ToV2Upgrade_BootSeedFromVM is the RED fix-priority #1 regression for the
// MAINNET in-place v1→v2 vote-guard upgrade window. A live mainnet node upgrading from the
// old consensus carries a LEGACY v1 guard file (no finalizedThrough → floor 0) whose
// below-tip decided-height slots were already pruned by the old build, and its certified
// ledger is a (0,false) hint until the first post-upgrade finalize (PART-A). Without a boot
// seed the sign gate's floor would be 0 in that window, so a re-gossiped sibling at a
// decided-below-tip height could be signed — the exact fork, now on mainnet.
//
// v1.35.5 seeds decidedFloor DIRECTLY from vm.LastAccepted (a durable, sound lower bound on
// the decided height) at Start, BEFORE the signing goroutines launch — so the floor is real
// from the first instant of boot, with no reliance on the transient SyncState hint or a
// post-upgrade finalize. This test builds exactly that state and proves a decided-below-tip
// height is refused immediately on boot. It FAILS without the boot seed (B is signed) and
// PASSES with it.
func TestRed_V1ToV2Upgrade_BootSeedFromVM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vote-guard")

	// Write a LEGACY v1 vote-guard file: version 1, ZERO records (the below-tip slots were
	// pruned pre-upgrade), NO finalizedThrough field → decodes with floor 0.
	body := []byte(voteGuardMagic)
	body = append(body, voteGuardVersionV1)
	var cnt [4]byte
	binary.BigEndian.PutUint32(cnt[:], 0)
	body = append(body, cnt[:]...)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.Checksum(body, voteGuardCRC))
	if err := os.WriteFile(path, append(body, crc[:]...), 0o600); err != nil {
		t.Fatalf("WriteFile(legacy v1): %v", err)
	}
	store, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(legacy v1): %v", err)
	}
	if store.FinalizedThrough() != 0 {
		t.Fatalf("a legacy v1 file must decode with floor 0, got %d", store.FinalizedThrough())
	}

	// A VM whose LAST-ACCEPTED head is the decided tip at height T. vm.LastAccepted is
	// durable and available from the first instant of boot — the seed source.
	const T = uint64(100)
	const belowTip = uint64(60) // a decided-below-tip height whose slot was pruned pre-upgrade
	vm := newMockImportVM()
	tipID := ids.GenerateTestID()
	vm.lastAcceptedID = tipID
	vm.blocks[tipID] = &mockBlock{id: tipID, height: T}

	vs := newTestValidatorSet(5)
	// Construct + Start: Start runs the boot seed from vm.LastAccepted BEFORE any signing.
	// NO SyncState is called — this isolates the DIRECT boot seed (not the transient hint).
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store), WithVM(vm))

	// Immediately on boot — legacy floor 0, no hint, no post-upgrade finalize — the decided
	// frontier must already be T, so a sibling at the decided-below-tip height is REFUSED.
	if fh, ok := e.consensus.GetFinalizedHeight(); ok {
		t.Fatalf("precondition: certified frontier must be unset on a fresh boot, got (%d,%v)", fh, ok)
	}
	B := ids.GenerateTestID()
	if e.reserveSlotForSign(belowTip, B) {
		t.Fatalf("V1→V2 UPGRADE-WINDOW FORK: engine signed sibling B at decided-below-tip height %d on boot from "+
			"a legacy v1 guard (floor 0). decidedFloor must be seeded from vm.LastAccepted (=%d) at Start, before "+
			"any post-upgrade finalize.", belowTip, T)
	}
	// The tip itself is refused; a height above the VM head stays signable (no false stall).
	if e.reserveSlotForSign(T, ids.GenerateTestID()) {
		t.Fatalf("the decided tip height %d must be unsignable on boot (the boot seed covers it)", T)
	}
	if !e.reserveSlotForSign(T+1, ids.GenerateTestID()) {
		t.Fatalf("a height above the VM head (%d) must remain signable — the boot seed must not stall progress", T)
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
	const wantFloor = uint64(1_000_005)
	if err := s.Persist(want, wantFloor); err != nil {
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
	if got := s2.FinalizedThrough(); got != wantFloor {
		t.Fatalf("reloaded finalizedThrough %d, want %d (the durable decided-through floor must round-trip)", got, wantFloor)
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
	if err := s.Persist(map[SlotKey]ids.ID{{Height: 5}: ids.GenerateTestID()}, 4); err != nil {
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

// TestViewChange_RecoveredLock_NoCrossRestartDoublePrecommit is the ROLLING-RESTART safety
// gate for the round-scoped view-change (RED item 2). Under ViewChange the in-session guard
// is RELAXED (the round+lock rule governs re-precommit) — but a binding RECOVERED from the
// durable vote-guard has no persisted lock ROUND, so the unlock rule cannot be evaluated. The
// engine must therefore treat a recovered binding as a HARD lock and REFUSE a conflicting
// value at that height even under ViewChange, so a rolling upgrade / correlated crash cannot
// make a node precommit a SECOND, conflicting value at a pre-crash height (double-precommit).
// A NEW (in-session) height still re-precommits freely.
func TestViewChange_RecoveredLock_NoCrossRestartDoublePrecommit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vote-guard")
	vs := newTestValidatorSet(5)
	p := params5Prod()
	p.ViewChange = true

	const H = uint64(50)
	X := ids.GenerateTestID()
	Y := ids.GenerateTestID()

	// pre-crash: bind X@H (unfinalized), fsync, "crash".
	store1, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(store1): %v", err)
	}
	e1, _ := newQuorumEngineOpts(t, p, vs, 0, &recordingGossiper{}, WithVoteGuard(store1))
	if !e1.reserveSlotForSign(H, X) {
		t.Fatal("engine 1 must bind X@H")
	}
	_ = e1.Stop(context.Background())

	// post-crash: recover under ViewChange. The recovered height is a HARD lock.
	store2, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(store2): %v", err)
	}
	if _, ok := store2.Snapshot()[SlotKey{Height: H}]; !ok {
		t.Fatal("precondition: X@H must be recovered from the durable guard")
	}
	e2, _ := newQuorumEngineOpts(t, p, vs, 0, &recordingGossiper{}, WithVoteGuard(store2))

	if !e2.reserveSlotForSign(H, X) {
		t.Fatal("the recovered value X@H must stay idempotently signable (safe re-solicit)")
	}
	if e2.reserveSlotForSign(H, Y) {
		t.Fatal("ROLLING-RESTART DOUBLE-PRECOMMIT: a RECOVERED height accepted a CONFLICTING value under " +
			"ViewChange — the lock round is not on disk, so a recovered binding MUST be a hard lock")
	}

	// A NEW in-session height is governed by the round+lock rule: it binds AND re-precommits
	// a conflicting value freely (that re-convergence is exactly what restores liveness).
	const H2 = uint64(51)
	A := ids.GenerateTestID()
	B := ids.GenerateTestID()
	if !e2.reserveSlotForSign(H2, A) {
		t.Fatal("in-session A@H2 must bind")
	}
	if !e2.reserveSlotForSign(H2, B) {
		t.Fatal("in-session re-precommit B@H2 must be PERMITTED under ViewChange (round+lock governs re-convergence)")
	}
}
