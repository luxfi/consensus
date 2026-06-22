// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
)

// ----------------------------------------------------------------------------
// Compact ↔ full encoding round-trip
// ----------------------------------------------------------------------------

func TestQuorumCert_FullRoundTrip(t *testing.T) {
	sc := fourMLDSA(t)
	wire, err := sc.cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal full: %v", err)
	}
	got, err := UnmarshalWeightedQuorumCert(wire)
	if err != nil {
		t.Fatalf("unmarshal full: %v", err)
	}
	if !sc.cert.Equal(got) {
		t.Fatal("full round-trip not byte-stable")
	}
	// And the decoded cert must still verify with real signatures.
	if err := got.Verify(sc.env, sc.cfg); err != nil {
		t.Fatalf("decoded full cert failed verify: %v", err)
	}
}

func TestQuorumCert_CompactRoundTrip(t *testing.T) {
	sc := fourMLDSA(t)
	compact := sc.cert.Compact()
	if !compact.IsCompact() {
		t.Fatal("Compact() did not produce a compact cert")
	}
	wire, err := compact.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal compact: %v", err)
	}
	got, err := UnmarshalWeightedQuorumCert(wire)
	if err != nil {
		t.Fatalf("unmarshal compact: %v", err)
	}
	if !compact.Equal(got) {
		t.Fatal("compact round-trip not byte-stable")
	}
	// A compact cert cannot be verified until records are re-attached.
	if err := got.Verify(sc.env, sc.cfg); !errors.Is(err, ErrQCCompactNoRecords) {
		t.Fatalf("compact verify err = %v, want ErrQCCompactNoRecords", err)
	}
}

// TestQuorumCert_CompactToFullViaDA proves the DA-hybrid flow: header carries
// the compact commitment; records are fetched from DA and re-attached; the
// re-attached cert verifies. A DA layer that serves a DIFFERENT record set
// is caught by the commitment recomputation.
func TestQuorumCert_CompactToFullViaDA(t *testing.T) {
	sc := fourMLDSA(t)
	compact := sc.cert.Compact()

	// Honest DA: the real records re-attach and verify.
	reattached := compact.WithRecords(sc.cert.Signers)
	if err := reattached.Verify(sc.env, sc.cfg); err != nil {
		t.Fatalf("honest re-attach failed verify: %v", err)
	}

	// Malicious DA: drop one record (different signer set than committed).
	tampered := compact.WithRecords(sc.cert.Signers[:3])
	err := tampered.Verify(sc.env, sc.cfg)
	// SignerCount (4) != records (3) is caught first; either count-mismatch
	// or commitment-mismatch is an acceptable clean rejection.
	if err == nil {
		t.Fatal("malicious DA record substitution accepted")
	}
	if !errors.Is(err, ErrQCSignerCountMismatch) && !errors.Is(err, ErrQCSignerCommitment) {
		t.Fatalf("malicious DA err = %v, want count or commitment mismatch", err)
	}
}

func TestQuorumCert_DecoderFailClosed(t *testing.T) {
	sc := fourMLDSA(t)
	wire, _ := sc.cert.MarshalBinary()

	// Truncation at every prefix length must error, never panic.
	for n := 0; n < len(wire); n++ {
		if _, err := UnmarshalWeightedQuorumCert(wire[:n]); err == nil {
			t.Fatalf("truncated-to-%d decoded without error", n)
		}
	}
	// Trailing garbage must error.
	withTrailer := append(append([]byte(nil), wire...), 0xFF)
	if _, err := UnmarshalWeightedQuorumCert(withTrailer); !errors.Is(err, ErrQCWireCorrupt) {
		t.Fatalf("trailing byte err = %v, want ErrQCWireCorrupt", err)
	}
	// Bad kind byte.
	badKind := append([]byte(nil), wire...)
	badKind[0] = 0x99
	if _, err := UnmarshalWeightedQuorumCert(badKind); !errors.Is(err, ErrQCWireCorrupt) {
		t.Fatalf("bad-kind err = %v, want ErrQCWireCorrupt", err)
	}
}

