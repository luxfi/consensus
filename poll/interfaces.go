// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package poll

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/bag"
)

// Set manages multiple polls
type Set interface {
	Add(requestID uint32, vdrs bag.Bag[ids.NodeID]) bool
	Vote(requestID uint32, vdr ids.NodeID, vote ids.ID) (bag.Bag[ids.ID], bool)
	Drop(requestID uint32, vdr ids.NodeID) (bag.Bag[ids.ID], bool)
	Len() int
}