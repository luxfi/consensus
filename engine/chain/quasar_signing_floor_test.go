// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"path/filepath"
	"testing"

	"github.com/luxfi/ids"
)

// The durable sign-refusal floor is the ONE piece of state a validator can never take back:
// it is fsync'd, monotonic, and no restart, resync, or peer can lower it. These tests pin the
// only sound source for it.
//
// Quorum intersection is 2α−n. At n=5, f=1:
//
//	Nova   ⌊n/2⌋+1 = 3  → 2α−n = 1  NOT > f  ⇒ two conflicting Nova histories may exist
//	Quasar strict ⅔ = 4  → 2α−n = 3      > f  ⇒ intersection guaranteed, irreversible
//
// So only a Quasar height is agreed by every honest peer. Closing a Nova height forever strands
// the validator at a height the fleet never agreed on — the mainnet-1098192 and testnet-11367
// halts, where a valid block was built, verified, and gossiped but was unsignable, so no
// certificate could ever assemble and the chain stopped with zero errors logged.
//
// Nova is NOT gated away: it keeps the ledger, VM.Accept, preference, catch-up, and rejoin.
// An earlier attempt to refuse Nova certs at intake broke three heal/rejoin tests, because Nova
// certs ARE the recovery transport. The cut is narrower and exact: Nova may move everything
// except the permanent floor.

// TestQuasarFloor_NovaAcceptDoesNotCloseHeights is the core regression. A Nova majority accept
// must advance the ledger and leave BOTH the export frontier and the durable guard floor
// untouched. The deleted line in acceptWithCertCore was
//
//	if highestAccepted > 0 { t.pruneCommittedSlotsBelow(highestAccepted) }
//
// which fsync'd a permanent refusal at a bare-majority height.
func TestQuasarFloor_NovaAcceptDoesNotCloseHeights(t *testing.T) {
	vs := newTestValidatorSet(5)
	store, err := OpenVoteGuard(filepath.Join(t.TempDir(), "vote-guard"))
	if err != nil {
		t.Fatalf("OpenVoteGuard: %v", err)
	}
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store))

	const H = uint64(500)
	A := ids.GenerateTestID()

	// A Nova accept: fold the block into the committed ledger, exactly as ApplyCert does.
	if _, err := e.consensus.FinalizeBranch(A, H, ids.Empty); err != nil {
		t.Fatalf("FinalizeBranch(nova accept): %v", err)
	}

	// The Nova ledger advanced — local execution authority, which is all a majority earns.
	if h, ok := e.consensus.GetFinalizedHeight(); !ok || h != H {
		t.Fatalf("nova ledger must advance to %d, got (%d,%v)", H, h, ok)
	}
	// The export frontier did NOT.
	if qh, ok := e.consensus.QuasarHeight(); ok && qh >= H {
		t.Fatalf("a NOVA accept must not advance the export frontier: got (%d,%v)", qh, ok)
	}
	if got := e.consensus.GetQuasarSigningFloor(); got != 0 {
		t.Fatalf("a NOVA accept must leave the quasar signing floor at 0, got %d", got)
	}
	// And neither did the durable guard floor — the thing that can never be taken back.
	if got := store.FinalizedThrough(); got != 0 {
		t.Fatalf("PERMANENT WELD: a NOVA accept fsync'd a sign-refusal floor of %d. Nova is reorgable "+
			"(2α−n = 1, NOT > f=1), so this height may never be certified and the refusal can never be "+
			"cleared — mainnet-1098192 / testnet-11367.", got)
	}
	// Therefore the height stays signable: a restarted node can still sign the honest rebuild.
	if !e.reserveSlotForSign(H, ids.GenerateTestID()) {
		t.Fatalf("height %d is Nova-accepted but not ⅔-certified — it MUST stay signable so a restart "+
			"can re-sign the rebuild the fleet agrees on", H)
	}
}

