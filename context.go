// Package consensus provides the core consensus protocols
package consensus

import (
	"context"

	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
)

// Export core types
type (
	// Context is now defined in ctx.go as alias to context.Context
	State       = interfaces.State
	Status      = interfaces.Status
	StateHolder = interfaces.StateHolder
	Block       = interfaces.Decidable // Blocks are decidable items
)

// Export constants
const (
	Bootstrapping = interfaces.Bootstrapping
	NormalOp      = interfaces.NormalOp

	Unknown    = interfaces.Unknown
	Processing = interfaces.Processing
	Rejected   = interfaces.Rejected
	Accepted   = interfaces.Accepted
)

// ExtendedContext provides heavy dependencies for consensus engines
// These are kept separate from context to avoid service locator pattern
type ExtendedContext struct {
	ChainDataDir   string
	SharedMemory   database.Database
	BCLookup       AliasLookup
	ValidatorState ValidatorState
	WarpSigner     WarpSigner
}

// AliasLookup provides chain alias lookups
type AliasLookup interface {
	PrimaryAlias(id ids.ID) (string, error)
}

// ValidatorState provides validator information
type ValidatorState interface {
	GetCurrentHeight() (uint64, error)
	GetValidatorSet(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error)
	GetSubnetID(chainID ids.ID) (ids.ID, error)
}

// WarpSigner provides BLS signing for warp messages
type WarpSigner interface {
	Sign(msg []byte) ([]byte, error)
}

// Message types for consensus engine communication
type Message interface{}

type PendingTxs struct{}

// VM defines the virtual machine interface for consensus
type VM interface {
	// ParseBlock parses a block from bytes
	ParseBlock(ctx context.Context, blockBytes []byte) (Block, error)
	// GetBlock retrieves a block by ID
	GetBlock(ctx context.Context, blockID ids.ID) (Block, error)
	// SetPreference sets the preferred block
	SetPreference(ctx context.Context, blockID ids.ID) error
	// LastAccepted returns the last accepted block ID
	LastAccepted(ctx context.Context) (ids.ID, error)
	// HealthCheck returns the health status
	HealthCheck(ctx context.Context) (interface{}, error)
	// Shutdown stops the VM
	Shutdown(ctx context.Context) error
}

// Vertex represents a vertex in a DAG consensus
type Vertex interface {
	// ID returns the vertex ID
	ID() ids.ID
	// Parents returns the parent vertex IDs
	Parents() []ids.ID
	// Height returns the vertex height
	Height() uint64
	// Bytes returns the vertex bytes
	Bytes() []byte
	// Accept accepts the vertex
	Accept(ctx context.Context) error
	// Reject rejects the vertex
	Reject(ctx context.Context) error
	// Verify verifies the vertex
	Verify(ctx context.Context) error
}
