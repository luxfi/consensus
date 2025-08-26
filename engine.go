// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

import (
	"context"

	"github.com/luxfi/ids"
)

// Engine represents a consensus engine
type Engine interface {
	// GetID returns the engine's chain ID
	GetID() ids.ID

	// Start starts the consensus engine
	Start(ctx context.Context) error

	// Stop stops the consensus engine
	Stop() error

	// Notify notifies the engine of an event
	Notify(msg Message) error
}

// Message represents a consensus message
type Message interface {
	// Type returns the message type
	Type() MessageType

	// Bytes returns the message bytes
	Bytes() []byte
}

// MessageType represents types of consensus messages
type MessageType uint32

const (
	// PutMsg requests a block
	PutMsg MessageType = iota
	// GetMsg responds with a block
	GetMsg
	// PushQueryMsg queries for block acceptance
	PushQueryMsg
	// PullQueryMsg queries for preferred block
	PullQueryMsg
	// ChitsMsg votes for blocks
	ChitsMsg
)

// QuantumEngine extends Engine with quantum features
type QuantumEngine interface {
	Engine

	// EnableQuantumMode enables quantum-resistant features
	EnableQuantumMode(enabled bool)

	// EnableBLSAggregation enables BLS signature aggregation
	EnableBLSAggregation(enabled bool)

	// EnableVerkleWitnesses enables Verkle witnesses
	EnableVerkleWitnesses(enabled bool)
}

// Block represents a consensus block
type Block interface {
	// ID returns the block ID
	ID() ids.ID

	// ParentID returns the parent block ID
	ParentID() ids.ID

	// Height returns the block height
	Height() uint64

	// Timestamp returns the block timestamp
	Timestamp() int64

	// Verify verifies the block
	Verify(ctx context.Context) error

	// Accept accepts the block
	Accept(ctx context.Context) error

	// Reject rejects the block
	Reject(ctx context.Context) error

	// Bytes returns the block bytes
	Bytes() []byte
}
