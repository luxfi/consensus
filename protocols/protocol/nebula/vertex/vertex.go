// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"context"

	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/ids"
)

// Tx represents a transaction in the vertex
type Tx interface {
	// ID returns the unique identifier of this transaction
	ID() ids.ID
	
	// Bytes returns the binary representation
	Bytes() []byte
}

// Vertex represents a vertex in the DAG
type Vertex interface {
	interfaces.Decidable

	// Returns the vertices this vertex depends on
	Parents() ([]Vertex, error)

	// Returns the height of this vertex. A vertex's height is defined by one
	// greater than the maximum height of the parents.
	Height() (uint64, error)

	// Returns a series of state transitions to be performed on acceptance
	Txs(context.Context) ([]Tx, error)

	// Returns the binary representation of this vertex
	Bytes() []byte
}