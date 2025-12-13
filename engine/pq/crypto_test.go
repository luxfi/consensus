// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pq

import (
	"testing"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestNewCertificateGenerator(t *testing.T) {
	blsKey := []byte("test-bls-key")
	pqKey := []byte("test-pq-key")

	cg := NewCertificateGenerator(blsKey, pqKey)

	require.NotNil(t, cg)
	require.Equal(t, blsKey, cg.blsKey)
	require.Equal(t, pqKey, cg.pqKey)
}

func TestGenerateBLSAggregate(t *testing.T) {
	blsKey := []byte("test-bls-key")
	pqKey := []byte("test-pq-key")
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()
	votes := map[string]int{
		"validator1": 5,
		"validator2": 3,
	}

	sig, err := cg.GenerateBLSAggregate(blockID, votes)

	require.NoError(t, err)
	require.NotNil(t, sig)
	require.Len(t, sig, 32) // SHA256 hash length
}

func TestGenerateBLSAggregate_EmptyKey(t *testing.T) {
	cg := NewCertificateGenerator(nil, []byte("pq-key"))

	blockID := ids.GenerateTestID()
	votes := map[string]int{"validator1": 5}

	sig, err := cg.GenerateBLSAggregate(blockID, votes)

	require.Error(t, err)
	require.Nil(t, sig)
	require.Contains(t, err.Error(), "BLS key not initialized")
}

func TestGenerateBLSAggregate_EmptyBlsKey(t *testing.T) {
	cg := NewCertificateGenerator([]byte{}, []byte("pq-key"))

	blockID := ids.GenerateTestID()
	votes := map[string]int{"validator1": 5}

	sig, err := cg.GenerateBLSAggregate(blockID, votes)

	require.Error(t, err)
	require.Nil(t, sig)
	require.Contains(t, err.Error(), "BLS key not initialized")
}

func TestGenerateBLSAggregate_NegativeCount(t *testing.T) {
	blsKey := []byte("test-bls-key")
	pqKey := []byte("test-pq-key")
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()
	votes := map[string]int{
		"validator1": -5, // negative count
		"validator2": 3,
	}

	sig, err := cg.GenerateBLSAggregate(blockID, votes)

	require.NoError(t, err)
	require.NotNil(t, sig)
	require.Len(t, sig, 32)
}

func TestGenerateBLSAggregate_EmptyVotes(t *testing.T) {
	blsKey := []byte("test-bls-key")
	pqKey := []byte("test-pq-key")
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()
	votes := map[string]int{}

	sig, err := cg.GenerateBLSAggregate(blockID, votes)

	require.NoError(t, err)
	require.NotNil(t, sig)
	require.Len(t, sig, 32)
}

func TestGeneratePQCertificate(t *testing.T) {
	blsKey := []byte("test-bls-key")
	pqKey := []byte("test-pq-key")
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()
	votes := map[string]int{
		"validator1": 5,
		"validator2": 3,
	}

	cert, err := cg.GeneratePQCertificate(blockID, votes)

	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Len(t, cert, 32) // SHA256 hash length
}

func TestGeneratePQCertificate_EmptyKey(t *testing.T) {
	cg := NewCertificateGenerator([]byte("bls-key"), nil)

	blockID := ids.GenerateTestID()
	votes := map[string]int{"validator1": 5}

	cert, err := cg.GeneratePQCertificate(blockID, votes)

	require.Error(t, err)
	require.Nil(t, cert)
	require.Contains(t, err.Error(), "PQ key not initialized")
}

func TestGeneratePQCertificate_EmptyPqKey(t *testing.T) {
	cg := NewCertificateGenerator([]byte("bls-key"), []byte{})

	blockID := ids.GenerateTestID()
	votes := map[string]int{"validator1": 5}

	cert, err := cg.GeneratePQCertificate(blockID, votes)

	require.Error(t, err)
	require.Nil(t, cert)
	require.Contains(t, err.Error(), "PQ key not initialized")
}

func TestGeneratePQCertificate_NegativeCount(t *testing.T) {
	blsKey := []byte("test-bls-key")
	pqKey := []byte("test-pq-key")
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()
	votes := map[string]int{
		"validator1": -10, // negative count
		"validator2": 3,
	}

	cert, err := cg.GeneratePQCertificate(blockID, votes)

	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Len(t, cert, 32)
}

func TestGeneratePQCertificate_EmptyVotes(t *testing.T) {
	blsKey := []byte("test-bls-key")
	pqKey := []byte("test-pq-key")
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()
	votes := map[string]int{}

	cert, err := cg.GeneratePQCertificate(blockID, votes)

	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Len(t, cert, 32)
}

func TestVerifyBLSAggregate(t *testing.T) {
	blockID := ids.GenerateTestID()
	votes := map[string]int{"validator1": 5}
	signature := make([]byte, 32) // Valid 32-byte signature
	publicKeys := [][]byte{[]byte("pubkey1")}

	err := VerifyBLSAggregate(blockID, votes, signature, publicKeys)

	require.NoError(t, err)
}

func TestVerifyBLSAggregate_EmptySignature(t *testing.T) {
	blockID := ids.GenerateTestID()
	votes := map[string]int{"validator1": 5}
	publicKeys := [][]byte{[]byte("pubkey1")}

	err := VerifyBLSAggregate(blockID, votes, nil, publicKeys)

	require.Error(t, err)
	require.Contains(t, err.Error(), "empty signature")
}

func TestVerifyBLSAggregate_InvalidSignatureLength(t *testing.T) {
	blockID := ids.GenerateTestID()
	votes := map[string]int{"validator1": 5}
	signature := make([]byte, 16) // Invalid length
	publicKeys := [][]byte{[]byte("pubkey1")}

	err := VerifyBLSAggregate(blockID, votes, signature, publicKeys)

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid signature length")
}

func TestVerifyPQCertificate(t *testing.T) {
	blockID := ids.GenerateTestID()
	votes := map[string]int{"validator1": 5}
	certificate := make([]byte, 32) // Valid 32-byte certificate
	publicKeys := [][]byte{[]byte("pubkey1")}

	err := VerifyPQCertificate(blockID, votes, certificate, publicKeys)

	require.NoError(t, err)
}

func TestVerifyPQCertificate_EmptyCertificate(t *testing.T) {
	blockID := ids.GenerateTestID()
	votes := map[string]int{"validator1": 5}
	publicKeys := [][]byte{[]byte("pubkey1")}

	err := VerifyPQCertificate(blockID, votes, nil, publicKeys)

	require.Error(t, err)
	require.Contains(t, err.Error(), "empty certificate")
}

func TestVerifyPQCertificate_InvalidCertificateLength(t *testing.T) {
	blockID := ids.GenerateTestID()
	votes := map[string]int{"validator1": 5}
	certificate := make([]byte, 16) // Invalid length
	publicKeys := [][]byte{[]byte("pubkey1")}

	err := VerifyPQCertificate(blockID, votes, certificate, publicKeys)

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid certificate length")
}

func TestGenerateTestKeys(t *testing.T) {
	blsKey, pqKey := GenerateTestKeys()

	require.NotNil(t, blsKey)
	require.NotNil(t, pqKey)
	require.NotEmpty(t, blsKey)
	require.NotEmpty(t, pqKey)

	// Test determinism - calling again should produce same keys
	blsKey2, pqKey2 := GenerateTestKeys()

	require.Equal(t, blsKey, blsKey2)
	require.Equal(t, pqKey, pqKey2)
}

func TestCertificateDeterminism(t *testing.T) {
	blsKey := []byte("deterministic-bls-key")
	pqKey := []byte("deterministic-pq-key")
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()
	votes := map[string]int{
		"validator1": 5,
	}

	// Generate twice and compare
	sig1, err1 := cg.GenerateBLSAggregate(blockID, votes)
	sig2, err2 := cg.GenerateBLSAggregate(blockID, votes)

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.Equal(t, sig1, sig2) // Should be deterministic

	cert1, err1 := cg.GeneratePQCertificate(blockID, votes)
	cert2, err2 := cg.GeneratePQCertificate(blockID, votes)

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.Equal(t, cert1, cert2) // Should be deterministic
}

func TestCertificateIntegration(t *testing.T) {
	// Test full flow: generate and verify
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)

	blockID := ids.GenerateTestID()
	votes := map[string]int{
		"validator1": 10,
		"validator2": 8,
		"validator3": 7,
	}

	// Generate certificates
	blsSig, err := cg.GenerateBLSAggregate(blockID, votes)
	require.NoError(t, err)

	pqCert, err := cg.GeneratePQCertificate(blockID, votes)
	require.NoError(t, err)

	// Verify certificates
	err = VerifyBLSAggregate(blockID, votes, blsSig, nil)
	require.NoError(t, err)

	err = VerifyPQCertificate(blockID, votes, pqCert, nil)
	require.NoError(t, err)
}