// TestQuasarFloor_PromotionAdvancesFloorAtomically proves the other half: a ⅔-by-stake cert DOES
// close the height, and the guard floor + binding compaction land in ONE fsync'd write.
func TestQuasarFloor_PromotionAdvancesFloorAtomically(t *testing.T) {
	vs := newTestValidatorSet(5)
	path := filepath.Join(t.TempDir(), "vote-guard")
	store, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard: %v", err)
	}
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store))

	const Q = uint64(300)
	// Bind two heights below the new floor and one above it, so the compaction is observable.
	for _, h := range []uint64{Q - 2, Q - 1, Q + 1} {
		if !e.reserveSlotForSign(h, ids.GenerateTestID()) {
			t.Fatalf("precondition: must be able to bind height %d", h)
		}
	}

	if err := e.compactVoteGuardThroughQuasar(Q); err != nil {
		t.Fatalf("compactVoteGuardThroughQuasar(%d): %v", Q, err)
	}

	if got := store.FinalizedThrough(); got != Q {
		t.Fatalf("a ⅔-stake certified height MUST advance the durable floor: FinalizedThrough=%d want %d", got, Q)
	}
	// In memory: strictly-below bindings dropped, the above-floor one kept.
	e.slotMu.Lock()
	_, has298 := e.committedSlot[SlotKey{Height: Q - 2}]
	_, has299 := e.committedSlot[SlotKey{Height: Q - 1}]
	_, has301 := e.committedSlot[SlotKey{Height: Q + 1}]
	e.slotMu.Unlock()
	if has298 || has299 {
		t.Fatalf("bindings strictly below the certified floor %d must be compacted away (298=%v 299=%v)",
			Q, has298, has299)
	}
	if !has301 {
		t.Fatalf("binding at %d is ABOVE the certified floor and must be RETAINED — that window is where "+
			"a restarted node still needs its per-height vote memory", Q+1)
	}
	// And durably. VoteGuardStore.Snapshot() is the OPEN-TIME view (never updated by Persist),
	// so the only honest probe of what landed on disk is a fresh open.
	re, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if re.FinalizedThrough() != Q {
		t.Fatalf("floor did not survive reopen: %d want %d", re.FinalizedThrough(), Q)
	}
	snap := re.Snapshot()
	for _, h := range []uint64{Q - 2, Q - 1} {
		if _, ok := snap[SlotKey{Height: h}]; ok {
			t.Fatalf("binding at %d is strictly below the certified floor %d and must not survive on disk", h, Q)
		}
	}
	if _, ok := snap[SlotKey{Height: Q + 1}]; !ok {
		t.Fatalf("above-floor binding did not survive reopen — the compaction write was not atomic with the floor")
	}
}

// TestQuasarFloor_GuardWriteFailureWithholdsExport proves the fail-closed ordering. If the guard
// write fails, the floor must NOT advance in memory (memory can never claim what disk does not
// carry) and the caller must NOT publish the export frontier — otherwise a crash leaves the VM's
// durable LastQuasarHeight naming a height whose compaction never landed, and the two durable
// records disagree about what is closed.
func TestQuasarFloor_GuardWriteFailureWithholdsExport(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(failingGuard{}))

	const Q = uint64(77)
	err := e.compactVoteGuardThroughQuasar(Q)
	if err == nil {
		t.Fatal("compactVoteGuardThroughQuasar MUST return an error when the durable write fails — the " +
			"caller uses it to withhold the export-frontier notification (fail-closed)")
	}
	e.slotMu.Lock()
	floor := e.decidedFloor
	e.slotMu.Unlock()
	if floor != 0 {
		t.Fatalf("in-memory floor advanced to %d despite a FAILED durable write — memory must never claim "+
			"a floor disk does not carry; it must roll back", floor)
	}
}

