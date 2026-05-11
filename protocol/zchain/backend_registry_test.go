// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zchain

import (
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
)

func TestRegisterBackendVerifier_HappyPath(t *testing.T) {
	resetBackendVerifiersForTest()
	called := false
	err := RegisterBackendVerifier(
		config.VerifierSP1CompressedSTARKPQ,
		BackendVerifierFunc(func(_ *VerifierManifest, _ *ZPublicInputs, _ *ZProofEnvelope) (bool, error) {
			called = true
			return true, nil
		}),
	)
	if err != nil {
		t.Fatalf("RegisterBackendVerifier: %v", err)
	}
	b := lookupBackendVerifier(config.VerifierSP1CompressedSTARKPQ)
	if b == nil {
		t.Fatalf("lookupBackendVerifier returned nil after register")
	}
	ok, err := b.Verify(nil, nil, nil)
	if err != nil {
		t.Fatalf("Verify returned %v", err)
	}
	if !ok || !called {
		t.Errorf("bound backend did not run")
	}
}

func TestRegisterBackendVerifier_RejectsNil(t *testing.T) {
	resetBackendVerifiersForTest()
	err := RegisterBackendVerifier(config.VerifierSP1CompressedSTARKPQ, nil)
	if !errors.Is(err, ErrBackendVerifierNil) {
		t.Errorf("got %v; want ErrBackendVerifierNil", err)
	}
}

func TestRegisterBackendVerifier_RejectsVerifierNone(t *testing.T) {
	resetBackendVerifiersForTest()
	err := RegisterBackendVerifier(
		config.VerifierNone,
		BackendVerifierFunc(func(_ *VerifierManifest, _ *ZPublicInputs, _ *ZProofEnvelope) (bool, error) {
			return true, nil
		}),
	)
	if !errors.Is(err, ErrBackendVerifierInvalidID) {
		t.Errorf("got %v; want ErrBackendVerifierInvalidID", err)
	}
}

func TestRegisterBackendVerifier_RejectsDuplicate(t *testing.T) {
	resetBackendVerifiersForTest()
	bv := BackendVerifierFunc(func(_ *VerifierManifest, _ *ZPublicInputs, _ *ZProofEnvelope) (bool, error) {
		return true, nil
	})
	if err := RegisterBackendVerifier(config.VerifierSP1CompressedSTARKPQ, bv); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := RegisterBackendVerifier(config.VerifierSP1CompressedSTARKPQ, bv)
	if !errors.Is(err, ErrBackendVerifierDuplicate) {
		t.Errorf("duplicate register: got %v; want ErrBackendVerifierDuplicate", err)
	}
}

func TestLookupBackendVerifier_Missing(t *testing.T) {
	resetBackendVerifiersForTest()
	if b := lookupBackendVerifier(config.VerifierRISC0SuccinctSTARKPQ); b != nil {
		t.Errorf("lookup on empty registry returned non-nil")
	}
}
