// Package core provides core consensus interfaces and contracts.
// This package has zero dependencies on consensus modules.
package core

import (
	"context"

	"github.com/luxfi/consensus/engine/interfaces"
	"github.com/luxfi/ids"
	"github.com/luxfi/vm"
)

// State represents consensus state
type State interface {
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

// Message and MessageType aliases to vm package for backwards compatibility
type (
	Message     = vm.Message
	MessageType = vm.MessageType
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

// VMState represents the state of the VM
type VMState uint32

const (
	// VMInitializing means the VM is still initializing
	VMInitializing VMState = iota
	// VMBootstrapping means the VM is bootstrapping
	VMBootstrapping
	// VMNormalOp means the VM is in normal operation
	VMNormalOp
)