func TestQuorumCert_MixedSchemeRoundTrip(t *testing.T) {
	signers := []*testSigner{
		newMLDSASigner(t, 0x01, 40),
		newSLHDSASigner(t, 0x02, 40),
		newMLDSASigner(t, 0x03, 40),
	}
	sc := buildScenario(t, signers, 100, nil)
	wire, err := sc.cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal mixed: %v", err)
	}
	got, err := UnmarshalWeightedQuorumCert(wire)
	if err != nil {
		t.Fatalf("unmarshal mixed: %v", err)
	}
	if !sc.cert.Equal(got) {
		t.Fatal("mixed-scheme round-trip not byte-stable")
	}
	if err := got.Verify(sc.env, sc.cfg); err != nil {
		t.Fatalf("decoded mixed cert failed verify: %v", err)
	}
}

// ----------------------------------------------------------------------------
// QuasarCert wiring
// ----------------------------------------------------------------------------

func TestQuasarCert_WeightedQuorumLeg(t *testing.T) {
	sc := fourMLDSA(t)

	qc, err := BuildQuasarCertFromWeightedQuorum(sc.cert)
	if err != nil {
		t.Fatalf("build QuasarCert: %v", err)
	}
	if !qc.HasWeightedQuorumLeg() {
		t.Fatal("QuasarCert does not report a weighted quorum leg")
	}
	if qc.Epoch != sc.cert.Epoch {
		t.Fatalf("QuasarCert epoch = %d, want %d", qc.Epoch, sc.cert.Epoch)
	}
	if qc.Validators != int(sc.cert.SignerCount) {
		t.Fatalf("QuasarCert validators = %d, want %d", qc.Validators, sc.cert.SignerCount)
	}

	// The clean Verify entry point over the QuasarCert.
	if err := qc.VerifyWeightedQuorumLeg(sc.env, sc.cfg); err != nil {
		t.Fatalf("VerifyWeightedQuorumLeg rejected a valid leg: %v", err)
	}

	// Extract round-trips to the same cert.
	extracted, err := ExtractWeightedQuorumLeg(qc.MLDSARollup)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !sc.cert.Equal(extracted) {
		t.Fatal("extracted leg != original cert")
	}

	// The whole QuasarCert must survive its own MarshalBinary/UnmarshalBinary.
	qcWire, err := qc.MarshalBinary()
	if err != nil {
		t.Fatalf("QuasarCert marshal: %v", err)
	}
	var qc2 QuasarCert
	if err := qc2.UnmarshalBinary(qcWire); err != nil {
		t.Fatalf("QuasarCert unmarshal: %v", err)
	}
	if err := qc2.VerifyWeightedQuorumLeg(sc.env, sc.cfg); err != nil {
		t.Fatalf("round-tripped QuasarCert leg failed verify: %v", err)
	}
}

func TestQuasarCert_LegDiscriminator(t *testing.T) {
	// A non-WQC MLDSARollup (e.g. a STARK proof blob) must be cleanly
	// distinguished, not misparsed as a WQC.
	starkish := []byte{0x10, 0x01, 0x02, 0x03} // does not start with the WQC tag
	if _, err := ExtractWeightedQuorumLeg(starkish); !errors.Is(err, ErrMLDSARollupNotWQC) {
		t.Fatalf("non-WQC leg err = %v, want ErrMLDSARollupNotWQC", err)
	}
	qc := &QuasarCert{MLDSARollup: starkish}
	if qc.HasWeightedQuorumLeg() {
		t.Fatal("non-WQC leg reported as a weighted quorum leg")
	}
}

