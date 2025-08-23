// Package core provides core consensus interfaces and contracts.
// This package has zero dependencies on consensus modules.
package core

import (
	"context"
	"github.com/luxfi/ids"
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
