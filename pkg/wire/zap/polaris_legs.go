// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"github.com/luxfi/zap"
)

// PolarisLegs is the LP-182 schema 0x05 wire view of the
// polaris-composition input (protocol/quasar/polaris.go::PolarisLegs).
//
// Wire transports already-serialized per-leg signature bytes plus the
// epoch / finality / validator-count metadata. The struct fields in
// protocol/quasar/polaris.go carry typed signatures (pulsar.Signature
// etc.); the wire's job is to transport the bytes those types produce.
type PolarisLegs struct {
	msg *zap.Message
	obj zap.Object
}

const (
	OffsetPolarisLegs_BLS              = 0
	OffsetPolarisLegs_Pulsar           = 8
	OffsetPolarisLegs_Corona           = 16
	OffsetPolarisLegs_MagnetarBytes    = 24
	OffsetPolarisLegs_MLDSARollup      = 32
	OffsetPolarisLegs_Epoch            = 40
	OffsetPolarisLegs_FinalityUnixNano = 48
	OffsetPolarisLegs_Validators       = 56
	SizePolarisLegs                    = 60
)

// PolarisLegsFields carries the per-field input set.
type PolarisLegsFields struct {
	BLS              []byte
	Pulsar           []byte
	Corona           []byte
	MagnetarBytes    []byte // already-encoded ValidatorAggregateCert (schema 0x04)
	MLDSARollup      []byte
	Epoch            uint64
	FinalityUnixNano int64
	Validators       uint32
}

// WrapPolarisLegs parses b and returns the typed view.
func WrapPolarisLegs(b []byte) (PolarisLegs, error) {
	msg, err := zap.Parse(b)
	if err != nil {
		return PolarisLegs{}, err
	}
	obj := msg.Root()
	if obj.IsNull() {
		return PolarisLegs{}, ErrInvalidQuasarCert
	}
	return PolarisLegs{msg: msg, obj: obj}, nil
}

// NewPolarisLegs builds a polaris-legs wire buffer.
func NewPolarisLegs(f PolarisLegsFields) PolarisLegs {
	b := zap.NewBuilder(256)
	ob := b.StartObject(SizePolarisLegs)
	ob.SetBytes(OffsetPolarisLegs_BLS, f.BLS)
	ob.SetBytes(OffsetPolarisLegs_Pulsar, f.Pulsar)
	ob.SetBytes(OffsetPolarisLegs_Corona, f.Corona)
	ob.SetBytes(OffsetPolarisLegs_MagnetarBytes, f.MagnetarBytes)
	ob.SetBytes(OffsetPolarisLegs_MLDSARollup, f.MLDSARollup)
	ob.SetUint64(OffsetPolarisLegs_Epoch, f.Epoch)
	ob.SetInt64(OffsetPolarisLegs_FinalityUnixNano, f.FinalityUnixNano)
	ob.SetUint32(OffsetPolarisLegs_Validators, f.Validators)
	ob.FinishAsRoot()
	buf := b.Finish()
	msg, err := zap.Parse(buf)
	if err != nil {
		panic("zap: NewPolarisLegs produced unparseable bytes: " + err.Error())
	}
	return PolarisLegs{msg: msg, obj: msg.Root()}
}

func (p PolarisLegs) BLS() []byte           { return p.obj.Bytes(OffsetPolarisLegs_BLS) }
func (p PolarisLegs) Pulsar() []byte        { return p.obj.Bytes(OffsetPolarisLegs_Pulsar) }
func (p PolarisLegs) Corona() []byte        { return p.obj.Bytes(OffsetPolarisLegs_Corona) }
func (p PolarisLegs) MagnetarBytes() []byte { return p.obj.Bytes(OffsetPolarisLegs_MagnetarBytes) }
func (p PolarisLegs) MLDSARollup() []byte   { return p.obj.Bytes(OffsetPolarisLegs_MLDSARollup) }
func (p PolarisLegs) Epoch() uint64         { return p.obj.Uint64(OffsetPolarisLegs_Epoch) }
func (p PolarisLegs) FinalityUnixNano() int64 {
	return p.obj.Int64(OffsetPolarisLegs_FinalityUnixNano)
}
func (p PolarisLegs) Validators() uint32 {
	return p.obj.Uint32(OffsetPolarisLegs_Validators)
}

// Bytes returns the underlying ZAP buffer.
func (p PolarisLegs) Bytes() []byte {
	if p.msg == nil {
		return nil
	}
	return p.msg.Bytes()
}

// IsZero reports whether the legs are the zero value.
func (p PolarisLegs) IsZero() bool { return p.msg == nil }
