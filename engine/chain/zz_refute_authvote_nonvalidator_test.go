// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// The transport-authenticated entry point proves WHO sent a message. It does not
// ask whether that sender is a validator of this chain. On the payload path the
// question is answered as a side effect: VerifyVote resolves the voter's key out
// of the set at the block's epoch height, so an id that is not in the set has no
// key and the vote dies. The transport path skips that resolution entirely, and
// nothing downstream re-asks: acceptVoters/rejectVoters are plain NodeID-keyed
// maps and alpha is a count over them.
//
// So the tallies count distinct PEERS, not distinct VALIDATORS — which is what
// the comment on those maps says they count. NodeIDs are self-minted (a hash of
// a cert the peer generates), peers may connect without being validators
// (RequireValidatorToConnect is off by default), and an inbound Chits is
// processed whether or not this node ever polled the sender.
func TestAuthVote_StrangerMovesTheAcceptTally(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5(), vs, 0, &recordingGossiper{})
	alpha := e.consensus.Alpha()

	blk := newTestBlock(90001, ids.GenerateTestID(), "stranger-accept")
	trackFollowedBlock(e, chainID, blk)

	// Every voter here is a stranger: freshly generated, never in vs.
	for i := 0; i < alpha; i++ {
		stranger := ids.GenerateTestNodeID()
		if vs.pub[stranger] != nil {
			t.Fatal("precondition: the voter must not be a validator")
		}
		// Refused at the door is the point. If it is queued instead, the tally
		// check below is what has to catch it.
		e.ReceiveAuthenticatedVote(stranger, blk.id, true)
	}

	reached := waitFor(2*time.Second, func() bool {
		return readVoteTally(e, blk).acceptVotes >= alpha
	})
	got := readVoteTally(e, blk)
	if reached {
		t.Fatalf("STRANGERS REACHED α: %d ids that hold no stake and sit in no validator "+
			"set moved the accept tally to %d and accepted=%v (%+v). alpha is supposed to mean "+
			"'α distinct VALIDATORS agreed'.", alpha, got.acceptVotes, got.accepted, got)
	}
}

// The same hole on the reject side, where the consequence is not a futile
// finalize attempt. Reaching α rejects sets block.rejected, drops the block from
// tips, and the engine commits VM.Reject inline — the accept path's cert gate
// does not stand in front of it ("Rejections carry no stake-safety concern (a
// block is dropped, not finalized) and are committed inline", engine.go).
//
// The peer does not choose the polarity: applyPollResponse derives it from THIS
// node's re-Verify, which fails routinely for reasons that are not disagreement
// (already-verified, missing parent, state conflict — the node comments say so).
// A stranger therefore does not need to lie. It names a block; if our re-Verify
// errors, its preference is filed as that stranger's REJECT.
func TestAuthVote_StrangerMovesTheRejectTally(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5(), vs, 0, &recordingGossiper{})
	alpha := e.consensus.Alpha()

	blk := newTestBlock(90002, ids.GenerateTestID(), "stranger-reject")
	trackFollowedBlock(e, chainID, blk)

	for i := 0; i < alpha; i++ {
		e.ReceiveAuthenticatedVote(ids.GenerateTestNodeID(), blk.id, false)
	}

	reached := waitFor(2*time.Second, func() bool {
		return readVoteTally(e, blk).rejectVotes >= alpha
	})
	got := readVoteTally(e, blk)
	if reached || e.consensus.IsRejected(blk.id) {
		t.Fatalf("STRANGERS REJECTED A BLOCK: reject tally %d, IsRejected=%v (%+v). The reject "+
			"road has no cert in front of it — this is the one the engine commits inline.",
			got.rejectVotes, e.consensus.IsRejected(blk.id), got)
	}
}

// Even a real validator id is insufficient without a signature over the exact
// consensus position. Transport identity is not a substitute for that proof.
func TestAuthVote_ValidatorsWithoutSignaturesAreRefused(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5(), vs, 0, &recordingGossiper{})
	alpha := e.consensus.Alpha()

	blk := newTestBlock(90003, ids.GenerateTestID(), "control")
	trackFollowedBlock(e, chainID, blk)

	for i := 0; i < alpha; i++ {
		e.ReceiveAuthenticatedVote(vs.nodeID(i), blk.id, true)
	}
	if waitFor(100*time.Millisecond, func() bool { return readVoteTally(e, blk).acceptVotes >= alpha }) {
		t.Fatalf("unsigned validator identities reached α (%+v)", readVoteTally(e, blk))
	}
}
