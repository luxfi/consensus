// Package chaintest provides test utilities for chain operations
package chaintest

import "testing"

// TestChain represents a test chain
type TestChain struct {
	ID      string
	Height  uint64
	Blocks  []string
}

// NewTestChain creates a new test chain
func NewTestChain(id string) *TestChain {
	return &TestChain{
		ID:     id,
		Height: 0,
		Blocks: []string{},
	}
}

// Helper provides test helper functions
func Helper(t *testing.T) {
	t.Helper()
}