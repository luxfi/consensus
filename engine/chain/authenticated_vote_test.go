// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"testing"

	"github.com/luxfi/ids"
)

// TestAuthenticatedVote_IsRefused pins the tombstone: transport identity does not
// turn an unsigned Chits preference into a cryptographic vote.
func TestAuthenticatedVote_IsRefused(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5(), vs, 0, &recordingGossiper{})
	alpha := e.consensus.Alpha()
	if e.consensus.K() <= 1 || e.voteVerifier == nil {
		t.Fatal("precondition: the gate only exists on a K>1 chain with a verifier")
	}

	blk := newTestBlock(16821, ids.GenerateTestID(), "authenticated-nova")
	trackFollowedBlock(e, chainID, blk)

	for i := 0; i < alpha; i++ {
		if e.ReceiveAuthenticatedVote(vs.nodeID(i), blk.id, true) {
			t.Fatalf("validator %d's unsigned preference was queued as a vote", i)
		}
	}

	if got := readVoteTally(e, blk); got.acceptVotes != 0 || got.voteCount != 0 || got.certVotes != 0 {
		t.Fatalf("unsigned transport preferences moved the quorum tally: %+v", got)
	}
}

// TestAuthenticatedVote_NeverBuildsACert is the portable-proof half: no number of
// unsigned transport messages can produce a certificate witness.
func TestAuthenticatedVote_NeverBuildsACert(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5(), vs, 0, &recordingGossiper{})

	blk := newTestBlock(16822, ids.GenerateTestID(), "nova-only")
	trackFollowedBlock(e, chainID, blk)

	for i := 0; i < 5; i++ {
		e.ReceiveAuthenticatedVote(vs.nodeID(i), blk.id, true)
	}

	if got := readVoteTally(e, blk); got.acceptVotes != 0 || got.certVotes != 0 {
		t.Fatalf("unsigned transport preferences entered finality state: %+v", got)
	}
}

// TestAuthenticatedVote_PayloadOriginStillRefused pins the boundary that makes the
// whole thing safe: the exemption belongs to the transport-supplied ORIGIN, not to
// "unsigned votes" generally. A vote built from payload fields — where NodeID is
// whatever the sender wrote — must still be dropped. `transportAuthenticated` is
// unexported precisely so no decoded value can carry it.
func TestAuthenticatedVote_PayloadOriginStillRefused(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5(), vs, 0, &recordingGossiper{})

	blk := newTestBlock(16823, ids.GenerateTestID(), "payload-origin")
	pos := trackFollowedBlock(e, chainID, blk)

	for i := 0; i < 5; i++ {
		e.handleVote(Vote{BlockID: blk.id, NodeID: vs.nodeID(i), Accept: true,
			ParentID: pos.ParentID, Round: pos.Round})
	}

	if got := readVoteTally(e, blk); got.acceptVotes != 0 || got.voteCount != 0 || got.certVotes != 0 {
		t.Fatalf("PAYLOAD-SUPPLIED ORIGIN COUNTED: %+v. NodeID is a struct field any peer can set "+
			"to any value; only an origin passed as a PARAMETER by the authenticated transport may "+
			"skip the signature gate.", got)
	}
}

// TestAuthenticatedVote_ReplaysRemainRefused ensures retries cannot revive the
// removed unsigned acceptance path.
func TestAuthenticatedVote_ReplaysRemainRefused(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5(), vs, 0, &recordingGossiper{})
	alpha := e.consensus.Alpha()

	blk := newTestBlock(16824, ids.GenerateTestID(), "authenticated-replay")
	trackFollowedBlock(e, chainID, blk)

	for i := 0; i < alpha+3; i++ {
		e.ReceiveAuthenticatedVote(vs.nodeID(0), blk.id, true)
	}

	if got := readVoteTally(e, blk); got.acceptVotes != 0 || got.voteCount != 0 {
		t.Fatalf("%d unsigned replays moved the tally: %+v", alpha+3, got)
	}
}
