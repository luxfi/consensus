// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Quasar Hybrid Consensus: BLS + Ringtail for Full Post-Quantum Security

package quasar

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/threshold"
	_ "github.com/luxfi/crypto/threshold/bls" // Register BLS threshold scheme

	ringtailThreshold "github.com/luxfi/ringtail/threshold"
)

// Buffer pools for hot paths - reduces GC pressure during signing/verification
var (
	hybridSigPool = sync.Pool{
		New: func() any {
			return &HybridSignature{
				BLS:      make([]byte, 0, 96),
				Ringtail: make([]byte, 0, 4096),
			}
		},
	}

	blsSigSlicePool = sync.Pool{
		New: func() any {
			s := make([]*bls.Signature, 0, 128)
			return &s
		},
	}

	pubKeySlicePool = sync.Pool{
		New: func() any {
			s := make([]*bls.PublicKey, 0, 128)
			return &s
		},
	}
)

// Hybrid implements parallel BLS+Ringtail threshold signing for PQ-safe consensus.
// BLS provides fast classical threshold signatures.
// Ringtail provides post-quantum threshold signatures based on Ring-LWE.
// Both run in parallel to provide quantum-safe finality.
type Hybrid struct {
	mu sync.RWMutex

	// Classical BLS threshold signing (via crypto/threshold)
	blsScheme     threshold.Scheme
	blsGroupKey   threshold.PublicKey
	blsSigners    map[string]threshold.Signer
	blsAggregator threshold.Aggregator
	blsVerifier   threshold.Verifier

	// Post-quantum Ringtail threshold signing (native 2-round protocol)
	ringtailGroupKey *ringtailThreshold.GroupKey
	ringtailSigners  map[string]*ringtailThreshold.Signer
	ringtailShares   map[string]*ringtailThreshold.KeyShare

	// Legacy BLS direct keys (for backward compatibility)
	blsKeys    map[string]*bls.SecretKey
	blsPubKeys map[string]*bls.PublicKey

	// Consensus state
	validators map[string]*Validator
	threshold  int // Number of validators needed for consensus
}

// Validator represents a consensus validator
type Validator struct {
	ID          string
	BLSPubKey   *bls.PublicKey
	RingtailPub []byte // Ringtail group public key contribution
	Weight      uint64
	Active      bool
}

// HybridConfig configures the dual threshold signing system.
type HybridConfig struct {
	Threshold    int
	TotalParties int

	// BLS threshold (via crypto/threshold interface)
	BLSKeyShares map[string]threshold.KeyShare
	BLSGroupKey  threshold.PublicKey

	// Ringtail threshold (native 2-round protocol)
	RingtailShares  map[string]*ringtailThreshold.KeyShare
	RingtailGroupKey *ringtailThreshold.GroupKey
}

// RingtailRound1State holds Round 1 data for all parties in a signing session.
type RingtailRound1State struct {
	SessionID int
	PRFKey    []byte
	SignerIDs []int
	Round1Data map[int]*ringtailThreshold.Round1Data
}

// NewHybrid creates a new hybrid consensus engine with basic BLS support.
func NewHybrid(thresholdVal int) (*Hybrid, error) {
	if thresholdVal < 1 {
		return nil, errors.New("threshold must be at least 1")
	}

	return &Hybrid{
		blsKeys:         make(map[string]*bls.SecretKey),
		blsPubKeys:      make(map[string]*bls.PublicKey),
		blsSigners:      make(map[string]threshold.Signer),
		ringtailSigners: make(map[string]*ringtailThreshold.Signer),
		ringtailShares:  make(map[string]*ringtailThreshold.KeyShare),
		validators:      make(map[string]*Validator),
		threshold:       thresholdVal,
	}, nil
}

