// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"crypto/rand"
	"testing"

	"github.com/luxfi/crypto/mldsa"
)

// TestVerifyWithRealKeys_PurePQ_NoBLS is the regression guard for the
// fully-PQ finality property: a QuasarCert that carries NO BLS leg
// (pure-PQ posture) MUST finalise on its lattice leg alone. Before the
// fix, VerifyWithRealKeys hard-required BLS (`if len(c.BLS)==0 return
// false`), which meant a quantum break of BLS-12-381 could forge any
// cert that passed verification — the classical fast path was load-
// bearing, not optional. BLS is now one OPTIONAL leg; a pure-PQ cert
// (ML-DSA-65 identity leg, BLS empty) verifies with a nil BLS key.
func TestVerifyWithRealKeys_PurePQ_NoBLS(t *testing.T) {
	msg := []byte("pure-pq-finality-digest-32-bytes")

	// One real per-validator ML-DSA-65 (FIPS 204) signature — the PQ leg.
	sk, err := mldsa.GenerateKey(rand.Reader, mldsa.MLDSA65)
	if err != nil {
		t.Fatalf("mldsa.GenerateKey: %v", err)
	}
	sig, err := sk.Sign(rand.Reader, msg, nil)
	if err != nil {
		t.Fatalf("mldsa.Sign: %v", err)
	}
	pk := sk.PublicKey

	cert := &QuasarCert{
		// BLS intentionally empty: pure-PQ posture.
		MLDSARollup: EncodeMLDSASigs([][]byte{sig}),
		Validators:  1,
	}

	// The fix: pure-PQ cert verifies with a NIL BLS key.
	if !cert.VerifyWithRealKeys(msg, nil, nil, []*mldsa.PublicKey{pk}) {
		t.Fatal("pure-PQ QuasarCert (no BLS) must verify on its ML-DSA leg with a nil BLS key")
	}

	// HasClassicalFastPath reports the documented contract: no BLS here.
	if cert.HasClassicalFastPath() {
		t.Fatal("pure-PQ cert must report HasClassicalFastPath()=false")
	}

	// Tampered PQ leg must still be rejected — the leg is cryptographically
	// checked, not merely present.
	badMsg := []byte("a-different-digest-not-what-signed")
	if cert.VerifyWithRealKeys(badMsg, nil, nil, []*mldsa.PublicKey{pk}) {
		t.Fatal("pure-PQ cert must reject when the ML-DSA leg does not verify against the message")
	}
}

// TestVerifyWithRealKeys_FailClosed_NoLeg guards the safety side of making
// BLS optional: a cert with NO verifiable leg (every leg empty, or bytes
// present but no key supplied) MUST be rejected. Finality may never rest
// on the ABSENCE of evidence.
func TestVerifyWithRealKeys_FailClosed_NoLeg(t *testing.T) {
	msg := []byte("fail-closed-finality-digest-3232")

	// Empty cert: no BLS, no Corona, no rollup → zero verified legs.
	empty := &QuasarCert{Validators: 1}
	if empty.VerifyWithRealKeys(msg, nil, nil, nil) {
		t.Fatal("empty cert (no legs) must be REJECTED — fail-closed, never finalise on absence of evidence")
	}

	// Rollup bytes present but no ML-DSA keys supplied → cannot verify the
	// only leg → reject (not silently skip into a 0-leg accept).
	sk, err := mldsa.GenerateKey(rand.Reader, mldsa.MLDSA65)
	if err != nil {
		t.Fatalf("mldsa.GenerateKey: %v", err)
	}
	sig, err := sk.Sign(rand.Reader, msg, nil)
	if err != nil {
		t.Fatalf("mldsa.Sign: %v", err)
	}
	rollupNoKeys := &QuasarCert{
		MLDSARollup: EncodeMLDSASigs([][]byte{sig}),
		Validators:  1,
	}
	if rollupNoKeys.VerifyWithRealKeys(msg, nil, nil, nil) {
		t.Fatal("cert with rollup bytes but no ML-DSA keys must be REJECTED, not accepted on a skipped leg")
	}
}
