// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"github.com/luxfi/zap"
)

// PrismCut is the LP-182 schema 0x08 wire view of a Prism uniform cut
// committee selection (protocol/prism/cut.go::UniformCut). Carries the
// sampled committee node IDs concatenated 32 bytes per ID.
type PrismCut struct {
	msg *zap.Message
	obj zap.Object
}

const (
	OffsetPrismCut_K       = 0 // uint32
	OffsetPrismCut_Peers   = 8 // bytes (N * 32B node IDs)
	SizePrismCut           = 16
)

// PrismCutFields carries the per-field input set.
type PrismCutFields struct {
	K     uint32
	Peers []byte // concatenated 32B node IDs
}

// WrapPrismCut parses b and returns the typed view.
func WrapPrismCut(b []byte) (PrismCut, error) {
	msg, err := zap.Parse(b)
	if err != nil {
		return PrismCut{}, err
	}
	obj := msg.Root()
	if obj.IsNull() {
		return PrismCut{}, ErrInvalidQuasarCert
	}
	return PrismCut{msg: msg, obj: obj}, nil
}

// NewPrismCut builds a prism-cut wire buffer.
func NewPrismCut(f PrismCutFields) PrismCut {
	b := zap.NewBuilder(256)
	ob := b.StartObject(SizePrismCut)
	ob.SetUint32(OffsetPrismCut_K, f.K)
	ob.SetBytes(OffsetPrismCut_Peers, f.Peers)
	ob.FinishAsRoot()
	buf := b.Finish()
	msg, err := zap.Parse(buf)
	if err != nil {
		panic("zap: NewPrismCut produced unparseable bytes: " + err.Error())
	}
	return PrismCut{msg: msg, obj: msg.Root()}
}

func (p PrismCut) K() uint32     { return p.obj.Uint32(OffsetPrismCut_K) }
func (p PrismCut) Peers() []byte { return p.obj.Bytes(OffsetPrismCut_Peers) }

// Bytes returns the underlying ZAP buffer.
func (p PrismCut) Bytes() []byte {
	if p.msg == nil {
		return nil
	}
	return p.msg.Bytes()
}

// IsZero reports whether the cut is the zero value.
func (p PrismCut) IsZero() bool { return p.msg == nil }
