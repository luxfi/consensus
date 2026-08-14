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
	return t.ReceiveVote(Vote{
		BlockID:                blockID,
		NodeID:                 origin,
		Accept:                 accept,
		SignedAt:               time.Now(),
		transportAuthenticated: true,
	})
}