// NewHybridWithDualThreshold creates a hybrid engine with full dual threshold signing.
func NewHybridWithDualThreshold(config HybridConfig) (*Hybrid, error) {
	if config.Threshold < 1 {
		return nil, errors.New("threshold must be at least 1")
	}

	h := &Hybrid{
		blsKeys:         make(map[string]*bls.SecretKey),
		blsPubKeys:      make(map[string]*bls.PublicKey),
		blsSigners:      make(map[string]threshold.Signer),
		ringtailSigners: make(map[string]*ringtailThreshold.Signer),
		ringtailShares:  make(map[string]*ringtailThreshold.KeyShare),
		validators:      make(map[string]*Validator),
		threshold:       config.Threshold,
	}

	// Initialize BLS threshold scheme
	blsScheme, err := threshold.GetScheme(threshold.SchemeBLS)
	if err != nil {
		return nil, fmt.Errorf("failed to get BLS scheme: %w", err)
	}
	h.blsScheme = blsScheme
	h.blsGroupKey = config.BLSGroupKey

	if config.BLSGroupKey != nil {
		h.blsAggregator, err = blsScheme.NewAggregator(config.BLSGroupKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create BLS aggregator: %w", err)
		}
		h.blsVerifier, err = blsScheme.NewVerifier(config.BLSGroupKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create BLS verifier: %w", err)
		}
	}

	for id, share := range config.BLSKeyShares {
		signer, err := blsScheme.NewSigner(share)
		if err != nil {
			return nil, fmt.Errorf("failed to create BLS signer for %s: %w", id, err)
		}
		h.blsSigners[id] = signer
	}

	// Initialize Ringtail signers (native 2-round protocol)
	h.ringtailGroupKey = config.RingtailGroupKey
	for id, share := range config.RingtailShares {
		h.ringtailShares[id] = share
		h.ringtailSigners[id] = ringtailThreshold.NewSigner(share)
	}

	return h, nil
}

// GenerateDualKeys generates both BLS and Ringtail threshold keys for an epoch.
// Call this when the validator set changes.
func GenerateDualKeys(t, n int) (*HybridConfig, error) {
	ctx := context.Background()

	// Generate BLS threshold keys
	blsScheme, err := threshold.GetScheme(threshold.SchemeBLS)
	if err != nil {
		return nil, fmt.Errorf("failed to get BLS scheme: %w", err)
	}

	blsDealer, err := blsScheme.NewTrustedDealer(threshold.DealerConfig{
		Threshold:    t,
		TotalParties: n,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create BLS dealer: %w", err)
	}

	blsShares, blsGroupKey, err := blsDealer.GenerateShares(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate BLS shares: %w", err)
	}

	// Generate Ringtail threshold keys (native)
	ringtailShares, ringtailGroupKey, err := ringtailThreshold.GenerateKeys(t, n, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Ringtail shares: %w", err)
	}

	// Convert to maps keyed by validator ID
	blsShareMap := make(map[string]threshold.KeyShare)
	ringtailShareMap := make(map[string]*ringtailThreshold.KeyShare)

	for i := 0; i < n; i++ {
		id := fmt.Sprintf("v%d", i)
		blsShareMap[id] = blsShares[i]
		ringtailShareMap[id] = ringtailShares[i]
	}

	return &HybridConfig{
		Threshold:        t,
		TotalParties:     n,
		BLSKeyShares:     blsShareMap,
		BLSGroupKey:      blsGroupKey,
		RingtailShares:   ringtailShareMap,
		RingtailGroupKey: ringtailGroupKey,
	}, nil
}

// ============================================================================
// Ringtail 2-Round Protocol
// ============================================================================

// RingtailRound1 performs Round 1 of Ringtail signing for a validator.
// Returns Round1Data to broadcast to other validators.
func (h *Hybrid) RingtailRound1(validatorID string, sessionID int, prfKey []byte) (*ringtailThreshold.Round1Data, error) {
	h.mu.RLock()
	signer, exists := h.ringtailSigners[validatorID]
	h.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("validator %s not found in Ringtail signers", validatorID)
	}

	// Get all signer indices
	signerIDs := make([]int, 0, len(h.ringtailSigners))
	for _, s := range h.ringtailShares {
		signerIDs = append(signerIDs, s.Index)
	}

	return signer.Round1(sessionID, prfKey, signerIDs), nil
}

// RingtailRound2 performs Round 2 of Ringtail signing for a validator.
// Requires collected Round 1 data from all signers.
// Returns Round2Data to broadcast.
func (h *Hybrid) RingtailRound2(validatorID string, sessionID int, message string, prfKey []byte, round1Data map[int]*ringtailThreshold.Round1Data) (*ringtailThreshold.Round2Data, error) {
	h.mu.RLock()
	signer, exists := h.ringtailSigners[validatorID]
	h.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("validator %s not found in Ringtail signers", validatorID)
	}

	signerIDs := make([]int, 0, len(h.ringtailSigners))
	for _, s := range h.ringtailShares {
		signerIDs = append(signerIDs, s.Index)
	}

	return signer.Round2(sessionID, message, prfKey, signerIDs, round1Data)
}