// TestQuasarFloor_ImportIsRecoveryHintNotDecision covers the RLP / state-sync / snapshot-clone
// path. Handing a node block DATA is not proof the network certified it — and a recovery import
// is precisely when a fleet is below quorum and those heights may still need re-deciding. The
// import must advance the Nova build anchor and leave the signing floor alone.
func TestQuasarFloor_ImportIsRecoveryHintNotDecision(t *testing.T) {
	vs := newTestValidatorSet(5)
	store, err := OpenVoteGuard(filepath.Join(t.TempDir(), "vote-guard"))
	if err != nil {
		t.Fatalf("OpenVoteGuard: %v", err)
	}
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store))

	const imported = uint64(1098726) // the real mainnet RLP recovery height
	if err := e.consensus.SyncState(ids.GenerateTestID(), imported); err != nil {
		t.Fatalf("SyncState(import): %v", err)
	}

	// The import moved the Nova/execution view — that is what an import authorizes.
	if got := e.consensus.GetNovaAcceptedFloor(); got != imported {
		t.Fatalf("import must advance the Nova accepted floor to %d, got %d", imported, got)
	}
	// It did NOT move the signing floor, in memory or on disk.
	if got := e.consensus.GetQuasarSigningFloor(); got != 0 {
		t.Fatalf("import must not advance the quasar signing floor, got %d", got)
	}
	if got := store.FinalizedThrough(); got != 0 {
		t.Fatalf("IMPORT WELD: an RLP import fsync'd a sign-refusal floor of %d. The operator supplied "+
			"block data, not a certificate — and an import happens exactly when the fleet is below quorum.", got)
	}
	if !e.reserveSlotForSign(imported, ids.GenerateTestID()) {
		t.Fatalf("an imported height (%d) carries no local certificate and MUST stay signable", imported)
	}
}

// TestQuasarFloor_RestartKeepsFloorAtQuasarNotNova is the rolling-upgrade acceptance test in
// miniature, and the property the owner actually asked for. A node runs ahead on Nova, restarts,
// and must come back with its floor at the CERTIFIED height — so every height the fleet has not
// agreed on is still available for it to re-sign.
func TestQuasarFloor_RestartKeepsFloorAtQuasarNotNova(t *testing.T) {
	vs := newTestValidatorSet(5)
	path := filepath.Join(t.TempDir(), "vote-guard")

	const Q = uint64(11363) // last ⅔-certified height (the real testnet value)
	const novaHead = uint64(11367)

	store1, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(store1): %v", err)
	}
	e1, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store1))

	// Certified through Q.
	if err := e1.compactVoteGuardThroughQuasar(Q); err != nil {
		t.Fatalf("compact(Q): %v", err)
	}
	// Then this node runs ahead ALONE on bare Nova majorities through novaHead, binding a vote
	// at each height — exactly what luxd-1 did while no cert formed.
	for h := Q + 1; h <= novaHead; h++ {
		if !e1.reserveSlotForSign(h, ids.GenerateTestID()) {
			t.Fatalf("precondition: must bind own vote at %d", h)
		}
		if _, err := e1.consensus.FinalizeBranch(ids.GenerateTestID(), h, ids.Empty); err != nil {
			// A gap-free ancestry is not the point of this test; the ledger fold may refuse.
			_ = err
		}
	}

	// RESTART.
	store2, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(store2): %v", err)
	}
	if got := store2.FinalizedThrough(); got != Q {
		t.Fatalf("ROLLING-UPGRADE BLOCKER: the durable floor came back at %d, but only %d was ⅔-certified. "+
			"A restarted node must never carry a refusal above the height its peers agree on.", got, Q)
	}
	e2, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store2))

	// The certified prefix stays closed forever.
	if e2.reserveSlotForSign(Q, ids.GenerateTestID()) {
		t.Fatalf("the certified height %d must remain unsignable across the restart", Q)
	}
	// The uncertified window is NOT closed by the floor — but each height there is still guarded
	// INDIVIDUALLY by its retained binding. That is the whole decomplection: one signature per
	// height, enforced by memory of the vote, not by a blunt permanent floor.
	for h := Q + 1; h <= novaHead; h++ {
		bound, ok := store2.Snapshot()[SlotKey{Height: h}]
		if !ok {
			t.Fatalf("binding at uncertified height %d was lost across the restart — without it the node "+
				"has no memory of its own vote and could equivocate", h)
		}
		if e2.reserveSlotForSign(h, ids.GenerateTestID()) {
			t.Fatalf("a CONFLICTING canonical at uncertified height %d must still be refused by the "+
				"retained binding (bound=%s)", h, bound)
		}
		if !e2.reserveSlotForSign(h, bound) {
			t.Fatalf("the SAME canonical this node already signed at %d must be re-signable (idempotent) — "+
				"this is how a restarted node re-issues its outstanding vote and lets the cert finally form", h)
		}
	}
}
