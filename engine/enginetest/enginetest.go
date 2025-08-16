// Package enginetest provides test utilities for consensus engine operations
package enginetest

import "testing"

// TestEngine represents a test consensus engine
type TestEngine struct {
	ID      string
	Running bool
}

// NewTestEngine creates a new test engine
func NewTestEngine(id string) *TestEngine {
	return &TestEngine{
		ID:      id,
		Running: false,
	}
}

// Helper provides test helper functions
func Helper(t *testing.T) {
	t.Helper()
}