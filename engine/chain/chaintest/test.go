// Package chaintest provides test utilities for chains
package chaintest

// TestChain provides a test implementation for chains
type TestChain struct {
	id     []byte
	height uint64
}

// NewTestChain creates a new test chain
func NewTestChain(id []byte) *TestChain {
	return &TestChain{
		id:     id,
		height: 0,
	}
}

// ID returns the chain ID
func (t *TestChain) ID() []byte {
	return t.id
}

// Height returns the current height
func (t *TestChain) Height() uint64 {
	return t.height
}

// SetHeight sets the chain height
func (t *TestChain) SetHeight(height uint64) {
	t.height = height
}
