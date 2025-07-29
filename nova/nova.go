// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nova

import (
	"context"
	"sync"
	
	"github.com/luxfi/ids"
)

// Nova represents the DAG consensus finalization stage
type Nova interface {
	// Finalize finalizes a DAG vertex
	Finalize(context.Context, ids.ID) error
	
	// IsFinalized checks if a vertex is finalized
	IsFinalized(ids.ID) bool
	
	// GetFinalized returns all finalized vertices
	GetFinalized() []ids.ID
	
	// RecordFinalization records a finalization event
	RecordFinalization(context.Context, ids.ID) error
	
	// HealthCheck returns the health status
	HealthCheck(context.Context) (interface{}, error)
}

// Parameters for Nova consensus
type Parameters struct {
	FinalizationDepth int
	MaxPending       int
}

// New creates a new Nova instance
func New(params Parameters) Nova {
	return &novaImpl{
		params:    params,
		finalized: make(map[ids.ID]bool),
		ordering:  []ids.ID{},
	}
}

type novaImpl struct {
	params    Parameters
	finalized map[ids.ID]bool
	ordering  []ids.ID
	mu        sync.RWMutex
}

func (n *novaImpl) Finalize(ctx context.Context, id ids.ID) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	// Only add if not already finalized
	if !n.finalized[id] {
		n.finalized[id] = true
		n.ordering = append(n.ordering, id)
	}
	return nil
}

func (n *novaImpl) IsFinalized(id ids.ID) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.finalized[id]
}

func (n *novaImpl) GetFinalized() []ids.ID {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	// Return a copy to avoid data races
	result := make([]ids.ID, len(n.ordering))
	copy(result, n.ordering)
	return result
}

func (n *novaImpl) RecordFinalization(ctx context.Context, id ids.ID) error {
	return n.Finalize(ctx, id)
}

func (n *novaImpl) HealthCheck(ctx context.Context) (interface{}, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	return map[string]interface{}{
		"healthy":   true,
		"finalized": len(n.finalized),
	}, nil
}