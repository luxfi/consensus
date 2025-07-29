// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package flare

import (
	"context"
	"fmt"
	"sync"
	"time"
	
	"github.com/luxfi/ids"
)

// Flare represents the rapid vertex ordering stage for DAG consensus
type Flare interface {
	// Add adds a new vertex to the DAG
	Add(context.Context, Tx) error
	
	// Order returns the current ordering of vertices
	Order() []ids.ID
	
	// RecordOrder records a new ordering decision
	RecordOrder(context.Context, []ids.ID) error
	
	// GetVertex returns a vertex by ID
	GetVertex(ids.ID) (Tx, error)
	
	// HealthCheck returns the health status
	HealthCheck(context.Context) (interface{}, error)
}

// Parameters for Flare consensus
type Parameters struct {
	MaxVertices      int
	OrderingInterval time.Duration
}

// New creates a new Flare instance
func New(params Parameters) Flare {
	return &flareImpl{
		params:   params,
		vertices: make(map[ids.ID]Tx),
		ordering: []ids.ID{},
	}
}

type flareImpl struct {
	mu       sync.RWMutex
	params   Parameters
	vertices map[ids.ID]Tx
	ordering []ids.ID
}

func (f *flareImpl) Add(ctx context.Context, tx Tx) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	f.vertices[tx.ID()] = tx
	return nil
}

func (f *flareImpl) Order() []ids.ID {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	return f.ordering
}

func (f *flareImpl) RecordOrder(ctx context.Context, order []ids.ID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	f.ordering = order
	return nil
}

func (f *flareImpl) GetVertex(id ids.ID) (Tx, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	tx, ok := f.vertices[id]
	if !ok {
		return nil, fmt.Errorf("vertex %s not found", id)
	}
	return tx, nil
}

func (f *flareImpl) HealthCheck(ctx context.Context) (interface{}, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	return map[string]interface{}{
		"healthy":  true,
		"vertices": len(f.vertices),
		"ordered":  len(f.ordering),
	}, nil
}