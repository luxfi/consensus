// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"github.com/luxfi/zap"
)

// WitnessProof is the LP-182 schema 0x03 wire view of a Verkle witness
// (protocol/quasar/witness.go::WitnessProof). Carries the Verkle
// commitment + IPA opening + the PQ finality signature.
type WitnessProof struct {
	msg *zap.Message
	obj zap.Object
}

const (
	OffsetWitnessProof_Commitment   = 0
	OffsetWitnessProof_Path         = 8
	OffsetWitnessProof_OpeningProof = 16
	OffsetWitnessProof_PQSignature  = 24
	OffsetWitnessProof_BLSAggregate = 32
	OffsetWitnessProof_CoronaBits   = 40
	OffsetWitnessProof_ValidatorSet = 48
	OffsetWitnessProof_StateRoot    = 56
	OffsetWitnessProof_BlockHeight  = 64
	OffsetWitnessProof_Timestamp    = 72
	SizeWitnessProof                = 80
)

// WitnessProofFields carries the per-field input set for NewWitnessProof.
type WitnessProofFields struct {
	Commitment   []byte
	Path         []byte
	OpeningProof []byte
	PQSignature  []byte
	BLSAggregate []byte
	CoronaBits   []byte
	ValidatorSet []byte
	StateRoot    []byte
	BlockHeight  uint64
	Timestamp    uint64
}

// WrapWitnessProof parses b as a ZAP message and returns the typed
// witness view.
func WrapWitnessProof(b []byte) (WitnessProof, error) {
	msg, err := zap.Parse(b)
	if err != nil {
		return WitnessProof{}, err
	}
	obj := msg.Root()
	if obj.IsNull() {
		return WitnessProof{}, ErrInvalidQuasarCert
	}
	return WitnessProof{msg: msg, obj: obj}, nil
}

// NewWitnessProof builds a witness wire buffer from fields.
func NewWitnessProof(f WitnessProofFields) WitnessProof {
	b := zap.NewBuilder(512)
	ob := b.StartObject(SizeWitnessProof)
	ob.SetBytes(OffsetWitnessProof_Commitment, f.Commitment)
	ob.SetBytes(OffsetWitnessProof_Path, f.Path)
	ob.SetBytes(OffsetWitnessProof_OpeningProof, f.OpeningProof)
	ob.SetBytes(OffsetWitnessProof_PQSignature, f.PQSignature)
	ob.SetBytes(OffsetWitnessProof_BLSAggregate, f.BLSAggregate)
	ob.SetBytes(OffsetWitnessProof_CoronaBits, f.CoronaBits)
	ob.SetBytes(OffsetWitnessProof_ValidatorSet, f.ValidatorSet)
	ob.SetBytes(OffsetWitnessProof_StateRoot, f.StateRoot)
	ob.SetUint64(OffsetWitnessProof_BlockHeight, f.BlockHeight)
	ob.SetUint64(OffsetWitnessProof_Timestamp, f.Timestamp)
	ob.FinishAsRoot()
	buf := b.Finish()
	msg, err := zap.Parse(buf)
	if err != nil {
		panic("zap: NewWitnessProof produced unparseable bytes: " + err.Error())
	}
	return WitnessProof{msg: msg, obj: msg.Root()}
}

func (w WitnessProof) Commitment() []byte {
	return w.obj.Bytes(OffsetWitnessProof_Commitment)
}
func (w WitnessProof) Path() []byte { return w.obj.Bytes(OffsetWitnessProof_Path) }
func (w WitnessProof) OpeningProof() []byte {
	return w.obj.Bytes(OffsetWitnessProof_OpeningProof)
}
func (w WitnessProof) PQSignature() []byte {
	return w.obj.Bytes(OffsetWitnessProof_PQSignature)
}
func (w WitnessProof) BLSAggregate() []byte {
	return w.obj.Bytes(OffsetWitnessProof_BLSAggregate)
}
func (w WitnessProof) CoronaBits() []byte {
	return w.obj.Bytes(OffsetWitnessProof_CoronaBits)
}
func (w WitnessProof) ValidatorSet() []byte {
	return w.obj.Bytes(OffsetWitnessProof_ValidatorSet)
}
func (w WitnessProof) StateRoot() []byte {
	return w.obj.Bytes(OffsetWitnessProof_StateRoot)
}
func (w WitnessProof) BlockHeight() uint64 {
	return w.obj.Uint64(OffsetWitnessProof_BlockHeight)
}
func (w WitnessProof) Timestamp() uint64 {
	return w.obj.Uint64(OffsetWitnessProof_Timestamp)
}

// Bytes returns the underlying ZAP buffer.
func (w WitnessProof) Bytes() []byte {
	if w.msg == nil {
		return nil
	}
	return w.msg.Bytes()
}

// IsZero reports whether the witness is the zero value.
func (w WitnessProof) IsZero() bool { return w.msg == nil }
