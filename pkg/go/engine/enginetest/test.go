// Package enginetest provides test utilities for consensus engines
package enginetest

import "context"

// TestEngine provides a test implementation for consensus engines
type TestEngine struct {
	started bool
	height  uint64
}

// NewTestEngine creates a new test engine
func NewTestEngine() *TestEngine {
	return &TestEngine{
		started: false,
		height:  0,
	}
}

// Start starts the test engine
func (t *TestEngine) Start(ctx context.Context) error {
	t.started = true
	return nil
}

// Stop stops the test engine
func (t *TestEngine) Stop(ctx context.Context) error {
	t.started = false
	return nil
}

// IsStarted returns whether the engine is started
func (t *TestEngine) IsStarted() bool {
	return t.started
}

// Height returns the current height
func (t *TestEngine) Height() uint64 {
	return t.height
}

// SetHeight sets the engine height
func (t *TestEngine) SetHeight(height uint64) {
	t.height = height
}
