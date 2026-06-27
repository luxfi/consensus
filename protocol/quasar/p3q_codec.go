// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// p3q_codec.go — deterministic, fail-closed wire codec for the P3Q rollup
// payload. Mirrors the consensus_cert_codec.go policy: bounds-checked, strict
// trailing-byte rejection, one canonical encoding per value, NEVER a panic and
// NEVER unbounded work (I13).
package quasar

import "encoding/binary"

// u32be / u64be return a fresh big-endian byte slice for a fixed-width value.
// Value-returning siblings of qblock.go's appendU32/appendU64, used where a
// part list (TupleHash) wants a standalone slice rather than an append target.
func u32be(v uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return b[:]
}

func u64be(v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return b[:]
}

// encodeP3QRollupPayload encodes a P3QRollupPayload deterministically:
//
//	suite_id_len:4 suite_id:S
//	rollup_root:32
//	cert_count:4
//	  for each: validator_id:32, weight:8, pubkey_len:4 pubkey:P, sig_len:4 sig:G
//	proof_len:4 proof:R
func encodeP3QRollupPayload(p *P3QRollupPayload) []byte {
	buf := make([]byte, 0, 4+len(p.SuiteID)+32+4+len(p.Proof)+4)
	buf = appendU32(buf, uint32(len(p.SuiteID)))
	buf = append(buf, p.SuiteID...)
	buf = append(buf, p.RollupRoot[:]...)
	buf = appendU32(buf, uint32(len(p.CertSet)))
	for i := range p.CertSet {
		c := p.CertSet[i]
		buf = append(buf, c.ValidatorID[:]...)
		buf = appendU64(buf, c.Weight)
		buf = appendU32(buf, uint32(len(c.PubKey)))
		buf = append(buf, c.PubKey...)
		buf = appendU32(buf, uint32(len(c.Sig)))
		buf = append(buf, c.Sig...)
	}
	buf = appendU32(buf, uint32(len(p.Proof)))
	buf = append(buf, p.Proof...)
	return buf
}

// decodeP3QRollupPayload is the strict inverse. Every read is bounds-checked by
// the shared qcReader; a truncated frame, an over-long length prefix, or any
// trailing byte is ErrEvidenceWireCorrupt. The cert count is NOT trusted as an
// allocation hint — leaves are appended as they are read, so a lying count
// fails on the first short read rather than pre-allocating.
func decodeP3QRollupPayload(data []byte) (*P3QRollupPayload, error) {
	r := &qcReader{buf: data}

	suiteID, err := r.lenPrefixed()
	if err != nil {
		return nil, ErrEvidenceWireCorrupt
	}
	var root [32]byte
	if err := r.read32(&root); err != nil {
		return nil, ErrEvidenceWireCorrupt
	}
	count, err := r.u32()
	if err != nil {
		return nil, ErrEvidenceWireCorrupt
	}

	certs := make([]MLDSAValidatorCert, 0)
	for i := uint32(0); i < count; i++ {
		var c MLDSAValidatorCert
		if err := r.read32(&c.ValidatorID); err != nil {
			return nil, ErrEvidenceWireCorrupt
		}
		if c.Weight, err = r.u64(); err != nil {
			return nil, ErrEvidenceWireCorrupt
		}
		if c.PubKey, err = r.lenPrefixed(); err != nil {
			return nil, ErrEvidenceWireCorrupt
		}
		if c.Sig, err = r.lenPrefixed(); err != nil {
			return nil, ErrEvidenceWireCorrupt
		}
		certs = append(certs, c)
	}

	proof, err := r.lenPrefixed()
	if err != nil {
		return nil, ErrEvidenceWireCorrupt
	}
	if len(r.buf) != 0 {
		return nil, ErrEvidenceWireCorrupt
	}

	return &P3QRollupPayload{
		SuiteID:    string(suiteID),
		RollupRoot: root,
		CertSet:    certs,
		Proof:      proof,
	}, nil
}
