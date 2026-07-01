// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// proposer_resolicit_test.go — the direct fails-before/passes-after proof of the
// build-path re-solicit alignment (engine.go buildBlocksLocked). avalanchego never
// silently drops a rebuild of a still-processing preferred block: its repoll keeps
// re-querying it until it decides. Lux's `continue` on an already-tracked block
// dropped the build signal, so a peer that missed the first PushQuery was only
// re-asked on the slower rePollAllPending backoff. This test parks the re-poll
// ticker and proves the BUILD path itself re-solicits an undecided own proposal.
package chain

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// TestProposerRebuild_ReSolicitsOwnUndecidedProposal drives two build passes for
// the SAME own block while it is still undecided (only the self-vote; α=4 not
// reached). The re-poll ticker is parked (RoundTO huge), so the ONLY thing that
// can re-solicit votes is the build path. With the fix the second build re-issues
// RequestVotes; the pre-fix bare `continue` leaves the count flat.
//
// PRE-FIX (revert the re-solicit block in buildBlocksLocked back to a bare
// `continue`): second == first → RED.
// POST-FIX: second > first → GREEN.
func TestProposerRebuild_ReSolicitsOwnUndecidedProposal(t *testing.T) {
	vs := newTestValidatorSet(5)
	params := params5Prod()
	params.RoundTO = 30 * time.Second // park the re-poll ticker: only the BUILD path may re-solicit
	rec := &recordingGossiper{}
	e, _ := newQuorumEngine(t, params, vs, 0, rec)

	cp := newReSolicitProbe()
	e.SetProposer(cp)

	blk := newTestBlock(1, ids.Empty, "rebuilt-own")
	vm := &prefRecordingVM{build: blk}
	e.SetVM(vm)

	// First build: tracks the own proposal and solicits its votes once.
	if err := e.Notify(context.Background(), Message{Type: PendingTxs}); err != nil {
		t.Fatalf("Notify #1: %v", err)
	}
	first := cp.requestCount(blk.id)
	if first == 0 {
		t.Fatalf("first build must solicit votes at least once (got %d)", first)
	}

	// The block is still undecided (self-vote only, below α=4). The VM re-offers the
	// SAME block (mempool still non-empty). Only the build-path re-solicit can fire.
	if err := e.Notify(context.Background(), Message{Type: PendingTxs}); err != nil {
		t.Fatalf("Notify #2: %v", err)
	}
	second := cp.requestCount(blk.id)

	if second <= first {
		t.Fatalf("REBUILD DROPPED: a rebuild of the UNDECIDED own proposal did not re-solicit votes "+
			"(RequestVotes stayed at %d). avalanchego re-queries a still-processing preferred block until it "+
			"decides; the bare `continue` on already-tracked silently drops the build signal, so a peer that "+
			"missed the first PushQuery is never re-asked on the build path.", second)
	}
}

// TestProposerRebuild_DoesNotReSolicitDecidedProposal proves the re-solicit is
// scoped: once a block is DECIDED, a spurious rebuild does NOT re-solicit (no
// wasted gossip for a finalized height). It also guards against re-soliciting a
// non-own block (the build path only ever handles own proposals, but the Decided
// gate is the belt-and-suspenders that keeps a finalized own block quiet).
func TestProposerRebuild_DoesNotReSolicitDecidedProposal(t *testing.T) {
	vs := newTestValidatorSet(5)
	params := params5Prod()
	params.RoundTO = 30 * time.Second
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params, vs, 0, rec)

	cp := newReSolicitProbe()
	e.SetProposer(cp)

	blk := newTestBlock(1, ids.Empty, "decided-own")
	vm := &prefRecordingVM{build: blk}
	e.SetVM(vm)

	// Track + finalize the block with a full α-of-K quorum so it is Decided.
	pos := trackProposal(e, chainID, blk, 0) // self = 1
	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos))
	e.ReceiveVote(vs.signedVote(3, pos)) // 4 of 5 → finalizes
	if !waitFor(2*time.Second, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("setup: block must finalize with its α-of-K quorum")
	}
	before := cp.requestCount(blk.id)

	// A rebuild of the now-DECIDED block must not re-solicit.
	if err := e.Notify(context.Background(), Message{Type: PendingTxs}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if after := cp.requestCount(blk.id); after != before {
		t.Fatalf("a DECIDED block must not be re-solicited on rebuild (RequestVotes went %d -> %d)", before, after)
	}
}