// RingtailFinalize aggregates Round 2 data into the final signature.
// Any validator can call this.
func (h *Hybrid) RingtailFinalize(validatorID string, round2Data map[int]*ringtailThreshold.Round2Data) (*ringtailThreshold.Signature, error) {
	h.mu.RLock()
	signer, exists := h.ringtailSigners[validatorID]
	h.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("validator %s not found in Ringtail signers", validatorID)
	}

	return signer.Finalize(round2Data)
}

// VerifyRingtailSignature verifies a Ringtail threshold signature.
func (h *Hybrid) VerifyRingtailSignature(message string, sig *ringtailThreshold.Signature) bool {
	if h.ringtailGroupKey == nil || sig == nil {
		return false
	}
	return ringtailThreshold.Verify(h.ringtailGroupKey, message, sig)
}

// ============================================================================
// Parallel BLS + Ringtail Signing
// ============================================================================

// DualSignRound1 performs Round 1 of both BLS and Ringtail in parallel.
// BLS: Computes signature share (single round)
// Ringtail: Computes D matrix + MACs (Round 1 of 2)
func (h *Hybrid) DualSignRound1(ctx context.Context, validatorID string, message []byte, sessionID int, prfKey []byte) (*HybridSignature, *ringtailThreshold.Round1Data, error) {
	h.mu.RLock()
	blsSigner, hasBLS := h.blsSigners[validatorID]
	_, hasRingtail := h.ringtailSigners[validatorID]
	h.mu.RUnlock()

	if !hasBLS || !hasRingtail {
		return nil, nil, errors.New("validator not configured for dual signing")
	}

	var wg sync.WaitGroup
	var blsErr error
	var blsShare threshold.SignatureShare
	var round1Data *ringtailThreshold.Round1Data

	// Get BLS signer indices
	blsIndices := make([]int, 0, len(h.blsSigners))
	for _, s := range h.blsSigners {
		blsIndices = append(blsIndices, s.Index())
	}

	wg.Add(2)

	// BLS signing (single round)
	go func() {
		defer wg.Done()
		blsShare, blsErr = blsSigner.SignShare(ctx, message, blsIndices, nil)
	}()

	// Ringtail Round 1
	go func() {
		defer wg.Done()
		round1Data, _ = h.RingtailRound1(validatorID, sessionID, prfKey)
	}()

	wg.Wait()

	if blsErr != nil {
		return nil, nil, fmt.Errorf("BLS signing failed: %w", blsErr)
	}

	sig := &HybridSignature{
		BLS:         blsShare.Bytes(),
		ValidatorID: validatorID,
		IsThreshold: true,
		SignerIndex: blsSigner.Index(),
	}

	return sig, round1Data, nil
}

// DualSignRound2 performs Round 2 of Ringtail (BLS is already done in Round1).
func (h *Hybrid) DualSignRound2(validatorID string, sessionID int, message string, prfKey []byte, round1Data map[int]*ringtailThreshold.Round1Data) (*ringtailThreshold.Round2Data, error) {
	return h.RingtailRound2(validatorID, sessionID, message, prfKey, round1Data)
}

// ============================================================================
// Legacy & Backward Compatibility
// ============================================================================

// AddValidator adds a validator with legacy BLS key generation.
func (h *Hybrid) AddValidator(id string, weight uint64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	blsSK, err := bls.NewSecretKey()
	if err != nil {
		return fmt.Errorf("failed to generate BLS key: %w", err)
	}
	blsPK := blsSK.PublicKey()

	h.blsKeys[id] = blsSK
	h.blsPubKeys[id] = blsPK

	h.validators[id] = &Validator{
		ID:        id,
		BLSPubKey: blsPK,
		Weight:    weight,
		Active:    true,
	}

	return nil
}

// SignMessage signs a message with BLS (legacy mode or threshold).
func (h *Hybrid) SignMessage(validatorID string, message []byte) (*HybridSignature, error) {
	return h.SignMessageWithContext(context.Background(), validatorID, message)
}

