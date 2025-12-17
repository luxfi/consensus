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
	"github.com/luxfi/crypto/threshold"
	_ "github.com/luxfi/crypto/threshold/bls" // Register BLS threshold scheme
)

// Buffer pools for hot paths - reduces GC pressure during signing/verification
var (
	// hybridSigPool pools HybridSignature structs for SignMessage
	hybridSigPool = sync.Pool{
		New: func() any {
			return &HybridSignature{
				BLS:      make([]byte, 0, 96),   // BLS sig size
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

	// Threshold scheme support (optional, for new consensus modes)
	thresholdScheme    threshold.Scheme
	thresholdGroupKey  threshold.PublicKey
	thresholdSigners   map[string]threshold.Signer
	thresholdAggregator threshold.Aggregator
	thresholdVerifier  threshold.Verifier
	useThreshold       bool
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
		return nil, fmt.Errorf("ringtail sign failed: %w", err)
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

// ThresholdConfig contains configuration for threshold signing mode.
type ThresholdConfig struct {
	// Scheme is the threshold scheme to use (SchemeBLS, SchemeRingtail, etc.)
	SchemeID threshold.SchemeID

	// Threshold is the minimum signers required (t in t+1-of-n)
	Threshold int

	// TotalParties is the total number of validators
	TotalParties int

	// KeyShares are the pre-generated key shares for each validator.
	// Map keys are validator IDs.
	KeyShares map[string]threshold.KeyShare

	// GroupKey is the group public key.
	GroupKey threshold.PublicKey
}

// NewHybridWithThreshold creates a hybrid consensus engine with threshold signing support.
func NewHybridWithThreshold(config ThresholdConfig) (*Hybrid, error) {
	if config.Threshold < 1 {
		return nil, errors.New("threshold must be at least 1")
	}

	// Get the threshold scheme
	scheme, err := threshold.GetScheme(config.SchemeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get threshold scheme: %w", err)
	}

	// Create aggregator and verifier for the group key
	aggregator, err := scheme.NewAggregator(config.GroupKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create aggregator: %w", err)
	}

	verifier, err := scheme.NewVerifier(config.GroupKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create verifier: %w", err)
	}

	// Create signers for each key share
	signers := make(map[string]threshold.Signer)
	for id, share := range config.KeyShares {
		signer, err := scheme.NewSigner(share)
		if err != nil {
			return nil, fmt.Errorf("failed to create signer for %s: %w", id, err)
		}
		signers[id] = signer
	}

	h := &Hybrid{
		blsThreshold:    config.Threshold,
		blsKeys:         make(map[string]*bls.SecretKey),
		blsPubKeys:      make(map[string]*bls.PublicKey),
		ringtailEngine:  RingtailPQ{mode: mldsa.MLDSA65},
		ringtailKeys:    make(map[string]*RingtailKeyPair),
		pendingBLS:      make(map[string][]*bls.Signature),
		pendingRingtail: make(map[string][]RingtailSignature),
		validators:      make(map[string]*Validator),
		threshold:       config.Threshold,

		// Threshold mode
		thresholdScheme:    scheme,
		thresholdGroupKey:  config.GroupKey,
		thresholdSigners:   signers,
		thresholdAggregator: aggregator,
		thresholdVerifier:  verifier,
		useThreshold:       true,
	}

	return h, nil
}

// IsThresholdMode returns true if threshold signing is enabled.
func (q *Hybrid) IsThresholdMode() bool {
	return q.useThreshold
}

// ThresholdScheme returns the threshold scheme if threshold mode is enabled.
func (q *Hybrid) ThresholdScheme() threshold.Scheme {
	return q.thresholdScheme
}

// ThresholdGroupKey returns the group public key if threshold mode is enabled.
func (q *Hybrid) ThresholdGroupKey() threshold.PublicKey {
	return q.thresholdGroupKey
}

// SignMessageThreshold signs a message using threshold signing.
// Returns a signature share that must be combined with other shares.
func (q *Hybrid) SignMessageThreshold(ctx context.Context, validatorID string, message []byte) (threshold.SignatureShare, error) {
	if !q.useThreshold {
		return nil, errors.New("threshold mode not enabled")
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	signer, exists := q.thresholdSigners[validatorID]
	if !exists {
		return nil, fmt.Errorf("validator %s not found in threshold signers", validatorID)
	}

	// Get list of all signer indices
	signerIndices := make([]int, 0, len(q.thresholdSigners))
	for _, s := range q.thresholdSigners {
		signerIndices = append(signerIndices, s.Index())
	}

	return signer.SignShare(ctx, message, signerIndices, nil)
}

// AggregateThresholdSignatures aggregates threshold signature shares.
func (q *Hybrid) AggregateThresholdSignatures(ctx context.Context, message []byte, shares []threshold.SignatureShare) (threshold.Signature, error) {
	if !q.useThreshold {
		return nil, errors.New("threshold mode not enabled")
	}

	if len(shares) <= q.threshold {
		return nil, fmt.Errorf("insufficient shares: need at least %d, got %d", q.threshold+1, len(shares))
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	return q.thresholdAggregator.Aggregate(ctx, message, shares, nil)
}

// VerifyThresholdSignature verifies a threshold signature.
func (q *Hybrid) VerifyThresholdSignature(message []byte, sig threshold.Signature) bool {
	if !q.useThreshold {
		return false
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.thresholdVerifier.Verify(message, sig)
}

// VerifyThresholdSignatureBytes verifies a serialized threshold signature.
func (q *Hybrid) VerifyThresholdSignatureBytes(message, sig []byte) bool {
	if !q.useThreshold {
		return false
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.thresholdVerifier.VerifyBytes(message, sig)
}

// AddValidatorThreshold adds a validator with a threshold key share.
func (q *Hybrid) AddValidatorThreshold(id string, keyShare threshold.KeyShare, weight uint64) error {
	if !q.useThreshold {
		return errors.New("threshold mode not enabled")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Create signer from key share
	signer, err := q.thresholdScheme.NewSigner(keyShare)
	if err != nil {
		return fmt.Errorf("failed to create signer: %w", err)
	}

	q.thresholdSigners[id] = signer

	// Also add to validators map for tracking
	q.validators[id] = &Validator{
		ID:     id,
		Weight: weight,
		Active: true,
	}

	return nil
}

// GenerateThresholdKeys generates threshold key shares using a trusted dealer.
// This is a convenience method for testing and development.
// In production, use distributed key generation (DKG) instead.
func GenerateThresholdKeys(schemeID threshold.SchemeID, t, n int) ([]threshold.KeyShare, threshold.PublicKey, error) {
	scheme, err := threshold.GetScheme(schemeID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get scheme: %w", err)
	}

	dealer, err := scheme.NewTrustedDealer(threshold.DealerConfig{
		Threshold:    t,
		TotalParties: n,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create dealer: %w", err)
	}

	return dealer.GenerateShares(context.Background())
}
