// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chaintest

import (
	"context"
	"time"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/consensus/chain"
	"github.com/luxfi/ids"
)

// TestBlock is a test implementation of a block
type TestBlock struct {
	IDV        ids.ID
	HeightV    uint64
	TimestampV time.Time
	ParentV    ids.ID
	BytesV     []byte
	StatusV    choices.Status
}

// Genesis is a test genesis block
var Genesis = &TestBlock{
	IDV:        ids.GenerateTestID(),
	HeightV:    0,
	TimestampV: time.Now(),
	ParentV:    ids.Empty,
	StatusV:    choices.Accepted,
}

// BuildChild builds a child block for testing
func BuildChild(parent *TestBlock) *TestBlock {
	return &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    parent.HeightV + 1,
		TimestampV: parent.TimestampV.Add(time.Second),
		ParentV:    parent.IDV,
		StatusV:    choices.Processing,
	}
}

// ID returns the block ID
func (b *TestBlock) ID() ids.ID {
	return b.IDV
}

// Parent returns the parent block ID
func (b *TestBlock) Parent() ids.ID {
	return b.ParentV
}

// Height returns the block height
func (b *TestBlock) Height() uint64 {
	return b.HeightV
}

// Timestamp returns the block timestamp
func (b *TestBlock) Timestamp() time.Time {
	return b.TimestampV
}

// Verify does nothing
func (b *TestBlock) Verify(ctx context.Context) error {
	return nil
}

// Bytes returns the block bytes
func (b *TestBlock) Bytes() []byte {
	return b.BytesV
}

// Accept marks the block as accepted
func (b *TestBlock) Accept(ctx context.Context) error {
	b.StatusV = choices.Accepted
	return nil
}

// Reject marks the block as rejected
func (b *TestBlock) Reject(ctx context.Context) error {
	b.StatusV = choices.Rejected
	return nil
}

// Status returns the block status
func (b *TestBlock) Status() choices.Status {
	return b.StatusV
}

// FPCVotes returns embedded fast-path vote references
func (b *TestBlock) FPCVotes() [][]byte {
	return nil // Test blocks don't have FPC votes
}

// EpochBit returns the epoch fence bit
func (b *TestBlock) EpochBit() bool {
	return false // Test blocks don't have epoch bits
}

var _ chain.Block = (*TestBlock)(nil)