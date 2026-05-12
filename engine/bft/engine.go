// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package bft is the sibling-engine adapter that lets the consensus
// toolkit expose `luxfi/bft` (epoch-based leader-rotation BFT, Simplex
// family) alongside Quasar's metastable engines (NewChain / NewDAG /
// NewPQ).
//
// The four engine factories cover four orthogonal consensus paradigms:
//
//	NewChain — linear Snow-family metastable (Nova)
//	NewDAG   — DAG Snow-family metastable (Nebula)
//	NewPQ    — post-quantum threshold Quasar
//	NewBFT   — classical leader-rotation BFT (this package)
//
// The Engine here satisfies github.com/luxfi/consensus/engine/interfaces.Engine
// by wrapping a *bft.Epoch and routing Start/Stop/HealthCheck/
// IsBootstrapped through the underlying epoch lifecycle.
//
// Orthogonal axes:
//
//	consensus paradigm       — picks engine (Chain/DAG/PQ/BFT)
//	ChainSecurityProfile     — picks admissible primitives (strict/permissive/fips)
//	FinalitySchemeID         — picks the threshold kernel (Pulsar/Corona/MPTC-MLDSA)
//
// A BFT engine under a strict profile with Pulsar-M-65 finality is a
// valid production combination: leader-rotation ordering with
// PQ-threshold-signed cert envelopes.
package bft

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/luxfi/bft"
)

// Engine is the consensus-toolkit-facing wrapper around a bft.Epoch.
// Constructed via NewEngine; the returned value satisfies
// github.com/luxfi/consensus/engine/interfaces.Engine.
//
// The wrapper keeps the BFT epoch encapsulated so callers do not need
// to know whether they hold a Chain/DAG/PQ/BFT engine — the engine
// surface is uniform.
type Engine struct {
	epoch        *bft.Epoch
	bootstrapped atomic.Bool
	stopCh       chan struct{}
	stopOnce     atomic.Bool
}

// NewEngine wraps a bft.Epoch as a consensus.Engine. The caller is
// responsible for constructing the Epoch with a fully-wired
// EpochConfig (Storage, Communication, Signer, BlockBuilder, WAL,
// etc.); this adapter takes ownership of its lifecycle.
//
// Returns nil if epoch is nil — every other call site assumes the
// embedded epoch is non-nil after construction.
func NewEngine(epoch *bft.Epoch) *Engine {
	if epoch == nil {
		return nil
	}
	return &Engine{
		epoch:  epoch,
		stopCh: make(chan struct{}),
	}
}

// ErrNilEpoch is returned by Start when the wrapper holds no Epoch.
// Should never happen in practice — NewEngine refuses nil — but
// callers that construct the struct literally must still handle it.
var ErrNilEpoch = errors.New("consensus/engine/bft: epoch is nil")

// Start launches the wrapped Epoch and marks the engine as
// bootstrapped. The requestID argument exists for engine-interface
// compatibility with the Quasar engines; BFT epochs do not consume
// it but the parameter is preserved so a caller can swap engines
// without changing call sites.
//
// One-way: Start may be called once. Subsequent Starts are no-ops.
func (e *Engine) Start(ctx context.Context, _ uint32) error {
	if e == nil || e.epoch == nil {
		return ErrNilEpoch
	}
	if e.bootstrapped.Load() {
		return nil
	}
	if err := e.epoch.Start(); err != nil {
		return err
	}
	e.bootstrapped.Store(true)
	return nil
}

// Stop halts the wrapped Epoch. Idempotent: subsequent Stops are
// no-ops. After Stop, IsBootstrapped continues to return true so the
// caller can distinguish "never started" from "started and stopped".
func (e *Engine) Stop(_ context.Context) error {
	if e == nil || e.epoch == nil {
		return nil
	}
	if !e.stopOnce.CompareAndSwap(false, true) {
		return nil
	}
	e.epoch.Stop()
	close(e.stopCh)
	return nil
}

// HealthCheck reports whether the wrapped Epoch is in a usable state.
// Returns a small map suitable for the engine-level health endpoint.
// The wrapper does not introspect the Epoch's internal round/wal
// state — bft does not expose that surface. A caller that needs
// deeper introspection should reach into the underlying bft.Epoch
// directly through Epoch().
func (e *Engine) HealthCheck(_ context.Context) (interface{}, error) {
	if e == nil || e.epoch == nil {
		return nil, ErrNilEpoch
	}
	stopped := e.stopOnce.Load()
	bootstrapped := e.bootstrapped.Load()
	return map[string]any{
		"engine":       "bft",
		"bootstrapped": bootstrapped,
		"stopped":      stopped,
	}, nil
}

// IsBootstrapped reports whether Start has completed at least once.
// Mirrors the contract of the Quasar engines: true means "the engine
// has finished its bootstrap phase and is now processing rounds."
func (e *Engine) IsBootstrapped() bool {
	if e == nil {
		return false
	}
	return e.bootstrapped.Load()
}

// Epoch returns the underlying bft.Epoch for callers that need to
// inject messages or advance time. The consensus.Engine surface is
// intentionally narrow; this accessor is the escape hatch.
//
// Returns nil if the wrapper was constructed without an Epoch.
func (e *Engine) Epoch() *bft.Epoch {
	if e == nil {
		return nil
	}
	return e.epoch
}

// AdvanceTime forwards the wall clock to the wrapped Epoch. Exposed
// at the wrapper layer because the engine.Engine interface itself
// does not have an AdvanceTime hook — clock-driven engines like BFT
// need this access point and Quasar engines don't.
func (e *Engine) AdvanceTime(t time.Time) {
	if e == nil || e.epoch == nil {
		return
	}
	e.epoch.AdvanceTime(t)
}

// HandleMessage forwards an incoming consensus message to the wrapped
// Epoch. Sibling to AdvanceTime: clock-driven engines surface this
// hook through the wrapper, sampling-driven Quasar engines don't.
func (e *Engine) HandleMessage(msg *bft.Message, from bft.NodeID) error {
	if e == nil || e.epoch == nil {
		return ErrNilEpoch
	}
	return e.epoch.HandleMessage(msg, from)
}
