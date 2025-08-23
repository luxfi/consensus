// Package blockmock provides mock implementations for testing
package blockmock

import (
	"context"
	"time"
)

// MockBlock provides a mock implementation for testing
type MockBlock struct {
	id       []byte
	height   uint64
	parent   []byte
	accepted bool
}

// NewMockBlock creates a new mock block
func NewMockBlock(id []byte, height uint64, parent []byte) *MockBlock {
	return &MockBlock{
		id:     id,
		height: height,
		parent: parent,
	}
}

// ID returns the block ID
func (m *MockBlock) ID() []byte {
	return m.id
}

// Height returns the block height
func (m *MockBlock) Height() uint64 {
	return m.height
}

// Parent returns the parent block ID
func (m *MockBlock) Parent() []byte {
	return m.parent
}

// Accept marks the block as accepted
func (m *MockBlock) Accept(ctx context.Context) error {
	m.accepted = true
	return nil
}

// Reject marks the block as rejected
func (m *MockBlock) Reject(ctx context.Context) error {
	return nil
}

// Status returns the block status
func (m *MockBlock) Status() int {
	if m.accepted {
		return 2 // Accepted
	}
	return 0 // Processing
}

// Timestamp returns the block timestamp
func (m *MockBlock) Timestamp() time.Time {
	return time.Now()
}

// Bytes returns the block bytes
func (m *MockBlock) Bytes() []byte {
	return m.id
}