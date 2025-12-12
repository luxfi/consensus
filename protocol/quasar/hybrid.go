// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Quasar Hybrid Consensus: BLS + Ringtail for Full Post-Quantum Security

package quasar

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sync"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
)

// Hybrid implements parallel BLS+Ringtail for PQ-safe consensus
type Hybrid struct {
	mu sync.RWMutex

	// BLS for classical threshold signatures
	blsThreshold int
	blsKeys      map[string]*bls.SecretKey
	blsPubKeys   map[string]*bls.PublicKey

	// Ringtail for post-quantum signatures
	ringtailEngine RingtailPQ
	ringtailKeys   map[string]*RingtailKeyPair

	// Hybrid signature aggregation
	pendingBLS      map[string][]*bls.Signature
	pendingRingtail map[string][]RingtailSignature

	// Consensus state
	validators map[string]*Validator
	threshold  int // Number of validators needed for consensus
}

// RingtailPQ provides real post-quantum signatures using ML-DSA
type RingtailPQ struct {
	mode mldsa.Mode
}

// RingtailKeyPair holds a post-quantum key pair
type RingtailKeyPair struct {
	PrivateKey *mldsa.PrivateKey
	PublicKey  *mldsa.PublicKey
}

// RingtailSignature represents a post-quantum signature
type RingtailSignature struct {
	Signature   []byte
	PublicKey   []byte
	ValidatorID string
}

// Validator represents a consensus validator
type Validator struct {
	ID          string
	BLSPubKey   *bls.PublicKey
	RingtailPub *mldsa.PublicKey
	Weight      uint64
	Active      bool
}

// NewHybrid creates a new hybrid consensus engine
func NewHybrid(threshold int) (*Hybrid, error) {
	if threshold < 1 {
		return nil, errors.New("threshold must be at least 1")
	}

	return &Hybrid{
		blsThreshold:    threshold,
		blsKeys:         make(map[string]*bls.SecretKey),
		blsPubKeys:      make(map[string]*bls.PublicKey),
		ringtailEngine:  RingtailPQ{mode: mldsa.MLDSA65}, // Level 3 security
		ringtailKeys:    make(map[string]*RingtailKeyPair),
		pendingBLS:      make(map[string][]*bls.Signature),
		pendingRingtail: make(map[string][]RingtailSignature),
		validators:      make(map[string]*Validator),
		threshold:       threshold,
	}, nil
}

// AddValidator adds a validator to the consensus
func (q *Hybrid) AddValidator(id string, weight uint64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Generate BLS key pair
	blsSK, err := bls.NewSecretKey()
	if err != nil {
		return fmt.Errorf("failed to generate BLS key: %w", err)
	}
	blsPK := blsSK.PublicKey()

	// Generate Ringtail (ML-DSA) key pair
	ringtailPriv, err := mldsa.GenerateKey(rand.Reader, q.ringtailEngine.mode)
	if err != nil {
		return fmt.Errorf("failed to generate Ringtail key: %w", err)
	}

	// Store keys
	q.blsKeys[id] = blsSK
	q.blsPubKeys[id] = blsPK
	q.ringtailKeys[id] = &RingtailKeyPair{
		PrivateKey: ringtailPriv,
		PublicKey:  ringtailPriv.PublicKey,
	}

	// Add validator
	q.validators[id] = &Validator{
		ID:          id,
		BLSPubKey:   blsPK,
		RingtailPub: ringtailPriv.PublicKey,
		Weight:      weight,
		Active:      true,
	}

	return nil
}

// SignMessage signs a message with both BLS and Ringtail
func (q *Hybrid) SignMessage(validatorID string, message []byte) (*HybridSignature, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Get keys
	blsSK, exists := q.blsKeys[validatorID]
	if !exists {
		return nil, errors.New("validator not found")
	}

	ringtailKP, exists := q.ringtailKeys[validatorID]
	if !exists {
		return nil, errors.New("ringtail keys not found")
	}

	// Create BLS signature
	blsSig, err := blsSK.Sign(message)
	if err != nil {
		return nil, fmt.Errorf("BLS sign failed: %w", err)
	}

	// Create Ringtail (ML-DSA) signature
	ringtailSig, err := ringtailKP.PrivateKey.Sign(rand.Reader, message, nil)
	if err != nil {
		return nil, fmt.Errorf("Ringtail sign failed: %w", err)
	}

	return &HybridSignature{
		BLS:         bls.SignatureToBytes(blsSig),
		Ringtail:    ringtailSig,
		ValidatorID: validatorID,
	}, nil
}

// VerifyHybridSignature verifies both BLS and Ringtail signatures
func (q *Hybrid) VerifyHybridSignature(message []byte, sig *HybridSignature) bool {
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

	// Verify Ringtail (ML-DSA) signature
	if !validator.RingtailPub.Verify(message, sig.Ringtail, nil) {
		return false
	}

	return true
}

// AggregateSignatures aggregates signatures for a message
func (q *Hybrid) AggregateSignatures(message []byte, signatures []*HybridSignature) (*AggregatedSignature, error) {
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

	// Collect Ringtail signatures (can't aggregate, but verify threshold)
	ringtailSigs := make([]RingtailSignature, 0, len(signatures))
	for _, sig := range signatures {
		ringtailSigs = append(ringtailSigs, RingtailSignature{
			Signature:   sig.Ringtail,
			ValidatorID: sig.ValidatorID,
		})
	}

	return &AggregatedSignature{
		BLSAggregated: bls.SignatureToBytes(aggregatedBLS),
		RingtailSigs:  ringtailSigs,
		SignerCount:   len(signatures),
	}, nil
}

// VerifyAggregatedSignature verifies an aggregated signature
func (q *Hybrid) VerifyAggregatedSignature(message []byte, aggSig *AggregatedSignature) bool {
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
	pubKeys := make([]*bls.PublicKey, 0, len(aggSig.RingtailSigs))
	for _, ringtailSig := range aggSig.RingtailSigs {
		validator, exists := q.validators[ringtailSig.ValidatorID]
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

	// Verify each Ringtail signature
	for _, ringtailSig := range aggSig.RingtailSigs {
		validator, exists := q.validators[ringtailSig.ValidatorID]
		if !exists {
			return false
		}

		if !validator.RingtailPub.Verify(message, ringtailSig.Signature, nil) {
			return false
		}
	}

	return true
}

// HybridSignature contains both BLS and Ringtail signatures
type HybridSignature struct {
	BLS         []byte
	Ringtail    []byte
	ValidatorID string
}

// AggregatedSignature contains aggregated BLS and individual Ringtail signatures
type AggregatedSignature struct {
	BLSAggregated []byte
	RingtailSigs  []RingtailSignature
	SignerCount   int
}

// GetActiveValidatorCount returns the number of active validators
func (q *Hybrid) GetActiveValidatorCount() int {
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
func (q *Hybrid) GetThreshold() int {
	return q.threshold
}
