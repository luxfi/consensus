// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrap

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/set"
)

type Prism interface {
	// GetPeers returns the set of peers whose opinion should be requested. It
	// is expected to repeatedly call this function along with [RecordOpinion]
	// until [Result] returns finalized.
	GetPeers(ctx context.Context) (peers set.Set[ids.NodeID])
	// RecordOpinion of a node whose opinion was requested.
	RecordOpinion(ctx context.Context, nodeID ids.NodeID, blkIDs set.Set[ids.ID]) error
	// Result returns the evaluation of all the peer's opinions along with a
	// flag to identify that the result has finished being calculated.
	Result(ctx context.Context) (blkIDs []ids.ID, finalized bool)
}
