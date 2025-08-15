// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package dag implements the DAG consensus engine with FPC enabled by default
package dag

import (
	"context"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/flare"
	"github.com/luxfi/consensus/focus"
	// "github.com/luxfi/consensus/horizon" // TODO: Use for ordering
	"github.com/luxfi/consensus/protocol/nebula"
	"github.com/luxfi/consensus/wave"
	"github.com/luxfi/consensus/wave/fpc"
)

// Parents returns the direct parents of a vertex.
type Parents[V comparable] func(V) []V

// Sampler is the base K-sampler used by the DAG engine
// (e.g., to sample peers; DAG-aware selection happens inside flare/nebula via prism+horizon).
type Sampler[V comparable] interface {
	Sample(k int) []V
}

// Hooks bind the DAG engine to your VM/network.
type Hooks[V comparable] struct {
	Tips    func() []V           // current DAG tips/frontier candidates
	Parents Parents[V]           // graph access
	Sampler Sampler[V]           // K-sampling source
	Rand    func(uint64) float64 // PRF for FPC
	
	// Add app/VM/network callbacks as needed.
	Propose func(context.Context) (V, error)
	Apply   func(context.Context, []V) error
	Send    func(context.Context, V, []V) error
}

// Engine composes DAG geometry (horizon+flare) with photonic rounds and nebula finality.
type Engine[V comparable] struct {
	cfg   config.Parameters
	sel   wave.Selector
	conf  focus.Confidence
	g     graph[V]        // horizon.Graph wrapper
	ord   flare.Orderer   // scheduler/ordering driver
	proto nebula.Protocol[V]
	fpc   *fpc.Engine
}

// New creates a new DAG engine with FPC enabled by default
func New[V comparable](cfg config.Parameters, hooks Hooks[V]) (*Engine[V], error) {
	if hooks.Rand == nil {
		hooks.Rand = func(uint64) float64 { return 0.5 }
	}
	
	// Create FPC selector
	selector := &fpc.Selector{
		ThetaMin: cfg.FPC.ThetaMin,
		ThetaMax: cfg.FPC.ThetaMax,
		Rand:     hooks.Rand,
	}
	
	// Create confidence counter
	conf := focus.New(cfg.Beta)

	// Create graph adapter
	g := graph[V]{parents: hooks.Parents}

	// Create orderer
	// TODO: Create proper sampler, round, and counter for flare
	// ord := flare.New(sampler, round, counter, cfg)

	// Create FPC engine if enabled
	var fpcEngine *fpc.Engine
	if cfg.FPC.Enable {
		fpcEngine = fpc.NewEngine(cfg.FPC)
	}

	// Create Nebula protocol
	p, err := nebula.New(nebula.Config[V]{
		Graph:      g,
		Tips:       hooks.Tips,
		Thresholds: selector,
		Confidence: conf,
		// Orderer:    ord, // TODO: Add orderer when created
		Propose:    hooks.Propose,
		Apply:      hooks.Apply,
		Send:       hooks.Send,
	})
	if err != nil {
		return nil, err
	}
	
	return &Engine[V]{
		cfg:   cfg,
		sel:   selector,
		conf:  conf,
		g:     g,
		// ord:   ord, // TODO: Add when orderer is created
		proto: p,
		fpc:   fpcEngine,
	}, nil
}

// Step executes one DAG consensus round
func (e *Engine[V]) Step(ctx context.Context) error {
	// Execute FPC voting if enabled
	if e.fpc != nil {
		if err := e.fpc.ProcessVotes(ctx); err != nil {
			// Log but don't fail consensus
			_ = err
		}
	}
	
	return e.proto.Step(ctx)
}

// Finalized returns finalized vertices
func (e *Engine[V]) Finalized() []V {
	return e.proto.Finalized()
}

// Reset resets the engine state
func (e *Engine[V]) Reset() {
	e.conf.Reset()
	e.proto.Reset()
	if e.fpc != nil {
		e.fpc.Reset()
	}
}

// OnVertexObserved is called when a new vertex is observed (for FPC)
func (e *Engine[V]) OnVertexObserved(ctx context.Context, vtx any) error {
	if e.fpc == nil {
		return nil
	}
	// Extract FPC votes from vertex if present
	return e.fpc.OnBlockObserved(ctx, vtx)
}

// OnVertexAccepted is called when a vertex is accepted (for FPC)
func (e *Engine[V]) OnVertexAccepted(ctx context.Context, vtx any) error {
	if e.fpc == nil {
		return nil
	}
	return e.fpc.OnBlockAccepted(ctx, vtx)
}

// GetFPCVotes returns the next FPC votes to include in a vertex
func (e *Engine[V]) GetFPCVotes(ctx context.Context, budget int) ([][]byte, error) {
	if e.fpc == nil {
		return nil, nil
	}
	
	votes := e.fpc.NextVotes(ctx, budget)
	result := make([][]byte, len(votes))
	for i, v := range votes {
		result[i] = v[:]
	}
	return result, nil
}

// graph adapts callbacks to horizon.Graph[V].
type graph[V comparable] struct {
	parents Parents[V]
}

func (g graph[V]) Parents(v V) []V {
	return g.parents(v)
}