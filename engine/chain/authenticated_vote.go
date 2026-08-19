// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import "github.com/luxfi/ids"

// ReceiveAuthenticatedVote is retained as a source-compatible tombstone for node
// versions that once promoted Chits into votes. It always refuses the input.
//
// An authenticated connection proves who sent a packet, but a Chits packet contains
// no signature over the consensus position or execution result. Counting it created
// a second, non-portable acceptance authority and even let a receiver manufacture the
// decision bit from its own re-Verify result. Multi-validator progress now comes from
// signed BroadcastVote messages; sole-validator progress uses the ordinary K==1
// ReceiveVote path.
func (t *Transitive) ReceiveAuthenticatedVote(origin ids.NodeID, blockID ids.ID, accept bool) bool {
	return false
}
