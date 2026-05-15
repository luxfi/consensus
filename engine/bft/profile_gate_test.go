// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bft

import (
	"errors"
	"testing"

	"github.com/luxfi/bft"

	"github.com/luxfi/consensus/config"
)

// classicalSigner is a fake bft.Signer that does NOT implement PQSigner.
// Mirrors the shape of an Ed25519-backed signer (the canonical classical
// signer the BFT package ships with). Used to exercise the CR-11 refusal
// path under a strict-PQ profile.
type classicalSigner struct{}

func (classicalSigner) Sign(_ []byte) ([]byte, error) {
	return []byte("classical-sig"), nil
}

// classicalVerifier is a fake bft.SignatureVerifier that does NOT
// implement PQVerifier. Pair to classicalSigner for the strict-PQ
// refusal tests.
type classicalVerifier struct{}

func (classicalVerifier) Verify(_ []byte, _ []byte, _ bft.NodeID) error {
	return nil
}

// pqSigner is a fake bft.Signer that DOES implement PQSigner. Used to
// exercise the strict-PQ acceptance path.
type pqSigner struct {
	scheme config.SigSchemeID
}

func (s pqSigner) Sign(_ []byte) ([]byte, error) {
	return []byte("pq-sig"), nil
}

func (s pqSigner) PQSchemeID() config.SigSchemeID {
	return s.scheme
}

// pqVerifier is a fake bft.SignatureVerifier that DOES implement
// PQVerifier. Pair to pqSigner.
type pqVerifier struct {
	scheme config.SigSchemeID
}

func (v pqVerifier) Verify(_ []byte, _ []byte, _ bft.NodeID) error {
	return nil
}

func (v pqVerifier) PQSchemeID() config.SigSchemeID {
	return v.scheme
}

// minimalEpoch constructs a *bft.Epoch literal carrying only the
// fields the CR-11 gate consults (Signer, Verifier). We do NOT call
// bft.NewEpoch because that would demand the full EpochConfig surface
// (Storage, Communication, WAL, BlockBuilder, ...) which is irrelevant
// to the gate under test.
func minimalEpoch(s bft.Signer, v bft.SignatureVerifier) *bft.Epoch {
	return &bft.Epoch{
		EpochConfig: bft.EpochConfig{
			Signer:   s,
			Verifier: v,
		},
	}
}

// TestBFTEngine_RefusesClassicalSignerUnderStrictPQ is the headline
// CR-11 regression. A chain that pins strict-PQ in its genesis MUST
// refuse a classical bft.Signer at construction — running an Ed25519
// leader-rotation kernel on a strict-PQ chain is the silent-classical-
// signing-under-PQ-banner attack.
func TestBFTEngine_RefusesClassicalSignerUnderStrictPQ(t *testing.T) {
	epoch := minimalEpoch(classicalSigner{}, pqVerifier{scheme: config.SigSchemePulsar65})
	_, err := NewEngineWithProfile(epoch, config.StrictPQ())
	if !errors.Is(err, ErrClassicalSignerUnderStrictPQ) {
		t.Fatalf("NewEngineWithProfile(strict, classical signer) = %v, want ErrClassicalSignerUnderStrictPQ", err)
	}
}

// TestBFTEngine_RefusesClassicalVerifierUnderStrictPQ — mirror of the
// signer case for the verifier side. A strict-PQ chain that wires a
// PQ signer with a classical verifier still has the verify-path leaking
// classical primitives; refuse it at construction.
func TestBFTEngine_RefusesClassicalVerifierUnderStrictPQ(t *testing.T) {
	epoch := minimalEpoch(pqSigner{scheme: config.SigSchemePulsar65}, classicalVerifier{})
	_, err := NewEngineWithProfile(epoch, config.StrictPQ())
	if !errors.Is(err, ErrClassicalVerifierUnderStrictPQ) {
		t.Fatalf("NewEngineWithProfile(strict, classical verifier) = %v, want ErrClassicalVerifierUnderStrictPQ", err)
	}
}

