// Package blocktest provides test utilities for block chain operations
package blocktest

import "testing"

// TestBlock represents a test block
type TestBlock struct {
	ID     string
	Height uint64
	Parent string
}

// NewTestBlock creates a new test block
func NewTestBlock(id string, height uint64, parent string) *TestBlock {
	return &TestBlock{
		ID:     id,
		Height: height,
		Parent: parent,
	}
}

// Helper provides test helper functions
func Helper(t *testing.T) {
	t.Helper()
}