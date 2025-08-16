// Package chaintest provides test utilities for chain operations
package chaintest

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/consensus/protocol/chain"
	"github.com/luxfi/ids"
)

// Block is a test implementation of chain.Block
type Block struct {
	id       ids.ID
	parentID ids.ID
	height   uint64
	status   choices.Status
	bytes    []byte
}

// Genesis is the genesis block
var Genesis = &Block{
	id:       ids.GenerateTestID(),
	parentID: ids.Empty,
	height:   0,
	status:   choices.Accepted,
	bytes:    []byte("genesis"),
}

// BuildChild builds a child block
func BuildChild(parent *Block) *Block {
	return &Block{
		id:       ids.GenerateTestID(),
		parentID: parent.id,
		height:   parent.height + 1,
		status:   choices.Processing,
		bytes:    []byte("child"),
	}
}

// ID returns the block ID
func (b *Block) ID() ids.ID {
	return b.id
}

// Parent returns the parent block ID
func (b *Block) Parent() ids.ID {
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

// Verify verifies the block
func (b *Block) Verify(context.Context) error {
	return nil
}

// Accept accepts the block
func (b *Block) Accept(context.Context) error {
	b.status = choices.Accepted
	return nil
}

// Reject rejects the block
func (b *Block) Reject(context.Context) error {
	b.status = choices.Rejected
	return nil
}

// Status returns the block status
func (b *Block) Status() choices.Status {
	return b.status
}

// Bytes returns the block bytes
func (b *Block) Bytes() []byte {
	return b.bytes
}

// EpochBit returns the epoch bit for FPC
func (b *Block) EpochBit() bool {
	return false
}

// FPCVotes returns embedded fast-path vote references
func (b *Block) FPCVotes() [][]byte {
	return nil
}

// Options returns the block options (oracle)
func (b *Block) Options(context.Context) ([2]chain.Block, error) {
	return [2]chain.Block{}, nil
}

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