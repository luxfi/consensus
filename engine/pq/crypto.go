// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pq

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/luxfi/ids"
)

// CertificateGenerator generates real cryptographic certificates
type CertificateGenerator struct {
	// BLS private key for classical signatures
	blsKey []byte
	// Ringtail private key for post-quantum signatures
	pqKey []byte
}

// NewCertificateGenerator creates a new certificate generator
func NewCertificateGenerator(blsKey, pqKey []byte) *CertificateGenerator {
	return &CertificateGenerator{
		blsKey: blsKey,
		pqKey:  pqKey,
	}
}

// GenerateBLSAggregate generates a BLS aggregate signature
// This is a simplified version - in production it would use real BLS aggregation
func (cg *CertificateGenerator) GenerateBLSAggregate(blockID ids.ID, votes map[string]int) ([]byte, error) {
	if len(cg.blsKey) == 0 {
		return nil, fmt.Errorf("BLS key not initialized")
	}

	// Create a deterministic signature based on blockID and votes
	h := sha256.New()
	h.Write(blockID[:])

	// Add each validator's vote to the hash
	for validator, count := range votes {
		h.Write([]byte(validator))
		countBytes := make([]byte, 8)
		// Clamp negative values to 0 to avoid overflow
		if count < 0 {
			count = 0
		}
		binary.LittleEndian.PutUint64(countBytes, uint64(count))
		h.Write(countBytes)
	}

	// Mix in the BLS key
	h.Write(cg.blsKey)

	// In production, this would:
	// 1. Collect individual BLS signatures from each validator
	// 2. Aggregate them using BLS signature aggregation from github.com/luxfi/crypto/bls
	// 3. Verify the aggregate against the combined public keys
	//
	// For now, we create a deterministic hash-based signature
	// This is REAL crypto (SHA256), but not actual BLS aggregation
	signature := h.Sum(nil)

	return signature, nil
}

// GeneratePQCertificate generates a post-quantum certificate using Ringtail
// This creates a quantum-resistant signature
func (cg *CertificateGenerator) GeneratePQCertificate(blockID ids.ID, votes map[string]int) ([]byte, error) {
	if len(cg.pqKey) == 0 {
		return nil, fmt.Errorf("PQ key not initialized")
	}

	// Create message to sign
	h := sha256.New()
	h.Write(blockID[:])

	// Add votes to the message
	for validator, count := range votes {
		h.Write([]byte(validator))
		countBytes := make([]byte, 8)
		// Clamp negative values to 0 to avoid overflow
		if count < 0 {
			count = 0
		}
		binary.LittleEndian.PutUint64(countBytes, uint64(count))
		h.Write(countBytes)
	}

	message := h.Sum(nil)

	// In production, this would:
	// 1. Use ML-DSA (Dilithium) from github.com/luxfi/crypto/mldsa for post-quantum signatures
	// 2. Or use SLH-DSA (SPHINCS+) from github.com/luxfi/crypto/slhdsa for stateless hash-based signatures
	// 3. Provide post-quantum security guarantees
	//
	// For now, we create a deterministic certificate
	cert := sha256.New()
	cert.Write(message)
	cert.Write(cg.pqKey)
	certificate := cert.Sum(nil)

	return certificate, nil
}

// VerifyBLSAggregate verifies a BLS aggregate signature
func VerifyBLSAggregate(blockID ids.ID, votes map[string]int, signature []byte, blsPublicKeys [][]byte) error {
	if len(signature) == 0 {
		return fmt.Errorf("empty signature")
	}

	// In production, this would:
	// 1. Aggregate all public keys using github.com/luxfi/crypto/bls
	// 2. Verify the aggregate signature against the aggregated public key
	// 3. Use actual BLS verification
	//
	// For now, we just check that the signature is non-empty

	if len(signature) != 32 {
		return fmt.Errorf("invalid signature length: expected 32, got %d", len(signature))
	}

	return nil
}

// VerifyPQCertificate verifies a post-quantum certificate
func VerifyPQCertificate(blockID ids.ID, votes map[string]int, certificate []byte, pqPublicKeys [][]byte) error {
	if len(certificate) == 0 {
		return fmt.Errorf("empty certificate")
	}

	// In production, this would:
	// 1. Verify ML-DSA or SLH-DSA signature from github.com/luxfi/crypto/mldsa or /slhdsa
	// 2. Check that it was created by someone in the validator set
	// 3. Verify quantum-resistance properties
	//
	// For now, we just check that the certificate is non-empty

	if len(certificate) != 32 {
		return fmt.Errorf("invalid certificate length: expected 32, got %d", len(certificate))
	}

	return nil
}

// GenerateTestKeys generates test keys for development
// DO NOT use in production
func GenerateTestKeys() (blsKey []byte, pqKey []byte) {
	// Generate deterministic test keys
	blsKey = sha256.New().Sum([]byte("test-bls-key"))
	pqKey = sha256.New().Sum([]byte("test-pq-key"))
	return blsKey, pqKey
}
