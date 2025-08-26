// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// Quasar Hybrid Consensus: BLS + Corona for Full Post-Quantum Security

package quasar

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sync"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
)

// QuasarHybridConsensus implements parallel BLS+Corona for PQ-safe consensus
type QuasarHybridConsensus struct {
	mu sync.RWMutex

	// BLS for classical threshold signatures
	blsThreshold int
	blsKeys      map[string]*bls.SecretKey
	blsPubKeys   map[string]*bls.PublicKey

	// Corona for post-quantum signatures
	coronaEngine CoronaPQ
	coronaKeys   map[string]*CoronaKeyPair

	// Hybrid signature aggregation
	pendingBLS      map[string][]*bls.Signature
	pendingCorona map[string][]CoronaSignature

	// Consensus state
	validators map[string]*Validator
	threshold  int // Number of validators needed for consensus
}

// CoronaPQ provides real post-quantum signatures using ML-DSA
type CoronaPQ struct {
	mode mldsa.Mode
}

// CoronaKeyPair holds a post-quantum key pair
type CoronaKeyPair struct {
	PrivateKey *mldsa.PrivateKey
	PublicKey  *mldsa.PublicKey
}

// CoronaSignature represents a post-quantum signature
type CoronaSignature struct {
	Signature   []byte
	PublicKey   []byte
	ValidatorID string
}

// Validator represents a consensus validator
type Validator struct {
	ID          string
	BLSPubKey   *bls.PublicKey
	CoronaPub *mldsa.PublicKey
	Weight      uint64
	Active      bool
}

// NewQuasarHybridConsensus creates a new hybrid consensus engine
func NewQuasarHybridConsensus(threshold int) (*QuasarHybridConsensus, error) {
	if threshold < 1 {
		return nil, errors.New("threshold must be at least 1")
	}

	return &QuasarHybridConsensus{
		blsThreshold:    threshold,
		blsKeys:         make(map[string]*bls.SecretKey),
		blsPubKeys:      make(map[string]*bls.PublicKey),
		coronaEngine:  CoronaPQ{mode: mldsa.MLDSA65}, // Level 3 security
		coronaKeys:    make(map[string]*CoronaKeyPair),
		pendingBLS:      make(map[string][]*bls.Signature),
		pendingCorona: make(map[string][]CoronaSignature),
		validators:      make(map[string]*Validator),
		threshold:       threshold,
	}, nil
}

// AddValidator adds a validator to the consensus
func (q *QuasarHybridConsensus) AddValidator(id string, weight uint64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Generate BLS key pair
	blsSK, err := bls.NewSecretKey()
	if err != nil {
		return fmt.Errorf("failed to generate BLS key: %w", err)
	}
	blsPK := blsSK.PublicKey()

	// Generate Corona (ML-DSA) key pair
	coronaPriv, err := mldsa.GenerateKey(rand.Reader, q.coronaEngine.mode)
	if err != nil {
		return fmt.Errorf("failed to generate Corona key: %w", err)
	}

	// Store keys
	q.blsKeys[id] = blsSK
	q.blsPubKeys[id] = blsPK
	q.coronaKeys[id] = &CoronaKeyPair{
		PrivateKey: coronaPriv,
		PublicKey:  coronaPriv.PublicKey,
	}

	// Add validator
	q.validators[id] = &Validator{
		ID:          id,
		BLSPubKey:   blsPK,
		CoronaPub: coronaPriv.PublicKey,
		Weight:      weight,
		Active:      true,
	}

	return nil
}

// SignMessage signs a message with both BLS and Corona
func (q *QuasarHybridConsensus) SignMessage(validatorID string, message []byte) (*HybridSignature, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Get keys
	blsSK, exists := q.blsKeys[validatorID]
	if !exists {
		return nil, errors.New("validator not found")
	}

	coronaKP, exists := q.coronaKeys[validatorID]
	if !exists {
		return nil, errors.New("corona keys not found")
	}

	// Create BLS signature
	blsSig := blsSK.Sign(message)

	// Create Corona (ML-DSA) signature
	coronaSig, err := coronaKP.PrivateKey.Sign(rand.Reader, message, nil)
	if err != nil {
		return nil, fmt.Errorf("Corona sign failed: %w", err)
	}

	return &HybridSignature{
		BLS:         bls.SignatureToBytes(blsSig),
		Corona:    coronaSig,
		ValidatorID: validatorID,
	}, nil
}

