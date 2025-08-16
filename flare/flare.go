// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package flare implements DAG ordering driver using Horizon order theory
package flare

import (
	"context"
	"sync"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/focus"
	"github.com/luxfi/consensus/horizon"
	"github.com/luxfi/consensus/prism"
	"github.com/luxfi/consensus/types"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/luxfi/consensus/utils/bag"
)

// Orderer returns vertices to deliver next in a stable topological slice
type Orderer interface {
	Schedule(ctx context.Context, frontier []types.VertexID) ([]types.VertexID, error)
}

// Engine implements DAG ordering using consensus primitives
type Engine struct {
	mu      sync.RWMutex
	sampler prism.Sampler
	wave    *wave.Wave
	counter focus.Counter
	cfg     config.Parameters

	// DAG state
	vertices map[types.VertexID]*Vertex
	ordered  []types.VertexID
	pending  map[types.VertexID]struct{}
}

// Vertex represents a vertex in the DAG
type Vertex struct {
	ID      types.VertexID
	Parents []types.VertexID
	Height  types.Height
	Data    []byte
}

// New creates a new DAG ordering engine
func New(sampler prism.Sampler, waveInstance *wave.Wave, counter focus.Counter, cfg config.Parameters) *Engine {
	return &Engine{
		sampler:  sampler,
		wave:     waveInstance,
		counter:  counter,
		cfg:      cfg,
		vertices: make(map[types.VertexID]*Vertex),
		pending:  make(map[types.VertexID]struct{}),
	}
}

// Schedule returns the next vertices to deliver in topological order
func (e *Engine) Schedule(ctx context.Context, frontier []types.VertexID) ([]types.VertexID, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Create graph adapter for horizon
	graph := &dagGraph{vertices: e.vertices}

	// Find antichain in frontier
	antichain := horizon.Antichain(graph, frontier)
	if len(antichain) == 0 {
		return nil, nil
	}

	// Order vertices by height and consensus
	scheduled := make([]types.VertexID, 0, len(antichain))
	for _, v := range antichain {
		// Check if already ordered
		if e.isOrdered(v) {
			continue
		}

		// Check if parents are ordered
		if !e.parentsOrdered(v) {
			e.pending[v] = struct{}{}
			continue
		}

		// Run consensus on this vertex
		voteBag := bag.Of[ids.ID]()
		// Simulate a poll - in real implementation this would query validators
		voteBag.Add(ids.ID(v))
		
		// Add the choice if not already added
		e.wave.Add(ids.ID(v))
		
		// Record the votes
		e.wave.RecordVotes(voteBag)
		
		if e.wave.Finalized() {
			// Update confidence counter
			e.counter.Tick(true)

			// Check if finalized
			if e.counter.Finalized(e.cfg.Beta) {
				scheduled = append(scheduled, v)
				e.ordered = append(e.ordered, v)
				delete(e.pending, v)
			}
		} else {
			e.counter.Tick(false)
		}
	}

	return scheduled, nil
}

// AddVertex adds a new vertex to the DAG
func (e *Engine) AddVertex(v *Vertex) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.vertices[v.ID] = v
	e.pending[v.ID] = struct{}{}
}

// GetOrdered returns all ordered vertices
func (e *Engine) GetOrdered() []types.VertexID {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]types.VertexID, len(e.ordered))
	copy(result, e.ordered)
	return result
}

// isOrdered checks if a vertex has been ordered
func (e *Engine) isOrdered(v types.VertexID) bool {
	for _, ordered := range e.ordered {
		if ordered == v {
			return true
		}
	}
	return false
}

// parentsOrdered checks if all parents of a vertex have been ordered
func (e *Engine) parentsOrdered(v types.VertexID) bool {
	vertex, ok := e.vertices[v]
	if !ok {
		return false
	}

	for _, parent := range vertex.Parents {
		if !e.isOrdered(parent) {
			return false
		}
	}
	return true
}

// dagGraph implements horizon.Graph interface
type dagGraph struct {
	vertices map[types.VertexID]*Vertex
}

func (g *dagGraph) Parents(v types.VertexID) []types.VertexID {
	if vertex, ok := g.vertices[v]; ok {
		return vertex.Parents
	}
	return nil
}
