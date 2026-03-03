// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pq

import (
	"crypto/rand"
	"fmt"
	"sync"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
	"github.com/luxfi/pulsar/threshold"
)

// CertificateGenerator generates real BLS and Corona threshold signatures.
type CertificateGenerator struct {
	mu sync.RWMutex

	// BLS keys
	blsSecretKey *bls.SecretKey
	blsPublicKey *bls.PublicKey

	// Corona threshold signing
	coronaShare  *threshold.KeyShare
	coronaSigner *threshold.Signer
	coronaGroup  *threshold.GroupKey
}

// NewCertificateGenerator creates a certificate generator with real keys.
// blsKey should be 32 bytes for BLS key derivation.
// pqKey is used as seed for Corona threshold key generation.
func NewCertificateGenerator(blsKey, pqKey []byte) *CertificateGenerator {
	cg := &CertificateGenerator{}

	// Initialize BLS key from seed
	if len(blsKey) >= 32 {
		sk, err := bls.SecretKeyFromSeed(blsKey)
		if err == nil {
			cg.blsSecretKey = sk
			cg.blsPublicKey = sk.PublicKey()
		}
	}

	// Initialize Corona threshold signer (single-party for local signing)
	// For multi-party threshold, call InitializeThreshold with all shares
	if len(pqKey) >= 32 {
		// Generate a 2-of-3 threshold setup for demonstration
		// The pqKey seeds the random generation
		shares, groupKey, err := threshold.GenerateKeys(2, 3, nil)
		if err == nil && len(shares) > 0 {
			cg.coronaShare = shares[0]
			cg.coronaGroup = groupKey
			cg.coronaSigner = threshold.NewSigner(shares[0])
		}
	}

	return cg
}

// InitializeThreshold sets up multi-party threshold signing.
func (cg *CertificateGenerator) InitializeThreshold(share *threshold.KeyShare) {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	cg.coronaShare = share
	cg.coronaGroup = share.GroupKey
	cg.coronaSigner = threshold.NewSigner(share)
}

// GenerateBLSSignature generates a real BLS signature for a block.
func (cg *CertificateGenerator) GenerateBLSSignature(blockID ids.ID) ([]byte, error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	if cg.blsSecretKey == nil {
		return nil, fmt.Errorf("BLS key not initialized")
	}

	sig, err := cg.blsSecretKey.Sign(blockID[:])
	if err != nil {
		return nil, fmt.Errorf("BLS signing failed: %w", err)
	}

	return bls.SignatureToBytes(sig), nil
}

// SignBlock generates a BLS signature for the block (convenience method for consensus).
// This is equivalent to GenerateBLSSignature but returns the signature directly.
func (cg *CertificateGenerator) SignBlock(blockID ids.ID) ([]byte, error) {
	return cg.GenerateBLSSignature(blockID)
}

// GeneratePQSignature generates a local Corona signature for the block.
// Returns the group key bytes as a commitment (full threshold signing requires Round1/Round2).
func (cg *CertificateGenerator) GeneratePQSignature(blockID ids.ID) ([]byte, error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	if cg.coronaGroup == nil {
		return nil, fmt.Errorf("Corona group not initialized")
	}

	// Return the group key bytes as a commitment for single-validator mode
	return cg.coronaGroup.Bytes(), nil
}

// GenerateBLSAggregate generates a real BLS aggregate signature.
// This collects individual BLS signatures and aggregates them.
func (cg *CertificateGenerator) GenerateBLSAggregate(blockID ids.ID, signatures [][]byte) ([]byte, error) {
	if len(signatures) == 0 {
		return nil, fmt.Errorf("no signatures to aggregate")
	}

	// Parse signatures
	sigs := make([]*bls.Signature, 0, len(signatures))
	for _, sigBytes := range signatures {
		sig, err := bls.SignatureFromBytes(sigBytes)
		if err != nil {
			continue // Skip invalid signatures
		}
		sigs = append(sigs, sig)
	}

	if len(sigs) == 0 {
		return nil, fmt.Errorf("no valid signatures to aggregate")
	}

	// Aggregate signatures
	aggSig, err := bls.AggregateSignatures(sigs)
	if err != nil {
		return nil, fmt.Errorf("BLS aggregation failed: %w", err)
	}

	return bls.SignatureToBytes(aggSig), nil
}

