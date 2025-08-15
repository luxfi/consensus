// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package focus implements �-consecutive confidence counters
package focus

import (
	"sync/atomic"
)

// Counter tracks consecutive successes for finalization
type Counter interface {
	Tick(ok bool) uint32
	Finalized(beta uint32) bool
	Reset()
}

// FocusCounter implements � confidence tracking
type FocusCounter struct {
	consecutive atomic.Uint32
	beta        uint32
}

// New creates a new focus counter with given beta
func New(beta uint32) Confidence {
	return &ConfidenceTracker{
		counter: &FocusCounter{beta: beta},
		beta:    beta,
	}
}

// Tick records a success or failure, returns current consecutive count
func (f *FocusCounter) Tick(ok bool) uint32 {
	if ok {
		return f.consecutive.Add(1)
	}
	f.consecutive.Store(0)
	return 0
}

// Finalized returns true if consecutive successes >= beta
func (f *FocusCounter) Finalized(beta uint32) bool {
	return f.consecutive.Load() >= beta
}

// Reset clears the consecutive counter
func (f *FocusCounter) Reset() {
	f.consecutive.Store(0)
}

// Confidence interface for consensus integration
type Confidence interface {
	Record(success bool) (finalized bool)
	Reset()
}

// ConfidenceTracker wraps FocusCounter for the Confidence interface
type ConfidenceTracker struct {
	counter *FocusCounter
	beta    uint32
}

// NewConfidence creates a new confidence tracker
func NewConfidence(beta uint32) Confidence {
	return &ConfidenceTracker{
		counter: &FocusCounter{beta: beta},
		beta:    beta,
	}
}

// Record records a success/failure and returns if finalized
func (c *ConfidenceTracker) Record(success bool) bool {
	count := c.counter.Tick(success)
	return count >= c.beta
}

// Reset resets the confidence counter
func (c *ConfidenceTracker) Reset() {
	c.counter.Reset()
}