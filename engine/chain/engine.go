// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
    "context"
    "github.com/luxfi/consensus/core/interfaces"
    "github.com/luxfi/consensus/chain"
    "github.com/luxfi/consensus/engine/chain/getter"
    "github.com/luxfi/consensus/engine/chain/block"
    "github.com/luxfi/consensus/networking/sender"
    "github.com/luxfi/trace"
)

// Config configures the chain engine
type Config struct {
    Ctx                 *interfaces.Runtime
    AllGetsServer       getter.Handler
    VM                  block.ChainVM
    Sender              sender.Sender
    Validators          interface{}
    ConnectedValidators interface{}
    Params              Parameters
    Consensus           chain.Consensus
}

// Parameters for the engine
type Parameters struct {
    // Engine parameters
}

// Engine is a chain consensus engine
type Engine interface {
    // Start the engine
    Start(ctx context.Context) error
    // Stop the engine
    Stop() error
}

// New creates a new chain engine
func New(ctx *interfaces.Runtime, params Parameters) (Engine, error) {
    return &noOpEngine{}, nil
}

// TraceEngine wraps an engine with tracing
func TraceEngine(engine Engine, tracer trace.Tracer) Engine {
    return engine
}

// Halter can halt operations
type Halter interface {
    Halt(context.Context)
}

type noOpEngine struct{}

func (n *noOpEngine) Start(ctx context.Context) error {
    return nil
}

func (n *noOpEngine) Stop() error {
    return nil
}