func BenchmarkGenerateBLSAggregate(b *testing.B) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)
	blockID := ids.GenerateTestID()
	votes := map[string]int{
		"validator1": 10,
		"validator2": 8,
		"validator3": 7,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cg.GenerateBLSAggregate(blockID, votes)
	}
}

func BenchmarkGeneratePQCertificate(b *testing.B) {
	blsKey, pqKey := GenerateTestKeys()
	cg := NewCertificateGenerator(blsKey, pqKey)
	blockID := ids.GenerateTestID()
	votes := map[string]int{
		"validator1": 10,
		"validator2": 8,
		"validator3": 7,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cg.GeneratePQCertificate(blockID, votes)
	}
}

func BenchmarkVerifyBLSAggregate(b *testing.B) {
	blockID := ids.GenerateTestID()
	votes := map[string]int{"validator1": 5}
	signature := make([]byte, 32)
	publicKeys := [][]byte{[]byte("pubkey1")}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VerifyBLSAggregate(blockID, votes, signature, publicKeys)
	}
}

func BenchmarkVerifyPQCertificate(b *testing.B) {
	blockID := ids.GenerateTestID()
	votes := map[string]int{"validator1": 5}
	certificate := make([]byte, 32)
	publicKeys := [][]byte{[]byte("pubkey1")}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VerifyPQCertificate(blockID, votes, certificate, publicKeys)
	}
}
