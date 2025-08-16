// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package nova implements the linear consensus protocol
package nova

import (
	"context"
	"sync"
)

// Config configures the Nova linear consensus protocol
type Config[T comparable] struct {
	Sampler    interface{ Sample(k int) []T }
	Thresholds interface {
		Alpha(k int, phase uint64) (int, int)
	}
	Confidence interface {
		Record(bool) bool
		Reset()
	}
	// App/VM hooks
	Propose func(context.Context) (T, error)
	Apply   func(context.Context, T) error
	Send    func(context.Context, T, []T) error
}

// Protocol defines the Nova linear consensus interface
type Protocol[T comparable] interface {
	Step(ctx context.Context) error
	Finalized() bool
	Reset()
}

// Engine implements the Nova linear consensus protocol
type Engine[T comparable] struct {
	mu         sync.RWMutex
	cfg        Config[T]
	phase      uint64
	preference T
	finalized  bool
	lastVotes  map[T]int
}

// New creates a new Nova protocol engine
func New[T comparable](c Config[T]) (Protocol[T], error) {
	return &Engine[T]{
		cfg:       c,
		lastVotes: make(map[T]int),
	}, nil
}

// Step executes one consensus round
func (e *Engine[T]) Step(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.finalized {
		return nil
	}

	// Get sample
	k := 21 // Default K value
	sample := e.cfg.Sampler.Sample(k)

	// Get thresholds for this phase
	alphaPref, alphaConf := e.cfg.Thresholds.Alpha(k, e.phase)

	// Query sample
	votes := make(map[T]int)
	for range sample {
		// In real implementation, query node for their preference
		// For now, simulate with proposal
		if prop, err := e.cfg.Propose(ctx); err == nil {
			votes[prop]++
		}
	}

	// Find strongest preference
	var bestChoice T
	maxVotes := 0
	for choice, count := range votes {
		if count > maxVotes {
			bestChoice = choice
			maxVotes = count
		}
	}

	// Check preference threshold
	if maxVotes >= alphaPref {
		e.preference = bestChoice

		// Check confidence threshold
		if maxVotes >= alphaConf {
			// Record success
			if e.cfg.Confidence.Record(true) {
				// Finalized!
				e.finalized = true
				if e.cfg.Apply != nil {
					return e.cfg.Apply(ctx, e.preference)
				}
			}
		} else {
			// Not confident enough
			e.cfg.Confidence.Record(false)
		}
	} else {
		// No strong preference
		e.cfg.Confidence.Record(false)
	}

	e.phase++
	e.lastVotes = votes
	return nil
}

// Finalized returns whether consensus has been reached
func (e *Engine[T]) Finalized() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.finalized
}

// Reset resets the protocol state
func (e *Engine[T]) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.phase = 0
	e.finalized = false
	e.lastVotes = make(map[T]int)
	e.cfg.Confidence.Reset()
}
