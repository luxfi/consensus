// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"github.com/luxfi/zap"
)

// QuasarSig is the LP-182 schema 0x06 wire view of a per-validator
// triple-signed Quasar signature (protocol/quasar/types.go::QuasarSignature).
//
// Carries the three parallel proof-paths a validator emits during
// consensus: BLS classical, Corona threshold (Ring-LWE), and ML-DSA
// identity proof.
type QuasarSig struct {
	msg *zap.Message
	obj zap.Object
}

const (
	OffsetQuasarSig_BLSSignature     = 0  // bytes
	OffsetQuasarSig_BLSValidatorID   = 8  // bytes (string)
	OffsetQuasarSig_BLSIsThreshold   = 16 // uint8 (bool)
	OffsetQuasarSig_BLSSignerIndex   = 17 // int32 (4)
	// pad to 8-byte align
	OffsetQuasarSig_CoronaSignature  = 24 // bytes
	OffsetQuasarSig_CoronaValidatorID = 32 // bytes
	OffsetQuasarSig_CoronaIsThreshold = 40 // uint8 (bool)
	OffsetQuasarSig_CoronaSignerIndex = 41 // int32 (4)
	OffsetQuasarSig_CoronaRound       = 45 // int32 (4)
	// pad
	OffsetQuasarSig_MLDSA            = 56 // bytes
	SizeQuasarSig                    = 64
)

// QuasarSigFields carries the per-field input set.
type QuasarSigFields struct {
	BLSSignature      []byte
	BLSValidatorID    string
	BLSIsThreshold    bool
	BLSSignerIndex    int32
	CoronaSignature   []byte
	CoronaValidatorID string
	CoronaIsThreshold bool
	CoronaSignerIndex int32
	CoronaRound       int32
	MLDSA             []byte
}

// WrapQuasarSig parses b and returns the typed view.
func WrapQuasarSig(b []byte) (QuasarSig, error) {
	msg, err := zap.Parse(b)
	if err != nil {
		return QuasarSig{}, err
	}
	obj := msg.Root()
	if obj.IsNull() {
		return QuasarSig{}, ErrInvalidQuasarCert
	}
	return QuasarSig{msg: msg, obj: obj}, nil
}

// NewQuasarSig builds a quasar-signature wire buffer.
func NewQuasarSig(f QuasarSigFields) QuasarSig {
	b := zap.NewBuilder(256)
	ob := b.StartObject(SizeQuasarSig)
	ob.SetBytes(OffsetQuasarSig_BLSSignature, f.BLSSignature)
	ob.SetText(OffsetQuasarSig_BLSValidatorID, f.BLSValidatorID)
	ob.SetBool(OffsetQuasarSig_BLSIsThreshold, f.BLSIsThreshold)
	ob.SetInt32(OffsetQuasarSig_BLSSignerIndex, f.BLSSignerIndex)
	ob.SetBytes(OffsetQuasarSig_CoronaSignature, f.CoronaSignature)
	ob.SetText(OffsetQuasarSig_CoronaValidatorID, f.CoronaValidatorID)
	ob.SetBool(OffsetQuasarSig_CoronaIsThreshold, f.CoronaIsThreshold)
	ob.SetInt32(OffsetQuasarSig_CoronaSignerIndex, f.CoronaSignerIndex)
	ob.SetInt32(OffsetQuasarSig_CoronaRound, f.CoronaRound)
	ob.SetBytes(OffsetQuasarSig_MLDSA, f.MLDSA)
	ob.FinishAsRoot()
	buf := b.Finish()
	msg, err := zap.Parse(buf)
	if err != nil {
		panic("zap: NewQuasarSig produced unparseable bytes: " + err.Error())
	}
	return QuasarSig{msg: msg, obj: msg.Root()}
}

func (q QuasarSig) BLSSignature() []byte {
	return q.obj.Bytes(OffsetQuasarSig_BLSSignature)
}
func (q QuasarSig) BLSValidatorID() string {
	return q.obj.Text(OffsetQuasarSig_BLSValidatorID)
}
func (q QuasarSig) BLSIsThreshold() bool {
	return q.obj.Bool(OffsetQuasarSig_BLSIsThreshold)
}
func (q QuasarSig) BLSSignerIndex() int32 {
	return q.obj.Int32(OffsetQuasarSig_BLSSignerIndex)
}
func (q QuasarSig) CoronaSignature() []byte {
	return q.obj.Bytes(OffsetQuasarSig_CoronaSignature)
}
func (q QuasarSig) CoronaValidatorID() string {
	return q.obj.Text(OffsetQuasarSig_CoronaValidatorID)
}
func (q QuasarSig) CoronaIsThreshold() bool {
	return q.obj.Bool(OffsetQuasarSig_CoronaIsThreshold)
}
func (q QuasarSig) CoronaSignerIndex() int32 {
	return q.obj.Int32(OffsetQuasarSig_CoronaSignerIndex)
}
func (q QuasarSig) CoronaRound() int32 {
	return q.obj.Int32(OffsetQuasarSig_CoronaRound)
}
func (q QuasarSig) MLDSA() []byte { return q.obj.Bytes(OffsetQuasarSig_MLDSA) }

// Bytes returns the underlying ZAP buffer.
func (q QuasarSig) Bytes() []byte {
	if q.msg == nil {
		return nil
	}
	return q.msg.Bytes()
}

// IsZero reports whether the sig is the zero value.
func (q QuasarSig) IsZero() bool { return q.msg == nil }
