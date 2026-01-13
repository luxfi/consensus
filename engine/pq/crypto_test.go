// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pq

import (
	"testing"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestNewCertificateGenerator(t *testing.T) {
	// Generate 32-byte keys for proper initialization
	blsKey := make([]byte, 32)
	pqKey := make([]byte, 32)
	for i := range blsKey {
		blsKey[i] = byte(i + 1)
		pqKey[i] = byte(i + 100)
	}

	cg := NewCertificateGenerator(blsKey, pqKey)

	require.NotNil(t, cg)
	// Keys are now internal - verify they were initialized by checking derived values
	require.NotNil(t, cg.blsSecretKey)
	require.NotNil(t, cg.blsPublicKey)
	require.NotNil(t, cg.ringtailGroup)
}

func TestNewCertificateGenerator_ShortKeys(t *testing.T) {
	// Short keys should result in nil internal keys
	blsKey := []byte("short")
	pqKey := []byte("short")

	cg := NewCertificateGenerator(blsKey, pqKey)

	require.NotNil(t, cg)
	require.Nil(t, cg.blsSecretKey) // Too short to initialize
	require.Nil(t, cg.ringtailGroup)
}

func TestGenerateBLSSignature(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()

	sig, err := cg.GenerateBLSSignature(blockID)

	require.NoError(t, err)
	require.NotNil(t, sig)
	require.Len(t, sig, 96) // BLS G2 signature is 96 bytes
}

func TestSignBlock(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()

	sig, err := cg.SignBlock(blockID)

	require.NoError(t, err)
	require.NotNil(t, sig)
	require.Len(t, sig, 96) // BLS G2 signature is 96 bytes
}

func TestGenerateBLSSignature_NoKey(t *testing.T) {
	cg := NewCertificateGenerator(nil, nil)

	blockID := ids.GenerateTestID()
	sig, err := cg.GenerateBLSSignature(blockID)

	require.Error(t, err)
	require.Nil(t, sig)
	require.Contains(t, err.Error(), "BLS key not initialized")
}

func TestGeneratePQSignature(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()

	sig, err := cg.GeneratePQSignature(blockID)

	require.NoError(t, err)
	require.NotNil(t, sig)
	require.NotEmpty(t, sig) // Returns group key bytes
}

func TestGeneratePQSignature_NoKey(t *testing.T) {
	cg := NewCertificateGenerator(nil, nil)

	blockID := ids.GenerateTestID()
	sig, err := cg.GeneratePQSignature(blockID)

	require.Error(t, err)
	require.Nil(t, sig)
	require.Contains(t, err.Error(), "Ringtail group not initialized")
}

func TestGenerateBLSAggregate(t *testing.T) {
	// Create two generators to get two signatures to aggregate
	blsKey1, pqKey1 := GenerateTestKeys()
	blsKey2, pqKey2 := GenerateTestKeys()

	cg1 := NewCertificateGenerator(blsKey1, pqKey1)
	cg2 := NewCertificateGenerator(blsKey2, pqKey2)

	blockID := ids.GenerateTestID()

	// Generate individual signatures
	sig1, err := cg1.GenerateBLSSignature(blockID)
	require.NoError(t, err)

	sig2, err := cg2.GenerateBLSSignature(blockID)
	require.NoError(t, err)

	// Aggregate signatures
	aggSig, err := cg1.GenerateBLSAggregate(blockID, [][]byte{sig1, sig2})

	require.NoError(t, err)
	require.NotNil(t, aggSig)
	require.Len(t, aggSig, 96) // Aggregate signature is also 96 bytes
}

func TestGenerateBLSAggregate_Empty(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()

	_, err := cg.GenerateBLSAggregate(blockID, [][]byte{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "no signatures to aggregate")
}

func TestGenerateBLSAggregate_InvalidSignatures(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()
	invalidSigs := [][]byte{
		[]byte("not-a-valid-signature"),
		[]byte("another-invalid-sig"),
	}

	_, err := cg.GenerateBLSAggregate(blockID, invalidSigs)

	require.Error(t, err)
	require.Contains(t, err.Error(), "no valid signatures to aggregate")
}

func TestGeneratePQCertificate(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()
	sessionID := 1
	prfKey := make([]byte, 32)
	for i := range prfKey {
		prfKey[i] = byte(i)
	}
	signers := []int{1, 2}

	round1Data, err := cg.GeneratePQCertificate(blockID, sessionID, prfKey, signers)

	require.NoError(t, err)
	require.NotNil(t, round1Data)
}

func TestGeneratePQCertificate_NoSigner(t *testing.T) {
	cg := NewCertificateGenerator(nil, nil)

	blockID := ids.GenerateTestID()
	_, err := cg.GeneratePQCertificate(blockID, 1, nil, nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "Ringtail signer not initialized")
}

func TestVerifyBLSAggregate(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()

	// Generate signature
	sig, err := cg.GenerateBLSSignature(blockID)
	require.NoError(t, err)

	// Get public key
	pubKey := cg.GetBLSPublicKey()
	require.NotNil(t, pubKey)

	// Verify (single signature counts as aggregate of 1)
	err = VerifyBLSAggregate(blockID[:], sig, [][]byte{pubKey})
	require.NoError(t, err)
}

func TestVerifyBLSAggregate_EmptySignature(t *testing.T) {
	err := VerifyBLSAggregate([]byte("msg"), nil, [][]byte{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty aggregate signature")
}

func TestVerifyBLSAggregate_InvalidSignature(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	pubKey := cg.GetBLSPublicKey()
	invalidSig := make([]byte, 96) // Valid length but garbage content

	err := VerifyBLSAggregate([]byte("msg"), invalidSig, [][]byte{pubKey})
	// Should return error because signature bytes don't decode to valid point
	require.Error(t, err)
}

func TestVerifyBLSAggregate_NoValidPublicKeys(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()
	sig, err := cg.GenerateBLSSignature(blockID)
	require.NoError(t, err)

	invalidPubKeys := [][]byte{
		[]byte("invalid-pubkey"),
		[]byte("another-invalid"),
	}

	err = VerifyBLSAggregate(blockID[:], sig, invalidPubKeys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no valid public keys")
}

func TestGetBLSPublicKey(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	pubKey := cg.GetBLSPublicKey()

	require.NotNil(t, pubKey)
	require.Len(t, pubKey, 48) // BLS G1 public key compressed is 48 bytes
}

func TestGetBLSPublicKey_NoKey(t *testing.T) {
	cg := NewCertificateGenerator(nil, nil)

	pubKey := cg.GetBLSPublicKey()

	require.Nil(t, pubKey)
}

func TestGetRingtailGroupKey(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	groupKey := cg.GetRingtailGroupKey()

	require.NotNil(t, groupKey)
}

func TestGetRingtailGroupKey_NoKey(t *testing.T) {
	cg := NewCertificateGenerator(nil, nil)

	groupKey := cg.GetRingtailGroupKey()

	require.Nil(t, groupKey)
}

func TestGenerateTestKeys(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()

	require.NotNil(t, blsKey)
	require.NotNil(t, pqKey)
	require.Len(t, blsKey, 32)
	require.Len(t, pqKey, 32)

	// Keys should be random, so calling again should produce different keys
	blsKey2, pqKey2 := GenerateTestKeys()

	// With random generation, keys should be different
	require.NotEqual(t, blsKey, blsKey2)
	require.NotEqual(t, pqKey, pqKey2)
}

func TestSignatureDeterminism(t *testing.T) {
	blsKey := make([]byte, 32)
	pqKey := make([]byte, 32)
	for i := range blsKey {
		blsKey[i] = byte(i + 1)
		pqKey[i] = byte(i + 100)
	}

	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()

	// Generate twice and compare - BLS signatures are deterministic
	sig1, err1 := cg.GenerateBLSSignature(blockID)
	sig2, err2 := cg.GenerateBLSSignature(blockID)

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.Equal(t, sig1, sig2) // BLS signatures are deterministic for same key+message
}

func TestCertificateIntegration(t *testing.T) {
	// Test full flow: generate and verify
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()

	// Generate BLS signature
	blsSig, err := cg.GenerateBLSSignature(blockID)
	require.NoError(t, err)
	require.Len(t, blsSig, 96)

	// Get public key
	pubKey := cg.GetBLSPublicKey()
	require.NotNil(t, pubKey)
	require.Len(t, pubKey, 48)

	// Verify the signature
	err = VerifyBLSAggregate(blockID[:], blsSig, [][]byte{pubKey})
	require.NoError(t, err)

	// Generate PQ signature (Round1 data)
	prfKey := make([]byte, 32)
	round1, err := cg.GeneratePQCertificate(blockID, 1, prfKey, []int{1})
	require.NoError(t, err)
	require.NotNil(t, round1)
}

func TestMultipleSignersAggregate(t *testing.T) {
	// Create 3 signers
	cg1 := NewCertificateGenerator(GenerateTestKeys())
	cg2 := NewCertificateGenerator(GenerateTestKeys())
	cg3 := NewCertificateGenerator(GenerateTestKeys())

	blockID := ids.GenerateTestID()

	// Each signer signs the block
	sig1, err := cg1.GenerateBLSSignature(blockID)
	require.NoError(t, err)

	sig2, err := cg2.GenerateBLSSignature(blockID)
	require.NoError(t, err)

	sig3, err := cg3.GenerateBLSSignature(blockID)
	require.NoError(t, err)

	// Aggregate all signatures
	aggSig, err := cg1.GenerateBLSAggregate(blockID, [][]byte{sig1, sig2, sig3})
	require.NoError(t, err)
	require.Len(t, aggSig, 96)

	// Collect public keys
	pubKeys := [][]byte{
		cg1.GetBLSPublicKey(),
		cg2.GetBLSPublicKey(),
		cg3.GetBLSPublicKey(),
	}

	// Verify aggregate
	err = VerifyBLSAggregate(blockID[:], aggSig, pubKeys)
	require.NoError(t, err)
}

func TestInitializeThreshold(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	// Get the existing share
	existingGroup := cg.GetRingtailGroupKey()
	require.NotNil(t, existingGroup)

	// The internal share should exist after construction
	require.NotNil(t, cg.ringtailShare)
	require.NotNil(t, cg.ringtailSigner)
}

func BenchmarkGenerateBLSSignature(b *testing.B) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)
	blockID := ids.GenerateTestID()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cg.GenerateBLSSignature(blockID)
	}
}

func BenchmarkGenerateBLSAggregate(b *testing.B) {
	cg1 := NewCertificateGenerator(GenerateTestKeys())
	cg2 := NewCertificateGenerator(GenerateTestKeys())
	cg3 := NewCertificateGenerator(GenerateTestKeys())

	blockID := ids.GenerateTestID()

	sig1, _ := cg1.GenerateBLSSignature(blockID)
	sig2, _ := cg2.GenerateBLSSignature(blockID)
	sig3, _ := cg3.GenerateBLSSignature(blockID)
	sigs := [][]byte{sig1, sig2, sig3}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cg1.GenerateBLSAggregate(blockID, sigs)
	}
}

func BenchmarkVerifyBLSAggregate(b *testing.B) {
	cg := NewCertificateGenerator(GenerateTestKeys())
	blockID := ids.GenerateTestID()

	sig, _ := cg.GenerateBLSSignature(blockID)
	pubKey := cg.GetBLSPublicKey()
	pubKeys := [][]byte{pubKey}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VerifyBLSAggregate(blockID[:], sig, pubKeys)
	}
}

func BenchmarkGeneratePQCertificate(b *testing.B) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)
	blockID := ids.GenerateTestID()
	prfKey := make([]byte, 32)
	signers := []int{1, 2}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cg.GeneratePQCertificate(blockID, i, prfKey, signers)
	}
}
