// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"context"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engines/dag"
	"github.com/luxfi/consensus/engines/pq"
)

// Runtime implements the Quasar runtime with PQ overlay
type Runtime struct {
	dag *dag.Engine
	pq  pq.Verifier
}

// New creates a new Quasar runtime
func New(cfg config.Parameters, dagEng *dag.Engine, pqv pq.Verifier) *Runtime {
	return &Runtime{dag: dagEng, pq: pqv}
}

// Start starts the runtime
func (r *Runtime) Start(ctx context.Context) error {
	go r.dag.Start(ctx)
	<-ctx.Done()
	return ctx.Err()
}

// Stop stops the runtime
func (r *Runtime) Stop(ctx context.Context) error {
	return nil
}