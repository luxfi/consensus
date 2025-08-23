// Package blocktest provides test utilities for blocks
package blocktest

import (
	"context"
	"time"
)

// TestBlock provides a test implementation for blocks
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