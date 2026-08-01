// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// dedup_tally_test.go — ALPHA MUST MEAN "N DISTINCT VALIDATORS AGREED".
//
// certVotes has always been a map keyed by NodeID ("de-dup: one vote per
// validator"). VoteCount and RejectCount were bare integers incremented on
// every arrival, so a single validator whose one signed vote arrived four
// times reached α=4-of-5 by itself. Polls ARE re-solicited and peers DO answer
// twice, so this is reachable with no malice at all — and a peer that wanted
// it could simply resend.
//
// The property belongs at the tally, not in a convention that every upstream
// caller must remember. This drives the real handleVote path with real
// signatures so that reverting the fix actually fails the test.
package chain

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// TestPlainTallyCountsDistinctVotersOnly — one validator, one signed vote,
// four arrivals, one count. FAILS on the pre-fix code (tally reads 4, and at
// α=4-of-5 that is a self-assembled quorum).
func TestPlainTallyCountsDistinctVotersOnly(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{})
	alpha := e.consensus.Alpha()
	if alpha < 2 {
		t.Fatalf("precondition: α must be a real quorum, got %d", alpha)
	}

	blk := newTestBlock(4242, ids.GenerateTestID(), "replayed-accept")
	pos := trackFollowedBlock(e, chainID, blk)

	// ONE validator's ONE genuinely-signed accept, delivered four times. Every
	// arrival is byte-identical and individually valid — the gate cannot reject
	// any of them, so only the tally can hold the line.
	solo := vs.signedVote(0, pos)
	if len(solo.Signature) == 0 {
		t.Fatal("harness precondition: the replayed vote must actually carry a signature")
	}
	for i := 0; i < 4; i++ {
		e.handleVote(solo)
	}

	got := readVoteTally(e, blk)
	if got.voteCount != 1 {
		t.Fatalf("QUORUM FORGEABLE BY ONE VALIDATOR: a single validator's vote arrived four times and "+
			"the tally reads %d, want 1 (%+v). At α=%d-of-5 that is a self-assembled quorum — alpha "+
			"means \"α distinct validators agreed\" or it means nothing.", got.voteCount, got, alpha)
	}
	if got.certVotes != 1 {
		t.Fatalf("certVotes has always been NodeID-keyed and must still read 1, got %d", got.certVotes)
	}
	if got.accepted || got.acceptVotes >= alpha {
		t.Fatalf("one validator reached the α predicate alone: %+v (α=%d)", got, alpha)
	}

	// POSITIVE CONTROL: without this, the fix could satisfy itself by never counting
	// anything at all. Kept at TWO voters — below the Nova cert quorum (⌊n/2⌋+1 = 3),
	// which decides before α=4 is ever reached (see nova-majority-cert-safety-hole).
	// Past it the block finalizes and drains out of pendingBlocks, and the counters
	// would then read zero for the wrong reason.
	e.handleVote(vs.signedVote(1, pos))
	if got = readVoteTally(e, blk); got.voteCount != 2 || got.acceptVotes != 2 {
		t.Fatalf("POSITIVE CONTROL FAILED: a second DISTINCT signed voter must reach voteCount=2 "+
			"and acceptVotes=2, got %+v", got)
	}
}

// TestRejectTallyCountsDistinctVotersOnly — the same rule on the reject side.
// Without it one validator can veto a block alone by answering twice.
func TestRejectTallyCountsDistinctVotersOnly(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{})

	blk := newTestBlock(4243, ids.GenerateTestID(), "replayed-reject")
	pos := trackFollowedBlock(e, chainID, blk)

	// No signedVote-style reject helper exists; build it directly over the
	// reject-bound canonical message (a reject signs a DIFFERENT message than an
	// accept, which is why the accept helper cannot be reused here).
	rej := Vote{
		BlockID:   blk.id,
		NodeID:    vs.nodeID(0),
		Accept:    false,
		SignedAt:  time.Now(),
		Signature: ed25519.Sign(vs.keys[vs.nodeID(0)], canonicalVoteMessageFor(pos, false)),
		ParentID:  pos.ParentID,
		Round:     pos.Round,
	}
	for i := 0; i < 3; i++ {
		e.handleVote(rej)
	}
	if len(rej.Signature) == 0 {
		t.Fatal("harness precondition: the replayed reject must actually carry a signature")
	}
	if got := readVoteTally(e, blk); got.rejectVotes != 1 {
		t.Fatalf("ONE VALIDATOR VETOES ALONE: one signed reject arrived three times and rejectVotes "+
			"reads %d, want 1 (%+v)", got.rejectVotes, got)
	}
}
