// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// magnetar_codec.go — deterministic, fail-closed wire codec for the
// Magnetar-Quorum rollup payload. Mirrors p3q_codec.go: bounds-checked, strict
// trailing-byte rejection, one canonical encoding per value, NEVER a panic and
// NEVER unbounded work (I13).
package quasar

// encodeMagnetarQuorumCert encodes a MagnetarQuorumCert deterministically:
//
//	suite_id_len:4 suite_id:S
//	rollup_root:32
//	cert_set_len:4 cert_set:C   (marshalled SLH-DSA WeightedQuorumCert; empty for succinct)
//	proof_len:4    proof:R      (succinct proof; empty for Direct)
func encodeMagnetarQuorumCert(mc *MagnetarQuorumCert) []byte {
	buf := make([]byte, 0, 4+len(mc.SuiteID)+32+4+len(mc.CertSet)+4+len(mc.Proof))
	buf = appendU32(buf, uint32(len(mc.SuiteID)))
	buf = append(buf, mc.SuiteID...)
	buf = append(buf, mc.RollupRoot[:]...)
	buf = appendU32(buf, uint32(len(mc.CertSet)))
	buf = append(buf, mc.CertSet...)
	buf = appendU32(buf, uint32(len(mc.Proof)))
	buf = append(buf, mc.Proof...)
	return buf
}

// DecodeMagnetarQuorumCert is the strict inverse. Every read is bounds-checked
// by the shared qcReader; a truncated frame, an over-long length prefix, or any
// trailing byte is ErrEvidenceWireCorrupt.
func DecodeMagnetarQuorumCert(data []byte) (*MagnetarQuorumCert, error) {
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

	return &MagnetarQuorumCert{
		SuiteID:    string(suiteID),
		RollupRoot: root,
		CertSet:    certSet,
		Proof:      proof,
	}, nil
}