// TestBFTEngine_RefusesClassicalSignerUnderFIPS — FIPS is the strict
// superset; refuse classical signers there too.
func TestBFTEngine_RefusesClassicalSignerUnderFIPS(t *testing.T) {
	epoch := minimalEpoch(classicalSigner{}, pqVerifier{scheme: config.SigSchemePulsar65})
	_, err := NewEngineWithProfile(epoch, config.FIPS())
	if !errors.Is(err, ErrClassicalSignerUnderStrictPQ) {
		t.Fatalf("NewEngineWithProfile(fips, classical signer) = %v, want ErrClassicalSignerUnderStrictPQ", err)
	}
}

// TestBFTEngine_AcceptsPQUnderStrictPQ — happy path: signer and
// verifier both implement the PQ marker and agree on scheme.
func TestBFTEngine_AcceptsPQUnderStrictPQ(t *testing.T) {
	epoch := minimalEpoch(
		pqSigner{scheme: config.SigSchemePulsar65},
		pqVerifier{scheme: config.SigSchemePulsar65},
	)
	e, err := NewEngineWithProfile(epoch, config.StrictPQ())
	if err != nil {
		t.Fatalf("NewEngineWithProfile(strict, PQ signer+verifier) returned %v", err)
	}
	if e == nil {
		t.Fatal("NewEngineWithProfile returned nil engine")
	}
}

// TestBFTEngine_RefusesPQSchemeMismatch — both sides PQ but
// advertising different SigSchemeIDs is a misconfiguration the adapter
// catches at construction.
func TestBFTEngine_RefusesPQSchemeMismatch(t *testing.T) {
	epoch := minimalEpoch(
		pqSigner{scheme: config.SigSchemePulsar65},
		pqVerifier{scheme: config.SigSchemePulsar87},
	)
	_, err := NewEngineWithProfile(epoch, config.StrictPQ())
	if !errors.Is(err, ErrPQSchemeMismatch) {
		t.Fatalf("NewEngineWithProfile(strict, mismatched schemes) = %v, want ErrPQSchemeMismatch", err)
	}
}

// TestBFTEngine_PermissiveProfile_AdmitsClassicalSigner — non-strict
// profiles preserve the existing legacy admission path.
func TestBFTEngine_PermissiveProfile_AdmitsClassicalSigner(t *testing.T) {
	epoch := minimalEpoch(classicalSigner{}, classicalVerifier{})
	e, err := NewEngineWithProfile(epoch, config.Permissive())
	if err != nil {
		t.Fatalf("NewEngineWithProfile(permissive, classical) returned %v", err)
	}
	if e == nil {
		t.Fatal("NewEngineWithProfile returned nil engine")
	}
}

// TestBFTEngine_NilProfile_AdmitsClassicalSigner — nil profile (no
// chain-wide enforcement) preserves the existing legacy path.
func TestBFTEngine_NilProfile_AdmitsClassicalSigner(t *testing.T) {
	epoch := minimalEpoch(classicalSigner{}, classicalVerifier{})
	e, err := NewEngineWithProfile(epoch, nil)
	if err != nil {
		t.Fatalf("NewEngineWithProfile(nil profile) returned %v", err)
	}
	if e == nil {
		t.Fatal("NewEngineWithProfile returned nil engine")
	}
}

// TestBFTEngine_NilEpoch_Returns_ErrNilEpoch asserts the constructor
// refuses a nil epoch regardless of profile.
func TestBFTEngine_NilEpoch_Returns_ErrNilEpoch(t *testing.T) {
	_, err := NewEngineWithProfile(nil, config.StrictPQ())
	if !errors.Is(err, ErrNilEpoch) {
		t.Fatalf("NewEngineWithProfile(nil epoch) = %v, want ErrNilEpoch", err)
	}
}
