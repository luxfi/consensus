// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package nebula implements the DAG finality protocol
package nebula

import (
	"context"
	"sync"
)

// Config configures the Nebula DAG finality protocol
type Config[V comparable] struct {
	Graph      interface{ Parents(V) []V }
	Tips       func() []V
	Thresholds interface {
		Alpha(k int, phase uint64) (int, int)
	}
	Confidence interface {
		Record(bool) bool
		Reset()
	}
	Orderer interface {
		Schedule(context.Context, []V) ([]V, error)
	}
	// App/VM hooks
	Propose func(context.Context) (V, error)
	Apply   func(context.Context, []V) error
	Send    func(context.Context, V, []V) error
}

// Protocol defines the Nebula DAG finality interface
type Protocol[V comparable] interface {
	Step(ctx context.Context) error
	Finalized() []V
	Reset()
}

// Engine implements the Nebula DAG finality protocol
type Engine[V comparable] struct {
	mu        sync.RWMutex
	cfg       Config[V]
	phase     uint64
	finalized map[V]bool
	pending   []V
	frontier  []V
}

// New creates a new Nebula protocol engine
func New[V comparable](c Config[V]) (Protocol[V], error) {
	return &Engine[V]{
		cfg:       c,
		finalized: make(map[V]bool),
	}, nil
}

// Step executes one DAG consensus round
func (e *Engine[V]) Step(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Get current tips/frontier
	tips := e.cfg.Tips()
	if len(tips) == 0 {
		return nil
	}

	// Schedule vertices for ordering
	scheduled, err := e.cfg.Orderer.Schedule(ctx, tips)
	if err != nil {
		return err
	}

	// Process scheduled vertices
	for _, v := range scheduled {
		if e.finalized[v] {
			continue
		}

		// Check if all parents are finalized
		parents := e.cfg.Graph.Parents(v)
		allParentsFinalized := true
		for _, p := range parents {
			if !e.finalized[p] {
				allParentsFinalized = false
				break
			}
		}

		if !allParentsFinalized {
			e.pending = append(e.pending, v)
			continue
		}

		// Run consensus on this vertex
		// In real implementation, this would involve sampling and voting
		// For now, simulate success
		if e.cfg.Confidence.Record(true) {
			e.finalized[v] = true

			// Apply finalized vertices
			if e.cfg.Apply != nil {
				if err := e.cfg.Apply(ctx, []V{v}); err != nil {
					return err
				}
			}
		}
	}

	// Update frontier
	e.frontier = tips
	e.phase++

	return nil
}

// Finalized returns all finalized vertices
func (e *Engine[V]) Finalized() []V {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]V, 0, len(e.finalized))
	for v := range e.finalized {
		result = append(result, v)
	}
	return result
}

// Reset resets the protocol state
func (e *Engine[V]) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.phase = 0
	e.finalized = make(map[V]bool)
	e.pending = nil
	e.frontier = nil
	if e.cfg.Confidence != nil {
		e.cfg.Confidence.Reset()
	}
}
