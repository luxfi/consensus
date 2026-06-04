// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"github.com/luxfi/zap"
)

// TxAuthEnvelope is the LP-182 schema 0x0A wire view of a transaction
// authorization envelope (protocol/auth/tx_envelope.go::TxAuthEnvelope).
//
// Every byte the wallet signs over is bound into SigningDigest at the
// transcript layer (cSHAKE256 + TupleHash256). The wire schema just
// transports the bytes. The fixed-section layout mirrors the canonical
// big-endian codec the prior hand-rolled binary used.
type TxAuthEnvelope struct {
	msg *zap.Message
	obj zap.Object
}

const (
	OffsetTxAuth_Version          = 0   // uint16
	OffsetTxAuth_ProfileID        = 2   // uint8
	OffsetTxAuth_ChainID          = 4   // uint32
	OffsetTxAuth_NetworkID        = 8   // uint32
	OffsetTxAuth_Nonce            = 16  // uint64
	OffsetTxAuth_ExpiryHeight     = 24  // uint64
	OffsetTxAuth_GasLimit         = 32  // uint64
	OffsetTxAuth_WalletSchemeID   = 40  // uint8
	OffsetTxAuth_HashSuiteID      = 41  // uint8
	// 6 bytes pad to 8-byte align
	OffsetTxAuth_AccountID        = 48  // bytes (48B)
	OffsetTxAuth_FeePayer         = 56  // bytes (48B)
	OffsetTxAuth_MaxFee           = 64  // bytes (32B)
	OffsetTxAuth_CallRoot         = 72  // bytes (32B)
	OffsetTxAuth_AccessListRoot   = 80  // bytes (32B)
	OffsetTxAuth_ZIdentityRoot    = 88  // bytes (32B)
	OffsetTxAuth_AccountStateRoot = 96  // bytes (32B)
	OffsetTxAuth_PublicKeyRef     = 104 // bytes (32B)
	OffsetTxAuth_Signature        = 112 // bytes
	SizeTxAuthEnvelope            = 120
)

// TxAuthEnvelopeFields carries the per-field input set.
type TxAuthEnvelopeFields struct {
	Version          uint16
	ProfileID        uint8
	ChainID          uint32
	NetworkID        uint32
	AccountID        [48]byte
	Nonce            uint64
	ExpiryHeight     uint64
	WalletSchemeID   uint8
	HashSuiteID      uint8
	FeePayer         [48]byte
	GasLimit         uint64
	MaxFee           [32]byte
	CallRoot         [32]byte
	AccessListRoot   [32]byte
	ZIdentityRoot    [32]byte
	AccountStateRoot [32]byte
	PublicKeyRef     [32]byte
	Signature        []byte
}

// WrapTxAuthEnvelope parses b and returns the typed view.
func WrapTxAuthEnvelope(b []byte) (TxAuthEnvelope, error) {
	msg, err := zap.Parse(b)
	if err != nil {
		return TxAuthEnvelope{}, err
	}
	obj := msg.Root()
	if obj.IsNull() {
		return TxAuthEnvelope{}, ErrInvalidQuasarCert
	}
	return TxAuthEnvelope{msg: msg, obj: obj}, nil
}

// NewTxAuthEnvelope builds a tx-auth envelope wire buffer.
func NewTxAuthEnvelope(f TxAuthEnvelopeFields) TxAuthEnvelope {
	b := zap.NewBuilder(512)
	ob := b.StartObject(SizeTxAuthEnvelope)
	ob.SetUint16(OffsetTxAuth_Version, f.Version)
	ob.SetUint8(OffsetTxAuth_ProfileID, f.ProfileID)
	ob.SetUint32(OffsetTxAuth_ChainID, f.ChainID)
	ob.SetUint32(OffsetTxAuth_NetworkID, f.NetworkID)
	ob.SetUint64(OffsetTxAuth_Nonce, f.Nonce)
	ob.SetUint64(OffsetTxAuth_ExpiryHeight, f.ExpiryHeight)
	ob.SetUint64(OffsetTxAuth_GasLimit, f.GasLimit)
	ob.SetUint8(OffsetTxAuth_WalletSchemeID, f.WalletSchemeID)
	ob.SetUint8(OffsetTxAuth_HashSuiteID, f.HashSuiteID)
	ob.SetBytes(OffsetTxAuth_AccountID, f.AccountID[:])
	ob.SetBytes(OffsetTxAuth_FeePayer, f.FeePayer[:])
	ob.SetBytes(OffsetTxAuth_MaxFee, f.MaxFee[:])
	ob.SetBytes(OffsetTxAuth_CallRoot, f.CallRoot[:])
	ob.SetBytes(OffsetTxAuth_AccessListRoot, f.AccessListRoot[:])
	ob.SetBytes(OffsetTxAuth_ZIdentityRoot, f.ZIdentityRoot[:])
	ob.SetBytes(OffsetTxAuth_AccountStateRoot, f.AccountStateRoot[:])
	ob.SetBytes(OffsetTxAuth_PublicKeyRef, f.PublicKeyRef[:])
	ob.SetBytes(OffsetTxAuth_Signature, f.Signature)
	ob.FinishAsRoot()
	buf := b.Finish()
	msg, err := zap.Parse(buf)
	if err != nil {
		panic("zap: NewTxAuthEnvelope produced unparseable bytes: " + err.Error())
	}
	return TxAuthEnvelope{msg: msg, obj: msg.Root()}
}

