// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quantum

import (
	"context"
	"sync"
	"time"

	"github.com/luxfi/ids"
)

// Vertex represents a DAG vertex in the Nova layer.
type Vertex interface {
	// ID returns the unique identifier of this vertex.
	ID() ids.ID

	// Parents returns the parent vertices.
	Parents() []ids.ID

	// Height returns the height of this vertex.
	Height() uint64

	// Timestamp returns when this vertex was created.
	Timestamp() time.Time

	// Transactions returns the transactions in this vertex.
	Transactions() []ids.ID

	// Verify verifies the vertex is valid.
	Verify(ctx context.Context) error

	// Bytes returns the serialized vertex.
	Bytes() []byte
}

// VertexConfidence tracks confidence for a vertex in the Nova DAG.
type VertexConfidence struct {
	mu         sync.RWMutex
	vertex     Vertex
	confidence int      // d(T) - confidence counter
	chits      int      // Number of positive votes
	preferred  bool     // Whether this vertex is preferred
	decided    bool     // Whether this vertex is decided
	timestamp  time.Time
}

// NewVertexConfidence creates a new vertex confidence tracker.
func NewVertexConfidence(vertex Vertex) *VertexConfidence {
	return &VertexConfidence{
		vertex:    vertex,
		timestamp: time.Now(),
	}
}

// AddChit adds a positive vote (chit) to this vertex.
func (vc *VertexConfidence) AddChit() {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.chits++
}

// UpdateConfidence updates the confidence counter based on k-peer sampling.
func (vc *VertexConfidence) UpdateConfidence(delta int) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.confidence += delta
}

// GetConfidence returns the current confidence level.
func (vc *VertexConfidence) GetConfidence() int {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.confidence
}

// SetPreferred marks this vertex as preferred once confidence crosses β₁ or β₂.
func (vc *VertexConfidence) SetPreferred() {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.preferred = true
}

// IsPreferred returns whether this vertex is preferred.
func (vc *VertexConfidence) IsPreferred() bool {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.preferred
}

// SetDecided marks this vertex as decided.
func (vc *VertexConfidence) SetDecided() {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.decided = true
}

// IsDecided returns whether this vertex is decided.
func (vc *VertexConfidence) IsDecided() bool {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.decided
}

// Frontier represents the current frontier of the DAG.
type Frontier struct {
	mu       sync.RWMutex
	vertices map[ids.ID]*VertexConfidence
	frontier []ids.ID // Current frontier vertex IDs
}

// NewFrontier creates a new DAG frontier.
func NewFrontier() *Frontier {
	return &Frontier{
		vertices: make(map[ids.ID]*VertexConfidence),
		frontier: make([]ids.ID, 0),
	}
}

// Add adds a vertex to the frontier.
func (f *Frontier) Add(vertex Vertex) *VertexConfidence {
	f.mu.Lock()
	defer f.mu.Unlock()

	vc := NewVertexConfidence(vertex)
	f.vertices[vertex.ID()] = vc
	f.updateFrontier()
	return vc
}

// Get retrieves a vertex confidence by ID.
func (f *Frontier) Get(id ids.ID) (*VertexConfidence, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	vc, ok := f.vertices[id]
	return vc, ok
}

// GetHighestConfidence returns the frontier vertex with highest confidence.
func (f *Frontier) GetHighestConfidence() (*VertexConfidence, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var best *VertexConfidence
	maxConfidence := -1

	for _, id := range f.frontier {
		if vc, ok := f.vertices[id]; ok {
			conf := vc.GetConfidence()
			if conf > maxConfidence {
				maxConfidence = conf
				best = vc
			}
		}
	}

	return best, best != nil
}

// updateFrontier updates the current frontier based on parent relationships.
func (f *Frontier) updateFrontier() {
	// Implementation: maintain vertices with no unprocessed children
	newFrontier := make([]ids.ID, 0)
	
	// Simple implementation: vertices that aren't parents of any undecided vertex
	childMap := make(map[ids.ID]bool)
	for _, vc := range f.vertices {
		if !vc.IsDecided() {
			for _, parent := range vc.vertex.Parents() {
				childMap[parent] = true
			}
		}
	}

	for id, vc := range f.vertices {
		if !vc.IsDecided() && !childMap[id] {
			newFrontier = append(newFrontier, id)
		}
	}

	f.frontier = newFrontier
}