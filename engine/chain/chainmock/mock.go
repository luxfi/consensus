// Package chainmock provides mock implementations for testing
package chainmock

// MockChain provides a mock implementation for testing
type MockChain struct {
	id     []byte
	height uint64
}

// NewMockChain creates a new mock chain
func NewMockChain(id []byte) *MockChain {
	return &MockChain{
		id:     id,
		height: 0,
	}
}

// ID returns the chain ID
func (m *MockChain) ID() []byte {
	return m.id
}

// Height returns the current height
func (m *MockChain) Height() uint64 {
	return m.height
}

// SetHeight sets the chain height
func (m *MockChain) SetHeight(height uint64) {
	m.height = height
}