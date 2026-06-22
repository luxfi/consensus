// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_topology.go — the vote/cert distribution handlers that make α-of-K
// finality both SAFE and LIVE.
//
// The topology in one paragraph: a follower verifies a gossiped block and
// BROADCASTS its signed accept vote to ALL validators (followVerifiedBlock,
// integration.go). Every validator feeds incoming signed votes into the engine
// (HandleIncomingVote), which collects them toward a QuorumCert. Whichever node
// first collects α distinct verified votes assembles the cert, GOSSIPS it
// (tryFinalizeBlock), and every node finalizes the block on receipt of that
// verifiable proof (HandleIncomingCert). No node finalizes without a cert
// (safety); the cert reaches everyone via vote-broadcast + cert-gossip + the
// poll-timeout re-request, so finality never hinges on one node's inbound Chits
// (liveness — the proposer-freeze cannot recur).
package chain

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// ErrVoteWireCorrupt is returned by decodeSignedVote on any structural defect.
var ErrVoteWireCorrupt = errors.New("chain: signed vote wire corrupt")

// signed-vote wire layout (big-endian):
//
//	node_id:20
//	sig_len:4  sig:sig_len
//
// The signed vote travels with the blockID in the gossip envelope (the network
// message already names the chain + block), so the canonical message a verifier
// rebuilds is derived from the RECEIVER's tracked position for that block — a
// vote cannot smuggle a different position because the signature is checked
// against the receiver's own (chain,height,round,block,parent).

// encodeSignedVote encodes (nodeID, signature) for broadcast.
func encodeSignedVote(nodeID ids.NodeID, sig []byte) ([]byte, error) {
	buf := make([]byte, 0, ids.NodeIDLen+4+len(sig))
	buf = append(buf, nodeID[:]...)
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], uint32(len(sig)))
	buf = append(buf, u32[:]...)
	buf = append(buf, sig...)
	return buf, nil
}

// decodeSignedVote is the inverse of encodeSignedVote. Strict trailing-bytes
// policy; fail-closed on short reads.
func decodeSignedVote(data []byte) (ids.NodeID, []byte, error) {
	var nodeID ids.NodeID
	if len(data) < ids.NodeIDLen+4 {
		return nodeID, nil, ErrVoteWireCorrupt
	}
	copy(nodeID[:], data[:ids.NodeIDLen])
	rest := data[ids.NodeIDLen:]
	sigLen := binary.BigEndian.Uint32(rest[:4])
	rest = rest[4:]
	if uint64(sigLen) != uint64(len(rest)) {
		return nodeID, nil, fmt.Errorf("%w: sig_len %d != remaining %d", ErrVoteWireCorrupt, sigLen, len(rest))
	}
	sig := make([]byte, sigLen)
	copy(sig, rest)
	return nodeID, sig, nil
}

// HandleIncomingVote ingests a signed accept vote received from another
// validator's broadcast. The vote is bound to a blockID (carried by the gossip
// envelope); the engine rebuilds the canonical message from ITS OWN tracked
// position for that block and verifies the signature before counting the vote.
// A vote for a block we are not tracking is dropped (we cannot know its
// position to verify against — the proposer's block gossip carries that, and
// arrives via HandleIncomingBlock).
//
// Returns true iff the vote verified and was counted toward the block's cert.
func (rt *Runtime) HandleIncomingVote(blockID ids.ID, voteBytes []byte) bool {
	nodeID, sig, err := decodeSignedVote(voteBytes)
	if err != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("incoming vote: decode failed", log.Stringer("blockID", blockID), log.Err(err))
		}
		return false
	}

	t := rt.Transitive
	t.mu.RLock()
	pending, exists := t.pendingBlocks[blockID]
	verifier := t.voteVerifier
	var pos VotePosition
	if exists {
		pos = t.blockPositionLocked(pending, blockID)
	}
	t.mu.RUnlock()

	if !exists || verifier == nil {
		return false
	}

	// Verify the signature against OUR position for this block. A vote that
	// signed a different position (different height/round/parent) fails here.
	if !verifier.VerifyVote(nodeID, CanonicalVoteMessage(pos), sig) {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("incoming vote: signature invalid",
				log.Stringer("blockID", blockID), log.Stringer("from", nodeID))
		}
		return false
	}

	// Count it. ReceiveVote routes through handleVote, which records the signed
	// vote toward the cert and triggers tryFinalizeBlock once alpha is reached.
	t.ReceiveVote(Vote{
		BlockID:   blockID,
		NodeID:    nodeID,
		Accept:    true,
		Signature: sig,
		ParentID:  pos.ParentID,
		Round:     pos.Round,
	})
	return true
}

