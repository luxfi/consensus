// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

// Package interfaces defines core consensus interfaces
package interfaces

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/vm"
)

// State is the VM lifecycle state, aliased from vm.State
type State = vm.State

// Re-export State constants from vm package
const (
	Unknown       = vm.Unknown
	Starting      = vm.Starting
	Syncing       = vm.Syncing
	Bootstrapping = vm.Bootstrapping
	Ready         = vm.Ready
	Degraded      = vm.Degraded
	Stopping      = vm.Stopping
	Stopped       = vm.Stopped
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
