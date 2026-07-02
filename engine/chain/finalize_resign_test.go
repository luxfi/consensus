// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// finalize_resign_test.go — the FINALIZE-THEN-RESIGN gate.
//
// This is the regression test for the fresh-net double-finalization fatal that survived
// the epoch-blind height-only slot fix. The remaining defect was in the guard's LIFECYCLE,
// not its key:
//
//   1. A node signs the winner A at height H (committedSlot{H}=A).
//   2. A's α-of-K cert forms; the node finalizes H. The sole finalizer
//      (acceptWithCertCore) advanced the certified frontier to H, then PRUNED the
//      equivocation guard for every height <= H — INCLUDING H itself.
//   3. A losing sibling B at height H with a DIFFERENT outer parent (a bare/pre-fork
//      envelope where the winner was proposervm-wrapped, or vice-versa) is NOT in
//      losingSubtrees(A) — losingSubtrees only rejects the OTHER CHILDREN OF A's parent —
//      so B stays tracked and UNDECIDED after A finalizes.
//   4. The convergence pass then re-offers height H (its slot was just deleted), and
//      reserveSlotForSign(H, B) found an EMPTY slot and admitted B — this node's SECOND
//      signature at height H. Enough nodes doing this assembled a second α-of-K cert at H
//      → fatal EQUIVOCATION → os.Exit(1) fleet-wide (the live devnet artifact: the
//      conflicting cert's canonical == its envelope, i.e. exactly the bare sibling).
//
// The fix is two-layered and this test pins both:
//   - PRUNE STRICTLY BELOW the finalized height (retain the tip's slot) — the in-memory belt.
//   - a DECIDED-HEIGHT GATE in reserveSlotForSign (refuse height <= finalizedHeight) — the
//     durable, monotonic backstop, mirroring avalanchego's lastAcceptedHeight frontier that
//     makes a decided height's siblings permanently unsignable.
//
// On the pre-fix code (inclusive prune, no gate) step 2's prune deletes slot{H} and nothing
// else refuses B, so reserveSlotForSign(H, B) returns TRUE — this test FAILS. On the fixed
// code it returns FALSE at every point below.
package chain

import (
	"testing"

	"github.com/luxfi/ids"
)

// TestFinalizeThenResign_DecidedHeightIsUnsignable drives the real functions the finalizer
// runs — consensus.FinalizeBranch (advances the certified frontier) and
// pruneCommittedSlotsBelow (the guard prune) — then proves a sibling at the just-decided
// height can never be signed, BOTH while the tip's slot is retained AND after it is pruned
// by a higher finalize (isolating the durable decided-height gate from the in-memory belt).
func TestFinalizeThenResign_DecidedHeightIsUnsignable(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, _ := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})

	const H = uint64(42)
	A := ids.GenerateTestID()  // the winner at height H (e.g. proposervm-wrapped)
	B := ids.GenerateTestID()  // a losing sibling at H with a DIFFERENT outer parent (bare envelope)
	A2 := ids.GenerateTestID() // the winner at height H+1, child of A

	// 1) This node casts its ONE signature at height H, on the winner A.
	if !e.reserveSlotForSign(H, A) {
		t.Fatal("first bind at height H must be permitted (no height decided yet)")
	}

	// 2) Height H finalizes. Reproduce EXACTLY what acceptWithCertCore does, in order:
	//    advance the certified frontier, then prune the guard.
	if _, err := e.consensus.FinalizeBranch(A, H, ids.Empty); err != nil {
		t.Fatalf("FinalizeBranch(H) seed finalize: %v", err)
	}
	if fh, ok := e.consensus.GetFinalizedHeight(); !ok || fh != H {
		t.Fatalf("finalized height must be %d after finalizing A@H, got (%d,%v)", H, fh, ok)
	}
	e.pruneCommittedSlotsBelow(H)

	// BELT: the just-finalized tip's slot is RETAINED (strictly-below prune), still bound to A.
	e.slotMu.Lock()
	tipBound, tipHeld := e.committedSlot[SlotKey{Height: H}]
	e.slotMu.Unlock()
	if !tipHeld || tipBound != A {
		t.Fatalf("prune must RETAIN the finalized tip's slot (strictly-below): held=%v bound=%s (want A=%s)",
			tipHeld, tipBound, A)
	}

	// A sibling B at the decided height H is refused. On the pre-fix inclusive prune this
	// slot was deleted and — with no decided-height gate — B would be admitted here (the
	// prune-then-resign fork). It must be FALSE.
	if e.reserveSlotForSign(H, B) {
		t.Fatal("PRUNE-THEN-RESIGN FORK: a sibling at the just-finalized height H was admitted for a " +
			"SECOND signature. The inclusive prune deleted the guard slot and no decided-height gate " +
			"refused it — two α-of-K certs at one height → the exit(1) equivocation fatal.")
	}

	// 3) Height H+1 finalizes. Its prune (strictly below H+1) now DROPS slot{H} — so from
	//    here the ONLY thing refusing a sibling at H is the durable decided-height gate.
	if _, err := e.consensus.FinalizeBranch(A2, H+1, A); err != nil {
		t.Fatalf("FinalizeBranch(H+1) extend finalize: %v", err)
	}
	e.pruneCommittedSlotsBelow(H + 1)

	e.slotMu.Lock()
	_, stillHeld := e.committedSlot[SlotKey{Height: H}]
	e.slotMu.Unlock()
	if stillHeld {
		t.Fatal("after finalizing H+1, slot{H} must be pruned (strictly below H+1) — it is now the gate's job")
	}

	// GATE ALONE (slot{H} gone): the decided height H is STILL unsignable — for the sibling B,
	// and even for the original winner A. A decided height's blocks are permanently unsignable.
	if e.reserveSlotForSign(H, B) {
		t.Fatal("DECIDED-HEIGHT GATE FAILED: sibling B admitted at decided height H after its slot was pruned")
	}
	if e.reserveSlotForSign(H, A) {
		t.Fatal("DECIDED-HEIGHT GATE FAILED: even the finalized canonical A must not be re-signed at decided height H")
	}

	// LIVENESS: a height ABOVE the frontier is an independent, signable slot — the gate
	// refuses only decided heights, never blocks progress. (H+1 is decided; H+2 is open.)
	if !e.reserveSlotForSign(H+2, ids.GenerateTestID()) {
		t.Fatal("a height above the finalized frontier must remain signable — the gate must not stall progress")
	}
}
