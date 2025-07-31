// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package testutils

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/utils/bag"
)

// Type aliases for convenience
type (
	Parameters = config.Parameters
)

// Consensus interface for testing
type Consensus interface {
	// Add a new choice to consensus
	Add(choice ids.ID)
	
	// RecordPrism records the results of a poll
	RecordPrism(votes bag.Bag[ids.ID])
	
	// Finalized returns true if consensus has been reached
	Finalized() bool
	
	// Preference returns the current preference
	Preference() ids.ID
}