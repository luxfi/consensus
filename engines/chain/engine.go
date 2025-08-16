// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package chain implements the linear consensus engine with FPC enabled by default
package chain

import (
	"context"
	"errors"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/focus"
	"github.com/luxfi/consensus/protocol/nova"
	"github.com/luxfi/consensus/wave"
	"github.com/luxfi/consensus/wave/fpc"
)

// Sampler is the minimal K-sampler the engine expects.
// Keep it generic so this engine never depends on DAG geometry.
type Sampler[T comparable] interface {
	Sample(k int) []T
}

// Hooks lets you inject app/VM/network specifics without polluting the engine.
type Hooks[T comparable] struct {
	Sampler Sampler[T]                 // source of K samples per poll (peers / candidates)
	Rand    func(phase uint64) float64 // PRF for FPC threshold draws; must be deterministic per phase

	// Add optional app callbacks here (build proposal, apply block, gossip, etc.)
	Propose func(context.Context) (T, error)
	Apply   func(context.Context, T) error
	Send    func(context.Context, T, []T) error
}

// Engine wires the photonic primitives into the linear protocol.
type Engine[T comparable] struct {
	cfg   config.Parameters
	sel   wave.Selector
	conf  focus.Confidence
	proto nova.Protocol[T]
	fpc   *fpc.Engine
}

// New composes photon→wave(FPC)→focus into protocol/nova.
// FPC is enabled by cfg.FPC.Enable (default true).
func New[T comparable](cfg config.Parameters, hooks Hooks[T]) (*Engine[T], error) {
	if hooks.Rand == nil {
		hooks.Rand = func(uint64) float64 { return 0.5 } // safe default; replace in prod
	}

	// Create FPC selector with thresholds
	selector := &fpc.Selector{
		ThetaMin: cfg.FPC.ThetaMin,
		ThetaMax: cfg.FPC.ThetaMax,
		Rand:     hooks.Rand,
	}

	// Create confidence counter
	conf := focus.New(cfg.Beta)

	// Create FPC engine if enabled
	var fpcEngine *fpc.Engine
	if cfg.FPC.Enable {
		fpcEngine = fpc.NewEngine(cfg.FPC)
	}

	// Create Nova protocol
	p, err := nova.New(nova.Config[T]{
		Sampler:    hooks.Sampler, // K-sampling
		Thresholds: selector,      // α_pref/α_conf per round
		Confidence: conf,          // β consecutive
		Propose:    hooks.Propose,
		Apply:      hooks.Apply,
		Send:       hooks.Send,
	})
	if err != nil {
		return nil, err
	}

	return &Engine[T]{
		cfg:   cfg,
		sel:   selector,
		conf:  conf,
		proto: p,
		fpc:   fpcEngine,
	}, nil
}

// Step executes one consensus round/poll.
// You may drive this from a timer (Δ) or on demand.
func (e *Engine[T]) Step(ctx context.Context) error {
	// Execute FPC voting if enabled
	if e.fpc != nil {
		if err := e.fpc.ProcessVotes(ctx); err != nil {
			// Log but don't fail consensus
			_ = err
		}
	}

	// Execute main consensus step
	return e.proto.Step(ctx)
}

// Finalized returns whether consensus has been reached
func (e *Engine[T]) Finalized() bool {
	return e.proto.Finalized()
}

// Reset resets the engine state
func (e *Engine[T]) Reset() {
	e.conf.Reset()
	e.proto.Reset()
	if e.fpc != nil {
		e.fpc.Reset()
	}
}

// OnBlockObserved is called when a new block is observed (for FPC)
func (e *Engine[T]) OnBlockObserved(ctx context.Context, blk any) error {
	if e.fpc == nil {
		return nil
	}
	return e.fpc.OnBlockObserved(ctx, blk)
}

// OnBlockAccepted is called when a block is accepted (for FPC)
func (e *Engine[T]) OnBlockAccepted(ctx context.Context, blk any) error {
	if e.fpc == nil {
		return nil
	}
	return e.fpc.OnBlockAccepted(ctx, blk)
}

// GetFPCVotes returns the next FPC votes to include in a block
func (e *Engine[T]) GetFPCVotes(ctx context.Context, budget int) ([][]byte, error) {
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

// ValidateEngine validates the engine configuration
func ValidateEngine[T comparable](cfg config.Parameters) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	if cfg.FPC.Enable && cfg.FPC.VoteLimitPerBlock <= 0 {
		return errors.New("FPC enabled but VoteLimitPerBlock not set")
	}

	return nil
}
