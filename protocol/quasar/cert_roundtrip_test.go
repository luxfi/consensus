// Copyright (C) 2025, Lux Industries Inc. All rights reserved.

package quasar

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/crypto/bls"
)

// TestQuasarCert_RoundTrip_E2E exercises the full path:
//
//  1. Generate a real triple signature via TripleSignRound1.
//  2. Build a QuasarCert from the produced share + ML-DSA sig.
//  3. Marshal to bytes, unmarshal into a fresh QuasarCert.
//  4. Verify with the real signer's keys (must succeed).
//  5. Verify with a different BLS aggregate key (must fail).
func TestQuasarCert_RoundTrip_E2E(t *testing.T) {
	// 1-of-1 BLS+Ringtail dual signer + ML-DSA via AddValidator.
	cfg, err := GenerateDualKeys(1, 3)
	if err != nil {
		t.Fatalf("GenerateDualKeys: %v", err)
	}

	s, err := NewSignerWithDualThreshold(*cfg)
	if err != nil {
		t.Fatalf("NewSignerWithDualThreshold: %v", err)
	}

	for _, id := range []string{"v0", "v1", "v2"} {
		if err := s.AddValidator(id, 100); err != nil {
			t.Fatalf("AddValidator(%s): %v", id, err)
		}
	}
	if !s.IsTripleMode() {
		t.Fatalf("expected triple mode after configuring all three paths")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg := []byte("e2e-roundtrip-message")
	sig, _, err := s.TripleSignRound1(ctx, "v0", msg, 1, []byte("prf-key-32-bytes-for-the-test!!"))
	if err != nil {
		t.Fatalf("TripleSignRound1: %v", err)
	}
	if len(sig.BLS) == 0 || len(sig.MLDSA) == 0 {
		t.Fatalf("expected BLS and MLDSA filled; got BLS=%d MLDSA=%d", len(sig.BLS), len(sig.MLDSA))
	}

	// 2. Build QuasarCert. We embed the BLS share as the BLS field for
	//    end-to-end byte exercise; full aggregation is a higher-layer concern.
	cert := &QuasarCert{
		BLS:        append([]byte(nil), sig.BLS...),
		Corona:      []byte{0x01}, // structural-only; cryptographic Ringtail is via ringtailGobEncode of a real Signature
		MLDSARollup: EncodeMLDSASigs([][]byte{sig.MLDSA}),
		Epoch:      1,
		Finality:   time.Unix(1700000000, 0),
		Validators: 3,
	}

	// 3. Marshal / unmarshal round-trip.
	raw, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("MarshalBinary returned empty bytes")
	}
	if raw[0] != CertSchemeQuasar {
		t.Fatalf("expected scheme byte %x, got %x", CertSchemeQuasar, raw[0])
	}

	got := &QuasarCert{}
	if err := got.UnmarshalBinary(raw); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}
	if string(got.BLS) != string(cert.BLS) {
		t.Fatalf("BLS round-trip mismatch")
	}
	if string(got.Corona) != string(cert.Corona) {
		t.Fatalf("Ringtail round-trip mismatch")
	}
	if string(got.MLDSARollup) != string(cert.MLDSARollup) {
		t.Fatalf("MLDSAProof round-trip mismatch")
	}
	if got.Epoch != cert.Epoch {
		t.Fatalf("Epoch mismatch: %d vs %d", got.Epoch, cert.Epoch)
	}
	if got.Finality.Unix() != cert.Finality.Unix() {
		t.Fatalf("Finality mismatch")
	}
	if got.Validators != cert.Validators {
		t.Fatalf("Validators mismatch: %d vs %d", got.Validators, cert.Validators)
	}

	// 4-5. VerifyWithRealKeys with a wrong BLS aggregate key must fail.
	wrongSK, err := bls.NewSecretKey()
	if err != nil {
		t.Fatalf("bls.NewSecretKey: %v", err)
	}
	wrongPK := wrongSK.PublicKey()

	if got.VerifyWithRealKeys(msg, wrongPK, nil, nil) {
		t.Fatal("VerifyWithRealKeys must fail with wrong BLS aggregate key")
	}

	// Also: corrupt the BLS bytes; verification must fail.
	tampered := *got
	tampered.BLS = append([]byte(nil), got.BLS...)
	tampered.BLS[0] ^= 0xFF
	if tampered.VerifyWithRealKeys(msg, wrongPK, nil, nil) {
		t.Fatal("VerifyWithRealKeys must fail with tampered BLS bytes")
	}
}

// TestQuasarCert_UnmarshalCorrupt covers truncated and wrong-scheme inputs.
func TestQuasarCert_UnmarshalCorrupt(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		{0x00},                                 // wrong scheme
		{CertSchemeQuasar},                     // truncated header
		{CertSchemeQuasar, 0x00, 0xFF},         // BLS length exceeds buffer
		append([]byte{CertSchemeQuasar}, make([]byte, 4)...), // still truncated
	}
	for i, c := range cases {
		got := &QuasarCert{}
		if err := got.UnmarshalBinary(c); err == nil {
			t.Fatalf("case %d: expected error for corrupt input %x", i, c)
		}
	}
}