// VerifyHybridSignature verifies both BLS and Corona signatures
func (q *QuasarHybridConsensus) VerifyHybridSignature(message []byte, sig *HybridSignature) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	validator, exists := q.validators[sig.ValidatorID]
	if !exists {
		return false
	}

	// Verify BLS signature
	blsSig, err := bls.SignatureFromBytes(sig.BLS)
	if err != nil {
		return false
	}

	// BLS verification using Verify method
	if !bls.Verify(validator.BLSPubKey, blsSig, message) {
		return false
	}

	// Verify Corona (ML-DSA) signature
	if !validator.CoronaPub.Verify(message, sig.Corona, nil) {
		return false
	}

	return true
}

// AggregateSignatures aggregates signatures for a message
func (q *QuasarHybridConsensus) AggregateSignatures(message []byte, signatures []*HybridSignature) (*AggregatedSignature, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(signatures) < q.threshold {
		return nil, fmt.Errorf("insufficient signatures: %d < %d", len(signatures), q.threshold)
	}

	// Aggregate BLS signatures
	blsSigs := make([]*bls.Signature, 0, len(signatures))
	for _, sig := range signatures {
		blsSig, err := bls.SignatureFromBytes(sig.BLS)
		if err != nil {
			return nil, fmt.Errorf("invalid BLS signature: %w", err)
		}
		blsSigs = append(blsSigs, blsSig)
	}

	aggregatedBLS, err := bls.AggregateSignatures(blsSigs)
	if err != nil {
		return nil, fmt.Errorf("BLS aggregation failed: %w", err)
	}

	// Collect Corona signatures (can't aggregate, but verify threshold)
	coronaSigs := make([]CoronaSignature, 0, len(signatures))
	for _, sig := range signatures {
		coronaSigs = append(coronaSigs, CoronaSignature{
			Signature:   sig.Corona,
			ValidatorID: sig.ValidatorID,
		})
	}

	return &AggregatedSignature{
		BLSAggregated: bls.SignatureToBytes(aggregatedBLS),
		CoronaSigs:  coronaSigs,
		SignerCount:   len(signatures),
	}, nil
}

// VerifyAggregatedSignature verifies an aggregated signature
func (q *QuasarHybridConsensus) VerifyAggregatedSignature(message []byte, aggSig *AggregatedSignature) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Check threshold
	if aggSig.SignerCount < q.threshold {
		return false
	}

	// Verify aggregated BLS signature
	blsSig, err := bls.SignatureFromBytes(aggSig.BLSAggregated)
	if err != nil {
		return false
	}

	// Collect public keys for BLS verification
	pubKeys := make([]*bls.PublicKey, 0, len(aggSig.CoronaSigs))
	for _, coronaSig := range aggSig.CoronaSigs {
		validator, exists := q.validators[coronaSig.ValidatorID]
		if !exists || !validator.Active {
			return false
		}
		pubKeys = append(pubKeys, validator.BLSPubKey)
	}

	aggPubKey, err := bls.AggregatePublicKeys(pubKeys)
	if err != nil {
		return false
	}

	if !bls.Verify(aggPubKey, blsSig, message) {
		return false
	}

	// Verify each Corona signature
	for _, coronaSig := range aggSig.CoronaSigs {
		validator, exists := q.validators[coronaSig.ValidatorID]
		if !exists {
			return false
		}

		if !validator.CoronaPub.Verify(message, coronaSig.Signature, nil) {
			return false
		}
	}

	return true
}

// HybridSignature contains both BLS and Corona signatures
type HybridSignature struct {
	BLS         []byte
	Corona    []byte
	ValidatorID string
}

// AggregatedSignature contains aggregated BLS and individual Corona signatures
type AggregatedSignature struct {
	BLSAggregated []byte
	CoronaSigs  []CoronaSignature
	SignerCount   int
}

// GetActiveValidatorCount returns the number of active validators
func (q *QuasarHybridConsensus) GetActiveValidatorCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	count := 0
	for _, v := range q.validators {
		if v.Active {
			count++
		}
	}
	return count
}

// GetThreshold returns the consensus threshold
func (q *QuasarHybridConsensus) GetThreshold() int {
	return q.threshold
}
