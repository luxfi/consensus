// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Quasar Hybrid Consensus: BLS + Ringtail for Full Post-Quantum Security

package quasar

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
)

// Buffer pools for hot paths - reduces GC pressure during signing/verification
var (
	// hybridSigPool pools HybridSignature structs for SignMessage
	hybridSigPool = sync.Pool{
		New: func() any {
			return &HybridSignature{
				BLS:      make([]byte, 0, 96),  // BLS sig size
				Ringtail: make([]byte, 0, 3309), // ML-DSA-65 sig size
			}
		},
	}

	// blsSigSlicePool pools slices for BLS signature aggregation
	blsSigSlicePool = sync.Pool{
		New: func() any {
			s := make([]*bls.Signature, 0, 128)
			return &s
		},
	}

	// ringtailSigSlicePool pools slices for Ringtail signature collection
	ringtailSigSlicePool = sync.Pool{
		New: func() any {
			s := make([]RingtailSignature, 0, 128)
			return &s
		},
	}

	// pubKeySlicePool pools slices for public key aggregation
	pubKeySlicePool = sync.Pool{
		New: func() any {
			s := make([]*bls.PublicKey, 0, 128)
			return &s
		},
	}
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

// SignMessage signs a message with both BLS and Ringtail.
// For long-running operations, use SignMessageWithContext.
func (q *Hybrid) SignMessage(validatorID string, message []byte) (*HybridSignature, error) {
	return q.SignMessageWithContext(context.Background(), validatorID, message)
}

// SignMessageWithContext signs a message with both BLS and Ringtail, respecting context cancellation.
func (q *Hybrid) SignMessageWithContext(ctx context.Context, validatorID string, message []byte) (*HybridSignature, error) {
	// Check context before acquiring lock
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	// Check context after acquiring lock
	if err := ctx.Err(); err != nil {
		return nil, err
	}

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

	// Check context between crypto operations
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Create Ringtail (ML-DSA) signature
	ringtailSig, err := ringtailKP.PrivateKey.Sign(rand.Reader, message, nil)
	if err != nil {
		return nil, fmt.Errorf("Ringtail sign failed: %w", err)
	}

	// Get pooled signature struct to reduce allocations
	sig := hybridSigPool.Get().(*HybridSignature)
	blsBytes := bls.SignatureToBytes(blsSig)
	sig.BLS = append(sig.BLS[:0], blsBytes...)
	sig.Ringtail = append(sig.Ringtail[:0], ringtailSig...)
	sig.ValidatorID = validatorID

	return sig, nil
}

// ReleaseHybridSignature returns a HybridSignature to the pool.
// Call this when done with signatures from SignMessage/SignMessageWithContext.
func ReleaseHybridSignature(sig *HybridSignature) {
	if sig == nil {
		return
	}
	sig.ValidatorID = ""
	hybridSigPool.Put(sig)
}

// VerifyHybridSignature verifies both BLS and Ringtail signatures.
// For long-running operations, use VerifyHybridSignatureWithContext.
func (q *Hybrid) VerifyHybridSignature(message []byte, sig *HybridSignature) bool {
	return q.VerifyHybridSignatureWithContext(context.Background(), message, sig)
}

// VerifyHybridSignatureWithContext verifies both BLS and Ringtail signatures, respecting context cancellation.
func (q *Hybrid) VerifyHybridSignatureWithContext(ctx context.Context, message []byte, sig *HybridSignature) bool {
	// Check context before acquiring lock
	if ctx.Err() != nil {
		return false
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	// Check context after acquiring lock
	if ctx.Err() != nil {
		return false
	}

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

	// Check context between crypto operations
	if ctx.Err() != nil {
		return false
	}

	// Verify Ringtail (ML-DSA) signature
	if !validator.RingtailPub.Verify(message, sig.Ringtail, nil) {
		return false
	}

	return true
}

// AggregateSignatures aggregates signatures for a message.
// For long-running operations, use AggregateSignaturesWithContext.
func (q *Hybrid) AggregateSignatures(message []byte, signatures []*HybridSignature) (*AggregatedSignature, error) {
	return q.AggregateSignaturesWithContext(context.Background(), message, signatures)
}

// AggregateSignaturesWithContext aggregates signatures for a message, respecting context cancellation.
func (q *Hybrid) AggregateSignaturesWithContext(ctx context.Context, message []byte, signatures []*HybridSignature) (*AggregatedSignature, error) {
	// Check context before acquiring lock
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Check context after acquiring lock
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(signatures) < q.threshold {
		return nil, fmt.Errorf("insufficient signatures: %d < %d", len(signatures), q.threshold)
	}

	// Get pooled BLS signature slice
	blsSigsPtr := blsSigSlicePool.Get().(*[]*bls.Signature)
	blsSigs := (*blsSigsPtr)[:0]
	defer func() {
		*blsSigsPtr = blsSigs[:0]
		blsSigSlicePool.Put(blsSigsPtr)
	}()

	for _, sig := range signatures {
		// Check context periodically during loop
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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

	// Get pooled Ringtail signature slice
	ringtailSigsPtr := ringtailSigSlicePool.Get().(*[]RingtailSignature)
	ringtailSigs := (*ringtailSigsPtr)[:0]

	for _, sig := range signatures {
		ringtailSigs = append(ringtailSigs, RingtailSignature{
			Signature:   sig.Ringtail,
			ValidatorID: sig.ValidatorID,
		})
	}

	// Copy to result (caller owns the slice)
	resultSigs := make([]RingtailSignature, len(ringtailSigs))
	copy(resultSigs, ringtailSigs)

	// Return pooled slice
	*ringtailSigsPtr = ringtailSigs[:0]
	ringtailSigSlicePool.Put(ringtailSigsPtr)

	return &AggregatedSignature{
		BLSAggregated: bls.SignatureToBytes(aggregatedBLS),
		RingtailSigs:  resultSigs,
		SignerCount:   len(signatures),
	}, nil
}

// VerifyAggregatedSignature verifies an aggregated signature.
// For long-running operations, use VerifyAggregatedSignatureWithContext.
func (q *Hybrid) VerifyAggregatedSignature(message []byte, aggSig *AggregatedSignature) bool {
	return q.VerifyAggregatedSignatureWithContext(context.Background(), message, aggSig)
}

// VerifyAggregatedSignatureWithContext verifies an aggregated signature, respecting context cancellation.
func (q *Hybrid) VerifyAggregatedSignatureWithContext(ctx context.Context, message []byte, aggSig *AggregatedSignature) bool {
	// Check context before acquiring lock
	if ctx.Err() != nil {
		return false
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	// Check context after acquiring lock
	if ctx.Err() != nil {
		return false
	}

	// Check threshold
	if aggSig.SignerCount < q.threshold {
		return false
	}

	// Verify aggregated BLS signature
	blsSig, err := bls.SignatureFromBytes(aggSig.BLSAggregated)
	if err != nil {
		return false
	}

	// Get pooled public key slice
	pubKeysPtr := pubKeySlicePool.Get().(*[]*bls.PublicKey)
	pubKeys := (*pubKeysPtr)[:0]
	defer func() {
		*pubKeysPtr = pubKeys[:0]
		pubKeySlicePool.Put(pubKeysPtr)
	}()

	for _, ringtailSig := range aggSig.RingtailSigs {
		// Check context periodically during loop
		if ctx.Err() != nil {
			return false
		}
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

	// Check context before Ringtail verification
	if ctx.Err() != nil {
		return false
	}

	// Verify each Ringtail signature
	for _, ringtailSig := range aggSig.RingtailSigs {
		// Check context periodically during loop
		if ctx.Err() != nil {
			return false
		}
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
