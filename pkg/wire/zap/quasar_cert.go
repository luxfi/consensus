// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"errors"

	"github.com/luxfi/zap"
)

// QuasarCert is the LP-182 schema 0x01 wire view of a Quasar finality
// certificate. The struct embeds a parsed *zap.Message buffer; field
// accessors are offset reads on that buffer. `Bytes()` returns the
// underlying buffer with no copy.
//
// Polaris five-leg layout (matches the prior CertSchemeQuasar=0x05
// hand-rolled binary, now decomplected into ZAP fixed-section + variable
// section pointers):
//
//	bls                   bytes (BLS-12-381 aggregate; empty in pure-PQ)
//	corona                bytes (Ring-LWE threshold sig)
//	pulsar                bytes (Module-LWE threshold sig)
//	magnetar              bytes (SLH-DSA per-validator aggregate)
//	mldsaRollup           bytes (per-validator ML-DSA-65, len-prefixed)
//	epoch                 uint64 (consensus epoch)
//	finalityUnixNano      int64  (assembly time, ns since unix epoch)
//	validators            uint32 (count of signing validators)
type QuasarCert struct {
	msg *zap.Message
	obj zap.Object
}

// Fixed-section offsets. Each bytes field is an 8-byte slot (4-byte
// relative-offset + 4-byte length). Each integer field is its natural
// width. The fixed section is 60 bytes; variable-length payloads follow.
const (
	OffsetQuasarCert_BLS              = 0
	OffsetQuasarCert_Corona           = 8
	OffsetQuasarCert_Pulsar           = 16
	OffsetQuasarCert_Magnetar         = 24
	OffsetQuasarCert_MLDSARollup      = 32
	OffsetQuasarCert_Epoch            = 40
	OffsetQuasarCert_FinalityUnixNano = 48
	OffsetQuasarCert_Validators       = 56
	SizeQuasarCert                    = 60
)

// ErrInvalidQuasarCert is returned by WrapQuasarCert when the input
// bytes do not parse as a ZAP message or the root object is null.
var ErrInvalidQuasarCert = errors.New("zap: invalid QuasarCert wire bytes")

// WrapQuasarCert parses b as a ZAP message and returns a zero-copy
// QuasarCert window. The returned value is read-only; mutating b after
// this call mutates the QuasarCert.
func WrapQuasarCert(b []byte) (QuasarCert, error) {
	msg, err := zap.Parse(b)
	if err != nil {
		return QuasarCert{}, err
	}
	obj := msg.Root()
	if obj.IsNull() {
		return QuasarCert{}, ErrInvalidQuasarCert
	}
	return QuasarCert{msg: msg, obj: obj}, nil
}

// NewQuasarCert writes a new QuasarCert into a fresh ZAP buffer and
// returns the typed window over it. `c.Bytes()` returns the buffer; the
// transcript layer and identity layer consume those bytes directly.
//
// Variable-length payloads (bls, corona, pulsar, magnetar, mldsaRollup)
// may be nil; the wire encodes a null pointer in that slot. The five
// integer fields are always written at their fixed offsets.
func NewQuasarCert(
	bls, corona, pulsar, magnetar, mldsaRollup []byte,
	epoch uint64,
	finalityUnixNano int64,
	validators uint32,
) QuasarCert {
	b := zap.NewBuilder(256)
	ob := b.StartObject(SizeQuasarCert)
	ob.SetBytes(OffsetQuasarCert_BLS, bls)
	ob.SetBytes(OffsetQuasarCert_Corona, corona)
	ob.SetBytes(OffsetQuasarCert_Pulsar, pulsar)
	ob.SetBytes(OffsetQuasarCert_Magnetar, magnetar)
	ob.SetBytes(OffsetQuasarCert_MLDSARollup, mldsaRollup)
	ob.SetUint64(OffsetQuasarCert_Epoch, epoch)
	ob.SetInt64(OffsetQuasarCert_FinalityUnixNano, finalityUnixNano)
	ob.SetUint32(OffsetQuasarCert_Validators, validators)
	ob.FinishAsRoot()
	buf := b.Finish()
	// Re-parse so the returned QuasarCert points at the finalized buffer.
	msg, err := zap.Parse(buf)
	if err != nil {
		// Builder produces well-formed messages by construction; a parse
		// failure here is an unrecoverable programmer error.
		panic("zap: NewQuasarCert produced unparseable bytes: " + err.Error())
	}
	return QuasarCert{msg: msg, obj: msg.Root()}
}

// BLS returns the BLS-12-381 aggregate signature bytes. Empty in pure-PQ
// posture.
func (c QuasarCert) BLS() []byte { return c.obj.Bytes(OffsetQuasarCert_BLS) }

// Corona returns the Ring-LWE threshold signature bytes.
func (c QuasarCert) Corona() []byte { return c.obj.Bytes(OffsetQuasarCert_Corona) }

// Pulsar returns the Module-LWE threshold signature bytes.
func (c QuasarCert) Pulsar() []byte { return c.obj.Bytes(OffsetQuasarCert_Pulsar) }

// Magnetar returns the SLH-DSA per-validator aggregate cert bytes
// (Polaris profile only; empty otherwise).
func (c QuasarCert) Magnetar() []byte { return c.obj.Bytes(OffsetQuasarCert_Magnetar) }

// MLDSARollup returns the per-validator ML-DSA-65 rollup bytes — either
// a STARK/Groth16 succinct proof, or a concatenation of per-validator
// ML-DSA-65 signatures with 4-byte length prefixes.
func (c QuasarCert) MLDSARollup() []byte { return c.obj.Bytes(OffsetQuasarCert_MLDSARollup) }

// Epoch returns the consensus epoch this cert finalises.
func (c QuasarCert) Epoch() uint64 { return c.obj.Uint64(OffsetQuasarCert_Epoch) }

// FinalityUnixNano returns the assembly wall-clock time (ns since unix
// epoch). Stored as int64 so negative values (pre-1970) round-trip.
func (c QuasarCert) FinalityUnixNano() int64 {
	return c.obj.Int64(OffsetQuasarCert_FinalityUnixNano)
}

// Validators returns the count of distinct signing validators.
func (c QuasarCert) Validators() uint32 { return c.obj.Uint32(OffsetQuasarCert_Validators) }

// Bytes returns the underlying ZAP buffer. The transcript layer feeds
// these bytes into TupleHash256; the identity layer hashes them via
// sha256.Sum256. No copy.
func (c QuasarCert) Bytes() []byte {
	if c.msg == nil {
		return nil
	}
	return c.msg.Bytes()
}

// IsZero reports whether the cert is the zero value (no buffer attached).
func (c QuasarCert) IsZero() bool { return c.msg == nil }
