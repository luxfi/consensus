// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package chaintest provides test utilities for chain testing
package chaintest

import (
	"context"
	"time"

	"github.com/luxfi/ids"
)

// TestBlock is a mock block for testing
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

// Parent returns the parent block ID
func (b *TestBlock) Parent() ids.ID {
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

// Accept accepts the block
func (b *TestBlock) Accept(ctx context.Context) error {
	return nil
}

// Reject rejects the block
func (b *TestBlock) Reject(ctx context.Context) error {
	return nil
}

// Genesis is the genesis test block
var Genesis = &TestBlock{
	id:       ids.GenerateTestID(),
	parentID: ids.Empty,
	height:   0,
	bytes:    []byte("genesis"),
}

// BuildChild builds a child block
func BuildChild(parent *TestBlock) *TestBlock {
	return &TestBlock{
		id:       ids.GenerateTestID(),
		parentID: parent.ID(),
		height:   parent.Height() + 1,
		bytes:    []byte("child"),
	}
}

// BuildChildWithTime builds a child block with timestamp
func BuildChildWithTime(parent *TestBlock, timestamp time.Time) *TestBlock {
	child := BuildChild(parent)
	// Add timestamp to bytes for uniqueness
	child.bytes = append(child.bytes, timestamp.Format(time.RFC3339)...)
	return child
}
