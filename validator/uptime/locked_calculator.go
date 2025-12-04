// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package uptime

import (
	"sync"
	"time"

	"github.com/luxfi/ids"
)

// LockedCalculator is a wrapper for a Calculator that ensures thread-safety
type LockedCalculator interface {
	Calculator
}

// NewLockedCalculator returns a new LockedCalculator with default NoOp behavior
func NewLockedCalculator() LockedCalculator {
	return &lockedCalculator{
		calculators: make(map[ids.ID]Calculator),
		fallback:    NoOpCalculator{},
	}
}

// NewLockedCalculatorWithFallback returns a new LockedCalculator with a custom fallback
func NewLockedCalculatorWithFallback(fallback Calculator) LockedCalculator {
	if fallback == nil {
		fallback = NoOpCalculator{}
	}
	return &lockedCalculator{
		calculators: make(map[ids.ID]Calculator),
		fallback:    fallback,
	}
}

type lockedCalculator struct {
	mu          sync.RWMutex
	calculators map[ids.ID]Calculator
	fallback    Calculator
}

func (l *lockedCalculator) CalculateUptime(nodeID ids.NodeID, subnetID ids.ID) (time.Duration, time.Duration, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if calc, ok := l.calculators[subnetID]; ok {
		return calc.CalculateUptime(nodeID, subnetID)
	}
	return l.fallback.CalculateUptime(nodeID, subnetID)
}

func (l *lockedCalculator) CalculateUptimePercent(nodeID ids.NodeID, subnetID ids.ID) (float64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if calc, ok := l.calculators[subnetID]; ok {
		return calc.CalculateUptimePercent(nodeID, subnetID)
	}
	return l.fallback.CalculateUptimePercent(nodeID, subnetID)
}

func (l *lockedCalculator) CalculateUptimePercentFrom(nodeID ids.NodeID, subnetID ids.ID, from time.Time) (float64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if calc, ok := l.calculators[subnetID]; ok {
		return calc.CalculateUptimePercentFrom(nodeID, subnetID, from)
	}
	return l.fallback.CalculateUptimePercentFrom(nodeID, subnetID, from)
}

func (l *lockedCalculator) SetCalculator(subnetID ids.ID, calc Calculator) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if calc != nil {
		l.calculators[subnetID] = calc
	}
	return nil
}