func TestQuasarCert_AttachLegPreservesOtherLegs(t *testing.T) {
	sc := fourMLDSA(t)
	base := &QuasarCert{
		BLS:    []byte{0xAA, 0xBB},
		Pulsar: []byte{0xCC, 0xDD},
		Epoch:  sc.cert.Epoch,
	}
	out, err := base.AttachWeightedQuorumLeg(sc.cert)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if string(out.BLS) != string(base.BLS) || string(out.Pulsar) != string(base.Pulsar) {
		t.Fatal("attach clobbered other legs")
	}
	if !out.HasWeightedQuorumLeg() {
		t.Fatal("attach did not install the weighted quorum leg")
	}
	if err := out.VerifyWeightedQuorumLeg(sc.env, sc.cfg); err != nil {
		t.Fatalf("attached leg failed verify: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Message function: QuorumMessageForCert derives from cert fields
// ----------------------------------------------------------------------------

func TestQuorumMessage_ForCertMatches(t *testing.T) {
	sc := fourMLDSA(t)
	// Build the message from the cert's own fields + the chain envelope.
	msg, err := QuorumMessageForCert(sc.env, sc.cert)
	if err != nil {
		t.Fatalf("QuorumMessageForCert: %v", err)
	}
	// Must equal the message the signers actually signed (the scenario built
	// the message from the same envelope before the cert existed).
	if string(msg) != string(sc.message) {
		t.Fatal("QuorumMessageForCert != the message signers signed")
	}
	// The rebuilt message bytes drive a passing predicate (this is exactly the
	// message the public Verify constructs internally).
	if err := sc.cert.verifyWithMessage(msg, sc.cfg); err != nil {
		t.Fatalf("cert failed verify under QuorumMessageForCert output: %v", err)
	}
	// And the public envelope-based Verify (which rebuilds the message itself)
	// accepts the same cert.
	if err := sc.cert.Verify(sc.env, sc.cfg); err != nil {
		t.Fatalf("cert failed envelope-based Verify: %v", err)
	}
}

func TestQuorumMessage_RefusesZeroEnvelope(t *testing.T) {
	// QuorumConsensusMessage must propagate ComputeRoundDigest's refusal of
	// zero-value security-relevant inputs (fail-closed).
	var env QuorumMessageEnvelope // all zero
	if _, err := QuorumConsensusMessage(env); err == nil {
		t.Fatal("zero envelope produced a message; want ComputeRoundDigest refusal")
	}
}

// TestProofBackendIDs pins the new wire IDs so a future renumber trips this
// test (mirrors the existing ID-pinning discipline in pq_mode_test.go).
func TestProofBackend_DirectWeightedQuorumIDs(t *testing.T) {
	if config.ProofBackendDirectWeightedQuorum != 0x30 {
		t.Fatalf("ProofBackendDirectWeightedQuorum = 0x%02x, want 0x30", uint8(config.ProofBackendDirectWeightedQuorum))
	}
	if config.ProofFormatDirectWeightedQuorumV1 != 0x30 {
		t.Fatalf("ProofFormatDirectWeightedQuorumV1 = 0x%02x, want 0x30", uint8(config.ProofFormatDirectWeightedQuorumV1))
	}
	if config.VerifierDirectWeightedQuorumPQ != 0x0030 {
		t.Fatalf("VerifierDirectWeightedQuorumPQ = 0x%04x, want 0x0030", uint16(config.VerifierDirectWeightedQuorumPQ))
	}
	if !config.ProofBackendDirectWeightedQuorum.IsProductionPQ() {
		t.Fatal("direct weighted quorum backend must be production-PQ")
	}
	if !config.ProofBackendDirectWeightedQuorum.IsDirectVerifiable() {
		t.Fatal("direct weighted quorum backend must report IsDirectVerifiable")
	}
	if config.ProofBackendDirectWeightedQuorum.IsForbiddenInPQMode() {
		t.Fatal("direct weighted quorum backend must not be forbidden in PQ mode")
	}
}
