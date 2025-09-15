// Package blocktest provides test utilities for blocks
package blocktest

import (
	"context"
	"time"

	"github.com/luxfi/consensus/consensustest"
	"github.com/luxfi/ids"
)

// Block provides a full test implementation for blocks
type Block struct {
	consensustest.Decidable
	HeightV    uint64
	ParentV    ids.ID
	BytesV     []byte
	TimestampV time.Time
	StatusV    uint8
	VerifyV    error
	AcceptV    error
	RejectV    error
}

// Height returns the block height
func (b *Block) Height() uint64 {
	return b.HeightV
}

// Parent returns the parent block ID
func (b *Block) Parent() ids.ID {
	return b.ParentV
}

// ParentID returns the parent block ID
func (b *Block) ParentID() ids.ID {
	return b.ParentV
}

// Bytes returns the block bytes
func (b *Block) Bytes() []byte {
	return b.BytesV
}

// Timestamp returns the block timestamp
func (b *Block) Timestamp() time.Time {
	if b.TimestampV.IsZero() {
		return time.Now()
	}
	return b.TimestampV
}

// Status returns the block status as uint8
func (b *Block) Status() uint8 {
	return b.StatusV
}

// Verify verifies the block
func (b *Block) Verify(ctx context.Context) error {
	if b.VerifyV != nil {
		return b.VerifyV
	}
	return nil
}

// Accept accepts the block
func (b *Block) Accept(ctx context.Context) error {
	if b.AcceptV != nil {
		return b.AcceptV
	}
	err := b.Decidable.Accept(ctx)
	if err != nil {
		return err
	}
	b.StatusV = consensustest.Accepted
	return nil
}

// Reject rejects the block
func (b *Block) Reject(ctx context.Context) error {
	if b.RejectV != nil {
		return b.RejectV
	}
	err := b.Decidable.Reject(ctx)
	if err != nil {
		return err
	}
	b.StatusV = consensustest.Rejected
	return nil
}

// TestBlock provides a test implementation for blocks (deprecated, use Block)
type TestBlock struct {
	id     []byte
	height uint64
	parent []byte
}

// NewTestBlock creates a new test block
func NewTestBlock(id []byte, height uint64, parent []byte) *TestBlock {
	return &TestBlock{
		id:     id,
		height: height,
		parent: parent,
	}
}

// ID returns the block ID
func (t *TestBlock) ID() []byte {
	return t.id
}

// Height returns the block height
func (t *TestBlock) Height() uint64 {
	return t.height
}

// Parent returns the parent block ID
func (t *TestBlock) Parent() []byte {
	return t.parent
}

// Accept marks the block as accepted
func (t *TestBlock) Accept(ctx context.Context) error {
	return nil
}

// Reject marks the block as rejected
func (t *TestBlock) Reject(ctx context.Context) error {
	return nil
}

// Timestamp returns the block timestamp
func (t *TestBlock) Timestamp() time.Time {
	return time.Now()
}

// Bytes returns the block bytes
func (t *TestBlock) Bytes() []byte {
	return t.id
}