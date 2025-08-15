// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"context"
	"fmt"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/protocol/quasar"
)

// Runtime implements the Quasar runtime with PQ overlay
type Runtime struct {
	engine *quasar.Engine
	ctx    *interfaces.Runtime
	params config.Parameters
}

// New creates a new Quasar runtime
func New(ctx *interfaces.Runtime, params config.Parameters) (*Runtime, error) {
	// Create quasar engine
	qParams := quasar.Parameters{
		K:               params.K,
		AlphaPreference: params.AlphaPreference,
		AlphaConfidence: params.AlphaConfidence,
		Beta:            int(params.Beta),
		Mode:            quasar.HybridMode,
		SecurityLevel:   quasar.SecurityMedium,
	}
	
	engine, err := quasar.New(ctx, qParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create quasar engine: %w", err)
	}
	
	return &Runtime{
		engine: engine,
		ctx:    ctx,
		params: params,
	}, nil
}

// Start starts the runtime
func (r *Runtime) Start(ctx context.Context) error {
	// Initialize engine
	if err := r.engine.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize engine: %w", err)
	}
	
	// Start engine
	if err := r.engine.Start(ctx); err != nil {
		return fmt.Errorf("failed to start engine: %w", err)
	}
	
	// Wait for context cancellation
	<-ctx.Done()
	return ctx.Err()
}

// Stop stops the runtime
func (r *Runtime) Stop(ctx context.Context) error {
	return r.engine.Stop(ctx)
}

// Engine returns the underlying quasar engine
func (r *Runtime) Engine() *quasar.Engine {
	return r.engine
}