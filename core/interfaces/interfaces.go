// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

// Package interfaces defines core consensus interfaces
package interfaces

import (
	"context"

	"github.com/luxfi/ids"
)

// State represents the operational state of a VM or consensus engine.
type State uint8

// State constants for VM lifecycle
const (
	Unknown       State = iota // Unknown state
	Starting                   // VM is starting up
	Syncing                    // VM is syncing state
	Bootstrapping              // VM is bootstrapping
	Ready                      // VM is ready for normal operation
	Degraded                   // VM is degraded but operational
	Stopping                   // VM is shutting down
	Stopped                    // VM has stopped
)

// ChainState represents the state of the consensus chain
type ChainState interface {
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

// VM defines the common VM interface for consensus engines
type VM interface {
	// Shutdown is called when the node is shutting down
	Shutdown(context.Context) error
}
