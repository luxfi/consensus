// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"github.com/luxfi/zap"
)

// StakeWeightedCut is the LP-182 schema 0x09 wire view of a
// stake-weighted committee selection
// (protocol/prism/stake_weighted_cut.go::StakeWeightedCut).
//
// Carries the validator block: N validators, each (32B NodeID + 8B
// uint64 weight). Wire layer transports the bytes; caller slices at
// 40B stride.
type StakeWeightedCut struct {
	msg *zap.Message
	obj zap.Object
}

const (
	OffsetStakeWeightedCut_K           = 0  // uint32
	OffsetStakeWeightedCut_TotalWeight = 8  // uint64
	OffsetStakeWeightedCut_Validators  = 16 // bytes (N * 40 = N * (32 NodeID + 8 weight))
	SizeStakeWeightedCut               = 24
)

// StakeWeightedCutFields carries the per-field input set.
type StakeWeightedCutFields struct {
	K           uint32
	TotalWeight uint64
	Validators  []byte // N * (32B NodeID + 8B BE uint64 weight)
}

// WrapStakeWeightedCut parses b and returns the typed view.
func WrapStakeWeightedCut(b []byte) (StakeWeightedCut, error) {
	msg, err := zap.Parse(b)
	if err != nil {
		return StakeWeightedCut{}, err
	}
	obj := msg.Root()
	if obj.IsNull() {
		return StakeWeightedCut{}, ErrInvalidQuasarCert
	}
	return StakeWeightedCut{msg: msg, obj: obj}, nil
}

// NewStakeWeightedCut builds a stake-weighted-cut wire buffer.
func NewStakeWeightedCut(f StakeWeightedCutFields) StakeWeightedCut {
	b := zap.NewBuilder(256)
	ob := b.StartObject(SizeStakeWeightedCut)
	ob.SetUint32(OffsetStakeWeightedCut_K, f.K)
	ob.SetUint64(OffsetStakeWeightedCut_TotalWeight, f.TotalWeight)
	ob.SetBytes(OffsetStakeWeightedCut_Validators, f.Validators)
	ob.FinishAsRoot()
	buf := b.Finish()
	msg, err := zap.Parse(buf)
	if err != nil {
		panic("zap: NewStakeWeightedCut produced unparseable bytes: " + err.Error())
	}
	return StakeWeightedCut{msg: msg, obj: msg.Root()}
}

func (s StakeWeightedCut) K() uint32 { return s.obj.Uint32(OffsetStakeWeightedCut_K) }
func (s StakeWeightedCut) TotalWeight() uint64 {
	return s.obj.Uint64(OffsetStakeWeightedCut_TotalWeight)
}
func (s StakeWeightedCut) Validators() []byte {
	return s.obj.Bytes(OffsetStakeWeightedCut_Validators)
}

// Bytes returns the underlying ZAP buffer.
func (s StakeWeightedCut) Bytes() []byte {
	if s.msg == nil {
		return nil
	}
	return s.msg.Bytes()
}

// IsZero reports whether the cut is the zero value.
func (s StakeWeightedCut) IsZero() bool { return s.msg == nil }
