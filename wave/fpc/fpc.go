// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package fpc implements Fast Path Consensus selector with adaptive thresholds.
package fpc

import (
	"context"
	"math"

	"github.com/luxfi/consensus/config"
)

// Selector implements FPC threshold selection using theta min/max.
type Selector struct {
	ThetaMin float64
	ThetaMax float64
	Rand     func(phase uint64) float64
}

// Alpha computes FPC thresholds for preference and confidence.
func (s *Selector) Alpha(k int, phase uint64) (alphaPref, alphaConf int) {
	// Generate random theta between ThetaMin and ThetaMax
	r := s.Rand(phase)
	theta := s.ThetaMin + r*(s.ThetaMax-s.ThetaMin)

	// Compute thresholds
	alphaPref = int(math.Ceil(theta * float64(k)))
	alphaConf = k // Always require all for confidence in FPC

	// Ensure alphaPref is at least majority
	minPref := (k + 1) / 2
	if alphaPref < minPref {
		alphaPref = minPref
	}

	return alphaPref, alphaConf
}

// NewSelector creates a new FPC selector.
func NewSelector(thetaMin, thetaMax float64, rand func(uint64) float64) *Selector {
	return &Selector{
		ThetaMin: thetaMin,
		ThetaMax: thetaMax,
		Rand:     rand,
	}
}

// Engine implements the FPC engine with vote tracking.
type Engine struct {
	config config.FPCConfig
	votes  map[interface{}]int
}

// NewEngine creates a new FPC engine.
func NewEngine(cfg config.FPCConfig) *Engine {
	return &Engine{
		config: cfg,
		votes:  make(map[interface{}]int),
	}
}

// ProcessVotes processes FPC votes.
func (e *Engine) ProcessVotes(ctx context.Context) error {
	// FPC vote processing logic
	// This is a placeholder - actual implementation would process votes
	return nil
}

// OnBlockObserved handles new block observations.
func (e *Engine) OnBlockObserved(ctx context.Context, blk interface{}) error {
	// Record block observation for FPC
	return nil
}

// OnBlockAccepted handles block acceptance.
func (e *Engine) OnBlockAccepted(ctx context.Context, blk interface{}) error {
	// Record block acceptance for FPC
	return nil
}

// NextVotes returns the next votes to include.
func (e *Engine) NextVotes(ctx context.Context, budget int) [][32]byte {
	// Return next votes up to budget
	result := make([][32]byte, 0, budget)
	// Placeholder - actual implementation would select votes
	return result
}

// Reset resets the FPC engine state.
func (e *Engine) Reset() {
	e.votes = make(map[interface{}]int)
}

// Validate validates FPC configuration.
func Validate(cfg config.FPCConfig) error {
	if cfg.ThetaMin < 0.5 || cfg.ThetaMin > 1.0 {
		return &config.ValidationError{Field: "ThetaMin", Value: cfg.ThetaMin}
	}
	if cfg.ThetaMax < cfg.ThetaMin || cfg.ThetaMax > 1.0 {
		return &config.ValidationError{Field: "ThetaMax", Value: cfg.ThetaMax}
	}
	return nil
}
