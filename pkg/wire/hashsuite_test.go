// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wire

import (
	"bytes"
	"testing"
)

// TestHashSuiteID_StableIntegers locks the wire bytes. Pulsar (SHA3-NIST)
// and Corona (BLAKE3-legacy) certs ship under PolicyPQ (5); HashSuiteID
// is the only signal of which kernel to verify under. Renumbering an
// entry breaks every cert ever signed under that mode.
//
// Numbering is NIST-aligned per HIP-0077: 0x00 None, 0x01 SHA3_NIST
// normative, 0x02 BLAKE3_LEGACY non-normative. SHAKE256 does not have its
// own ID — SHAKE256 is FIPS 202 and is part of SHA3_NIST.
func TestHashSuiteID_StableIntegers(t *testing.T) {
	cases := []struct {
		suite HashSuiteID
		want  uint8
	}{
		{HashSuiteNone, 0x00},
		{HashSuiteSHA3NIST, 0x01},
		{HashSuiteBLAKE3Legacy, 0x02},
	}
	for _, c := range cases {
		if uint8(c.suite) != c.want {
			t.Errorf("HashSuiteID %q = 0x%02x, want 0x%02x", c.suite.String(), uint8(c.suite), c.want)
		}
	}
}

// TestSigSchemeID_StableIntegers locks the wire bytes for every defined
// signature scheme. Numbering blocks: 0x10 BLS, 0x20 Corona, 0x30
// Pulsar.R, 0x40 Pulsar.M (low nibble = parameter set).
func TestSigSchemeID_StableIntegers(t *testing.T) {
	cases := []struct {
		scheme SigSchemeID
		want   uint8
	}{
		{SigSchemeNone, 0x00},
		{SigSchemeBLS12381, 0x10},
		{SigSchemeNasua, 0x20},
		{SigSchemePulsarR, 0x30},
		{SigSchemePulsarM44, 0x41},
		{SigSchemePulsarM65, 0x42}, // production default
		{SigSchemePulsarM87, 0x43},
	}
	for _, c := range cases {
		if uint8(c.scheme) != c.want {
			t.Errorf("SigSchemeID %q = 0x%02x, want 0x%02x", c.scheme.String(), uint8(c.scheme), c.want)
		}
	}
}

// TestSigSchemeID_String pins canonical wire names.
func TestSigSchemeID_String(t *testing.T) {
	cases := []struct {
		scheme SigSchemeID
		want   string
	}{
		{SigSchemeNone, "none"},
		{SigSchemeBLS12381, "bls12-381"},
		{SigSchemeNasua, "nasua"},
		{SigSchemePulsarR, "pulsar-r"},
		{SigSchemePulsarM44, "pulsar-m-44"},
		{SigSchemePulsarM65, "pulsar-m-65"},
		{SigSchemePulsarM87, "pulsar-m-87"},
	}
	for _, c := range cases {
		if got := c.scheme.String(); got != c.want {
			t.Errorf("SigSchemeID(0x%02x).String() = %q, want %q", uint8(c.scheme), got, c.want)
		}
	}
}

// TestCertificate_HashSuiteID_JSON_RoundTrip — the byte survives JSON
// marshal / unmarshal unchanged. Cross-language interop (Python clients,
// audit pipelines) depends on this.
func TestCertificate_HashSuiteID_JSON_RoundTrip(t *testing.T) {
	candidateID := DeriveItemID([]byte("transcript-binding-test"))
	for _, suite := range []HashSuiteID{
		HashSuiteNone,
		HashSuiteSHA3NIST,
		HashSuiteBLAKE3Legacy,
	} {
		cert := NewCertificateWithSuite(candidateID, 42, PolicyPQ, suite, []byte("proof-bytes"))
		cert.Signers = []byte("signers-bitmap")

		data, err := MarshalCertificate(cert)
		if err != nil {
			t.Fatalf("MarshalCertificate(%s): %v", suite, err)
		}

		got, err := UnmarshalCertificate(data)
		if err != nil {
			t.Fatalf("UnmarshalCertificate(%s): %v", suite, err)
		}
		if got.HashSuiteID != suite {
			t.Fatalf("HashSuiteID round-trip: got %d, want %d", got.HashSuiteID, suite)
		}
		if got.PolicyID != PolicyPQ {
			t.Fatalf("PolicyID round-trip: got %d, want %d", got.PolicyID, PolicyPQ)
		}
		if got.CandidateID != cert.CandidateID {
			t.Fatalf("CandidateID round-trip mismatch")
		}
		if !bytes.Equal(got.Proof, cert.Proof) {
			t.Fatalf("Proof round-trip mismatch")
		}
	}
}

