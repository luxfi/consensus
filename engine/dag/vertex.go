// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package dag

import (
	"context"
	"fmt"
	"sync"

	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/ids"
)

// Vertex represents a vertex in the DAG
type Vertex struct {
	id        ids.ID
	parentIDs []ids.ID
	height    uint64
	timestamp int64
	data      []byte

	// Consensus state - using Lux consensus with Prism DAG protocol
	mu           sync.RWMutex
	luxConsensus *engine.LuxConsensus
	accepted     bool
	rejected     bool
	processing   bool

	// Dependencies tracking
	parents  []*Vertex
	children []*Vertex
}

// NewVertex creates a new vertex
func NewVertex(id ids.ID, parentIDs []ids.ID, height uint64, timestamp int64, data []byte) *Vertex {
	return &Vertex{
		id:         id,
		parentIDs:  parentIDs,
		height:     height,
		timestamp:  timestamp,
		data:       data,
		accepted:   false,
		rejected:   false,
		processing: false,
		parents:    make([]*Vertex, 0),
		children:   make([]*Vertex, 0),
	}
}

// ID returns the vertex ID
func (v *Vertex) ID() ids.ID {
	return v.id
}

// Parent returns the first parent ID (for interface compatibility)
func (v *Vertex) Parent() ids.ID {
	if len(v.parentIDs) > 0 {
		return v.parentIDs[0]
	}
	return ids.Empty
}

// ParentIDs returns all parent IDs
func (v *Vertex) ParentIDs() []ids.ID {
	return v.parentIDs
}

// Height returns the vertex height
func (v *Vertex) Height() uint64 {
	return v.height
}

// Bytes returns the vertex data
func (v *Vertex) Bytes() []byte {
	return v.data
}

// Verify verifies the vertex
func (v *Vertex) Verify(ctx context.Context) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.id == ids.Empty {
		return fmt.Errorf("invalid vertex ID")
	}

	// Verify all parents exist and are valid
	for _, parentID := range v.parentIDs {
		if parentID == ids.Empty {
			return fmt.Errorf("invalid parent ID")
		}
	}

	return nil
}

// Accept accepts the vertex
func (v *Vertex) Accept(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.rejected {
		return fmt.Errorf("vertex already rejected: %s", v.id)
	}

	v.accepted = true
	v.processing = false

	return nil
}

// Reject rejects the vertex
func (v *Vertex) Reject(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.accepted {
		return fmt.Errorf("vertex already accepted: %s", v.id)
	}

	v.rejected = true
	v.processing = false

	return nil
}

// IsAccepted returns whether the vertex is accepted
func (v *Vertex) IsAccepted() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.accepted
}

// IsRejected returns whether the vertex is rejected
func (v *Vertex) IsRejected() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.rejected
}

// IsProcessing returns whether the vertex is being processed
func (v *Vertex) IsProcessing() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.processing
}

// SetProcessing sets the processing state
func (v *Vertex) SetProcessing(processing bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.processing = processing
}

// AddChild adds a child vertex
func (v *Vertex) AddChild(child *Vertex) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.children = append(v.children, child)
}

// AddParent adds a parent vertex
func (v *Vertex) AddParent(parent *Vertex) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.parents = append(v.parents, parent)
}

// Parents returns all parent vertices
func (v *Vertex) Parents() []*Vertex {
	v.mu.RLock()
	defer v.mu.RUnlock()
	result := make([]*Vertex, len(v.parents))
	copy(result, v.parents)
	return result
}

// Children returns all child vertices
func (v *Vertex) Children() []*Vertex {
	v.mu.RLock()
	defer v.mu.RUnlock()
	result := make([]*Vertex, len(v.children))
	copy(result, v.children)
	return result
}

// SetLuxConsensus sets the Lux consensus instance for this vertex
func (v *Vertex) SetLuxConsensus(lc *engine.LuxConsensus) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.luxConsensus = lc
}

// LuxConsensus returns the Lux consensus instance
func (v *Vertex) LuxConsensus() *engine.LuxConsensus {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.luxConsensus
}
