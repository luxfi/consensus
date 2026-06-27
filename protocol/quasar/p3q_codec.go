// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// p3q_codec.go — deterministic, fail-closed wire codec for the P3Q rollup
// payload. Mirrors the consensus_cert_codec.go policy: bounds-checked, strict
// trailing-byte rejection, one canonical encoding per value, NEVER a panic and
// NEVER unbounded work (I13).
package quasar

import "encoding/binary"

// u64be returns a fresh big-endian byte slice for a uint64. Value-returning
// sibling of qblock.go's appendU64, used where a TupleHash part list wants a
// standalone slice rather than an append target.
func u64be(v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return b[:]
}

// encodeP3QRollupPayload encodes a P3QRollupPayload deterministically:
//
//	suite_id_len:4 suite_id:S
//	rollup_root:32
//	cert_set_len:4 cert_set:C   (marshalled WeightedQuorumCert; empty for succinct)
//	proof_len:4    proof:R      (succinct proof; empty for Direct)
func encodeP3QRollupPayload(p *P3QRollupPayload) []byte {
	buf := make([]byte, 0, 4+len(p.SuiteID)+32+4+len(p.CertSet)+4+len(p.Proof))
	buf = appendU32(buf, uint32(len(p.SuiteID)))
	buf = append(buf, p.SuiteID...)
	buf = append(buf, p.RollupRoot[:]...)
	buf = appendU32(buf, uint32(len(p.CertSet)))
	buf = append(buf, p.CertSet...)
	buf = appendU32(buf, uint32(len(p.Proof)))
	buf = append(buf, p.Proof...)
	return buf
}

// decodeP3QRollupPayload is the strict inverse. Every read is bounds-checked by
// the shared qcReader; a truncated frame, an over-long length prefix, or any
// trailing byte is ErrEvidenceWireCorrupt.
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
	certSet, err := r.lenPrefixed()
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

	return &P3QRollupPayload{
		SuiteID:    string(suiteID),
		RollupRoot: root,
		CertSet:    certSet,
		Proof:      proof,
	}, nil
}