func (t TxAuthEnvelope) Version() uint16      { return t.obj.Uint16(OffsetTxAuth_Version) }
func (t TxAuthEnvelope) ProfileID() uint8     { return t.obj.Uint8(OffsetTxAuth_ProfileID) }
func (t TxAuthEnvelope) ChainID() uint32      { return t.obj.Uint32(OffsetTxAuth_ChainID) }
func (t TxAuthEnvelope) NetworkID() uint32    { return t.obj.Uint32(OffsetTxAuth_NetworkID) }
func (t TxAuthEnvelope) Nonce() uint64        { return t.obj.Uint64(OffsetTxAuth_Nonce) }
func (t TxAuthEnvelope) ExpiryHeight() uint64 { return t.obj.Uint64(OffsetTxAuth_ExpiryHeight) }
func (t TxAuthEnvelope) GasLimit() uint64     { return t.obj.Uint64(OffsetTxAuth_GasLimit) }
func (t TxAuthEnvelope) WalletSchemeID() uint8 {
	return t.obj.Uint8(OffsetTxAuth_WalletSchemeID)
}
func (t TxAuthEnvelope) HashSuiteID() uint8 { return t.obj.Uint8(OffsetTxAuth_HashSuiteID) }

func (t TxAuthEnvelope) AccountID() [48]byte {
	var out [48]byte
	copy(out[:], t.obj.Bytes(OffsetTxAuth_AccountID))
	return out
}
func (t TxAuthEnvelope) FeePayer() [48]byte {
	var out [48]byte
	copy(out[:], t.obj.Bytes(OffsetTxAuth_FeePayer))
	return out
}
func (t TxAuthEnvelope) MaxFee() [32]byte {
	var out [32]byte
	copy(out[:], t.obj.Bytes(OffsetTxAuth_MaxFee))
	return out
}
func (t TxAuthEnvelope) CallRoot() [32]byte {
	var out [32]byte
	copy(out[:], t.obj.Bytes(OffsetTxAuth_CallRoot))
	return out
}
func (t TxAuthEnvelope) AccessListRoot() [32]byte {
	var out [32]byte
	copy(out[:], t.obj.Bytes(OffsetTxAuth_AccessListRoot))
	return out
}
func (t TxAuthEnvelope) ZIdentityRoot() [32]byte {
	var out [32]byte
	copy(out[:], t.obj.Bytes(OffsetTxAuth_ZIdentityRoot))
	return out
}
func (t TxAuthEnvelope) AccountStateRoot() [32]byte {
	var out [32]byte
	copy(out[:], t.obj.Bytes(OffsetTxAuth_AccountStateRoot))
	return out
}
func (t TxAuthEnvelope) PublicKeyRef() [32]byte {
	var out [32]byte
	copy(out[:], t.obj.Bytes(OffsetTxAuth_PublicKeyRef))
	return out
}

func (t TxAuthEnvelope) Signature() []byte { return t.obj.Bytes(OffsetTxAuth_Signature) }

// Bytes returns the underlying ZAP buffer.
func (t TxAuthEnvelope) Bytes() []byte {
	if t.msg == nil {
		return nil
	}
	return t.msg.Bytes()
}

// IsZero reports whether the envelope is the zero value.
func (t TxAuthEnvelope) IsZero() bool { return t.msg == nil }
