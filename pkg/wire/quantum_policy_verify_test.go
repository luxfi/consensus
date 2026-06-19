// Copyright (C) 2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wire

import (
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/quasar"
	"github.com/luxfi/crypto/bls"
	"github.com/stretchr/testify/require"
)

// quantum_policy_verify_test.go proves Red wiring task 3 deliverable (d):
// the LIVE wire-level cert verifier (QuantumPolicy) is now policy-driven —
// it routes through quasar.QuasarCert.VerifyUnderPolicy with the chain's
// CertPolicy, NOT the legacy BLS-permissive VerifyWithRealKeys path. A
// strict-PQ chain therefore REJECTS a leg-stripped (e.g. BLS-only) cert.
//
// The cryptographic half (every leg's signature verifying) is covered
// exhaustively in protocol/quasar/cert_policy_verify_test.go. Here we
// prove the WIRING: which legs are mandatory comes from the chain's
// profile-derived CertPolicy, and the live QuantumPolicy enforces it. The
// rejection below is at VerifyUnderPolicy Gate 1 (required leg / key
// present), which fires BEFORE any signature math — exactly the CRQC
// downgrade attack: forge the one leg you can break (BLS), strip every PQ
// leg, and submit. Policy-driven verification refuses it.

// quantumCert wraps qc as a wire Certificate at the given height.
func quantumCert(t *testing.T, qc *quasar.QuasarCert) *Certificate {
	t.Helper()
	proof, err := qc.MarshalBinary()
	require.NoError(t, err)
	return &Certificate{
		CandidateID: CandidateID{0xAB},
		Height:      7,
		PolicyID:    PolicyQuantum,
		Proof:       proof,
	}
}

// realBLSOnlyCert returns a cert carrying ONLY a real BLS leg (every PQ
// leg stripped) and the matching BLS public key. The BLS signature is over
// the canonical cert digest, so BLS itself verifies — the cert is rejected
// (under a strict-PQ policy) purely because the PQ legs the policy requires
// are absent, never because BLS failed.
func realBLSOnlyCert(t *testing.T) (*Certificate, *bls.PublicKey) {
	t.Helper()
	cert := &Certificate{CandidateID: CandidateID{0xAB}, Height: 7, PolicyID: PolicyQuantum}
	msg := certMessageDigest(cert)

	sk, err := bls.NewSecretKey()
	require.NoError(t, err)
	sig, err := sk.Sign(msg)
	require.NoError(t, err)

	qc := &quasar.QuasarCert{BLS: bls.SignatureToBytes(sig)}
	full := quantumCert(t, qc)
	return full, sk.PublicKey()
}

// TestQuantumPolicy_StrictPQ_RejectsLegStrippedCert is deliverable (d):
// on a strict-PQ chain, the LIVE verifier rejects a BLS-only (PQ-legs-
// stripped) cert. The policy comes from the StrictPQ security profile via
// QuantumPolicy.NewQuantumPolicyForProfile -> profile.CertPolicy()
// (Variant=Strict ⇒ BLS not a required leg; PQ legs required).
func TestQuantumPolicy_StrictPQ_RejectsLegStrippedCert(t *testing.T) {
	p := NewQuantumPolicyForProfile(2, config.StrictPQ())
	cert, blsPK := realBLSOnlyCert(t)

	// Live path with the BLS key supplied (the attacker's forged-leg key).
	// Under the strict-PQ policy this MUST be rejected: the required PQ
	// legs (Pulsar, Corona) are absent.
	err := p.VerifyWithKeys(cert, blsPK, nil, nil)
	require.Error(t, err, "strict-PQ chain must reject a BLS-only (leg-stripped) cert on the live verify path")

	// And via the canonical full-keys entry with only a BLS key supplied:
	// same rejection. Proves the live path does NOT degenerate to BLS-only.
	err = p.VerifyCertUnderPolicy(cert, quasar.CertKeys{BLS: blsPK})
	require.Error(t, err, "VerifyCertUnderPolicy must reject the leg-stripped cert under the strict-PQ policy")
}

// TestQuantumPolicy_StrictPQ_PolicyIsStrictVariant documents the source of
// truth: the strict-PQ QuantumPolicy's CertPolicy is Strict-variant (BLS
// NOT in RequiredLegs) and post-quantum. This is what makes the rejection
// above policy-driven rather than a special-case.
func TestQuantumPolicy_StrictPQ_PolicyIsStrictVariant(t *testing.T) {
	p := NewQuantumPolicyForProfile(2, config.StrictPQ())
	cp := p.certPolicy

	require.Equal(t, config.CertVariantStrict, cp.Variant,
		"strict-PQ profile must yield a Strict-variant cert policy (no BLS-permissive)")
	require.True(t, cp.IsPostQuantum(), "strict-PQ cert policy must require PQ legs")
	for _, leg := range cp.RequiredLegs() {
		require.NotEqualf(t, config.LegBLS, leg,
			"strict-PQ RequiredLegs must NOT include BLS; got %v", cp.RequiredLegs())
	}
	require.Contains(t, cp.RequiredLegs(), config.LegCorona,
		"strict-PQ RequiredLegs must include the Corona PQ leg")
}

// TestQuantumPolicy_PermissiveDefault_IsHybrid confirms the converse: the
// default (Permissive-profile) QuantumPolicy is Hybrid — BLS IS a required
// leg there (classical fast-path retained alongside the PQ legs). This is
// why the leg-stripping rejection is specific to the strict-PQ profile,
// proving the switch is profile-driven.
func TestQuantumPolicy_PermissiveDefault_IsHybrid(t *testing.T) {
	p := NewQuantumPolicy(2)
	cp := p.certPolicy
	require.Equal(t, config.CertVariantHybrid, cp.Variant)
	require.Contains(t, cp.RequiredLegs(), config.LegBLS,
		"permissive/hybrid policy keeps BLS as a required leg")
}

// TestQuantumPolicy_WrongPolicyID rejects a non-quantum certificate on the
// live path (defensive: the verifier only accepts PolicyQuantum certs).
func TestQuantumPolicy_WrongPolicyID(t *testing.T) {
	p := NewQuantumPolicyForProfile(2, config.StrictPQ())
	cert := &Certificate{PolicyID: PolicyNone}
	require.Error(t, p.VerifyCertUnderPolicy(cert, quasar.CertKeys{}))
}