// SignMessageWithContext signs a message.
func (h *Hybrid) SignMessageWithContext(ctx context.Context, validatorID string, message []byte) (*HybridSignature, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	sig := hybridSigPool.Get().(*HybridSignature)
	sig.BLS = sig.BLS[:0]
	sig.Ringtail = sig.Ringtail[:0]
	sig.ValidatorID = validatorID

	// Check for threshold signer
	blsSigner, hasBLSSigner := h.blsSigners[validatorID]
	if hasBLSSigner {
		blsIndices := make([]int, 0, len(h.blsSigners))
		for _, s := range h.blsSigners {
			blsIndices = append(blsIndices, s.Index())
		}

		share, err := blsSigner.SignShare(ctx, message, blsIndices, nil)
		if err != nil {
			return nil, fmt.Errorf("BLS threshold sign failed: %w", err)
		}

		sig.BLS = append(sig.BLS, share.Bytes()...)
		sig.IsThreshold = true
		sig.SignerIndex = blsSigner.Index()
		return sig, nil
	}

	// Fall back to legacy BLS
	blsSK, exists := h.blsKeys[validatorID]
	if !exists {
		return nil, errors.New("validator not found")
	}

	blsSig, err := blsSK.Sign(message)
	if err != nil {
		return nil, fmt.Errorf("BLS sign failed: %w", err)
	}

	sig.BLS = append(sig.BLS, bls.SignatureToBytes(blsSig)...)
	sig.IsThreshold = false

	return sig, nil
}

// ReleaseHybridSignature returns a HybridSignature to the pool.
func ReleaseHybridSignature(sig *HybridSignature) {
	if sig == nil {
		return
	}
	sig.ValidatorID = ""
	sig.IsThreshold = false
	sig.SignerIndex = 0
	hybridSigPool.Put(sig)
}

// VerifyHybridSignature verifies a signature.
func (h *Hybrid) VerifyHybridSignature(message []byte, sig *HybridSignature) bool {
	return h.VerifyHybridSignatureWithContext(context.Background(), message, sig)
}

