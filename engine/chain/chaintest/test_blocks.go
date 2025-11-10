// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package chaintest provides test utilities for chain testing
package chaintest

import (
	"context"
	"time"

	"github.com/luxfi/consensus/core/choices"
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
	// Additional fields for BFT testing
	BytesV    []byte
	ParentV   ids.ID
	Decidable choices.Decidable
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
	if b.BytesV != nil {
		return b.BytesV
	}
	return b.bytes
}

// Status returns the block status
func (b *TestBlock) Status() uint8 {
	return b.status
}

// Accept accepts the block
func (b *TestBlock) Accept(ctx context.Context) error {
	b.status = 2 // Accepted
	b.Decidable.Status = choices.Accepted
	return nil
}

// Reject rejects the block
func (b *TestBlock) Reject(ctx context.Context) error {
	b.status = 3 // Rejected
	b.Decidable.Status = choices.Rejected
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
	status:    2, // Accepted
	timestamp: time.Now(),
	BytesV:    []byte("genesis"),
	ParentV:   ids.Empty,
	Decidable: choices.Decidable{Status: choices.Accepted},
}

// BuildChild builds a child block
func BuildChild(parent *TestBlock) *TestBlock {
	childID := ids.GenerateTestID()
	// Make bytes unique by including the ID
	childBytes := append([]byte("child-"), childID[:]...)
	return &TestBlock{
		id:        childID,
		parentID:  parent.ID(),
		height:    parent.Height() + 1,
		bytes:     childBytes,
		status:    1, // Processing
		timestamp: time.Now(),
		BytesV:    childBytes,
		ParentV:   parent.ID(),
		Decidable: choices.Decidable{Status: choices.Processing},
	}
}

// BuildChildWithTime builds a child block with timestamp
func BuildChildWithTime(parent *TestBlock, timestamp time.Time) *TestBlock {
	childID := ids.GenerateTestID()
	// Make bytes unique by including the ID
	childBytes := append([]byte("child-"), childID[:]...)
	return &TestBlock{
		id:        childID,
		parentID:  parent.ID(),
		height:    parent.Height() + 1,
		bytes:     childBytes,
		status:    1, // Processing
		timestamp: timestamp,
		BytesV:    childBytes,
		ParentV:   parent.ID(),
		Decidable: choices.Decidable{Status: choices.Processing},
	}
}
