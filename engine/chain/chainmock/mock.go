// Package chainmock provides mock implementations for testing
package chainmock

import (
	"context"
	"time"

	"github.com/luxfi/ids"
)

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

// Block provides a mock Block implementation
type Block struct {
	id       ids.ID
	parentID ids.ID
	height   uint64
	status   uint8
	bytes    []byte
}

// NewBlock creates a new mock block
func NewBlock(id ids.ID, parentID ids.ID, height uint64) *Block {
	return &Block{
		id:       id,
		parentID: parentID,
		height:   height,
		status:   0,
		bytes:    id[:],
	}
}

// ID returns the block ID
func (b *Block) ID() ids.ID {
	return b.id
}

// Parent returns the parent block ID (alias for ParentID)
func (b *Block) Parent() ids.ID {
	return b.parentID
}

// ParentID returns the parent block ID
func (b *Block) ParentID() ids.ID {
	return b.parentID
}

// Height returns the block height
func (b *Block) Height() uint64 {
	return b.height
}

// Timestamp returns the block timestamp
func (b *Block) Timestamp() time.Time {
	return time.Now()
}

// Status returns the block status
func (b *Block) Status() uint8 {
	return b.status
}

// Verify verifies the block
func (b *Block) Verify(ctx context.Context) error {
	return nil
}

// Accept accepts the block
func (b *Block) Accept(ctx context.Context) error {
	b.status = 2 // Accepted
	return nil
}

// Reject rejects the block
func (b *Block) Reject(ctx context.Context) error {
	b.status = 1 // Rejected
	return nil
}

// Bytes returns the block bytes
func (b *Block) Bytes() []byte {
	return b.bytes
}