// HandleIncomingCert ingests a finality cert gossiped by another validator and,
// if it verifies as a valid α-of-K witness for a block we have verified,
// finalizes that block. This is the SAFE replacement for fast-follow: a
// follower commits a gossiped block ONLY against a verifiable α-of-K proof.
//
// Safety gates:
//   - the cert must decode and Verify under our VoteVerifier (α distinct
//     correctly-signed accepts over the cert's position),
//   - the cert's position chain must match ours,
//   - we must have VERIFIED the block locally (it is in pendingBlocks). We do
//     NOT accept a block whose contents we have not validated, even with a
//     valid cert — the cert proves agreement, local Verify proves validity;
//     both are required.
//
// Returns true iff the block was finalized as a result of this cert.
func (rt *Runtime) HandleIncomingCert(certBytes []byte) bool {
	cert, err := UnmarshalQuorumCert(certBytes)
	if err != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("incoming cert: decode failed", log.Err(err))
		}
		return false
	}

	t := rt.Transitive
	t.mu.RLock()
	verifier := t.voteVerifier
	chainID := t.chainID
	pending, exists := t.pendingBlocks[cert.Position.BlockID]
	t.mu.RUnlock()

	if verifier == nil {
		return false
	}
	// The cert's threshold MUST meet our own α floor — a cert that asserts a
	// LOWER threshold than this chain requires is rejected even if its internal
	// signatures verify (sub-quorum finality forgery defence, mirrors quasar's
	// MinThreshold floor).
	if alpha := t.consensus.Alpha(); alpha > 0 && cert.Threshold < uint32(alpha) {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("incoming cert: threshold below chain alpha floor",
				log.Stringer("blockID", cert.Position.BlockID),
				log.Uint32("certThreshold", cert.Threshold),
				log.Int("alphaFloor", alpha))
		}
		return false
	}
	if cert.Position.ChainID != chainID {
		return false
	}
	if err := cert.Verify(verifier); err != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("incoming cert: verification failed",
				log.Stringer("blockID", cert.Position.BlockID), log.Err(err))
		}
		return false
	}

	// Cert is a valid α-of-K finality proof. Finalize the block IF we have
	// verified it locally. If we have not yet seen/verified the block, we cannot
	// safely Accept it — record nothing; the block gossip will arrive and the
	// cert can be re-requested/re-applied. (A production node may also fetch the
	// block by ID here; that fetch path is the node layer's responsibility.)
	if !exists {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("incoming cert: valid but block not locally verified yet; deferring",
				log.Stringer("blockID", cert.Position.BlockID))
		}
		return false
	}

	ctx := context.Background()
	if c := t.ctx; c != nil {
		ctx = c
	}

	// Record the cert and mark accepted-via-cert in consensus, then run the
	// shared finalize path (VM.Accept + SetPreference + pipeline).
	t.mu.Lock()
	if pending.Decided {
		t.mu.Unlock()
		return false
	}
	pending.cert = cert
	t.mu.Unlock()

	t.consensus.AcceptViaCert(cert.Position.BlockID)

	rt.fastFollowMu.Lock()
	if cert.Position.Height > rt.fastFollowHeight {
		rt.fastFollowHeight = cert.Position.Height
	}
	rt.fastFollowMu.Unlock()

	t.finalizePendingLocked(ctx, cert.Position.BlockID)
	if !rt.config.Logger.IsZero() {
		rt.config.Logger.Info("finalized block via α-of-K quorum cert",
			log.Stringer("blockID", cert.Position.BlockID),
			log.Uint64("height", cert.Position.Height),
			log.Int("voters", cert.VoterCount()))
	}
	return true
}
