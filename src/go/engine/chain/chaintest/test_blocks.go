// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package chaintest provides test utilities for chain testing
package chaintest

import (
	"context"
	"time"

	"github.com/luxfi/consensus/consensustest"
	"github.com/luxfi/ids"
)

// TestBlock is a mock block for testing
type TestBlock struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	bytes     []byte
	status    uint8
	timestamp time.Time
}

// ID returns the block ID
func (b *TestBlock) ID() ids.ID {
	return b.id
}

// Parent returns the parent block ID (alias for ParentID)
func (b *TestBlock) Parent() ids.ID {
	return b.parentID
}

// ParentID returns the parent block ID
func (b *TestBlock) ParentID() ids.ID {
	return b.parentID
}

// Height returns the block height
func (b *TestBlock) Height() uint64 {
	return b.height
}

// Timestamp returns the block timestamp
func (b *TestBlock) Timestamp() time.Time {
	return b.timestamp
}

// Bytes returns the block bytes
func (b *TestBlock) Bytes() []byte {
	return b.bytes
}

// Status returns the block status
func (b *TestBlock) Status() uint8 {
	return b.status
}

// Accept accepts the block
func (b *TestBlock) Accept(ctx context.Context) error {
	b.status = consensustest.Accepted
	return nil
}

// Reject rejects the block
func (b *TestBlock) Reject(ctx context.Context) error {
	b.status = consensustest.Rejected
	return nil
}

// Verify verifies the block
func (b *TestBlock) Verify(ctx context.Context) error {
	return nil
}

// Genesis is the genesis test block
var Genesis = &TestBlock{
	id:        ids.GenerateTestID(),
	parentID:  ids.Empty,
	height:    0,
	bytes:     []byte("genesis"),
	status:    consensustest.Accepted,
	timestamp: time.Now(),
}

// BuildChild builds a child block
func BuildChild(parent *TestBlock) *TestBlock {
	return &TestBlock{
		id:        ids.GenerateTestID(),
		parentID:  parent.ID(),
		height:    parent.Height() + 1,
		bytes:     []byte("child"),
		status:    consensustest.Processing,
		timestamp: time.Now(),
	}
}

// BuildChildWithTime builds a child block with timestamp
func BuildChildWithTime(parent *TestBlock, timestamp time.Time) *TestBlock {
	return &TestBlock{
		id:        ids.GenerateTestID(),
		parentID:  parent.ID(),
		height:    parent.Height() + 1,
		bytes:     append([]byte("child"), timestamp.Format(time.RFC3339)...),
		status:    consensustest.Processing,
		timestamp: timestamp,
	}
}
