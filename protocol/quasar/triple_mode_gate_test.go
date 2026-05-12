// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
)

// TestGenerateCert_RefusesPartialTriple is the CR-10 regression: under a
// strict-PQ profile the certifier MUST NOT emit a single-layer cert.
// The legacy SHA-256 placeholder path silently downgraded; the realCert
// path with no Corona aggregate also downgraded. Both close here.
func TestGenerateCert_RefusesPartialTriple(t *testing.T) {
	c, err := newCertifier(1)
	if err != nil {
		t.Fatalf("newCertifier: %v", err)
	}
	c.SetProfile(config.StrictPQ())
	c.AddValidator("v1", 1)

	block := &Block{
		ID:        [32]byte{1, 2, 3},
		ChainID:   [32]byte{4, 5, 6},
		ChainName: "test",
		Height:    100,
		Timestamp: time.Now(),
	}

	// No signer attached → no path to a real cert. Under strict-PQ we
	// MUST refuse rather than fall through to the SHA-256 placeholder.
	cert := c.generateCert(block)
	if cert != nil {
		t.Fatalf("strict-PQ profile + no signer: expected nil cert, got %+v", cert)
	}
}

// TestGenerateCert_RefusesPartialTriple_WithSigner_NoCorona proves
// the realCert fallback path is also closed: a signer can produce a
// BLS+MLDSA cert (Corona empty because aggregation is at the
// BundleSigner layer), and under a triple-mode profile we MUST refuse
// that single-layer artefact.
func TestGenerateCert_RefusesPartialTriple_WithSigner_NoCorona(t *testing.T) {
	c, err := newCertifier(1)
	if err != nil {
		t.Fatalf("newCertifier: %v", err)
	}
	c.SetProfile(config.StrictPQ())
	c.AddValidator("v1", 1)

	// Basic signer: AddValidator creates BLS + MLDSA keys but no
	// Corona share. realCert will produce a cert with Corona=nil;
	// the triple-mode gate MUST refuse it.
	s, err := NewSigner(1)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	if err := s.AddValidator("v1", 1); err != nil {
		t.Fatalf("AddValidator: %v", err)
	}
	c.AttachSigner(nil, s)

	block := &Block{
		ID:        [32]byte{7, 8, 9},
		ChainID:   [32]byte{1, 2, 3},
		ChainName: "test",
		Height:    101,
		Timestamp: time.Now(),
	}

	cert := c.generateCert(block)
	if cert != nil {
		t.Fatalf("strict-PQ profile + basic signer: expected nil cert, got Corona=%v MLDSAProof_len=%d",
			cert.Corona, len(cert.MLDSAProof))
	}
}

// TestGenerateCert_PermissiveProfile_AcceptsPlaceholder — non-strict
// profiles preserve the legacy SHA-256 placeholder path. The CR-10
// gate is profile-driven, not heuristic.
func TestGenerateCert_PermissiveProfile_AcceptsPlaceholder(t *testing.T) {
	c, err := newCertifier(1)
	if err != nil {
		t.Fatalf("newCertifier: %v", err)
	}
	c.SetProfile(config.Permissive())
	c.AddValidator("v1", 1)

	block := &Block{
		ID:        [32]byte{1, 2, 3},
		ChainID:   [32]byte{4, 5, 6},
		ChainName: "test",
		Height:    102,
		Timestamp: time.Now(),
	}

	cert := c.generateCert(block)
	if cert == nil {
		t.Fatal("permissive profile + no signer: expected placeholder cert, got nil")
	}
}

// TestGenerateCert_NilProfile_AcceptsPlaceholder — pre-locked-profile
// callers (legacy unit tests) keep working with nil profile.
func TestGenerateCert_NilProfile_AcceptsPlaceholder(t *testing.T) {
	c, err := newCertifier(1)
	if err != nil {
		t.Fatalf("newCertifier: %v", err)
	}
	// No SetProfile → nil profile.
	c.AddValidator("v1", 1)

	block := &Block{
		ID:        [32]byte{1, 2, 3},
		ChainID:   [32]byte{4, 5, 6},
		ChainName: "test",
		Height:    103,
		Timestamp: time.Now(),
	}

	cert := c.generateCert(block)
	if cert == nil {
		t.Fatal("nil profile + no signer: expected placeholder cert, got nil")
	}
}

