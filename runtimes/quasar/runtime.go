// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"context"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/quasar"
)

// Runtime implements the Quasar runtime with PQ overlay
type Runtime struct {
	dag interface{} // TODO: Specify concrete type when dag.Engine is instantiated
	pq  *quasar.Engine
}

// New creates a new Quasar runtime
func New(_ config.Parameters, dagEng interface{}, pqEng *quasar.Engine) *Runtime {
	return &Runtime{dag: dagEng, pq: pqEng}
}

// Start starts the runtime
func (r *Runtime) Start(ctx context.Context) error {
	// TODO: Call Start on dag when the interface is known
	// go r.dag.Start(ctx)
	<-ctx.Done()
	return ctx.Err()
}

// Stop stops the runtime
func (r *Runtime) Stop(ctx context.Context) error {
	return nil
}