// TestCertificate_TranscriptHash_BindsHashSuiteID is the headline F1 test:
// flipping HashSuiteID after construction changes the cert digest. The
// threshold signature signs over TranscriptHash, so a post-sign HashSuiteID
// mutation breaks signature verification — a cross-suite confusion attack
// fails on the signature check, not just on a receiver-side comparison.
//
// Mirrors the warp/pulsar TestKernelVerifierRejectsHashSuiteFieldMutation
// pattern at the consensus layer.
func TestCertificate_TranscriptHash_BindsHashSuiteID(t *testing.T) {
	candidateID := DeriveItemID([]byte("F1-cross-suite-confusion"))

	pulsar := NewCertificateWithSuite(candidateID, 100, PolicyPQ, HashSuiteSHA3NIST, []byte("p"))
	corona := NewCertificateWithSuite(candidateID, 100, PolicyPQ, HashSuiteBLAKE3Legacy, []byte("p"))

	pulsarHash := pulsar.TranscriptHash()
	coronaHash := corona.TranscriptHash()

	if pulsarHash == coronaHash {
		t.Fatalf("transcript hash collides across HashSuiteID: Pulsar(SHA3-NIST) and Corona(BLAKE3-legacy) share PolicyPQ; F1 fix requires HashSuiteID to break the digest")
	}

	// Sanity: flipping HashSuiteID alone (no other field change) MUST flip
	// the digest. This is the explicit post-sign-mutation attack.
	mutated := *pulsar
	mutated.HashSuiteID = HashSuiteBLAKE3Legacy
	if mutated.TranscriptHash() == pulsarHash {
		t.Fatalf("mutating HashSuiteID did not change transcript hash; F1 attack would succeed")
	}
}

// TestCertificate_TranscriptHash_Deterministic — same inputs produce the
// same digest. If this ever flakes, transcript binding is non-deterministic
// and a signer cannot reliably sign the same cert twice.
func TestCertificate_TranscriptHash_Deterministic(t *testing.T) {
	candidateID := DeriveItemID([]byte("determinism"))
	cert := NewCertificateWithSuite(candidateID, 7, PolicyQuantum, HashSuiteSHA3NIST, []byte("proof"))
	cert.Signers = []byte("signers")

	a := cert.TranscriptHash()
	b := cert.TranscriptHash()
	if a != b {
		t.Fatalf("TranscriptHash non-deterministic: %x vs %x", a, b)
	}
}

// TestCertificate_TranscriptHash_BindsEveryField — flipping any of the
// covered fields changes the digest. Locks the canonical encoding so a
// future refactor can't silently exclude a field from the binding.
func TestCertificate_TranscriptHash_BindsEveryField(t *testing.T) {
	base := NewCertificateWithSuite(
		DeriveItemID([]byte("base")), 1, PolicyPQ, HashSuiteSHA3NIST, []byte("proof"),
	)
	base.Signers = []byte("signers")
	baseHash := base.TranscriptHash()

	cases := []struct {
		name string
		mut  func(c *Certificate)
	}{
		{"CandidateID", func(c *Certificate) { c.CandidateID = DeriveItemID([]byte("other")) }},
		{"Height", func(c *Certificate) { c.Height++ }},
		{"PolicyID", func(c *Certificate) { c.PolicyID = PolicyQuantum }},
		{"HashSuiteID", func(c *Certificate) { c.HashSuiteID = HashSuiteBLAKE3Legacy }},
		{"Proof", func(c *Certificate) { c.Proof = []byte("other-proof") }},
		{"Signers", func(c *Certificate) { c.Signers = []byte("other-signers") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := *base
			// Defensive copies for slices (mutations otherwise alias base).
			cc.Proof = append([]byte(nil), base.Proof...)
			cc.Signers = append([]byte(nil), base.Signers...)
			tc.mut(&cc)
			if cc.TranscriptHash() == baseHash {
				t.Fatalf("flipping %s did not change TranscriptHash; field is not bound", tc.name)
			}
		})
	}
}

// TestCertificate_TranscriptHash_TimestampNotBound — informational metadata
// (TimestampMs) is deliberately excluded from the transcript. A signer
// disconnected from wall clock can still produce a valid cert; replays at
// a different ms tick verify identically.
func TestCertificate_TranscriptHash_TimestampNotBound(t *testing.T) {
	cert := NewCertificateWithSuite(
		DeriveItemID([]byte("ts")), 1, PolicyPQ, HashSuiteSHA3NIST, []byte("p"),
	)
	a := cert.TranscriptHash()
	cert.TimestampMs += 12345
	if cert.TranscriptHash() != a {
		t.Fatalf("TimestampMs leaked into transcript hash; it is informational metadata only")
	}
}
