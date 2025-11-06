// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

// Package tracker provides consensus tracking utilities
package tracker

import (
	"sync"
	"time"

	"github.com/luxfi/ids"
)

// Tracker tracks consensus operations
type Tracker interface {
	// Track starts tracking an operation
	Track(id ids.ID) time.Time

	// Stop stops tracking an operation
	Stop(id ids.ID) time.Duration

	// IsTracked checks if an operation is being tracked
	IsTracked(id ids.ID) bool

	// Len returns the number of tracked operations
	Len() int
}

// TimeTracker tracks operation times
type TimeTracker struct {
	mu     sync.RWMutex
	starts map[ids.ID]time.Time
}

// NewTimeTracker creates a new time tracker
func NewTimeTracker() *TimeTracker {
	return &TimeTracker{
		starts: make(map[ids.ID]time.Time),
	}
}

// Track starts tracking an operation
func (t *TimeTracker) Track(id ids.ID) time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	t.starts[id] = now
	return now
}

// Stop stops tracking an operation
func (t *TimeTracker) Stop(id ids.ID) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	start, ok := t.starts[id]
	if !ok {
		return 0
	}

	delete(t.starts, id)
	return time.Since(start)
}

// IsTracked checks if an operation is being tracked
func (t *TimeTracker) IsTracked(id ids.ID) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	_, ok := t.starts[id]
	return ok
}

// Len returns the number of tracked operations
func (t *TimeTracker) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return len(t.starts)
}
