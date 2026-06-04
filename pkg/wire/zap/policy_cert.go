// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"github.com/luxfi/zap"
)

// PolicyCert is the LP-182 schema 0x0D wire view of a finality-policy
// certificate (pkg/wire/policies.go::Certificate / pkg/wire/wire.go).
//
// Used by every FinalityPolicy implementation (None, Quorum, Sample,
// L1, Quantum) to ship a policy-typed proof + signer bitmap across the
// wire. The Proof field is the policy-specific payload (e.g. a
// QuasarCert wire-bytes for PolicyQuantum, an L1 inclusion proof for
// PolicyL1Inclusion, aggregated BLS bytes for PolicyQuorum).
type PolicyCert struct {
	msg *zap.Message
	obj zap.Object
}

const (
	OffsetPolicyCert_CandidateID = 0  // bytes (32B)
	OffsetPolicyCert_Height      = 8  // uint64
	OffsetPolicyCert_PolicyID    = 16 // uint32
	OffsetPolicyCert_Proof       = 24 // bytes
	OffsetPolicyCert_Signers     = 32 // bytes
	SizePolicyCert               = 40
)

// PolicyCertFields carries the per-field input set.
type PolicyCertFields struct {
	CandidateID [32]byte
	Height      uint64
	PolicyID    uint32
	Proof       []byte
	Signers     []byte
}

// WrapPolicyCert parses b and returns the typed view.
func WrapPolicyCert(b []byte) (PolicyCert, error) {
	msg, err := zap.Parse(b)
	if err != nil {
		return PolicyCert{}, err
	}
	obj := msg.Root()
	if obj.IsNull() {
		return PolicyCert{}, ErrInvalidQuasarCert
	}
	return PolicyCert{msg: msg, obj: obj}, nil
}

// NewPolicyCert builds a policy-cert wire buffer.
func NewPolicyCert(f PolicyCertFields) PolicyCert {
	b := zap.NewBuilder(512)
	ob := b.StartObject(SizePolicyCert)
	ob.SetBytes(OffsetPolicyCert_CandidateID, f.CandidateID[:])
	ob.SetUint64(OffsetPolicyCert_Height, f.Height)
	ob.SetUint32(OffsetPolicyCert_PolicyID, f.PolicyID)
	ob.SetBytes(OffsetPolicyCert_Proof, f.Proof)
	ob.SetBytes(OffsetPolicyCert_Signers, f.Signers)
	ob.FinishAsRoot()
	buf := b.Finish()
	msg, err := zap.Parse(buf)
	if err != nil {
		panic("zap: NewPolicyCert produced unparseable bytes: " + err.Error())
	}
	return PolicyCert{msg: msg, obj: msg.Root()}
}

func (p PolicyCert) CandidateID() [32]byte {
	var out [32]byte
	copy(out[:], p.obj.Bytes(OffsetPolicyCert_CandidateID))
	return out
}
func (p PolicyCert) Height() uint64   { return p.obj.Uint64(OffsetPolicyCert_Height) }
func (p PolicyCert) PolicyID() uint32 { return p.obj.Uint32(OffsetPolicyCert_PolicyID) }
func (p PolicyCert) Proof() []byte    { return p.obj.Bytes(OffsetPolicyCert_Proof) }
func (p PolicyCert) Signers() []byte  { return p.obj.Bytes(OffsetPolicyCert_Signers) }

// Bytes returns the underlying ZAP buffer.
func (p PolicyCert) Bytes() []byte {
	if p.msg == nil {
		return nil
	}
	return p.msg.Bytes()
}

// IsZero reports whether the cert is the zero value.
func (p PolicyCert) IsZero() bool { return p.msg == nil }