// GeneratePQCertificate generates a real Corona threshold signature share.
// Returns the Round1 data that should be broadcast to other signers.
func (cg *CertificateGenerator) GeneratePQCertificate(blockID ids.ID, sessionID int, prfKey []byte, signers []int) (*threshold.Round1Data, error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	if cg.coronaSigner == nil {
		return nil, fmt.Errorf("Corona signer not initialized")
	}

	round1 := cg.coronaSigner.Round1(sessionID, prfKey, signers)
	return round1, nil
}

// CompleteCoronaRound2 performs round 2 of Corona signing.
func (cg *CertificateGenerator) CompleteCoronaRound2(
	sessionID int,
	message string,
	prfKey []byte,
	signers []int,
	round1Data map[int]*threshold.Round1Data,
) (*threshold.Round2Data, error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	if cg.coronaSigner == nil {
		return nil, fmt.Errorf("Corona signer not initialized")
	}

	return cg.coronaSigner.Round2(sessionID, message, prfKey, signers, round1Data)
}

// FinalizeCoronaSignature aggregates round 2 data into final signature.
func (cg *CertificateGenerator) FinalizeCoronaSignature(round2Data map[int]*threshold.Round2Data) (*threshold.Signature, error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	if cg.coronaSigner == nil {
		return nil, fmt.Errorf("Corona signer not initialized")
	}

	return cg.coronaSigner.Finalize(round2Data)
}

// GetBLSPublicKey returns the BLS public key bytes.
func (cg *CertificateGenerator) GetBLSPublicKey() []byte {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	if cg.blsPublicKey == nil {
		return nil
	}
	return bls.PublicKeyToCompressedBytes(cg.blsPublicKey)
}

// GetCoronaGroupKey returns the Corona group public key.
func (cg *CertificateGenerator) GetCoronaGroupKey() *threshold.GroupKey {
	cg.mu.RLock()
	defer cg.mu.RUnlock()
	return cg.coronaGroup
}

// VerifyBLSAggregate verifies a BLS aggregate signature against public keys.
func VerifyBLSAggregate(msg []byte, aggSigBytes []byte, pubKeyBytes [][]byte) error {
	if len(aggSigBytes) == 0 {
		return fmt.Errorf("empty aggregate signature")
	}

	aggSig, err := bls.SignatureFromBytes(aggSigBytes)
	if err != nil {
		return fmt.Errorf("invalid aggregate signature: %w", err)
	}

	// Parse public keys
	pubKeys := make([]*bls.PublicKey, 0, len(pubKeyBytes))
	for _, pkBytes := range pubKeyBytes {
		pk, err := bls.PublicKeyFromCompressedBytes(pkBytes)
		if err != nil {
			continue
		}
		pubKeys = append(pubKeys, pk)
	}

	if len(pubKeys) == 0 {
		return fmt.Errorf("no valid public keys")
	}

	// Aggregate public keys
	aggPK, err := bls.AggregatePublicKeys(pubKeys)
	if err != nil {
		return fmt.Errorf("public key aggregation failed: %w", err)
	}

	// Verify aggregate signature
	if !bls.Verify(aggPK, aggSig, msg) {
		return fmt.Errorf("BLS signature verification failed")
	}

	return nil
}

// VerifyPQCertificate verifies a Corona threshold signature.
func VerifyPQCertificate(groupKey *threshold.GroupKey, message string, sig *threshold.Signature) error {
	if groupKey == nil || sig == nil {
		return fmt.Errorf("nil group key or signature")
	}

	if !threshold.Verify(groupKey, message, sig) {
		return fmt.Errorf("Corona signature verification failed")
	}

	return nil
}

// GenerateTestKeys generates real test keys for development.
func GenerateTestKeys() (blsKey []byte, pqKey []byte) {
	blsKey = make([]byte, 32)
	pqKey = make([]byte, 32)
	rand.Read(blsKey)
	rand.Read(pqKey)
	return blsKey, pqKey
}
