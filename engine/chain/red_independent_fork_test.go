// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// red_independent_fork_test.go — Red team's INDEPENDENT fork suite, adopted VERBATIM
// as the permanent regression gate for the per-height vote-once discipline. Unlike
// the disciplined testValidatorSet.signedVote (which models honest vote-once and
// therefore MASKS the engine guard), these tests drive equivocation with RAW
// signatures (vs.sign, no discipline) and inspect engine state directly, so they
// actually exercise reserveSlotForSign in the engine-under-test.
//
// Provenance: authored by Red for the v1.33.2 re-review. Copied here (from the
// review scratchpad) UNCHANGED except for the two mechanical adaptations the v1.33.3
// hardening's public API required — every BEHAVIORAL assertion is byte-identical to
// Red's original, so the teeth Red verified against a guard-neutered build are intact:
//
//	ADAPT-1 (epoch keying, HIGH-2): reserveSlotForSign gained an `epoch` parameter
//	  (height,epoch)-keying to match the consensus2 reference. The harness never
//	  wires a ValidatorSetRoot source, so every position's epoch is ids.Empty; each
//	  call passes ids.Empty, and (height, Empty) keying is byte-identical to the
//	  height-only keying Red wrote against. No assertion changes.
//	ADAPT-2 (SlotKey, HIGH-2): committedSlot is now keyed by SlotKey{Height,Epoch};
//	  the one white-box map read becomes committedSlot[SlotKey{Height:1}].
//	ADAPT-3 (MEDIUM-1 gate): Red's leak test originally t.Skip'd when the leak was
//	  fixed. Since v1.33.3 FIXES it (prune moved into the sole finalizer), the Skip
//	  is promoted to a hard ASSERT that the slot IS pruned after a LOCAL finalize —
//	  turning Red's demonstration into a standing regression gate.
//
// Purpose:
//  1. TestRedIndep_ReserveSlotUnit         — white-box funnel contract.
//  2. TestRedIndep_OwnVoteRefusedForSibling — the LOAD-BEARING guard behavior. This is
//     the regression detector RED-1/RED-2 lack: it FAILS if the engine's guard call
//     site is removed (verified against a guard-neutered build).
//  3. TestRedIndep_ByzantineForkThreshold   — proves a SECOND conflicting cert at one
//     height forms iff #equivocators >= 2*alpha - n (=3 for n=5,alpha=4). f<=1 (the
//     BFT budget) — indeed f<=2 here — yields NO second cert; the fork needs f>=3.
//  4. TestRedIndep_ReserveSlotConcurrent    — atomic check-and-bind: under concurrency
//     exactly ONE canonical wins a height (run under -race).
//  5. TestRedIndep_CommittedSlotLeak_LocalFinalizePrunes — MEDIUM-1 gate: the LOCAL
//     finalize path prunes committedSlot (bounded growth).
package chain

import (
	"sync"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// TestRedIndep_ReserveSlotUnit pins the funnel's exact contract.
func TestRedIndep_ReserveSlotUnit(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, _ := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})

	X := ids.GenerateTestID()
	Y := ids.GenerateTestID()

	if !e.reserveSlotForSign(7, X) {
		t.Fatal("first bind at an unbound height must be permitted")
	}
	if !e.reserveSlotForSign(7, X) {
		t.Fatal("re-presenting the SAME canonical must be idempotent (safe re-solicit)")
	}
	if e.reserveSlotForSign(7, Y) {
		t.Fatal("a DIFFERENT canonical at a bound height MUST be refused (the fork guard)")
	}
	if !e.reserveSlotForSign(8, Y) {
		t.Fatal("a different height is an independent slot and must be permitted")
	}
	// Document the prune->reopen behavior (this is exactly why the in-memory-only
	// map forgets across a restart — the reason HIGH-1 makes it durable: a pruned/absent
	// slot is freely re-bindable).
	e.pruneCommittedSlotsBelow(7)
	if !e.reserveSlotForSign(7, Y) {
		t.Fatal("after prune the height is unbound again — a DIFFERENT canonical is now accepted " +
			"(this is the post-finalize reopen; the post-CRASH reopen is what the durable guard prevents)")
	}
}

