// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"time"

	"github.com/luxfi/ids"
)

// ReceiveAuthenticatedVote counts a peer's preference toward the NOVA tally when
// the peer's identity came from the authenticated transport.
//
// THE PROBLEM IT SOLVES. A Chits message carries no signature field, so on a K>1
// chain every inbound preference was dropped at the engine's signature gate and α
// was unreachable BY CONSTRUCTION: blocks were built, verified, and preferred by
// every node while the tally stayed at zero. Testnet sat at 16820 and devnet at
// 7323 in exactly this state, each node logging "vote is UNSIGNED and cannot be
// counted" once per peer per poll, forever.
//
// WHY IT IS SAFE, WHEN COUNTING A PAYLOAD-SUPPLIED Vote{NodeID} WOULD NOT BE.
// The distinction is where origin comes from, and it is the same one the reference implementation
// relies on: its engine receives the voter as `Chits(nodeID, requestID, …)`, a
// PARAMETER the router fills in from the authenticated connection, so a peer
// cannot claim to be someone else. Lux's Vote carries NodeID as a struct FIELD,
// which any peer can set to any value — counting that would make finality
// forgeable by anyone who can send 32 bytes.
//
// So origin is a PARAMETER here too. The caller must be the inbound-message path
// that already authenticated the sender; the identity is not taken from anything
// the sender serialized. `transportAuthenticated` is unexported, so no decoded
// value can carry it.
//
// WHAT IT DELIBERATELY DOES NOT DO. It contributes to the Nova (liveness) tally
// only. It never reaches recordCertVoteLocked, so it can never become part of a
// QuorumCert: a cert is a PORTABLE claim that travels to nodes which did not
// authenticate this connection, and for those nodes only a signature carries
// meaning. Quasar therefore still requires signatures, unchanged — local liveness
// rests on transport authentication, exported finality rests on cryptography.
func (t *Transitive) ReceiveAuthenticatedVote(origin ids.NodeID, blockID ids.ID, accept bool) bool {
	if origin == ids.EmptyNodeID || blockID == ids.Empty {
		return false
	}
	if !t.holdsStake(origin, blockID) {
		return false
	}
	return t.ReceiveVote(Vote{
		BlockID:                blockID,
		NodeID:                 origin,
		Accept:                 accept,
		SignedAt:               time.Now(),
		transportAuthenticated: true,
	})
}

// holdsStake answers whether origin is a validator of this chain at the block's
// epoch.
//
// The signed path never has to ask. VerifyVote resolves the voter's key out of
// the set at that epoch, so an id that is not in the set has no key and its vote
// dies there — membership is answered as a side effect of checking the
// signature. This path has no signature to resolve, and the tallies are keyed by
// NodeID, so without asking directly α would count distinct PEERS. A peer need
// not be a validator to connect, and its NodeID is a hash of a certificate it
// generates for itself, so distinct peers are free to mint.
//
// Without a stake model there is no membership to check against, so the answer
// is no. Every chain that runs a quorum has one: a K>1 chain refuses to start
// unless height-indexed validator state is wired, because zero stake at every
// height would stall finality anyway. A chain reaching here with none is a chain
// that cannot say who its validators are, and a vote it cannot attribute is a
// vote it cannot count.
func (t *Transitive) holdsStake(origin ids.NodeID, blockID ids.ID) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.stakeSource == nil {
		return false
	}
	return t.stakeSource.Weight(origin, t.epochHeightLocked(t.pendingBlocks[blockID])) > 0
}
