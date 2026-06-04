// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"github.com/luxfi/zap"
)

// MagnetarAggregateCert is the LP-182 schema 0x04 wire view of a
// magnetar.ValidatorAggregateCert — the SLH-DSA per-validator aggregate
// embedded in the Polaris-profile Magnetar leg.
//
// The wire encodes the four shape fields plus the three parallel
// variable-length payloads (signers concatenated 32 bytes each, pubkeys
// concatenated, sigs concatenated). Per-element byte widths are derived
// from the magnetar.ParamsFor(mode) lookup at the caller — this wire
// layer is shape-agnostic and just transports the bytes.
type MagnetarAggregateCert struct {
	msg *zap.Message
	obj zap.Object
}

const (
	OffsetMagnetarAggregate_Mode    = 0  // uint8
	OffsetMagnetarAggregate_Count   = 1  // uint32
	OffsetMagnetarAggregate_Signers = 8  // bytes
	OffsetMagnetarAggregate_PubKeys = 16 // bytes
	OffsetMagnetarAggregate_Sigs    = 24 // bytes
	SizeMagnetarAggregate           = 32
)

// MagnetarAggregateFields carries the per-field input set.
//
// Signers, PubKeys, and Sigs are concatenated payloads — caller already
// flattens the parallel slices. Encoding the concatenation at the
// build site keeps this schema simple (three byte fields) and lets the
// magnetar/pulsar params layer stay responsible for shape.
type MagnetarAggregateFields struct {
	Mode    uint8
	Count   uint32
	Signers []byte // N * 32 bytes
	PubKeys []byte // N * PublicKeySize(Mode)
	Sigs    []byte // N * SignatureSize(Mode)
}

// WrapMagnetarAggregateCert parses b and returns the typed view.
func WrapMagnetarAggregateCert(b []byte) (MagnetarAggregateCert, error) {
	msg, err := zap.Parse(b)
	if err != nil {
		return MagnetarAggregateCert{}, err
	}
	obj := msg.Root()
	if obj.IsNull() {
		return MagnetarAggregateCert{}, ErrInvalidQuasarCert
	}
	return MagnetarAggregateCert{msg: msg, obj: obj}, nil
}

// NewMagnetarAggregateCert builds a magnetar aggregate wire buffer.
func NewMagnetarAggregateCert(f MagnetarAggregateFields) MagnetarAggregateCert {
	b := zap.NewBuilder(256)
	ob := b.StartObject(SizeMagnetarAggregate)
	ob.SetUint8(OffsetMagnetarAggregate_Mode, f.Mode)
	ob.SetUint32(OffsetMagnetarAggregate_Count, f.Count)
	ob.SetBytes(OffsetMagnetarAggregate_Signers, f.Signers)
	ob.SetBytes(OffsetMagnetarAggregate_PubKeys, f.PubKeys)
	ob.SetBytes(OffsetMagnetarAggregate_Sigs, f.Sigs)
	ob.FinishAsRoot()
	buf := b.Finish()
	msg, err := zap.Parse(buf)
	if err != nil {
		panic("zap: NewMagnetarAggregateCert produced unparseable bytes: " + err.Error())
	}
	return MagnetarAggregateCert{msg: msg, obj: msg.Root()}
}

func (m MagnetarAggregateCert) Mode() uint8 { return m.obj.Uint8(OffsetMagnetarAggregate_Mode) }
func (m MagnetarAggregateCert) Count() uint32 {
	return m.obj.Uint32(OffsetMagnetarAggregate_Count)
}
func (m MagnetarAggregateCert) Signers() []byte {
	return m.obj.Bytes(OffsetMagnetarAggregate_Signers)
}
func (m MagnetarAggregateCert) PubKeys() []byte {
	return m.obj.Bytes(OffsetMagnetarAggregate_PubKeys)
}
func (m MagnetarAggregateCert) Sigs() []byte {
	return m.obj.Bytes(OffsetMagnetarAggregate_Sigs)
}

// Bytes returns the underlying ZAP buffer.
func (m MagnetarAggregateCert) Bytes() []byte {
	if m.msg == nil {
		return nil
	}
	return m.msg.Bytes()
}

// IsZero reports whether the cert is the zero value.
func (m MagnetarAggregateCert) IsZero() bool { return m.msg == nil }
