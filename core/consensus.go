// Package core provides core consensus interfaces and contracts.
// This package has zero dependencies on consensus modules.
package core

import (
	"context"

	"github.com/luxfi/consensus/engine/interfaces"
	"github.com/luxfi/ids"
	"github.com/luxfi/vm"
)

// BlockState represents block storage for consensus
type BlockState interface {
	// GetBlock gets a block
	GetBlock(ids.ID) (Block, error)

	// PutBlock puts a block
	PutBlock(Block) error

	// GetLastAccepted gets last accepted
	GetLastAccepted() (ids.ID, error)

	// SetLastAccepted sets last accepted
	SetLastAccepted(ids.ID) error
}

// Block represents a block
type Block interface {
	ID() ids.ID
	ParentID() ids.ID
	Height() uint64
	Timestamp() int64
	Bytes() []byte
	Verify(context.Context) error
	Accept(context.Context) error
	Reject(context.Context) error
}

// Tx represents a transaction
type Tx interface {
	ID() ids.ID
	Bytes() []byte
	Verify(context.Context) error
	Accept(context.Context) error
}

// UTXO represents an unspent transaction output
type UTXO interface {
	ID() ids.ID
	TxID() ids.ID
	OutputIndex() uint32
	Amount() uint64
}

// Type aliases to vm package
type (
	Message     = vm.Message
	MessageType = vm.MessageType
	Fx          = vm.Fx
	FxLifecycle = vm.FxLifecycle
)

// Constants re-exported from vm package
const (
	PendingTxs    = vm.PendingTxs
	StateSyncDone = vm.StateSyncDone
)

// VM is an alias to engine/interfaces.VM for backwards compatibility
// Import from github.com/luxfi/consensus/engine/interfaces for the full VM interface
type VM = interfaces.VM

// AppError represents an application-level error
type AppError struct {
	Code    int
	Message string
}

func (e AppError) Error() string { return e.Message }

// State is the VM lifecycle state, re-exported from vm.State
type State = vm.State

// VMState is an alias for State (for compatibility with older code)
type VMState = vm.State

// Re-export state constants from vm package
const (
	Unknown       = vm.Unknown
	Starting      = vm.Starting
	Syncing       = vm.Syncing
	Bootstrapping = vm.Bootstrapping
	Ready         = vm.Ready
	Degraded      = vm.Degraded
	Stopping      = vm.Stopping
	Stopped       = vm.Stopped

	// Legacy aliases for VMState constants (old naming convention)
	VMStateSyncing  = vm.Syncing
	VMBootstrapping = vm.Bootstrapping
	VMNormalOp      = vm.Ready
)
