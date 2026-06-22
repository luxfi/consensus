// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// consensus_cert_codec.go — deterministic, fail-closed wire codecs for the
// ConsensusCert envelope and its per-leg evidence payloads.
//
// Every decoder is bounds-checked and rejects trailing bytes (strict canonical
// language): a malformed payload yields a typed error, NEVER a panic and NEVER
// unbounded work (I13). The encoders are deterministic — equal values encode to
// equal bytes — so a cert has exactly one canonical encoding.
//
// The fixed-frame readers reuse qcReader (quorum_cert.go): one bounds-checked
// sequential reader in the package, never two.
package quasar

import (
	"github.com/luxfi/crypto/bls"
)

// ----------------------------------------------------------------------------
// Per-leg evidence payload codecs.
// ----------------------------------------------------------------------------

// encodeThresholdSigPayload encodes a ThresholdSigPayload deterministically:
//
//	flags:1            bit0 = accountability present
//	sig_len:4 sig:N
//	[ if accountability present ]
//	  signer_root:32
//	  aggregate_weight:8
func encodeThresholdSigPayload(p *ThresholdSigPayload) []byte {
	buf := make([]byte, 0, 1+4+len(p.Signature)+32+8)
	var flags byte
	if p.Accountability != nil {
		flags |= 0x01
	}
	buf = append(buf, flags)
	buf = appendU32(buf, uint32(len(p.Signature)))
	buf = append(buf, p.Signature...)
	if p.Accountability != nil {
		buf = append(buf, p.Accountability.SignerRoot[:]...)
		buf = appendU64(buf, p.Accountability.AggregateWeight)
	}
	return buf
}

// decodeThresholdSigPayload is the strict inverse. A truncated frame, a
// length-prefix overrun, or trailing bytes is ErrEvidenceWireCorrupt.
func decodeThresholdSigPayload(data []byte) (*ThresholdSigPayload, error) {
	r := &qcReader{buf: data}
	flags, err := r.u8()
	if err != nil {
		return nil, ErrEvidenceWireCorrupt
	}
	// Strict canonical: only bit0 (accountability-present) is defined. Any other
	// bit set is a non-canonical frame (it would re-encode to a different flags
	// byte) — reject so the encoding is one-to-one (no malleability).
	if flags&^byte(0x01) != 0 {
		return nil, ErrEvidenceWireCorrupt
	}
	sig, err := r.lenPrefixed()
	if err != nil {
		return nil, ErrEvidenceWireCorrupt
	}
	p := &ThresholdSigPayload{Signature: sig}
	if flags&0x01 != 0 {
		var acct ThresholdAccountability
		if err := r.read32(&acct.SignerRoot); err != nil {
			return nil, ErrEvidenceWireCorrupt
		}
		if acct.AggregateWeight, err = r.u64(); err != nil {
			return nil, ErrEvidenceWireCorrupt
		}
		p.Accountability = &acct
	}
	if len(r.buf) != 0 {
		return nil, ErrEvidenceWireCorrupt
	}
	return p, nil
}

// encodeStarkCompressedPayload encodes a StarkCompressedPayload deterministically:
//
//	public_inputs_len:4 public_inputs:N
//	proof_len:4         proof:M
func encodeStarkCompressedPayload(p *StarkCompressedPayload) []byte {
	buf := make([]byte, 0, 4+len(p.PublicInputs)+4+len(p.Proof))
	buf = appendU32(buf, uint32(len(p.PublicInputs)))
	buf = append(buf, p.PublicInputs...)
	buf = appendU32(buf, uint32(len(p.Proof)))
	buf = append(buf, p.Proof...)
	return buf
}

// decodeStarkCompressedPayload is the strict inverse.
func decodeStarkCompressedPayload(data []byte) (*StarkCompressedPayload, error) {
	r := &qcReader{buf: data}
	pub, err := r.lenPrefixed()
	if err != nil {
		return nil, ErrEvidenceWireCorrupt
	}
	proof, err := r.lenPrefixed()
	if err != nil {
		return nil, ErrEvidenceWireCorrupt
	}
	if len(r.buf) != 0 {
		return nil, ErrEvidenceWireCorrupt
	}
	return &StarkCompressedPayload{PublicInputs: pub, Proof: proof}, nil
}

// encodeClassicalAggregatePayload encodes a ClassicalAggregatePayload:
//
//	scheme:1
//	payload_len:4 payload:N
func encodeClassicalAggregatePayload(p *ClassicalAggregatePayload) []byte {
	buf := make([]byte, 0, 1+4+len(p.Payload))
	buf = append(buf, byte(p.Scheme))
	buf = appendU32(buf, uint32(len(p.Payload)))
	buf = append(buf, p.Payload...)
	return buf
}

// decodeClassicalAggregatePayload is the strict inverse.
func decodeClassicalAggregatePayload(data []byte) (*ClassicalAggregatePayload, error) {
	r := &qcReader{buf: data}
	sch, err := r.u8()
	if err != nil {
		return nil, ErrEvidenceWireCorrupt
	}
	pl, err := r.lenPrefixed()
	if err != nil {
		return nil, ErrEvidenceWireCorrupt
	}
	if len(r.buf) != 0 {
		return nil, ErrEvidenceWireCorrupt
	}
	return &ClassicalAggregatePayload{Scheme: ClassicalScheme(sch), Payload: pl}, nil
}

// ----------------------------------------------------------------------------
// bls helper — thin wrapper so the classical leg does not import bls directly
// (keeps the leg file's imports minimal; one constructor in one place).
// ----------------------------------------------------------------------------

// blsPublicKeyFromBytes parses a compressed BLS-12-381 aggregate public key.
func blsPublicKeyFromBytes(keyBytes []byte) (*bls.PublicKey, error) {
	return bls.PublicKeyFromCompressedBytes(keyBytes)
}
