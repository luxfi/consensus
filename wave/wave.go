// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package wave provides threshold scheduling and round management for consensus.
package wave

import (
	"context"
	"fmt"
)

// Stage represents the consensus stage (Wave or FPC)
type Stage int

const (
	// StageWave is the standard wave consensus
	StageWave Stage = iota
	// StageFPC is the fast path consensus stage
	StageFPC
)

// Selector provides alpha thresholds for preference and confidence.
type Selector interface {
	// Alpha returns the preference and confidence thresholds for a given K and phase.
	Alpha(k int, phase uint64) (alphaPref, alphaConf int)
}

// Result represents the result of a polling round
type Result struct {
	Success bool
	Choice  []byte // opaque preference (e.g., block or vertex head)
}

// Round manages voting rounds and thresholds.
type Round interface {
	// Poll executes a polling round
	Poll(ctx context.Context) (Result, error)
	// Record records votes and returns whether preference and confidence are met.
	Record(votes int) (preferOK, confOK bool)
	// Reset resets the round state.
	Reset()
}

// round implements the Round interface.
type round struct {
	k        int
	selector Selector
	phase    uint64
}

// NewRound creates a new round manager.
func NewRound(k int, selector Selector) Round {
	return &round{
		k:        k,
		selector: selector,
		phase:    0,
	}
}

// Poll executes a polling round
func (r *round) Poll(ctx context.Context) (Result, error) {
	// This is a simplified implementation
	// In production, this would query validators and collect votes
	alphaPref, alphaConf := r.selector.Alpha(r.k, r.phase)
	
	// Simulate vote collection (in real implementation, would query network)
	votes := r.k / 2 + 1 // Simple majority for now
	
	r.phase++
	
	if votes >= alphaConf {
		return Result{Success: true, Choice: []byte("consensus")}, nil
	}
	return Result{Success: false}, nil
}

// Record records votes for this round.
func (r *round) Record(votes int) (preferOK, confOK bool) {
	alphaPref, alphaConf := r.selector.Alpha(r.k, r.phase)
	
	preferOK = votes >= alphaPref
	confOK = votes >= alphaConf
	
	r.phase++
	return preferOK, confOK
}

// Reset resets the round state.
func (r *round) Reset() {
	r.phase = 0
}

// DefaultSelector implements a simple majority selector.
type DefaultSelector struct{}

// Alpha returns simple majority thresholds.
func (d *DefaultSelector) Alpha(k int, phase uint64) (alphaPref, alphaConf int) {
	alphaPref = (k + 1) / 2 // Simple majority for preference
	alphaConf = k           // All for confidence
	return alphaPref, alphaConf
}

// State represents the wave consensus state for an item.
type State struct {
	Stage    Stage
	Step     Step
	Decided  bool
	Result   int
}

// Step represents a single consensus step.
type Step struct {
	Prefer bool
	Conf   int
}

// Manager manages wave consensus for multiple items.
type Manager[T comparable] struct {
	selector Selector
	states   map[T]*State
}

// NewManager creates a new wave consensus manager.
func NewManager[T comparable](selector Selector) *Manager[T] {
	if selector == nil {
		selector = &DefaultSelector{}
	}
	return &Manager[T]{
		selector: selector,
		states:   make(map[T]*State),
	}
}

// GetState returns the state for an item.
func (m *Manager[T]) GetState(item T) (*State, bool) {
	state, exists := m.states[item]
	return state, exists
}

// UpdateState updates the state for an item.
func (m *Manager[T]) UpdateState(item T, state *State) {
	m.states[item] = state
}

// Validate validates wave configuration.
func Validate(k int, beta int) error {
	if k <= 0 {
		return fmt.Errorf("invalid k: %d", k)
	}
	if beta <= 0 {
		return fmt.Errorf("invalid beta: %d", beta)
	}
	return nil
}