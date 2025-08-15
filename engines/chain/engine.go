// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"

	"github.com/luxfi/consensus/beam"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/focus"
	"github.com/luxfi/consensus/wave"
)

// Store interface for persistence
type Store interface{} // fill in

// Net interface for networking
type Net interface{} // fill in

// Logger interface for logging
type Logger interface {
	Info(string, ...any)
	Error(string, ...any)
}

// Engine implements linear chain consensus (Photon→Wave→Focus→Beam)
type Engine struct {
	cfg   config.Parameters
	store Store
	net   Net
	log   Logger

	wave  wave.Tally
	focus focus.Confidence
	beam  beam.Finalizer
}

// New creates a new chain consensus engine
func New(cfg config.Parameters, store Store, net Net, log Logger) *Engine {
	return &Engine{
		cfg:   cfg,
		store: store,
		net:   net,
		log:   log,
		wave:  wave.New(cfg.AlphaPreference, cfg.AlphaConfidence),
		focus: focus.New(cfg.Beta),
		beam:  beam.New(),
	}
}

// Start starts the engine
func (e *Engine) Start(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}
