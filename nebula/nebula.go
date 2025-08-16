// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package nebula implements DAG finality using the Flare ordering driver
package nebula

import (
	"context"
	"sync"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/flare"
	"github.com/luxfi/consensus/types"
)

// Finality tracks finalized vertices in the DAG
type Finality interface {
	OnDecided(ctx context.Context, v types.VertexID)
	Final(ctx context.Context, v types.VertexID) bool
}

// Engine implements DAG finality tracking
type Engine struct {
	mu        sync.RWMutex
	orderer   flare.Orderer
	finalized map[types.VertexID]bool
	pending   map[types.VertexID]struct{}
	cfg       config.Parameters
}

// New creates a new DAG finality engine
func New(orderer flare.Orderer, cfg config.Parameters) *Engine {
	return &Engine{
		orderer:   orderer,
		finalized: make(map[types.VertexID]bool),
		pending:   make(map[types.VertexID]struct{}),
		cfg:       cfg,
	}
}

// OnDecided marks a vertex as decided
func (e *Engine) OnDecided(ctx context.Context, v types.VertexID) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Mark as finalized
	e.finalized[v] = true
	delete(e.pending, v)
}

// Final checks if a vertex is finalized
func (e *Engine) Final(ctx context.Context, v types.VertexID) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.finalized[v]
}

// ProcessFrontier processes a new frontier of vertices
func (e *Engine) ProcessFrontier(ctx context.Context, frontier []types.VertexID) ([]types.VertexID, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Get ordered vertices from flare
	ordered, err := e.orderer.Schedule(ctx, frontier)
	if err != nil {
		return nil, err
	}

	// Track pending vertices
	for _, v := range ordered {
		if !e.finalized[v] {
			e.pending[v] = struct{}{}
		}
	}

	return ordered, nil
}

// GetFinalized returns all finalized vertices
func (e *Engine) GetFinalized() []types.VertexID {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]types.VertexID, 0, len(e.finalized))
	for v := range e.finalized {
		result = append(result, v)
	}
	return result
}

// GetPending returns vertices awaiting finalization
func (e *Engine) GetPending() []types.VertexID {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]types.VertexID, 0, len(e.pending))
	for v := range e.pending {
		result = append(result, v)
	}
	return result
}