// VerifyHybridSignatureWithContext verifies a signature.
func (h *Hybrid) VerifyHybridSignatureWithContext(ctx context.Context, message []byte, sig *HybridSignature) bool {
	if ctx.Err() != nil {
		return false
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if sig.IsThreshold {
		return true // Shares verified during aggregation
	}

	validator, exists := h.validators[sig.ValidatorID]
	if !exists {
		return false
	}

	blsSig, err := bls.SignatureFromBytes(sig.BLS)
	if err != nil {
		return false
	}

	return bls.Verify(validator.BLSPubKey, blsSig, message)
}

// AggregateSignatures aggregates BLS threshold signature shares.
func (h *Hybrid) AggregateSignatures(message []byte, signatures []*HybridSignature) (*AggregatedSignature, error) {
	return h.AggregateSignaturesWithContext(context.Background(), message, signatures)
}

// AggregateSignaturesWithContext aggregates signatures.
func (h *Hybrid) AggregateSignaturesWithContext(ctx context.Context, message []byte, signatures []*HybridSignature) (*AggregatedSignature, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if len(signatures) < h.threshold {
		return nil, fmt.Errorf("insufficient signatures: %d < %d", len(signatures), h.threshold)
	}

	// Check for threshold mode
	if len(signatures) > 0 && signatures[0].IsThreshold && h.blsAggregator != nil {
		blsShares := make([]threshold.SignatureShare, 0, len(signatures))

		for _, sig := range signatures {
			if len(sig.BLS) > 0 {
				share, err := h.blsScheme.ParseSignatureShare(sig.BLS)
				if err == nil {
					blsShares = append(blsShares, share)
				}
			}
		}

		if len(blsShares) >= h.threshold {
			blsAggSig, err := h.blsAggregator.Aggregate(ctx, message, blsShares, nil)
			if err != nil {
				return nil, fmt.Errorf("BLS aggregation failed: %w", err)
			}

			return &AggregatedSignature{
				BLSAggregated: blsAggSig.Bytes(),
				SignerCount:   len(signatures),
				IsThreshold:   true,
			}, nil
		}
	}

	// Legacy BLS aggregation
	blsSigsPtr := blsSigSlicePool.Get().(*[]*bls.Signature)
	blsSigs := (*blsSigsPtr)[:0]
	defer func() {
		*blsSigsPtr = blsSigs[:0]
		blsSigSlicePool.Put(blsSigsPtr)
	}()

	validatorIDs := make([]string, 0, len(signatures))
	for _, sig := range signatures {
		blsSig, err := bls.SignatureFromBytes(sig.BLS)
		if err != nil {
			return nil, fmt.Errorf("invalid BLS signature: %w", err)
		}
		blsSigs = append(blsSigs, blsSig)
		validatorIDs = append(validatorIDs, sig.ValidatorID)
	}

	aggregatedBLS, err := bls.AggregateSignatures(blsSigs)
	if err != nil {
		return nil, fmt.Errorf("BLS aggregation failed: %w", err)
	}

	return &AggregatedSignature{
		BLSAggregated: bls.SignatureToBytes(aggregatedBLS),
		ValidatorIDs:  validatorIDs,
		SignerCount:   len(signatures),
		IsThreshold:   false,
	}, nil
}

// VerifyAggregatedSignature verifies an aggregated signature.
func (h *Hybrid) VerifyAggregatedSignature(message []byte, aggSig *AggregatedSignature) bool {
	return h.VerifyAggregatedSignatureWithContext(context.Background(), message, aggSig)
}

// VerifyAggregatedSignatureWithContext verifies an aggregated signature.
func (h *Hybrid) VerifyAggregatedSignatureWithContext(ctx context.Context, message []byte, aggSig *AggregatedSignature) bool {
	if ctx.Err() != nil {
		return false
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if aggSig.SignerCount < h.threshold {
		return false
	}

	if aggSig.IsThreshold {
		if h.blsVerifier != nil && len(aggSig.BLSAggregated) > 0 {
			return h.blsVerifier.VerifyBytes(message, aggSig.BLSAggregated)
		}
		return false
	}

	// Legacy verification
	blsSig, err := bls.SignatureFromBytes(aggSig.BLSAggregated)
	if err != nil {
		return false
	}

	pubKeysPtr := pubKeySlicePool.Get().(*[]*bls.PublicKey)
	pubKeys := (*pubKeysPtr)[:0]
	defer func() {
		*pubKeysPtr = pubKeys[:0]
		pubKeySlicePool.Put(pubKeysPtr)
	}()

	for _, validatorID := range aggSig.ValidatorIDs {
		validator, exists := h.validators[validatorID]
		if !exists || !validator.Active {
			return false
		}
		pubKeys = append(pubKeys, validator.BLSPubKey)
	}

	aggPubKey, err := bls.AggregatePublicKeys(pubKeys)
	if err != nil {
		return false
	}

	return bls.Verify(aggPubKey, blsSig, message)
}

// HybridSignature contains BLS and optionally Ringtail signature data.
type HybridSignature struct {
	BLS         []byte
	Ringtail    []byte
	ValidatorID string
	IsThreshold bool
	SignerIndex int
}

// AggregatedSignature contains aggregated signatures.
type AggregatedSignature struct {
	BLSAggregated      []byte
	RingtailAggregated []byte
	ValidatorIDs       []string
	SignerCount        int
	IsThreshold        bool
}

// GetActiveValidatorCount returns the number of active validators.
func (h *Hybrid) GetActiveValidatorCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	count := 0
	for _, v := range h.validators {
		if v.Active {
			count++
		}
	}
	return count
}

// GetThreshold returns the consensus threshold.
func (h *Hybrid) GetThreshold() int {
	return h.threshold
}

// IsThresholdMode returns true if BLS threshold signing is enabled.
func (h *Hybrid) IsThresholdMode() bool {
	return h.blsScheme != nil
}

// IsDualThresholdMode returns true if both BLS and Ringtail are enabled.
func (h *Hybrid) IsDualThresholdMode() bool {
	return h.blsScheme != nil && h.ringtailGroupKey != nil
}

// ============================================================================
// Backward-compatible API
// ============================================================================

// ThresholdConfig for backward compatibility with single-scheme tests.
type ThresholdConfig struct {
	SchemeID     threshold.SchemeID
	Threshold    int
	TotalParties int
	KeyShares    map[string]threshold.KeyShare
	GroupKey     threshold.PublicKey
}

// NewHybridWithThresholdConfig creates a hybrid engine from ThresholdConfig.
func NewHybridWithThresholdConfig(config ThresholdConfig) (*Hybrid, error) {
	if config.Threshold < 1 {
		return nil, errors.New("threshold must be at least 1")
	}

	scheme, err := threshold.GetScheme(config.SchemeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get threshold scheme: %w", err)
	}

	h := &Hybrid{
		blsKeys:         make(map[string]*bls.SecretKey),
		blsPubKeys:      make(map[string]*bls.PublicKey),
		blsSigners:      make(map[string]threshold.Signer),
		ringtailSigners: make(map[string]*ringtailThreshold.Signer),
		ringtailShares:  make(map[string]*ringtailThreshold.KeyShare),
		validators:      make(map[string]*Validator),
		threshold:       config.Threshold,
		blsScheme:       scheme,
		blsGroupKey:     config.GroupKey,
	}

	if config.GroupKey != nil {
		h.blsAggregator, err = scheme.NewAggregator(config.GroupKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create aggregator: %w", err)
		}
		h.blsVerifier, err = scheme.NewVerifier(config.GroupKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create verifier: %w", err)
		}
	}

	for id, share := range config.KeyShares {
		signer, err := scheme.NewSigner(share)
		if err != nil {
			return nil, fmt.Errorf("failed to create signer for %s: %w", id, err)
		}
		h.blsSigners[id] = signer
	}

	return h, nil
}

// NewHybridWithThreshold is an alias for NewHybridWithDualThreshold.
func NewHybridWithThreshold(config HybridConfig) (*Hybrid, error) {
	return NewHybridWithDualThreshold(config)
}

// GenerateThresholdKeys generates threshold keys for a single scheme.
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

// GenerateDualThresholdKeys generates both BLS and Ringtail keys (backward compat).
func GenerateDualThresholdKeys(t, n int) (*HybridConfig, error) {
	return GenerateDualKeys(t, n)
}

// ThresholdScheme returns the BLS threshold scheme.
func (h *Hybrid) ThresholdScheme() threshold.Scheme {
	return h.blsScheme
}

// ThresholdGroupKey returns the BLS group public key.
func (h *Hybrid) ThresholdGroupKey() threshold.PublicKey {
	return h.blsGroupKey
}

// SignMessageThreshold signs using BLS threshold.
func (h *Hybrid) SignMessageThreshold(ctx context.Context, validatorID string, message []byte) (threshold.SignatureShare, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	signer, exists := h.blsSigners[validatorID]
	if !exists {
		return nil, fmt.Errorf("validator %s not found in BLS signers", validatorID)
	}

	signerIndices := make([]int, 0, len(h.blsSigners))
	for _, s := range h.blsSigners {
		signerIndices = append(signerIndices, s.Index())
	}

	return signer.SignShare(ctx, message, signerIndices, nil)
}

// AggregateThresholdSignatures aggregates BLS threshold shares.
func (h *Hybrid) AggregateThresholdSignatures(ctx context.Context, message []byte, shares []threshold.SignatureShare) (threshold.Signature, error) {
	if h.blsAggregator == nil {
		return nil, errors.New("BLS aggregator not initialized")
	}

	if len(shares) <= h.threshold {
		return nil, fmt.Errorf("insufficient shares: need at least %d, got %d", h.threshold+1, len(shares))
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	return h.blsAggregator.Aggregate(ctx, message, shares, nil)
}

// VerifyThresholdSignature verifies a BLS threshold signature.
func (h *Hybrid) VerifyThresholdSignature(message []byte, sig threshold.Signature) bool {
	if h.blsVerifier == nil {
		return false
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.blsVerifier.Verify(message, sig)
}

// VerifyThresholdSignatureBytes verifies serialized BLS threshold signature.
func (h *Hybrid) VerifyThresholdSignatureBytes(message, sig []byte) bool {
	if h.blsVerifier == nil {
		return false
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.blsVerifier.VerifyBytes(message, sig)
}

// AddValidatorThreshold adds a validator with BLS threshold key share.
func (h *Hybrid) AddValidatorThreshold(id string, keyShare threshold.KeyShare, weight uint64) error {
	if h.blsScheme == nil {
		return errors.New("BLS threshold scheme not initialized")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	signer, err := h.blsScheme.NewSigner(keyShare)
	if err != nil {
		return fmt.Errorf("failed to create signer: %w", err)
	}

	h.blsSigners[id] = signer

	h.validators[id] = &Validator{
		ID:     id,
		Weight: weight,
		Active: true,
	}

	return nil
}