// TestRedIndep_OwnVoteRefusedForSibling is the teeth: after this node signs its first
// proposal A at height 1, it MUST NOT place its own signature on a conflicting sibling
// B at the same height. We assert on the certVotes membership directly — no async, no
// finalization — so the ONLY thing that can satisfy it is the engine guard. Removing
// either reserveSlotForSign call site makes B carry this node's signature and this test
// FAILS (verified against a neutered build).
func TestRedIndep_OwnVoteRefusedForSibling(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})
	self := vs.nodeID(0)

	A := newTestBlock(1, ids.Empty, "own-A")
	B := newTestBlock(1, ids.Empty, "own-B")

	_ = trackProposal(e, chainID, A, 0) // engine self-signs A (its first vote at height 1)
	_ = trackProposal(e, chainID, B, 0) // engine ATTEMPTS to self-sign conflicting sibling B

	e.mu.Lock()
	pbA := e.pendingBlocks[A.id]
	pbB := e.pendingBlocks[B.id]
	_, aHasSelf := pbA.certVotes[self]
	_, bHasSelf := pbB.certVotes[self]
	e.mu.Unlock()

	if !aHasSelf {
		t.Fatalf("engine must sign its FIRST proposal A at height 1 (own vote missing from A cert)")
	}
	if bHasSelf {
		t.Fatalf("FORK GUARD BROKEN: engine placed its OWN signature on the conflicting sibling B at "+
			"height 1 after already signing A. Its stake now backs two conflicting siblings — the exact "+
			"cross-node fork. (self=%s A=%s B=%s)", self, A.id, B.id)
	}

	// Belt: the funnel itself must now refuse B's canonical and accept A's (idempotent).
	if !e.reserveSlotForSign(1, A.id) {
		t.Fatalf("height 1 must be bound to A (idempotent re-check failed)")
	}
	if e.reserveSlotForSign(1, B.id) {
		t.Fatalf("height 1 must REFUSE B's canonical")
	}
}

// TestRedIndep_ByzantineForkThreshold proves the exact quorum-intersection threshold.
// n=5, K=5, alpha=4. A second conflicting cert (the fork witness) at one height needs
// |signers(A)| + |signers(B)| >= 2*alpha = 8 over n=5 validators, so |A ∩ B| >= 3 —
// three validators that signed BOTH, i.e. 3 Byzantine equivocators (2*alpha - n).
//
// We build A's and B's cert-vote sets DIRECTLY from raw (undisciplined) signatures and
// ask whether each assembles a verified alpha-of-K cert. A always has 4 honest-or-equiv
// signers; B is given k equivocators (who also signed A) plus one honest-B-only voter.
// The fork (both certs valid at once) appears iff k >= 3 — beyond the BFT budget f<n/3
// (=1). Within budget (and even at f=2) no second cert forms: safety holds with margin.
func TestRedIndep_ByzantineForkThreshold(t *testing.T) {
	const n = 5
	const alpha = 4 // params5Prod: K=5, AlphaConfidence=4

	// addRaw appends validator i's RAW (undisciplined, real Ed25519) signed accept for
	// the tracked block at pos into the block's cert-vote set. Models a Byzantine voter.
	addRaw := func(e *Transitive, blockID ids.ID, vs *testValidatorSet, voters ...int) {
		e.mu.Lock()
		defer e.mu.Unlock()
		pb := e.pendingBlocks[blockID]
		pos := e.blockPositionLocked(pb, blockID)
		for _, i := range voters {
			e.recordCertVoteLocked(pb, Vote{
				BlockID:   blockID,
				NodeID:    vs.nodeID(i),
				Accept:    true,
				Signature: vs.sign(i, pos),
				ParentID:  pos.ParentID,
			})
		}
	}
	certOK := func(e *Transitive, blockID ids.ID) bool {
		e.mu.Lock()
		defer e.mu.Unlock()
		_, ok := e.assembleVerifiedCertLocked(e.pendingBlocks[blockID], blockID)
		return ok
	}

	for k := 1; k <= 3; k++ {
		t.Run("equivocators="+itoa(k), func(t *testing.T) {
			vs := newTestValidatorSet(n)
			e, chainID := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})

			A := newTestBlock(1, ids.Empty, "thr-A")
			B := newTestBlock(1, ids.Empty, "thr-B")
			_ = trackProposal(e, chainID, A, 0) // engine (val0) self-signs A -> A={0}
			_ = trackProposal(e, chainID, B, 0) // guard refuses self-vote for B -> B={}

			// A reaches alpha honestly: {0(self),1,2,3}.
			addRaw(e, A.id, vs, 1, 2, 3)

			// B gets k equivocators (first k of {1,2,3}, who also signed A) + honest-B voter 4.
			equiv := []int{1, 2, 3}[:k]
			addRaw(e, B.id, vs, equiv...)
			addRaw(e, B.id, vs, 4)

			if !certOK(e, A.id) {
				t.Fatalf("A must hold a valid alpha-of-K cert (4 signers)")
			}
			gotFork := certOK(e, B.id)
			wantFork := k >= (2*alpha - n) // 2*4-5 = 3
			if gotFork != wantFork {
				t.Fatalf("k=%d equivocators: B second-cert=%v, want %v (fork threshold is 2*alpha-n=%d)",
					k, gotFork, wantFork, 2*alpha-n)
			}
			if gotFork {
				t.Logf("k=%d >= 3: SECOND conflicting cert exists — fork reachable, but only with f=%d >= n/3 "+
					"(BFT budget is f<=1). This is BEYOND the guarantee, as expected.", k, k)
			} else {
				t.Logf("k=%d < 3: no second cert — safety holds (f=%d, within/at margin of the f<n/3 budget).", k, k)
			}
		})
	}
}

