// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package blocktest

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/consensus/consensustest"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// StateSummary is a test implementation of block.StateSummary
type StateSummary struct {
	T         *testing.T
	CantID    bool
	CantAccept bool
	CantHeight bool
	CantBytes  bool

	IDV     ids.ID
	HeightV uint64
	BytesV  []byte

	AcceptF func(context.Context) (block.StateSyncMode, error)
}

func (s *StateSummary) ID() ids.ID {
	if s.CantID && s.T != nil {
		s.T.Fatal("unexpected ID")
	}
	return s.IDV
}

func (s *StateSummary) Accept(ctx context.Context) (block.StateSyncMode, error) {
	if s.AcceptF != nil {
		return s.AcceptF(ctx)
	}
	if s.CantAccept && s.T != nil {
		s.T.Fatal("unexpected Accept")
	}
	return block.StateSyncSkipped, nil
}

func (s *StateSummary) Height() uint64 {
	if s.CantHeight && s.T != nil {
		s.T.Fatal("unexpected Height")
	}
	return s.HeightV
}

func (s *StateSummary) Bytes() []byte {
	if s.CantBytes && s.T != nil {
		s.T.Fatal("unexpected Bytes")
	}
	return s.BytesV
}

// Block is a test implementation of block.Block
type Block struct {
	consensustest.Decidable
	
	T            *testing.T
	CantParent   bool
	CantHeight   bool
	CantTimestamp bool
	CantVerify   bool
	CantBytes    bool
	CantSetStatus bool

	HeightV    uint64
	ParentV    ids.ID
	TimestampV time.Time
	BytesV     []byte

	VerifyF   func(context.Context) error
	SetStatusF func(choices.Status)
}

func (b *Block) Parent() ids.ID {
	if b.CantParent && b.T != nil {
		b.T.Fatal("unexpected Parent")
	}
	return b.ParentV
}

func (b *Block) Height() uint64 {
	if b.CantHeight && b.T != nil {
		b.T.Fatal("unexpected Height")
	}
	return b.HeightV
}

func (b *Block) Timestamp() time.Time {
	if b.CantTimestamp && b.T != nil {
		b.T.Fatal("unexpected Timestamp")
	}
	return b.TimestampV
}

func (b *Block) Verify(ctx context.Context) error {
	if b.VerifyF != nil {
		return b.VerifyF(ctx)
	}
	if b.CantVerify && b.T != nil {
		b.T.Fatal("unexpected Verify")
	}
	return nil
}

func (b *Block) Bytes() []byte {
	if b.CantBytes && b.T != nil {
		b.T.Fatal("unexpected Bytes")
	}
	return b.BytesV
}

func (b *Block) SetStatus(status choices.Status) {
	if b.SetStatusF != nil {
		b.SetStatusF(status)
		return
	}
	if b.CantSetStatus && b.T != nil {
		b.T.Fatal("unexpected SetStatus")
	}
}