// TestQuasarCert_Verify_RejectsMissingCorona proves the structural
// gate at the vote-acceptance layer: cert.Verify() refuses a cert that
// doesn't carry every layer, so a single-layer cert is rejected even
// if generateCert somehow emits it.
func TestQuasarCert_Verify_RejectsMissingCorona(t *testing.T) {
	cert := &QuasarCert{
		BLS:        []byte("bls"),
		Corona:   nil, // missing
		MLDSAProof: []byte("mldsa"),
		Epoch:      1,
		Finality:   time.Now(),
		Validators: 1,
	}
	if cert.Verify(nil) {
		t.Fatal("QuasarCert.Verify accepted cert with nil Corona")
	}
}

// TestQuasarCert_Verify_RejectsMissingMLDSA — same shape for the MLDSA
// layer. Tests for symmetry with the Corona case.
func TestQuasarCert_Verify_RejectsMissingMLDSA(t *testing.T) {
	cert := &QuasarCert{
		BLS:        []byte("bls"),
		Corona:   []byte("rt"),
		MLDSAProof: nil, // missing
		Epoch:      1,
		Finality:   time.Now(),
		Validators: 1,
	}
	if cert.Verify(nil) {
		t.Fatal("QuasarCert.Verify accepted cert with nil MLDSAProof")
	}
}

// TestQuasarCert_Verify_RejectsMissingBLS — same shape for BLS.
func TestQuasarCert_Verify_RejectsMissingBLS(t *testing.T) {
	cert := &QuasarCert{
		BLS:        nil, // missing
		Corona:   []byte("rt"),
		MLDSAProof: []byte("mldsa"),
		Epoch:      1,
		Finality:   time.Now(),
		Validators: 1,
	}
	if cert.Verify(nil) {
		t.Fatal("QuasarCert.Verify accepted cert with nil BLS")
	}
}

// TestAddVoteLocked_StrictProfile_RefusesSingleLayerVote is the
// vote-accept-time gate (CR-10). When the profile is strict-PQ and the
// signer can produce triple-mode sigs, addVoteLocked MUST refuse any
// per-validator sig that doesn't carry all required layers.
func TestAddVoteLocked_StrictProfile_RefusesSingleLayerVote(t *testing.T) {
	q, err := NewTestQuasar(1)
	if err != nil {
		t.Fatalf("NewTestQuasar: %v", err)
	}
	q.SetProfile(config.StrictPQ())

	// Seed a pending block so addVoteLocked has something to act on.
	q.mu.Lock()
	qBlock := &QuantumBlock{
		Height:        1,
		QuantumHash:   "h",
		ValidatorSigs: make(map[string]*QuasarSig),
		CreatedAt:     time.Now(),
	}
	q.pendingBlocks["h"] = qBlock

	// Single-layer sig: only BLS, no MLDSA. Under strict profile this
	// MUST be rejected without running the (real-crypto) verify path.
	bad := &QuasarSig{
		BLS:         []byte("just-bls"),
		MLDSA:       nil,
		ValidatorID: "v0",
	}
	accepted := q.addVoteLocked("h", "v0", bad)
	q.mu.Unlock()
	if accepted {
		t.Fatal("strict-PQ profile: addVoteLocked accepted single-layer sig")
	}
}

// TestAddVoteLocked_NilProfile_AcceptsValidVotes — legacy callers
// (no profile) keep accepting the BLS+MLDSA-only votes that the test
// quasar uses for its self-vote during processBlock.
func TestAddVoteLocked_NilProfile_PreservesLegacyAcceptance(t *testing.T) {
	q, err := NewTestQuasar(1)
	if err != nil {
		t.Fatalf("NewTestQuasar: %v", err)
	}
	// Profile stays nil — legacy path.
	if q.demandsTripleLocked() {
		t.Fatal("nil profile: demandsTripleLocked = true, want false")
	}
}
