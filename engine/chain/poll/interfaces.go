// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package poll

import (
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/consensus/utils/formatting"
)

// Set is a collection of polls
type Set interface {
	fmt.Stringer

	Add(requestID uint32, vdrs bag.Bag[ids.NodeID]) bool
	Vote(requestID uint32, vdr ids.NodeID, vote ids.ID) (ids.ID, bool)
	Drop(requestID uint32, vdr ids.NodeID) (ids.ID, bool)
	Len() int
}

// Poll is an outstanding poll
type Poll interface {
	fmt.Stringer
	formatting.PrefixedStringer

	Vote(vdr ids.NodeID, vote ids.ID)
	Drop(vdr ids.NodeID)
	Finished() bool
	Result() (ids.ID, bool)
}

// Factory creates a new Poll
type Factory interface {
	New(vdrs bag.Bag[ids.NodeID]) Poll
}