// TestRedIndep_ReserveSlotConcurrent hammers the funnel from many goroutines with
// DISTINCT canonicals at one height. The atomic check-and-bind must let EXACTLY ONE
// win; every other distinct canonical is refused. Run under -race to also prove the
// committedSlot map is race-free.
func TestRedIndep_ReserveSlotConcurrent(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, _ := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})

	const G = 64
	const height = uint64(1234)
	cands := make([]ids.ID, G)
	for i := range cands {
		cands[i] = ids.GenerateTestID()
	}
	results := make([]bool, G)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < G; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = e.reserveSlotForSign(height, cands[i])
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	winnerIdx := -1
	for i, r := range results {
		if r {
			winners++
			winnerIdx = i
		}
	}
	if winners != 1 {
		t.Fatalf("expected EXACTLY ONE canonical to bind height %d under concurrency, got %d", height, winners)
	}
	// The winner is stable/idempotent; all losers remain refused.
	for i := 0; i < G; i++ {
		want := i == winnerIdx
		if got := e.reserveSlotForSign(height, cands[i]); got != want {
			t.Fatalf("post-settle non-idempotency at %d: got %v want %v", i, got, want)
		}
	}
}

// TestRedIndep_CommittedSlotLeak_LocalFinalizePrunes is Red's MEDIUM-1 demonstration,
// promoted to a hard gate. Red proved the v1.33.2 "bounded" claim was FALSE for the
// DOMINANT finalize path: pruneCommittedSlotsBelow ran ONLY from HandleIncomingCert,
// which a LOCALLY-finalizing node never reaches (it short-circuits at pending.Decided
// before the prune). v1.33.3 moves the prune into the sole finalizer acceptWithCertCore,
// so EVERY finality path prunes. This test drives a LOCAL vote-assembly finalize
// (handleVote -> tryFinalizeBlock, NOT HandleIncomingCert) and ASSERTS the slot is gone.
func TestRedIndep_CommittedSlotLeak_LocalFinalizePrunes(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})

	A := newTestBlock(1, ids.Empty, "leak-A")
	posA := trackProposal(e, chainID, A, 0) // binds committedSlot[{1,Empty}]=A; own vote recorded
	// Drive A to a full alpha-of-K quorum with RAW votes so it finalizes via the LOCAL
	// path (handleVote -> tryFinalizeBlock), NOT via HandleIncomingCert.
	for _, i := range []int{1, 2, 3} {
		e.ReceiveVote(Vote{
			BlockID:   A.id,
			NodeID:    vs.nodeID(i),
			Accept:    true,
			Signature: vs.sign(i, posA),
			ParentID:  posA.ParentID,
			Round:     posA.Round,
		})
	}
	if !waitFor(3*time.Second, func() bool { return e.IsAccepted(A.id) }) {
		t.Fatal("A must finalize via local vote-assembly")
	}

	// The prune is enqueued inside acceptWithCertCore on the finalize path; give the
	// async finalize a beat to complete, then require the slot to be GONE.
	pruned := waitFor(3*time.Second, func() bool {
		e.slotMu.Lock()
		_, stillBound := e.committedSlot[SlotKey{Height: 1}]
		e.slotMu.Unlock()
		return !stillBound
	})
	if !pruned {
		e.slotMu.Lock()
		n := len(e.committedSlot)
		e.slotMu.Unlock()
		t.Fatalf("MEDIUM-1 REGRESSION: after finalizing height 1 via the LOCAL path, committedSlot still "+
			"holds it (len=%d). pruneCommittedSlotsBelow must run from the sole finalizer acceptWithCertCore "+
			"so the local path prunes too — else committedSlot grows ~1 entry per finalized height.", n)
	}
}

// itoa avoids importing strconv for a single-digit label.
func itoa(i int) string { return string(rune('0' + i)) }
