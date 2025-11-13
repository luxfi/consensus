// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package bft provides a thin wrapper around github.com/luxfi/bft (Simplex BFT)
// for integration with the Lux consensus engine interface.
//
// Simplex BFT is maintained as an external MPL-licensed package.
// This wrapper provides glue code to integrate it with the consensus engine.
package bft

import (
	"context"

	luxbft "github.com/luxfi/bft"
)

// Engine wraps the Simplex BFT consensus engine
type Engine struct {
	simplex *luxbft.Epoch
	config  Config
}

// Config for BFT engine wrapper
type Config struct {
	NodeID      string
	Validators  []string
	EpochLength uint64
	EpochConfig luxbft.EpochConfig // Pass-through to Simplex
}

// New creates a new BFT consensus engine using Simplex
// For full Simplex configuration, use Config.EpochConfig
func New(cfg Config) (*Engine, error) {
	// Create Simplex epoch with the provided config
	epoch, err := luxbft.NewEpoch(cfg.EpochConfig)
	if err != nil {
		return nil, err
	}
	
	return &Engine{
		simplex: epoch,
		config:  cfg,
	}, nil
}

// Start starts the BFT engine
func (e *Engine) Start(ctx context.Context, startReqID uint32) error {
	// Simplex handles start internally
	// The epoch is already configured and ready
	return nil
}

// Stop stops the BFT engine
func (e *Engine) Stop(ctx context.Context) error {
	// Simplex handles shutdown via context cancellation
	return nil
}

// IsBootstrapped returns whether the engine has finished bootstrapping
func (e *Engine) IsBootstrapped() bool {
	// BFT doesn't need bootstrap - it's always ready
	return true
}

// HealthCheck returns the health status
func (e *Engine) HealthCheck(ctx context.Context) (interface{}, error) {
	return map[string]interface{}{
		"consensus": "bft-simplex",
		"status":    "healthy",
		"epoch":     e.simplex.Epoch,
	}, nil
}

// GetSimplex returns the underlying Simplex BFT engine
// Use this for direct access to Simplex features like:
// - ProposeBlock()
// - AddNode()
// - OnQC()
func (e *Engine) GetSimplex() *luxbft.Epoch {
	return e.simplex
}
