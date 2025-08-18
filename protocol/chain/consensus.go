// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"time"

	"github.com/luxfi/ids"
)

// Consensus defines the chain consensus interface
type Consensus interface {
	// Initialize the consensus
	Initialize(ctx context.Context) error

	// Add a block to consensus
	Add(ctx context.Context, block Block) error

	// RecordPoll records poll responses
	RecordPoll(ctx context.Context, responses []ids.ID) error

	// Finalized returns whether a block is finalized
	Finalized(ctx context.Context, blockID ids.ID) (bool, error)
}

// Block represents a blockchain block
type Block interface {
	// ID returns the block ID
	ID() ids.ID

	// Parent returns the parent block ID
	Parent() ids.ID

	// Height returns the block height
	Height() uint64

	// Timestamp returns the block timestamp
	Timestamp() time.Time

	// Bytes returns the block bytes
	Bytes() []byte

	// Accept accepts the block
	Accept(ctx context.Context) error

	// Reject rejects the block
	Reject(ctx context.Context) error

	// Verify verifies the block
	Verify(ctx context.Context) error
}