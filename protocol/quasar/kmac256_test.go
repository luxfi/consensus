// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/luxfi/consensus/config"
)

// TestKMAC256_Deterministic — same (key, msg, outLen, customization)
// inputs MUST produce byte-identical outputs across invocations. This
// is the foundational KAT property — without determinism, KMAC256
// outputs cannot be pinned into golden vectors and the consensus path
// is unverifiable.
func TestKMAC256_Deterministic(t *testing.T) {
	key := []byte("kmac256-determinism-key-32bytes!")
	msg := []byte("the quick brown fox jumps over the lazy dog")
	customization := customQuasarEventHorizonBLSMAC

	a := kmac256(key, msg, kmacMACOutLen, customization)
	b := kmac256(key, msg, kmacMACOutLen, customization)
	if !bytes.Equal(a, b) {
		t.Fatalf("KMAC256 not deterministic:\n a=%x\n b=%x", a, b)
	}
	if len(a) != kmacMACOutLen {
		t.Fatalf("KMAC256 output length: got %d, want %d", len(a), kmacMACOutLen)
	}

	// Cross-length determinism: smaller outLen MUST be a prefix-distinct
	// stream — KMAC256 absorbs outLen via right_encode, so changing
	// outLen changes the absorbed bytes and therefore the entire stream.
	short := kmac256(key, msg, 32, customization)
	if bytes.Equal(a[:32], short) {
		t.Fatalf("KMAC256 must bind outLen into absorption — 32-byte and 48-byte outputs cannot share a 32-byte prefix")
	}
}

// TestKMAC256_DomainSeparation_DifferentCustomizationsDifferentTags
// proves that two KMAC256 outputs with the SAME (key, msg, outLen)
// but DIFFERENT customizations are byte-distinct. This is the SP
// 800-185 §4 domain-separation guarantee — every quasar call site
// declares its own customization so the resulting MACs are
// independent oracles even when adversarially fed the same preimage.
func TestKMAC256_DomainSeparation_DifferentCustomizationsDifferentTags(t *testing.T) {
	key := []byte("kmac256-domain-separation-key!!")
	msg := []byte("shared message across customizations")

	tags := []string{
		customQuasarEventHorizonBLSMAC,
		customQuasarEventHorizonPQMAC,
		customQuasarHorizonSigBLSMAC,
		customQuasarHorizonSigPQMAC,
	}

	outputs := make(map[string]string, len(tags))
	for _, tag := range tags {
		out := kmac256(key, msg, kmacMACOutLen, tag)
		hexout := hex.EncodeToString(out)
		for prevTag, prevHex := range outputs {
			if prevHex == hexout {
				t.Fatalf("domain separation broken: tags %q and %q produced identical MAC %s",
					prevTag, tag, hexout)
			}
		}
		outputs[tag] = hexout
	}

	if len(outputs) != len(tags) {
		t.Fatalf("got %d distinct outputs, want %d", len(outputs), len(tags))
	}
}

// TestQuasarEventHorizonMAC_UsesKMAC256_UnderStrictPQ — under the
// strict-PQ profile, phaseII MUST emit KMAC256-shaped bytes (canonical
// width = kmacMACOutLen). Verifies the emit path was actually
// re-pointed at KMAC256 and that the width matches the SHA3-384
// suite the strict-PQ profile pins via MinHashOutputBits=384.
func TestQuasarEventHorizonMAC_UsesKMAC256_UnderStrictPQ(t *testing.T) {
	cfg := config.DefaultParams()
	q := NewBLS(cfg, newMockStore())

	blsKey := []byte("strict-pq-bls-key-canonical-32!")
	pqKey := []byte("strict-pq-pq-key-canonical-32!!")
	if err := q.Initialize(nil, blsKey, pqKey); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	q.SetProfile(config.LuxStrictPQ())

	// Build a vote map that crosses the alpha threshold.
	votes := map[string]int{"alpha-block": 18, "beta-block": 2}
	cert := q.phaseII(votes, "alpha-block")
	if cert == nil {
		t.Fatal("phaseII returned nil cert at supermajority")
	}

	if len(cert.BLSAgg) != kmacMACOutLen {
		t.Errorf("BLSAgg width: got %d, want %d (KMAC256 canonical)", len(cert.BLSAgg), kmacMACOutLen)
	}
	if len(cert.PQCert) != kmacMACOutLen {
		t.Errorf("PQCert width: got %d, want %d (KMAC256 canonical)", len(cert.PQCert), kmacMACOutLen)
	}

	// Cross-check: recomputing KMAC256 with the canonical customization
	// reproduces the emitted bytes exactly.
	wantBLS := kmac256(blsKey, cert.Message, kmacMACOutLen, customQuasarEventHorizonBLSMAC)
	wantPQ := kmac256(pqKey, cert.Message, kmacMACOutLen, customQuasarEventHorizonPQMAC)
	if !bytes.Equal(cert.BLSAgg, wantBLS) {
		t.Errorf("BLSAgg is not KMAC256 with QUASAR_EVENT_HORIZON_BLS_MAC_V1 customization")
	}
	if !bytes.Equal(cert.PQCert, wantPQ) {
		t.Errorf("PQCert is not KMAC256 with QUASAR_EVENT_HORIZON_PQ_MAC_V1 customization")
	}

	// And the standard VerifyWithKeys path accepts the emitted cert.
	if !cert.VerifyWithKeys(blsKey, pqKey) {
		t.Error("VerifyWithKeys rejected a freshly-emitted KMAC256 cert")
	}
	if !cert.VerifyWithKeysUnderProfile(blsKey, pqKey, config.LuxStrictPQ()) {
		t.Error("VerifyWithKeysUnderProfile rejected a canonical-width KMAC256 cert under strict-PQ")
	}
}

