// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package blocktest

import (
	"context"
	"fmt"
	"time"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/consensus/protocol/chain"
	"github.com/luxfi/ids"
)

var (
	nextID = uint64(0)

	GenesisID        = ids.GenerateTestID()
	GenesisHeight    = uint64(0)
	GenesisTimestamp = time.Unix(0, 0)
	GenesisBytes     = []byte("genesis")

	Genesis = &Block{
		IDV:        GenesisID,
		HeightV:    GenesisHeight,
		TimestampV: GenesisTimestamp,
		BytesV:     GenesisBytes,
		StatusV:    choices.Accepted,
	}
)

// Block is a test block implementation
type Block struct {
	IDV        ids.ID
	HeightV    uint64
	TimestampV time.Time
	ParentV    ids.ID
	BytesV     []byte
	StatusV    choices.Status

	ShouldVerifyV bool
	ErrV          error
}

func (b *Block) ID() ids.ID {
	return b.IDV
}

func (b *Block) Height() uint64 {
	return b.HeightV
}

func (b *Block) Timestamp() time.Time {
	return b.TimestampV
}

func (b *Block) Parent() ids.ID {
	return b.ParentV
}

func (b *Block) ParentID() ids.ID {
	return b.ParentV
}

func (b *Block) Bytes() []byte {
	return b.BytesV
}

func (b *Block) Status() uint8 {
	return uint8(b.StatusV)
}

func (b *Block) Verify(context.Context) error {
	if !b.ShouldVerifyV {
		return b.ErrV
	}
	return nil
}

func (b *Block) Accept(context.Context) error {
	b.StatusV = choices.Accepted
	return nil
}

func (b *Block) Reject(context.Context) error {
	b.StatusV = choices.Rejected
	return nil
}

// BuildChild creates a child block of the given parent
func BuildChild(parent chain.Block) *Block {
	nextID++
	blockID := ids.ID{}
	copy(blockID[:], fmt.Sprintf("block_%d", nextID))

	timestamp := parent.Timestamp().Add(time.Second)

	return &Block{
		IDV:           blockID,
		HeightV:       parent.Height() + 1,
		TimestampV:    timestamp,
		ParentV:       parent.ID(),
		BytesV:        []byte(fmt.Sprintf("block_%d", nextID)),
		StatusV:       choices.Processing,
		ShouldVerifyV: true,
	}
}
