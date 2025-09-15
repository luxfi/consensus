// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package interfaces defines core consensus interfaces
package interfaces

import (
	"context"

	"github.com/luxfi/ids"
)

// State represents the state of the consensus
type State interface {
	// GetBlock retrieves a block by its ID
	GetBlock(ctx context.Context, blockID ids.ID) (Block, error)

	// PutBlock stores a block
	PutBlock(ctx context.Context, block Block) error

	// GetLastAccepted returns the last accepted block ID
	GetLastAccepted(ctx context.Context) (ids.ID, error)

	// SetLastAccepted sets the last accepted block ID
	SetLastAccepted(ctx context.Context, blockID ids.ID) error
}

// Block represents a consensus block
type Block interface {
	// ID returns the block's unique identifier
	ID() ids.ID

	// Parent returns the parent block's ID
	Parent() ids.ID

	// Height returns the block's height
	Height() uint64

	// Bytes returns the serialized block
	Bytes() []byte

	// Accept marks the block as accepted
	Accept(context.Context) error

	// Reject marks the block as rejected
	Reject(context.Context) error
}