// TestConsensusMAC_RejectsHMACSHA256_UnderStrictPQ — negative test.
// A CertBundle whose bytes were produced by the old HMAC-SHA256
// kernel (32-byte width) MUST be rejected by
// VerifyWithKeysUnderProfile when the profile is strict-PQ. The
// rejection is structural: strict-PQ pins HashSuiteSHA3NIST, which
// admits only KMAC256 widths. The verifier refuses the legacy bytes
// before even running the MAC comparison.
//
// Closes the requirement: "any code that wants to emit an HMAC-SHA256
// MAC must check profile.HashSuiteID first; under strict-PQ, HMAC-SHA256
// emission/acceptance is forbidden."
func TestConsensusMAC_RejectsHMACSHA256_UnderStrictPQ(t *testing.T) {
	blsKey := []byte("hmac-rejection-bls-key-32-byte!")
	pqKey := []byte("hmac-rejection-pq-key-32-byte!!")
	message := []byte("strict-pq must refuse this HMAC-SHA256 bundle")

	// Build a CertBundle whose bytes are the OLD HMAC-SHA256 outputs —
	// what the prior code path would have produced. Crucially, we
	// build them with crypto/hmac directly so the test isn't merely
	// re-verifying a KMAC256 round trip.
	blsMAC := hmac.New(sha256.New, blsKey)
	blsMAC.Write(message)
	pqMAC := hmac.New(sha256.New, pqKey)
	pqMAC.Write(message)

	legacy := &CertBundle{
		BLSAgg:  blsMAC.Sum(nil), // 32 bytes — HMAC-SHA256 width
		PQCert:  pqMAC.Sum(nil),  // 32 bytes — HMAC-SHA256 width
		Message: message,
	}

	if len(legacy.BLSAgg) != sha256.Size || len(legacy.PQCert) != sha256.Size {
		t.Fatalf("test precondition: expected 32-byte HMAC-SHA256 widths, got BLS=%d PQ=%d",
			len(legacy.BLSAgg), len(legacy.PQCert))
	}

	// Strict-PQ profile MUST refuse the legacy 32-byte bytes outright.
	strictPQ := config.LuxStrictPQ()
	if legacy.VerifyWithKeysUnderProfile(blsKey, pqKey, strictPQ) {
		t.Fatal("strict-PQ profile MUST reject HMAC-SHA256-width (32-byte) MACs")
	}

	// FIPS profile (also pins SHA3-NIST) MUST also refuse.
	if legacy.VerifyWithKeysUnderProfile(blsKey, pqKey, config.LuxFIPS()) {
		t.Fatal("FIPS profile MUST reject HMAC-SHA256-width (32-byte) MACs")
	}

	// Sanity check: a canonical-width KMAC256 bundle IS accepted by the
	// same VerifyWithKeysUnderProfile under strict-PQ. This guarantees
	// the rejection above is specific to the legacy width, not a
	// blanket refusal.
	canonical := &CertBundle{
		BLSAgg:  kmac256(blsKey, message, kmacMACOutLen, customQuasarEventHorizonBLSMAC),
		PQCert:  kmac256(pqKey, message, kmacMACOutLen, customQuasarEventHorizonPQMAC),
		Message: message,
	}
	if !canonical.VerifyWithKeysUnderProfile(blsKey, pqKey, strictPQ) {
		t.Fatal("strict-PQ profile MUST accept canonical-width KMAC256 MACs")
	}
}

// TestKMAC256_CustomizationStrings_Pinned guards against an accidental
// rename / tag bump of any canonical customization. The strings ARE the
// schema identity — if any of them changes, every prior MAC under the
// old tag becomes unverifiable. Pin them here as a golden registry.
func TestKMAC256_CustomizationStrings_Pinned(t *testing.T) {
	wanted := map[string]string{
		"event_horizon_bls": "QUASAR_EVENT_HORIZON_BLS_MAC_V1",
		"event_horizon_pq":  "QUASAR_EVENT_HORIZON_PQ_MAC_V1",
		"horizon_sig_bls":   "QUASAR_HORIZON_SIG_BLS_MAC_V1",
		"horizon_sig_pq":    "QUASAR_HORIZON_SIG_PQ_MAC_V1",
		"vote_digest":       "QUASAR_VOTE_DIGEST_V1",
		"proposal_id":       "QUASAR_PROPOSAL_ID_V1",
		"horizon_sig_dgst":  "QUASAR_HORIZON_SIG_DIGEST_V1",
	}
	got := map[string]string{
		"event_horizon_bls": customQuasarEventHorizonBLSMAC,
		"event_horizon_pq":  customQuasarEventHorizonPQMAC,
		"horizon_sig_bls":   customQuasarHorizonSigBLSMAC,
		"horizon_sig_pq":    customQuasarHorizonSigPQMAC,
		"vote_digest":       customVoteDigest,
		"proposal_id":       customProposalID,
		"horizon_sig_dgst":  customHorizonSigDigest,
	}
	for k, want := range wanted {
		if got[k] != want {
			t.Errorf("customization %s: got %q, want %q", k, got[k], want)
		}
	}
}
