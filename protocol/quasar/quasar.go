// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Quasar: accretion-powered finality with BLS + Corona signatures

package quasar

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"crypto/rand"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
	"github.com/luxfi/crypto/threshold"
	_ "github.com/luxfi/crypto/threshold/bls" // Register BLS threshold scheme

	// Pulsar threshold is the corrected lattice kernel that replaced
	// upstream Nasua. Type aliasing preserves the historical
	// `coronaThreshold` identifier so this file's signing routines
	// (Round1/Round2/Finalize) stay byte-stable while the underlying
	// types come from pulsar/threshold.
	coronaThreshold "github.com/luxfi/corona/threshold"
)

// Buffer pools for hot paths - reduces GC pressure during signing/verification
var (
	sigPool = sync.Pool{
		New: func() any {
			return &QuasarSig{
				BLS:      make([]byte, 0, 96),
				Corona:    make([]byte, 0, 4096),
				MLDSA:    make([]byte, 0, 3309), // ML-DSA-65 sig size
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

// Signer implements parallel BLS + Corona + ML-DSA signing for PQ-safe consensus.
//
// Three independent signing paths:
//   - BLS12-381: classical threshold signatures (ECDL hardness)
//   - Corona:    Ring-LWE 2-round threshold signatures (Module-LWE hardness)
//   - ML-DSA-65: FIPS 204 post-quantum identity signatures (Module-LWE + Module-SIS)
//
// All three run in parallel via TripleSign. Each can be enabled independently.
type signer struct {
	mu sync.RWMutex

	// Classical BLS threshold signing (via crypto/threshold)
	blsScheme     threshold.Scheme
	blsGroupKey   threshold.PublicKey
	blsSigners    map[string]threshold.Signer
	blsAggregator threshold.Aggregator
	blsVerifier   threshold.Verifier

	// Post-quantum Corona threshold signing (native 2-round protocol)
	coronaGroupKey *coronaThreshold.GroupKey
	coronaSigners  map[string]*coronaThreshold.Signer
	coronaShares   map[string]*coronaThreshold.KeyShare

	// Post-quantum ML-DSA-65 identity signing (FIPS 204)
	mldsaKeys    map[string]*mldsa.PrivateKey
	mldsaPubKeys map[string]*mldsa.PublicKey

	// BLS direct keys (classical signing)
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
	CoronaPub []byte           // Corona group public key contribution
	MLDSAPubKey *mldsa.PublicKey // ML-DSA-65 identity key (nil if not configured)
	Weight      uint64
	Active      bool
}

// SignerConfig configures the dual threshold signing system.
type SignerConfig struct {
	Threshold    int
	TotalParties int

	// BLS threshold (via crypto/threshold interface)
	BLSKeyShares map[string]threshold.KeyShare
	BLSGroupKey  threshold.PublicKey

	// Corona threshold (native 2-round protocol)
	CoronaShares   map[string]*coronaThreshold.KeyShare
	CoronaGroupKey *coronaThreshold.GroupKey
}

// RingtailRound1State holds Round 1 data for all parties in a signing session.
type RingtailRound1State struct {
	SessionID  int
	PRFKey     []byte
	SignerIDs  []int
	Round1Data map[int]*coronaThreshold.Round1Data
}

// NewSigner creates a new signer engine with basic BLS support.
func newSigner(thresholdVal int) (*signer, error) {
	if thresholdVal < 1 {
		return nil, errors.New("threshold must be at least 1")
	}

	return &signer{
		blsKeys:         make(map[string]*bls.SecretKey),
		blsPubKeys:      make(map[string]*bls.PublicKey),
		blsSigners:      make(map[string]threshold.Signer),
		coronaSigners: make(map[string]*coronaThreshold.Signer),
		coronaShares:  make(map[string]*coronaThreshold.KeyShare),
		mldsaKeys:       make(map[string]*mldsa.PrivateKey),
		mldsaPubKeys:    make(map[string]*mldsa.PublicKey),
		validators:      make(map[string]*Validator),
		threshold:       thresholdVal,
	}, nil
}

// NewSignerWithDualThreshold creates a signer with full dual threshold signing.
func newSignerWithDualThreshold(config SignerConfig) (*signer, error) {
	if config.Threshold < 1 {
		return nil, errors.New("threshold must be at least 1")
	}

	h := &signer{
		blsKeys:         make(map[string]*bls.SecretKey),
		blsPubKeys:      make(map[string]*bls.PublicKey),
		blsSigners:      make(map[string]threshold.Signer),
		coronaSigners: make(map[string]*coronaThreshold.Signer),
		coronaShares:  make(map[string]*coronaThreshold.KeyShare),
		mldsaKeys:       make(map[string]*mldsa.PrivateKey),
		mldsaPubKeys:    make(map[string]*mldsa.PublicKey),
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

	// Initialize Corona signers (native 2-round protocol)
	h.coronaGroupKey = config.CoronaGroupKey
	for id, share := range config.CoronaShares {
		h.coronaShares[id] = share
		h.coronaSigners[id] = coronaThreshold.NewSigner(share)
	}

	return h, nil
}

// GenerateDualKeys generates both BLS and Corona threshold keys for an epoch.
// Call this when the validator set changes.
func GenerateDualKeys(t, n int) (*SignerConfig, error) {
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

	// Generate Corona threshold keys (native)
	coronaShares, coronaGroupKey, err := coronaThreshold.GenerateKeys(t, n, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Corona shares: %w", err)
	}

	// Convert to maps keyed by validator ID
	blsShareMap := make(map[string]threshold.KeyShare)
	ringtailShareMap := make(map[string]*coronaThreshold.KeyShare)

	for i := 0; i < n; i++ {
		id := fmt.Sprintf("v%d", i)
		blsShareMap[id] = blsShares[i]
		ringtailShareMap[id] = coronaShares[i]
	}

	return &SignerConfig{
		Threshold:        t,
		TotalParties:     n,
		BLSKeyShares:     blsShareMap,
		BLSGroupKey:      blsGroupKey,
		CoronaShares:   ringtailShareMap,
		CoronaGroupKey: coronaGroupKey,
	}, nil
}

// ============================================================================
// Corona 2-Round Protocol
// ============================================================================

// CoronaRound1 performs Round 1 of Corona signing for a validator.
// Returns Round1Data to broadcast to other validators.
func (s *signer) CoronaRound1(validatorID string, sessionID int, prfKey []byte) (*coronaThreshold.Round1Data, error) {
	s.mu.RLock()
	signer, exists := s.coronaSigners[validatorID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("validator %s not found in Corona signers", validatorID)
	}

	// Get all signer indices
	signerIDs := make([]int, 0, len(s.coronaSigners))
	for _, share := range s.coronaShares {
		signerIDs = append(signerIDs, share.Index)
	}

	return signer.Round1(sessionID, prfKey, signerIDs), nil
}

// CoronaRound2 performs Round 2 of Corona signing for a validator.
// Requires collected Round 1 data from all signers.
// Returns Round2Data to broadcast.
func (s *signer) CoronaRound2(validatorID string, sessionID int, message string, prfKey []byte, round1Data map[int]*coronaThreshold.Round1Data) (*coronaThreshold.Round2Data, error) {
	s.mu.RLock()
	signer, exists := s.coronaSigners[validatorID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("validator %s not found in Corona signers", validatorID)
	}

	signerIDs := make([]int, 0, len(s.coronaSigners))
	for _, share := range s.coronaShares {
		signerIDs = append(signerIDs, share.Index)
	}

	return signer.Round2(sessionID, message, prfKey, signerIDs, round1Data)
}

// CoronaFinalize aggregates Round 2 data into the final signature.
// Any validator can call this.
func (s *signer) CoronaFinalize(validatorID string, round2Data map[int]*coronaThreshold.Round2Data) (*coronaThreshold.Signature, error) {
	s.mu.RLock()
	signer, exists := s.coronaSigners[validatorID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("validator %s not found in Corona signers", validatorID)
	}

	return signer.Finalize(round2Data)
}

// VerifyRingtailSignature verifies a Corona threshold signature.
func (s *signer) VerifyRingtailSignature(message string, sig *coronaThreshold.Signature) bool {
	if s.coronaGroupKey == nil || sig == nil {
		return false
	}
	return coronaThreshold.Verify(s.coronaGroupKey, message, sig)
}

// ============================================================================
// Parallel BLS + Corona Signing
// ============================================================================

// DualSignRound1 performs Round 1 of both BLS and Corona in parallel.
// BLS: Computes signature share (single round)
// Corona:    Computes D matrix + MACs (Round 1 of 2)
func (s *signer) DualSignRound1(ctx context.Context, validatorID string, message []byte, sessionID int, prfKey []byte) (*QuasarSig, *coronaThreshold.Round1Data, error) {
	s.mu.RLock()
	blsSigner, hasBLS := s.blsSigners[validatorID]
	_, hasRingtail := s.coronaSigners[validatorID]
	s.mu.RUnlock()

	if !hasBLS || !hasRingtail {
		return nil, nil, errors.New("validator not configured for dual signing")
	}

	var wg sync.WaitGroup
	var blsErr, rtErr error
	var blsShare threshold.SignatureShare
	var round1Data *coronaThreshold.Round1Data

	// Get BLS signer indices
	blsIndices := make([]int, 0, len(s.blsSigners))
	for _, blsSigner := range s.blsSigners {
		blsIndices = append(blsIndices, blsSigner.Index())
	}

	wg.Add(2)

	// BLS signing (single round)
	go func() {
		defer wg.Done()
		blsShare, blsErr = blsSigner.SignShare(ctx, message, blsIndices, nil)
	}()

	// Corona Round 1
	go func() {
		defer wg.Done()
		round1Data, rtErr = s.CoronaRound1(validatorID, sessionID, prfKey)
	}()

	wg.Wait()

	if blsErr != nil {
		return nil, nil, fmt.Errorf("BLS signing failed: %w", blsErr)
	}
	if rtErr != nil {
		return nil, nil, fmt.Errorf("Corona Round1 failed: %w", rtErr)
	}

	sig := &QuasarSig{
		BLS:         blsShare.Bytes(),
		ValidatorID: validatorID,
		IsThreshold: true,
		SignerIndex: blsSigner.Index(),
	}

	return sig, round1Data, nil
}

// DualSignRound2 performs Round 2 of Corona (BLS is already done in Round1).
func (s *signer) DualSignRound2(validatorID string, sessionID int, message string, prfKey []byte, round1Data map[int]*coronaThreshold.Round1Data) (*coronaThreshold.Round2Data, error) {
	return s.CoronaRound2(validatorID, sessionID, message, prfKey, round1Data)
}

// ============================================================================
// Quasar: BLS + Corona + ML-DSA (all 3 in parallel)
// ============================================================================

// TripleSignRound1 performs Round 1 of all three signing paths in parallel:
//   - BLS: threshold share (single round, complete)
//   - Corona:    Round 1 of 2-round protocol (D matrix + MACs)
//   - ML-DSA: full signature (single round, complete)
//
// Returns the QuasarSig with BLS + MLDSA filled, plus Corona Round1Data
// for the 2-round protocol continuation.
func (s *signer) TripleSignRound1(ctx context.Context, validatorID string, message []byte, sessionID int, prfKey []byte) (*QuasarSig, *coronaThreshold.Round1Data, error) {
	s.mu.RLock()
	blsSigner, hasBLS := s.blsSigners[validatorID]
	_, hasRingtail := s.coronaSigners[validatorID]
	mldsaSK, hasMLDSA := s.mldsaKeys[validatorID]
	s.mu.RUnlock()

	if !hasBLS {
		return nil, nil, errors.New("validator not configured for BLS signing")
	}

	var wg sync.WaitGroup
	var blsErr, rtErr, mldsaErr error
	var blsShare threshold.SignatureShare
	var round1Data *coronaThreshold.Round1Data
	var mldsaSig []byte

	blsIndices := make([]int, 0, len(s.blsSigners))
	for _, bs := range s.blsSigners {
		blsIndices = append(blsIndices, bs.Index())
	}

	paths := 1 // BLS always
	if hasRingtail {
		paths++
	}
	if hasMLDSA {
		paths++
	}
	wg.Add(paths)

	// Path 1: BLS (classical threshold)
	go func() {
		defer wg.Done()
		blsShare, blsErr = blsSigner.SignShare(ctx, message, blsIndices, nil)
	}()

	// Path 2: Corona Round 1 (PQ lattice threshold)
	if hasRingtail {
		go func() {
			defer wg.Done()
			round1Data, rtErr = s.CoronaRound1(validatorID, sessionID, prfKey)
		}()
	}

	// Path 3: ML-DSA-65 (PQ identity, FIPS 204)
	if hasMLDSA {
		go func() {
			defer wg.Done()
			mldsaSig, mldsaErr = mldsaSK.Sign(rand.Reader, message, nil)
		}()
	}

	wg.Wait()

	if blsErr != nil {
		return nil, nil, fmt.Errorf("BLS signing failed: %w", blsErr)
	}
	if rtErr != nil {
		return nil, nil, fmt.Errorf("Corona Round1 failed: %w", rtErr)
	}
	if mldsaErr != nil {
		return nil, nil, fmt.Errorf("ML-DSA signing failed: %w", mldsaErr)
	}

	sig := &QuasarSig{
		BLS:         blsShare.Bytes(),
		MLDSA:       mldsaSig,
		ValidatorID: validatorID,
		IsThreshold: true,
		SignerIndex: blsSigner.Index(),
	}

	return sig, round1Data, nil
}

// IsTripleMode returns true if all three signing paths are configured.
func (s *signer) IsTripleMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.blsSigners) > 0 && len(s.coronaSigners) > 0 && len(s.mldsaKeys) > 0
}

// ============================================================================
// Legacy & Backward Compatibility
// ============================================================================

// AddValidator adds a validator with legacy BLS key generation.
func (s *signer) AddValidator(id string, weight uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// BLS key (classical)
	blsSK, err := bls.NewSecretKey()
	if err != nil {
		return fmt.Errorf("failed to generate BLS key: %w", err)
	}
	blsPK := blsSK.PublicKey()
	s.blsKeys[id] = blsSK
	s.blsPubKeys[id] = blsPK

	// ML-DSA-65 key (post-quantum identity, FIPS 204)
	mldsaSK, err := mldsa.GenerateKey(rand.Reader, mldsa.MLDSA65)
	if err != nil {
		return fmt.Errorf("failed to generate ML-DSA key: %w", err)
	}
	s.mldsaKeys[id] = mldsaSK
	s.mldsaPubKeys[id] = mldsaSK.PublicKey

	s.validators[id] = &Validator{
		ID:          id,
		BLSPubKey:   blsPK,
		MLDSAPubKey: mldsaSK.PublicKey,
		Weight:      weight,
		Active:      true,
	}

	return nil
}

// SignMessage signs a message with BLS (legacy mode or threshold).
func (s *signer) SignMessage(validatorID string, message []byte) (*QuasarSig, error) {
	return s.SignMessageWithContext(context.Background(), validatorID, message)
}

// SignMessageWithContext signs a message.
func (s *signer) SignMessageWithContext(ctx context.Context, validatorID string, message []byte) (*QuasarSig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sig := sigPool.Get().(*QuasarSig)
	sig.BLS = sig.BLS[:0]
	sig.Corona = sig.Corona[:0]
	sig.MLDSA = sig.MLDSA[:0]
	sig.ValidatorID = validatorID

	// Check for threshold signer
	blsSigner, hasBLSSigner := s.blsSigners[validatorID]
	if hasBLSSigner {
		blsIndices := make([]int, 0, len(s.blsSigners))
		for _, blsSigner := range s.blsSigners {
			blsIndices = append(blsIndices, blsSigner.Index())
		}

		share, err := blsSigner.SignShare(ctx, message, blsIndices, nil)
		if err != nil {
			return nil, fmt.Errorf("BLS threshold sign failed: %w", err)
		}

		sig.BLS = append(sig.BLS, share.Bytes()...)
		sig.IsThreshold = true
		sig.SignerIndex = blsSigner.Index()

		// Also sign with ML-DSA if available (Quasar)
		if mldsaSK, ok := s.mldsaKeys[validatorID]; ok {
			mldsaSig, err := mldsaSK.Sign(rand.Reader, message, nil)
			if err == nil {
				sig.MLDSA = append(sig.MLDSA, mldsaSig...)
			}
		}

		return sig, nil
	}

	// Fall back to legacy BLS + ML-DSA
	blsSK, exists := s.blsKeys[validatorID]
	if !exists {
		return nil, errors.New("validator not found")
	}

	blsSig, err := blsSK.Sign(message)
	if err != nil {
		return nil, fmt.Errorf("BLS sign failed: %w", err)
	}

	sig.BLS = append(sig.BLS, bls.SignatureToBytes(blsSig)...)
	sig.IsThreshold = false

	// Also sign with ML-DSA if available (Quasar)
	if mldsaSK, ok := s.mldsaKeys[validatorID]; ok {
		mldsaSig, err := mldsaSK.Sign(rand.Reader, message, nil)
		if err == nil {
			sig.MLDSA = append(sig.MLDSA, mldsaSig...)
		}
	}

	return sig, nil
}

// ReleaseQuasarSig returns a QuasarSig to the pool.
func ReleaseQuasarSig(sig *QuasarSig) {
	if sig == nil {
		return
	}
	sig.ValidatorID = ""
	sig.IsThreshold = false
	sig.SignerIndex = 0
	sig.MLDSA = sig.MLDSA[:0]
	sigPool.Put(sig)
}

// VerifyQuasarSig verifies a signature.
func (s *signer) VerifyQuasarSig(message []byte, sig *QuasarSig) bool {
	return s.VerifyQuasarSigWithContext(context.Background(), message, sig)
}

// VerifyQuasarSigWithContext verifies a signature.
// Verifies all present paths: BLS (always), ML-DSA (when sig.MLDSA is non-empty).
// Corona threshold verification is handled separately via VerifyRingtailSignature.
func (s *signer) VerifyQuasarSigWithContext(ctx context.Context, message []byte, sig *QuasarSig) bool {
	if ctx.Err() != nil {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Verify BLS (classical path — always required)
	blsOK := false
	if sig.IsThreshold {
		if s.blsVerifier == nil || len(sig.BLS) == 0 {
			return false
		}
		blsOK = s.blsVerifier.VerifyBytes(message, sig.BLS)
	} else {
		validator, exists := s.validators[sig.ValidatorID]
		if !exists {
			return false
		}
		blsSig, err := bls.SignatureFromBytes(sig.BLS)
		if err != nil {
			return false
		}
		blsOK = bls.Verify(validator.BLSPubKey, blsSig, message)
	}
	if !blsOK {
		return false
	}

	// Verify ML-DSA (PQ identity path — when present)
	if len(sig.MLDSA) > 0 {
		validator, exists := s.validators[sig.ValidatorID]
		if !exists || validator.MLDSAPubKey == nil {
			return false
		}
		if !validator.MLDSAPubKey.Verify(message, sig.MLDSA, nil) {
			return false
		}
	}

	return true
}

// AggregateSignatures aggregates BLS threshold signature shares.
func (s *signer) AggregateSignatures(message []byte, signatures []*QuasarSig) (*AggregatedSignature, error) {
	return s.AggregateSignaturesWithContext(context.Background(), message, signatures)
}

// AggregateSignaturesWithContext aggregates signatures.
func (s *signer) AggregateSignaturesWithContext(ctx context.Context, message []byte, signatures []*QuasarSig) (*AggregatedSignature, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(signatures) < s.threshold {
		return nil, fmt.Errorf("insufficient signatures: %d < %d", len(signatures), s.threshold)
	}

	// Check for threshold mode
	if len(signatures) > 0 && signatures[0].IsThreshold && s.blsAggregator != nil {
		blsShares := make([]threshold.SignatureShare, 0, len(signatures))

		for _, sig := range signatures {
			if len(sig.BLS) > 0 {
				share, err := s.blsScheme.ParseSignatureShare(sig.BLS)
				if err == nil {
					blsShares = append(blsShares, share)
				}
			}
		}

		if len(blsShares) >= s.threshold {
			blsAggSig, err := s.blsAggregator.Aggregate(ctx, message, blsShares, nil)
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
func (s *signer) VerifyAggregatedSignature(message []byte, aggSig *AggregatedSignature) bool {
	return s.VerifyAggregatedSignatureWithContext(context.Background(), message, aggSig)
}

// VerifyAggregatedSignatureWithContext verifies an aggregated signature.
func (s *signer) VerifyAggregatedSignatureWithContext(ctx context.Context, message []byte, aggSig *AggregatedSignature) bool {
	if ctx.Err() != nil {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if aggSig.SignerCount < s.threshold {
		return false
	}

	if aggSig.IsThreshold {
		if s.blsVerifier != nil && len(aggSig.BLSAggregated) > 0 {
			return s.blsVerifier.VerifyBytes(message, aggSig.BLSAggregated)
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
		validator, exists := s.validators[validatorID]
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

// QuasarSig contains BLS and optionally Corona signature data.
type QuasarSig struct {
	BLS         []byte // BLS-12-381 aggregate (classical fast-path; empty in pure-PQ)
	Corona      []byte // Corona (Ring-LWE) threshold signature
	Pulsar      []byte // Pulsar-M (Module-LWE) threshold signature
	MLDSA       []byte // Per-validator ML-DSA-65 (FIPS 204 identity attestation)
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

// AnyValidatorID returns any configured validator ID, or "" if none.
// Used by drivers that need to drive the signer for a single contributor
// (e.g. PostQuantum.GenerateQuantumProof).
func (s *signer) AnyValidatorID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for id := range s.blsSigners {
		return id
	}
	for id := range s.blsKeys {
		return id
	}
	return ""
}

// GetActiveValidatorCount returns the number of active validators.
func (s *signer) GetActiveValidatorCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, v := range s.validators {
		if v.Active {
			count++
		}
	}
	return count
}

// GetThreshold returns the consensus threshold.
func (s *signer) GetThreshold() int {
	return s.threshold
}

// IsThresholdMode returns true if BLS threshold signing is enabled.
func (s *signer) IsThresholdMode() bool {
	return s.blsScheme != nil
}

// IsDualThresholdMode returns true if both BLS and Corona are enabled.
func (s *signer) IsDualThresholdMode() bool {
	return s.blsScheme != nil && s.coronaGroupKey != nil
}

// ThresholdConfig for single-scheme threshold signing.
type ThresholdConfig struct {
	SchemeID     threshold.SchemeID
	Threshold    int
	TotalParties int
	KeyShares    map[string]threshold.KeyShare
	GroupKey     threshold.PublicKey
}

// newSignerWithThresholdConfig creates a signer from ThresholdConfig.
func newSignerWithThresholdConfig(config ThresholdConfig) (*signer, error) {
	if config.Threshold < 1 {
		return nil, errors.New("threshold must be at least 1")
	}

	scheme, err := threshold.GetScheme(config.SchemeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get threshold scheme: %w", err)
	}

	h := &signer{
		blsKeys:         make(map[string]*bls.SecretKey),
		blsPubKeys:      make(map[string]*bls.PublicKey),
		blsSigners:      make(map[string]threshold.Signer),
		coronaSigners: make(map[string]*coronaThreshold.Signer),
		coronaShares:  make(map[string]*coronaThreshold.KeyShare),
		mldsaKeys:       make(map[string]*mldsa.PrivateKey),
		mldsaPubKeys:    make(map[string]*mldsa.PublicKey),
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

// GenerateDualThresholdKeys generates both BLS and Corona keys.
func GenerateDualThresholdKeys(t, n int) (*SignerConfig, error) {
	return GenerateDualKeys(t, n)
}

// ThresholdScheme returns the BLS threshold scheme.
func (s *signer) ThresholdScheme() threshold.Scheme {
	return s.blsScheme
}

// ThresholdGroupKey returns the BLS group public key.
func (s *signer) ThresholdGroupKey() threshold.PublicKey {
	return s.blsGroupKey
}

// SignMessageThreshold signs using BLS threshold.
func (s *signer) SignMessageThreshold(ctx context.Context, validatorID string, message []byte) (threshold.SignatureShare, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	signer, exists := s.blsSigners[validatorID]
	if !exists {
		return nil, fmt.Errorf("validator %s not found in BLS signers", validatorID)
	}

	signerIndices := make([]int, 0, len(s.blsSigners))
	for _, blsSigner := range s.blsSigners {
		signerIndices = append(signerIndices, blsSigner.Index())
	}

	return signer.SignShare(ctx, message, signerIndices, nil)
}

// AggregateThresholdSignatures aggregates BLS threshold shares.
func (s *signer) AggregateThresholdSignatures(ctx context.Context, message []byte, shares []threshold.SignatureShare) (threshold.Signature, error) {
	if s.blsAggregator == nil {
		return nil, errors.New("BLS aggregator not initialized")
	}

	if len(shares) <= s.threshold {
		return nil, fmt.Errorf("insufficient shares: need at least %d, got %d", s.threshold+1, len(shares))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.blsAggregator.Aggregate(ctx, message, shares, nil)
}

// VerifyThresholdSignature verifies a BLS threshold signature.
func (s *signer) VerifyThresholdSignature(message []byte, sig threshold.Signature) bool {
	if s.blsVerifier == nil {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.blsVerifier.Verify(message, sig)
}

// VerifyThresholdSignatureBytes verifies serialized BLS threshold signature.
func (s *signer) VerifyThresholdSignatureBytes(message, sig []byte) bool {
	if s.blsVerifier == nil {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.blsVerifier.VerifyBytes(message, sig)
}

// AddValidatorThreshold adds a validator with BLS threshold key share.
func (s *signer) AddValidatorThreshold(id string, keyShare threshold.KeyShare, weight uint64) error {
	if s.blsScheme == nil {
		return errors.New("BLS threshold scheme not initialized")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	signer, err := s.blsScheme.NewSigner(keyShare)
	if err != nil {
		return fmt.Errorf("failed to create signer: %w", err)
	}

	s.blsSigners[id] = signer

	s.validators[id] = &Validator{
		ID:     id,
		Weight: weight,
		Active: true,
	}

	return nil
}
