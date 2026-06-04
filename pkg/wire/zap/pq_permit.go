// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"github.com/luxfi/zap"
)

// PQPermit is the LP-182 schema 0x0B wire view of a PQ-native ERC-2612
// permit replacement (protocol/auth/permit.go::PQPermit).
//
// One owner authorises one spender to draw up to Value before Deadline,
// against OwnerAccountID's monotonic Nonce. Every byte the wallet signs
// over is bound into Digest() at the transcript layer via TupleHash256;
// the wire schema transports the bytes.
type PQPermit struct {
	msg *zap.Message
	obj zap.Object
}

const (
	OffsetPQPermit_Version           = 0  // uint16
	OffsetPQPermit_ProfileID         = 2  // uint8
	OffsetPQPermit_AuthSchemeID      = 3  // uint8
	OffsetPQPermit_HashSuiteID       = 4  // uint8
	// 3 bytes pad
	OffsetPQPermit_ChainID           = 8  // uint32
	OffsetPQPermit_Nonce             = 16 // uint64
	OffsetPQPermit_Deadline          = 24 // uint64
	OffsetPQPermit_VerifyingContract = 32 // bytes (48B)
	OffsetPQPermit_OwnerAccountID    = 40 // bytes (48B)
	OffsetPQPermit_Spender           = 48 // bytes (48B)
	OffsetPQPermit_Value             = 56 // bytes (32B)
	OffsetPQPermit_Signature         = 64 // bytes
	SizePQPermit                     = 72
)

// PQPermitFields carries the per-field input set.
type PQPermitFields struct {
	Version           uint16
	ProfileID         uint8
	ChainID           uint32
	VerifyingContract [48]byte
	OwnerAccountID    [48]byte
	Spender           [48]byte
	Value             [32]byte
	Nonce             uint64
	Deadline          uint64
	AuthSchemeID      uint8
	HashSuiteID       uint8
	Signature         []byte
}

// WrapPQPermit parses b and returns the typed view.
func WrapPQPermit(b []byte) (PQPermit, error) {
	msg, err := zap.Parse(b)
	if err != nil {
		return PQPermit{}, err
	}
	obj := msg.Root()
	if obj.IsNull() {
		return PQPermit{}, ErrInvalidQuasarCert
	}
	return PQPermit{msg: msg, obj: obj}, nil
}

// NewPQPermit builds a PQ-permit wire buffer.
func NewPQPermit(f PQPermitFields) PQPermit {
	b := zap.NewBuilder(512)
	ob := b.StartObject(SizePQPermit)
	ob.SetUint16(OffsetPQPermit_Version, f.Version)
	ob.SetUint8(OffsetPQPermit_ProfileID, f.ProfileID)
	ob.SetUint8(OffsetPQPermit_AuthSchemeID, f.AuthSchemeID)
	ob.SetUint8(OffsetPQPermit_HashSuiteID, f.HashSuiteID)
	ob.SetUint32(OffsetPQPermit_ChainID, f.ChainID)
	ob.SetUint64(OffsetPQPermit_Nonce, f.Nonce)
	ob.SetUint64(OffsetPQPermit_Deadline, f.Deadline)
	ob.SetBytes(OffsetPQPermit_VerifyingContract, f.VerifyingContract[:])
	ob.SetBytes(OffsetPQPermit_OwnerAccountID, f.OwnerAccountID[:])
	ob.SetBytes(OffsetPQPermit_Spender, f.Spender[:])
	ob.SetBytes(OffsetPQPermit_Value, f.Value[:])
	ob.SetBytes(OffsetPQPermit_Signature, f.Signature)
	ob.FinishAsRoot()
	buf := b.Finish()
	msg, err := zap.Parse(buf)
	if err != nil {
		panic("zap: NewPQPermit produced unparseable bytes: " + err.Error())
	}
	return PQPermit{msg: msg, obj: msg.Root()}
}

func (p PQPermit) Version() uint16      { return p.obj.Uint16(OffsetPQPermit_Version) }
func (p PQPermit) ProfileID() uint8     { return p.obj.Uint8(OffsetPQPermit_ProfileID) }
func (p PQPermit) AuthSchemeID() uint8  { return p.obj.Uint8(OffsetPQPermit_AuthSchemeID) }
func (p PQPermit) HashSuiteID() uint8   { return p.obj.Uint8(OffsetPQPermit_HashSuiteID) }
func (p PQPermit) ChainID() uint32      { return p.obj.Uint32(OffsetPQPermit_ChainID) }
func (p PQPermit) Nonce() uint64        { return p.obj.Uint64(OffsetPQPermit_Nonce) }
func (p PQPermit) Deadline() uint64     { return p.obj.Uint64(OffsetPQPermit_Deadline) }

func (p PQPermit) VerifyingContract() [48]byte {
	var out [48]byte
	copy(out[:], p.obj.Bytes(OffsetPQPermit_VerifyingContract))
	return out
}
func (p PQPermit) OwnerAccountID() [48]byte {
	var out [48]byte
	copy(out[:], p.obj.Bytes(OffsetPQPermit_OwnerAccountID))
	return out
}
func (p PQPermit) Spender() [48]byte {
	var out [48]byte
	copy(out[:], p.obj.Bytes(OffsetPQPermit_Spender))
	return out
}
func (p PQPermit) Value() [32]byte {
	var out [32]byte
	copy(out[:], p.obj.Bytes(OffsetPQPermit_Value))
	return out
}
func (p PQPermit) Signature() []byte { return p.obj.Bytes(OffsetPQPermit_Signature) }

// Bytes returns the underlying ZAP buffer.
func (p PQPermit) Bytes() []byte {
	if p.msg == nil {
		return nil
	}
	return p.msg.Bytes()
}

// IsZero reports whether the permit is the zero value.
func (p PQPermit) IsZero() bool { return p.msg == nil }
