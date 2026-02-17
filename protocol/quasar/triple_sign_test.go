// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// Tests for the Quasar signing path: BLS + Ringtail + ML-DSA.

package quasar

import (
	"context"
	"testing"
	"time"
)

// TestTripleSignBasic creates a signer with threshold=1, adds a validator,
// signs a message, and verifies that both BLS and MLDSA fields are filled.
func TestTripleSignBasic(t *testing.T) {
	s, err := NewSigner(1)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}

	if err := s.AddValidator("v0", 100); err != nil {
		t.Fatalf("AddValidator: %v", err)
	}

	msg := []byte("triple-sign-basic-test")
	sig, err := s.SignMessage("v0", msg)
	if err != nil {
		t.Fatalf("SignMessage: %v", err)
	}

	if len(sig.BLS) == 0 {
		t.Fatal("BLS field is empty")
	}
	if len(sig.MLDSA) == 0 {
		t.Fatal("MLDSA field is empty")
	}

	if !s.VerifyQuasarSig(msg, sig) {
		t.Fatal("VerifyQuasarSig returned false for valid signature")
	}
}

// TestTripleSignVerify creates a signer, signs a message, and independently
// verifies that both the BLS and ML-DSA paths pass verification.
func TestTripleSignVerify(t *testing.T) {
	s, err := NewSigner(1)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}

	if err := s.AddValidator("v0", 100); err != nil {
		t.Fatalf("AddValidator: %v", err)
	}

	msg := []byte("triple-sign-verify-test")
	sig, err := s.SignMessage("v0", msg)
	if err != nil {
		t.Fatalf("SignMessage: %v", err)
	}

	// Full verification (BLS + MLDSA)
	if !s.VerifyQuasarSig(msg, sig) {
		t.Fatal("full verification failed")
	}

	// Verify that a tampered message fails
	bad := []byte("tampered-message")
	if s.VerifyQuasarSig(bad, sig) {
		t.Fatal("verification should fail for tampered message")
	}

	// Verify BLS alone passes (strip MLDSA)
	blsOnly := &QuasarSig{
		BLS:         sig.BLS,
		ValidatorID: sig.ValidatorID,
		IsThreshold: sig.IsThreshold,
		SignerIndex:  sig.SignerIndex,
	}
	if !s.VerifyQuasarSig(msg, blsOnly) {
		t.Fatal("BLS-only verification failed")
	}

	// Verify that corrupted MLDSA fails
	corruptMLDSA := &QuasarSig{
		BLS:         sig.BLS,
		MLDSA:       []byte("not-a-valid-mldsa-sig"),
		ValidatorID: sig.ValidatorID,
		IsThreshold: sig.IsThreshold,
		SignerIndex:  sig.SignerIndex,
	}
	if s.VerifyQuasarSig(msg, corruptMLDSA) {
		t.Fatal("verification should fail with corrupted MLDSA")
	}
}

// TestIsTripleMode verifies that IsTripleMode correctly reflects the
// presence of all three signing paths: BLS threshold + Ringtail + ML-DSA.
func TestIsTripleMode(t *testing.T) {
	// A basic signer with AddValidator has BLS (legacy) + ML-DSA but
	// no threshold BLS signers and no Ringtail signers.
	basic, err := NewSigner(1)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	if err := basic.AddValidator("v0", 100); err != nil {
		t.Fatalf("AddValidator: %v", err)
	}
	if basic.IsTripleMode() {
		t.Fatal("basic signer should not be in triple mode (no threshold BLS, no Ringtail)")
	}

	// A dual-threshold signer has BLS threshold + Ringtail but no ML-DSA.
	config, err := GenerateDualKeys(1, 3)
	if err != nil {
		t.Fatalf("GenerateDualKeys: %v", err)
	}
	dual, err := NewSignerWithDualThreshold(*config)
	if err != nil {
		t.Fatalf("NewSignerWithDualThreshold: %v", err)
	}
	if dual.IsTripleMode() {
		t.Fatal("dual signer should not be in triple mode (no ML-DSA)")
	}

	// Adding validators with ML-DSA keys to the dual signer should enable
	// triple mode (blsSigners + ringtailSigners + mldsaKeys all populated).
	for _, id := range []string{"v0", "v1", "v2"} {
		if err := dual.AddValidator(id, 100); err != nil {
			t.Fatalf("AddValidator(%s): %v", id, err)
		}
	}
	if !dual.IsTripleMode() {
		t.Fatal("signer with BLS threshold + Ringtail + ML-DSA should be in triple mode")
	}
}

// TestTripleSignRound1 creates a full triple signer (BLS threshold + Ringtail +
// ML-DSA) and calls TripleSignRound1 to verify all three fields are produced.
func TestTripleSignRound1(t *testing.T) {
	config, err := GenerateDualKeys(1, 3)
	if err != nil {
		t.Fatalf("GenerateDualKeys: %v", err)
	}

	s, err := NewSignerWithDualThreshold(*config)
	if err != nil {
		t.Fatalf("NewSignerWithDualThreshold: %v", err)
	}

	// Add ML-DSA keys for the validators
	for _, id := range []string{"v0", "v1", "v2"} {
		if err := s.AddValidator(id, 100); err != nil {
			t.Fatalf("AddValidator(%s): %v", id, err)
		}
	}

	if !s.IsTripleMode() {
		t.Fatal("expected triple mode after adding ML-DSA keys")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg := []byte("triple-round1-test-message")
	sessionID := 42
	prfKey := []byte("test-prf-key-for-triple!!")

	sig, round1Data, err := s.TripleSignRound1(ctx, "v0", msg, sessionID, prfKey)
	if err != nil {
		t.Fatalf("TripleSignRound1: %v", err)
	}

	// BLS threshold share must be present
	if len(sig.BLS) == 0 {
		t.Fatal("BLS field is empty after TripleSignRound1")
	}

	// ML-DSA signature must be present
	if len(sig.MLDSA) == 0 {
		t.Fatal("MLDSA field is empty after TripleSignRound1")
	}

	// Ringtail Round1 data must be present (for the 2-round continuation)
	if round1Data == nil {
		t.Fatal("Ringtail Round1Data is nil")
	}
	if len(round1Data.D) == 0 {
		t.Fatal("Ringtail Round1Data.D is empty")
	}

	// Verify the signature is marked as threshold
	if !sig.IsThreshold {
		t.Fatal("signature should be marked as threshold")
	}
	if sig.ValidatorID != "v0" {
		t.Fatalf("expected ValidatorID v0, got %s", sig.ValidatorID)
	}
}
