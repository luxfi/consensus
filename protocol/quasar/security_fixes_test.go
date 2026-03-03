// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Security regression tests for red team findings.

package quasar

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCRITICAL1_ForgedThresholdSigRejected verifies that a forged signature
// with IsThreshold=true is rejected by VerifyQuasarSig.
//
// Before the fix, VerifyQuasarSigWithContext returned true for any sig with
// IsThreshold=true without verifying the BLS bytes. This allowed a complete
// consensus bypass.
func TestCRITICAL1_ForgedThresholdSigRejected(t *testing.T) {
	s, err := newSigner(2)
	require.NoError(t, err)

	err = s.AddValidator("v1", 100)
	require.NoError(t, err)

	message := []byte("consensus-block-hash")

	// Construct a forged signature with IsThreshold=true and garbage BLS bytes
	forgedSig := &QuasarSig{
		BLS:         []byte("this-is-not-a-valid-bls-signature-at-all"),
		ValidatorID: "v1",
		IsThreshold: true,
		SignerIndex:  0,
	}

	require.False(t, s.VerifyQuasarSig(message, forgedSig),
		"CRITICAL: forged threshold signature must be rejected")
}

// TestCRITICAL1_ThresholdSigWithEmptyBLS verifies that an empty BLS field
// with IsThreshold=true is rejected.
func TestCRITICAL1_ThresholdSigWithEmptyBLS(t *testing.T) {
	s, err := newSigner(2)
	require.NoError(t, err)

	err = s.AddValidator("v1", 100)
	require.NoError(t, err)

	emptySig := &QuasarSig{
		BLS:         nil,
		ValidatorID: "v1",
		IsThreshold: true,
	}

	require.False(t, s.VerifyQuasarSig([]byte("msg"), emptySig),
		"threshold sig with empty BLS must be rejected")
}

// TestCRITICAL1_ThresholdSigWithoutVerifier verifies that threshold sig
// verification fails when no BLS verifier is configured.
func TestCRITICAL1_ThresholdSigWithoutVerifier(t *testing.T) {
	// newSigner creates a signer without threshold scheme (no blsVerifier)
	s, err := newSigner(2)
	require.NoError(t, err)

	sig := &QuasarSig{
		BLS:         make([]byte, 96), // valid length, garbage content
		ValidatorID: "v1",
		IsThreshold: true,
	}

	require.False(t, s.VerifyQuasarSig([]byte("msg"), sig),
		"threshold sig must fail without configured verifier")
}

// TestCRITICAL2_BlockCertVerifyRejectGarbage verifies that BlockCert.Verify
// with garbage BLS/PQ bytes returns false.
//
// Before the fix, Verify only checked len(BLS) > 0 && len(PQ) > 0, meaning
// any non-empty bytes passed verification. No cryptographic check.
func TestCRITICAL2_BlockCertVerifyRejectGarbage(t *testing.T) {
	cert := &BlockCert{
		BLS:  []byte("garbage-bls-not-a-real-signature"),
		PQ:   []byte("garbage-pq-not-a-real-certificate"),
		Sigs: make(map[string][]byte),
	}

	require.False(t, cert.Verify([]string{"v1", "v2", "v3"}),
		"CRITICAL: BlockCert.Verify must not pass with garbage bytes")
}

// TestCRITICAL2_BlockCertVerifyWithKeysNilCert verifies nil safety.
func TestCRITICAL2_BlockCertVerifyWithKeysNilCert(t *testing.T) {
	var cert *BlockCert
	require.False(t, cert.VerifyWithKeys([]byte("key"), []byte("pq")),
		"nil cert must fail")
}

// TestCRITICAL2_BlockCertVerifyWithKeysEmptyFields verifies that empty
// BLS or PQ fields are rejected.
func TestCRITICAL2_BlockCertVerifyWithKeysEmptyFields(t *testing.T) {
	// Empty BLS
	cert := &BlockCert{BLS: nil, PQ: []byte("pq")}
	require.False(t, cert.VerifyWithKeys([]byte("key"), []byte("pq")))

	// Empty PQ
	cert = &BlockCert{BLS: []byte("bls"), PQ: nil}
	require.False(t, cert.VerifyWithKeys([]byte("key"), []byte("pq")))

	// Empty group key
	cert = &BlockCert{BLS: []byte("bls"), PQ: []byte("pq")}
	require.False(t, cert.VerifyWithKeys(nil, []byte("pq")))
}

// TestHIGH3_NewEngineRejectsThresholdOne verifies that NewEngine rejects
// threshold=1 which allows single-validator consensus bypass.
func TestHIGH3_NewEngineRejectsThresholdOne(t *testing.T) {
	cfg := Config{QThreshold: 1, QuasarTimeout: 30}
	_, err := NewEngine(cfg)
	require.ErrorIs(t, err, ErrThresholdTooLow,
		"NewEngine must reject threshold=1")
}

// TestHIGH3_NewEngineRejectsThresholdZero verifies threshold=0 rejection.
func TestHIGH3_NewEngineRejectsThresholdZero(t *testing.T) {
	cfg := Config{QThreshold: 0, QuasarTimeout: 30}
	_, err := NewEngine(cfg)
	require.ErrorIs(t, err, ErrThresholdTooLow,
		"NewEngine must reject threshold=0")
}

// TestHIGH3_NewEngineRejectsNegativeThreshold verifies negative threshold rejection.
func TestHIGH3_NewEngineRejectsNegativeThreshold(t *testing.T) {
	cfg := Config{QThreshold: -1, QuasarTimeout: 30}
	_, err := NewEngine(cfg)
	require.ErrorIs(t, err, ErrThresholdTooLow,
		"NewEngine must reject negative threshold")
}

// TestHIGH3_NewQuasarRejectsThresholdOne verifies that NewQuasar rejects
// threshold=1.
func TestHIGH3_NewQuasarRejectsThresholdOne(t *testing.T) {
	_, err := NewQuasar(1)
	require.ErrorIs(t, err, ErrThresholdTooLow,
		"NewQuasar must reject threshold=1")
}

// TestHIGH3_NewEngineAcceptsThresholdTwo verifies threshold=2 is accepted.
func TestHIGH3_NewEngineAcceptsThresholdTwo(t *testing.T) {
	cfg := Config{QThreshold: 2, QuasarTimeout: 30}
	engine, err := NewEngine(cfg)
	require.NoError(t, err)
	require.NotNil(t, engine)
}

// TestHIGH3_NewTestEngineAllowsThresholdOne verifies the test-only
// constructor accepts threshold=1 for single-node testing.
func TestHIGH3_NewTestEngineAllowsThresholdOne(t *testing.T) {
	cfg := Config{QThreshold: 1, QuasarTimeout: 30}
	engine, err := NewTestEngine(cfg)
	require.NoError(t, err)
	require.NotNil(t, engine)
}

// TestHIGH3_NewTestQuasarAllowsThresholdOne verifies the test-only
// constructor accepts threshold=1.
func TestHIGH3_NewTestQuasarAllowsThresholdOne(t *testing.T) {
	qa, err := NewTestQuasar(1)
	require.NoError(t, err)
	require.NotNil(t, qa)
}
