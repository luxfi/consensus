// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package dag

import (
	"context"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/flare"
	"github.com/luxfi/consensus/focus"
	"github.com/luxfi/consensus/nova"
	"github.com/luxfi/consensus/wave"
)

// Store interface for persistence
type Store interface{}

// Net interface for networking
type Net interface{}

// Logger interface for logging
type Logger interface {
	Info(string, ...any)
	Error(string, ...any)
}

// Engine implements DAG consensus (Photon→Wave→Focus→Flare→Nova)
type Engine struct {
	cfg   config.Parameters
	store Store
	net   Net
	log   Logger

	wave  wave.Tally
	focus focus.Confidence
	flare flare.Graph
	nova  nova.Finality
}

// New creates a new DAG consensus engine
func New(cfg config.Parameters, store Store, net Net, log Logger,
	flareGraph flare.Graph, novaFin nova.Finality) *Engine {
	return &Engine{
		cfg:   cfg,
		store: store,
		net:   net,
		log:   log,
		wave:  wave.New(cfg.AlphaPreference, cfg.AlphaConfidence),
		focus: focus.New(cfg.Beta),
		flare: flareGraph,
		nova:  novaFin,
	}
}

// Start starts the engine
func (e *Engine) Start(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}