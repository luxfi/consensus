// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chainmock

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/ids"
)

// NewBlock creates a new Block mock
// Note: ctrl parameter is for gomock compatibility but not used
func NewBlock(ctrl interface{}) *Block {
	return &Block{}
}

// Block is a mock implementation of chain.Block
type Block struct {
	T                   *testing.T
	CantID              bool
	CantAccept          bool
	CantReject          bool
	CantStatus          bool
	CantParent          bool
	CantHeight          bool
	CantTimestamp       bool
	CantVerify          bool
	CantBytes           bool
	CantSetStatus       bool

	IDF       func() ids.ID
	AcceptF   func(context.Context) error
	RejectF   func(context.Context) error
	StatusF   func() choices.Status
	ParentF   func() ids.ID
	HeightF   func() uint64
	TimestampF func() time.Time
	VerifyF   func(context.Context) error
	BytesF    func() []byte
	SetStatusF func(choices.Status)
}

func (b *Block) ID() ids.ID {
	if b.IDF != nil {
		return b.IDF()
	}
	if b.CantID && b.T != nil {
		b.T.Fatal("unexpected ID")
	}
	return ids.Empty
}

func (b *Block) Accept(ctx context.Context) error {
	if b.AcceptF != nil {
		return b.AcceptF(ctx)
	}
	if b.CantAccept && b.T != nil {
		b.T.Fatal("unexpected Accept")
	}
	return nil
}

func (b *Block) Reject(ctx context.Context) error {
	if b.RejectF != nil {
		return b.RejectF(ctx)
	}
	if b.CantReject && b.T != nil {
		b.T.Fatal("unexpected Reject")
	}
	return nil
}

func (b *Block) Status() choices.Status {
	if b.StatusF != nil {
		return b.StatusF()
	}
	if b.CantStatus && b.T != nil {
		b.T.Fatal("unexpected Status")
	}
	return choices.Unknown
}

func (b *Block) Parent() ids.ID {
	if b.ParentF != nil {
		return b.ParentF()
	}
	if b.CantParent && b.T != nil {
		b.T.Fatal("unexpected Parent")
	}
	return ids.Empty
}

func (b *Block) Height() uint64 {
	if b.HeightF != nil {
		return b.HeightF()
	}
	if b.CantHeight && b.T != nil {
		b.T.Fatal("unexpected Height")
	}
	return 0
}

func (b *Block) Timestamp() time.Time {
	if b.TimestampF != nil {
		return b.TimestampF()
	}
	if b.CantTimestamp && b.T != nil {
		b.T.Fatal("unexpected Timestamp")
	}
	return time.Time{}
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
	if b.BytesF != nil {
		return b.BytesF()
	}
	if b.CantBytes && b.T != nil {
		b.T.Fatal("unexpected Bytes")
	}
	return nil
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