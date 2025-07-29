// Copyright (C) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

import (
	"context"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/consensus/flare"
)

// Vertex represents a vertex in the DAG
type Vertex interface {
	choices.Decidable

	// Returns the vertices this vertex depends on
	Parents() ([]Vertex, error)

	// Returns the height of this vertex. A vertex's height is defined by one
	// greater than the maximum height of the parents.
	Height() (uint64, error)

	// Returns a series of state transitions to be performed on acceptance
	Txs(context.Context) ([]flare.Tx, error)

	// Returns the binary representation of this vertex
	Bytes() []byte
}