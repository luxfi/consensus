// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// verify_authority_test.go -- Item7(b) regression lock (shipped v1.35.30
// surface): the structural QuasarCert.Verify is NOT the finalization
// authority; QuasarCert.VerifyUnderPolicy (the policy-driven cryptographic
// verifier that pkg/wire's quantum finality actually calls) is. This proves
// it by constructing a cert that is correctly SHAPED (every leg the
// structural gate demands is non-empty) but entirely FORGED (garbage bytes),
// showing Verify accepts it while VerifyUnderPolicy — and VerifyWithRealKeys
// — reject it.

package quasar

import (
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/crypto/bls"
)

// TestQuasarCertVerify_StructuralOnly_IsNotFinalizationAuthority locks the
// invariant documented on QuasarCert.Verify.
func TestQuasarCertVerify_StructuralOnly_IsNotFinalizationAuthority(t *testing.T) {
	// Forged-but-shaped: BLS + Corona + MLDSARollup all present (garbage),
	// which is exactly what the structural gate checks.
	forged := &QuasarCert{
		BLS:         []byte("not-a-real-bls-signature-at-all"),
		Corona:      []byte{0x01, 0x02, 0x03},
		MLDSARollup: []byte{0x04, 0x05, 0x06},
	}

	// (1) The structural gate passes it.
	if !forged.Verify(nil) {
		t.Fatal("expected structural Verify() to pass a forged-but-shaped cert")
	}

	// (2) The production authority rejects it. Policy (PQ-off, Hybrid)
	//     requires exactly the BLS leg; a real (fresh) BLS key is supplied,
	//     so VerifyUnderPolicy reaches the BLS signature check and fails
	//     there — the forged BLS bytes verify under no key.
	sk, err := bls.NewSecretKey()
	if err != nil {
		t.Fatalf("bls.NewSecretKey: %v", err)
	}
	pk := sk.PublicKey()

	policy := config.CertPolicy{Mode: config.CertModeOff, Variant: config.CertVariantHybrid}
	if got := policy.RequiredLegs(); len(got) != 1 || got[0] != config.LegBLS {
		t.Fatalf("precondition: (PQ-off,Hybrid) RequiredLegs = %v, want [LegBLS]", got)
	}

	keys := CertKeys{BLS: pk}
	if forged.VerifyUnderPolicy([]byte("some finality message"), policy, keys) {
		t.Fatal("VerifyUnderPolicy (the production authority) must reject a cert that only passes the structural gate")
	}

	// (3) The lower-level real-key verifier rejects it too.
	if forged.VerifyWithRealKeys([]byte("some finality message"), pk, nil, nil) {
		t.Fatal("VerifyWithRealKeys must reject a cert that only passes the structural gate")
	}
	if forged.VerifyWithRealKeysPolaris([]byte("some finality message"), pk, nil, nil, nil, nil) {
		t.Fatal("VerifyWithRealKeysPolaris must reject a cert that only passes the structural gate")
	}
}

// TestQuasarCertVerify_NilAndEmptyCert confirms the structural gate itself
// fails closed on degenerate inputs.
func TestQuasarCertVerify_NilAndEmptyCert(t *testing.T) {
	var nilCert *QuasarCert
	if nilCert.Verify(nil) {
		t.Fatal("nil *QuasarCert must not verify")
	}
	if (&QuasarCert{}).Verify(nil) {
		t.Fatal("empty QuasarCert (no legs) must not verify")
	}
}
