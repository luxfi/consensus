// Package blocktest provides test utilities for blocks
package blocktest

import (
	"context"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// StateSummary provides a test implementation for state summaries
type StateSummary struct {
	IDV     ids.ID
	HeightV uint64
	BytesV  []byte
	AcceptF func(context.Context) (block.StateSyncMode, error)
}

// ID returns the state summary ID
func (s *StateSummary) ID() ids.ID {
	return s.IDV
}

// Height returns the state summary height
func (s *StateSummary) Height() uint64 {
	return s.HeightV
}

// Bytes returns the state summary bytes
func (s *StateSummary) Bytes() []byte {
	return s.BytesV
}

// Accept accepts the state summary
func (s *StateSummary) Accept(ctx context.Context) (block.StateSyncMode, error) {
	if s.AcceptF != nil {
		return s.AcceptF(ctx)
	}
	return block.StateSyncSkipped, nil
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
