// Package chaintest provides test utilities for chains
package chaintest

import (
	"context"
	"github.com/luxfi/ids"
	"github.com/luxfi/node/consensus/choices"
	"github.com/luxfi/node/consensus/protocol/chain"
)

// TestBlock provides a test implementation for blocks
type TestBlock struct {
	id       ids.ID
	parentID ids.ID
	height   uint64
	bytes    []byte
}

// ID returns the block ID
func (b *TestBlock) ID() ids.ID {
	return b.id
}

// ParentID returns the parent block ID
func (b *TestBlock) ParentID() ids.ID {
	return b.parentID
}

// Height returns the block height
func (b *TestBlock) Height() uint64 {
	return b.height
}

// Bytes returns the block bytes
func (b *TestBlock) Bytes() []byte {
	return b.bytes
}

// Timestamp returns the block timestamp
func (b *TestBlock) Timestamp() int64 {
	return 0
}

// Accept accepts the block
func (b *TestBlock) Accept(ctx context.Context) error {
	return nil
}

// Reject rejects the block
func (b *TestBlock) Reject(ctx context.Context) error {
	return nil
}

// Verify verifies the block
func (b *TestBlock) Verify(ctx context.Context) error {
	return nil
}

// Status returns the block status
func (b *TestBlock) Status() choices.Status {
	return choices.Processing
}

// Genesis is the genesis block for testing
var Genesis chain.Block = &TestBlock{
	id:       ids.GenerateTestID(),
	parentID: ids.Empty,
	height:   0,
	bytes:    []byte("genesis"),
}

// BuildChild builds a child block for testing
func BuildChild(parent chain.Block) chain.Block {
	return &TestBlock{
		id:       ids.GenerateTestID(),
		parentID: parent.ID(),
		height:   parent.Height() + 1,
		bytes:    []byte("child"),
	}
}

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
