// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nova

import "github.com/luxfi/consensus/types"

// Finality manages DAG vertex finalization
type Finality interface {
	Decide(id types.VertexID) error
	Finalized(id types.VertexID) bool
	Reset()
}

// nova implements DAG finality
type nova struct {
	finalized map[types.VertexID]bool
}

// New creates a new Nova DAG finalizer
func New() Finality {
	return &nova{
		finalized: make(map[types.VertexID]bool),
	}
}

// Decide marks a vertex as finalized
func (n *nova) Decide(id types.VertexID) error {
	n.finalized[id] = true
	return nil
}

// Finalized checks if a vertex has been finalized
func (n *nova) Finalized(id types.VertexID) bool {
	return n.finalized[id]
}

// Reset clears all finalization state
func (n *nova) Reset() {
	n.finalized = make(map[types.VertexID]bool)
}
