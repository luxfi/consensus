// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package interfaces

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/bag"
)

// Consensus represents a consensus instance that can process items
type Consensus interface {
	// Add an item to consensus
	Add(ids.ID) error
	
	// Check if consensus has finalized
	Finalized() bool
	
	// Get the current preference
	Preference() ids.ID
	
	// Record votes from a poll
	RecordVotes(bag.Bag[ids.ID]) error
	
	// Record a poll/prism with the given votes
	RecordPrism(bag.Bag[ids.ID]) error
}