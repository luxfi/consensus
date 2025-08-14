// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowman

import (
	"context"
	"time"

	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/ids"
)

// Block is a snowman block that can be decided by nova consensus
type Block interface {
	interfaces.Decidable

	// Parent returns the ID of this block's parent
	Parent() ids.ID

	// Height returns the height of this block
	Height() uint64

	// Timestamp returns when this block was created
	Timestamp() time.Time

	// Verify that the state transition this block would make is valid
	Verify(context.Context) error

	// Bytes returns the binary representation of this block
	Bytes() []byte
}

// Consensus represents the nova consensus protocol for chains
type Consensus interface {
	// Add a block to the consensus
	Add(context.Context, Block) error

	// Processing returns the set of blocks currently being processed
	Processing() []ids.ID

	// IsPreferred returns whether the block is currently preferred
	IsPreferred(ids.ID) bool

	// Preference returns the ID of the preferred block
	Preference() ids.ID

	// Finalized returns whether consensus has been reached
	Finalized() bool

	// HealthCheck returns information about the consensus health
	HealthCheck(context.Context) (interface{}, error)

	// RecordPoll records the results of a network poll
	RecordPoll(context.Context, []ids.ID) error

	// Quiesce handles graceful shutdown
	Quiesce(context.Context) error

	// Shutdown handles immediate shutdown  
	Shutdown(context.Context) error
}