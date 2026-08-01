// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// TestAuthenticatedVote_ReachesAlphaAndFinalizes is the LIVENESS regression for
// the halt. A Chits carries no signature; before ReceiveAuthenticatedVote existed
// every inbound preference on a K>1 chain died at the signature gate and α was
// unreachable no matter how many honest peers agreed. Testnet held at 16820 and
// devnet at 7323 in exactly this state.
func TestAuthenticatedVote_ReachesAlphaAndFinalizes(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{})
	alpha := e.consensus.Alpha()
	if e.consensus.K() <= 1 || e.voteVerifier == nil {
		t.Fatal("precondition: the gate only exists on a K>1 chain with a verifier")
	}

	blk := newTestBlock(16821, ids.GenerateTestID(), "authenticated-nova")
	trackFollowedBlock(e, chainID, blk)

	for i := 0; i < alpha; i++ {
		if !e.ReceiveAuthenticatedVote(vs.nodeID(i), blk.id, true) {
			t.Fatalf("validator %d's authenticated preference was not queued", i)
		}
	}

	if !waitFor(2*time.Second, func() bool { return readVoteTally(e, blk).acceptVotes >= alpha }) {
		t.Fatalf("HALT REPRODUCED: %d peers, each authenticated by the transport, agreed on the "+
			"block and the tally is %+v. A Chits has no signature, so if these are dropped α is "+
			"unreachable BY CONSTRUCTION and the chain stops — which is what testnet 16820 and "+
			"devnet 7323 were doing.", alpha, readVoteTally(e, blk))
	}
}

// TestAuthenticatedVote_IsNovaOnly_NeverBuildsACert is the SAFETY half. Transport
// authentication is meaningful only to the node that performed it; a QuorumCert is
// a portable claim that travels to nodes which did not. So these votes must never
// become cert witnesses, no matter how many arrive.
func TestAuthenticatedVote_IsNovaOnly_NeverBuildsACert(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{})

	blk := newTestBlock(16822, ids.GenerateTestID(), "nova-only")
	trackFollowedBlock(e, chainID, blk)

	for i := 0; i < 5; i++ {
		e.ReceiveAuthenticatedVote(vs.nodeID(i), blk.id, true)
	}

	if got := readVoteTally(e, blk); got.certVotes != 0 {
		t.Fatalf("EXPORTABLE FINALITY FROM UNSIGNED VOTES: %d cert witnesses were assembled from "+
			"transport-authenticated preferences (%+v). A cert must be verifiable by a node that "+
			"never saw this connection, so only signatures may enter it.", got.certVotes, got)
	}
}

// TestAuthenticatedVote_PayloadOriginStillRefused pins the boundary that makes the
// whole thing safe: the exemption belongs to the transport-supplied ORIGIN, not to
// "unsigned votes" generally. A vote built from payload fields — where NodeID is
// whatever the sender wrote — must still be dropped. `transportAuthenticated` is
// unexported precisely so no decoded value can carry it.
func TestAuthenticatedVote_PayloadOriginStillRefused(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{})

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

// TestAuthenticatedVote_OneVotePerValidator: transport authentication proves WHO
// sent a message, not how many times they may be counted. A peer answering the
// same poll repeatedly must move the tally once.
func TestAuthenticatedVote_OneVotePerValidator(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{})
	alpha := e.consensus.Alpha()

	blk := newTestBlock(16824, ids.GenerateTestID(), "authenticated-replay")
	trackFollowedBlock(e, chainID, blk)

	for i := 0; i < alpha+3; i++ {
		e.ReceiveAuthenticatedVote(vs.nodeID(0), blk.id, true)
	}

	if got := readVoteTally(e, blk); got.acceptVotes > 1 {
		t.Fatalf("ONE PEER REACHED α ALONE: %d authenticated replays from a single validator moved "+
			"the tally to %d (%+v). Authentication names the sender; it does not entitle them to "+
			"vote α times.", alpha+3, got.acceptVotes, got)
	}
}
