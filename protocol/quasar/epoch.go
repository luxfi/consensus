// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Epoch-based Ringtail key management for quantum-safe consensus.
// Keys rotate when validator sets change, with rate limiting for security.

package quasar

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	ringtailThreshold "github.com/luxfi/ringtail/threshold"
)

const (
	// MinEpochDuration is the minimum time between key rotations.
	// Changing keys frequently makes quantum attacks harder since any
	// progress toward breaking keys is invalidated when keys change.
	// At 10 minutes per epoch, uint64 supports 351 trillion years of epochs.
	MinEpochDuration = 10 * time.Minute

	// MaxEpochDuration is the maximum time keys can be used.
	// Forces rotation even if validator set hasn't changed.
	// With 10-minute epochs, force rotation after 1 hour (6 epochs).
	MaxEpochDuration = 1 * time.Hour

	// DefaultHistoryLimit is the default number of old epochs to keep.
	// With 10-minute epochs, 6 epochs = 1 hour of history for verification.
	DefaultHistoryLimit = 6

	// QuantumCheckpointInterval is how often we create quantum-safe signatures.
	// Every 3 seconds provides frequent quantum-safe anchors while keys rotate
	// every 10 minutes. With <100 validators, 3-second signing is achievable.
	QuantumCheckpointInterval = 3 * time.Second
)

var (
	ErrEpochRateLimited = errors.New("epoch keygen rate limited: must wait at least 10 minutes between rotations")
	ErrNoValidatorChange  = errors.New("validator set unchanged, no keygen needed")
	ErrEpochNotFound      = errors.New("epoch not found")
	ErrInvalidValidatorSet = errors.New("invalid validator set configuration")
)

// EpochManager manages Ringtail key epochs for the validator set.
// Fresh keys are generated when validators change, with rate limiting
// to prevent excessive key churn while still rotating frequently enough
// to frustrate quantum attacks.
type EpochManager struct {
	mu sync.RWMutex

	// Current epoch state
	currentEpoch    uint64
	currentKeys     *EpochKeys
	lastKeygenTime  time.Time

	// Historical epochs for signature verification
	// We need to verify signatures from recent epochs during transitions
	epochHistory    map[uint64]*EpochKeys
	historyLimit    int // How many old epochs to keep

	// Validator set tracking
	currentValidators []string
	threshold         int
}

// EpochKeys holds the Ringtail keys for a specific epoch.
type EpochKeys struct {
	Epoch           uint64
	CreatedAt       time.Time
	ExpiresAt       time.Time
	ValidatorSet    []string
	Threshold       int
	TotalParties    int
	GroupKey        *ringtailThreshold.GroupKey
	Shares          map[string]*ringtailThreshold.KeyShare
	Signers         map[string]*ringtailThreshold.Signer
}

// NewEpochManager creates a new epoch manager.
func NewEpochManager(threshold int, historyLimit int) *EpochManager {
	if historyLimit < 1 {
		historyLimit = DefaultHistoryLimit // Keep enough epochs for cross-epoch verification
	}

	return &EpochManager{
		epochHistory:  make(map[uint64]*EpochKeys),
		historyLimit:  historyLimit,
		threshold:     threshold,
	}
}

// InitializeEpoch creates the first epoch with the initial validator set.
// This should be called once at genesis or node startup.
func (em *EpochManager) InitializeEpoch(validators []string) (*EpochKeys, error) {
	em.mu.Lock()
	defer em.mu.Unlock()

	if len(validators) < 2 {
		return nil, fmt.Errorf("%w: need at least 2 validators", ErrInvalidValidatorSet)
	}

	if em.threshold >= len(validators) {
		return nil, fmt.Errorf("%w: threshold %d must be less than validator count %d",
			ErrInvalidValidatorSet, em.threshold, len(validators))
	}

	keys, err := em.generateEpochKeys(0, validators)
	if err != nil {
		return nil, err
	}

	em.currentEpoch = 0
	em.currentKeys = keys
	em.currentValidators = validators
	em.lastKeygenTime = time.Now()
	em.epochHistory[0] = keys

	return keys, nil
}

// RotateEpoch generates new keys for a new validator set.
// Returns ErrEpochRateLimited if called within MinEpochDuration of last rotation.
// Returns ErrNoValidatorChange if validator set hasn't changed (unless force=true).
func (em *EpochManager) RotateEpoch(validators []string, force bool) (*EpochKeys, error) {
	em.mu.Lock()
	defer em.mu.Unlock()

	now := time.Now()

	// Rate limiting: at most 1 rotation per 10 minutes
	if !em.lastKeygenTime.IsZero() {
		elapsed := now.Sub(em.lastKeygenTime)
		if elapsed < MinEpochDuration {
			remaining := MinEpochDuration - elapsed
			return nil, fmt.Errorf("%w: %v remaining", ErrEpochRateLimited, remaining.Round(time.Second))
		}
	}

	// Check if validator set actually changed
	if !force && em.validatorSetUnchanged(validators) {
		return nil, ErrNoValidatorChange
	}

	// Validate new set
	if len(validators) < 2 {
		return nil, fmt.Errorf("%w: need at least 2 validators", ErrInvalidValidatorSet)
	}

	effectiveThreshold := em.threshold
	if effectiveThreshold >= len(validators) {
		effectiveThreshold = len(validators) - 1
	}

	newEpoch := em.currentEpoch + 1
	keys, err := em.generateEpochKeysWithThreshold(newEpoch, validators, effectiveThreshold)
	if err != nil {
		return nil, err
	}

	// Store old epoch in history
	if em.currentKeys != nil {
		em.epochHistory[em.currentEpoch] = em.currentKeys
	}

	// Prune old epochs
	em.pruneHistory()

	// Update current state
	em.currentEpoch = newEpoch
	em.currentKeys = keys
	em.currentValidators = validators
	em.lastKeygenTime = now
	em.epochHistory[newEpoch] = keys

	return keys, nil
}

// ForceRotateIfExpired rotates keys if MaxEpochDuration has passed.
// This ensures keys don't stay valid indefinitely even if validator set is stable.
func (em *EpochManager) ForceRotateIfExpired() (*EpochKeys, bool, error) {
	em.mu.RLock()
	if em.currentKeys == nil {
		em.mu.RUnlock()
		return nil, false, nil
	}

	expired := time.Now().After(em.currentKeys.ExpiresAt)
	validators := em.currentValidators
	em.mu.RUnlock()

	if !expired {
		return nil, false, nil
	}

	keys, err := em.RotateEpoch(validators, true)
	if err != nil {
		return nil, false, err
	}

	return keys, true, nil
}

// GetCurrentKeys returns the current epoch's keys.
func (em *EpochManager) GetCurrentKeys() *EpochKeys {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return em.currentKeys
}

// GetEpochKeys returns keys for a specific epoch (current or historical).
func (em *EpochManager) GetEpochKeys(epoch uint64) (*EpochKeys, error) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	if keys, exists := em.epochHistory[epoch]; exists {
		return keys, nil
	}
	return nil, fmt.Errorf("%w: epoch %d", ErrEpochNotFound, epoch)
}

// GetCurrentEpoch returns the current epoch number.
func (em *EpochManager) GetCurrentEpoch() uint64 {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return em.currentEpoch
}

// GetSigner returns the Ringtail signer for a validator in the current epoch.
func (em *EpochManager) GetSigner(validatorID string) (*ringtailThreshold.Signer, error) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	if em.currentKeys == nil {
		return nil, errors.New("no current epoch keys")
	}

	signer, exists := em.currentKeys.Signers[validatorID]
	if !exists {
		return nil, fmt.Errorf("validator %s not in current epoch", validatorID)
	}

	return signer, nil
}

// GetSignerForEpoch returns the signer for a validator in a specific epoch.
func (em *EpochManager) GetSignerForEpoch(validatorID string, epoch uint64) (*ringtailThreshold.Signer, error) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	keys, exists := em.epochHistory[epoch]
	if !exists {
		return nil, fmt.Errorf("%w: epoch %d", ErrEpochNotFound, epoch)
	}

	signer, exists := keys.Signers[validatorID]
	if !exists {
		return nil, fmt.Errorf("validator %s not in epoch %d", validatorID, epoch)
	}

	return signer, nil
}

// VerifySignatureForEpoch verifies a Ringtail signature using the epoch's keys.
func (em *EpochManager) VerifySignatureForEpoch(message string, sig *ringtailThreshold.Signature, epoch uint64) bool {
	em.mu.RLock()
	keys, exists := em.epochHistory[epoch]
	em.mu.RUnlock()

	if !exists || keys.GroupKey == nil || sig == nil {
		return false
	}

	return ringtailThreshold.Verify(keys.GroupKey, message, sig)
}

// TimeUntilNextRotation returns how long until the next rotation is allowed.
func (em *EpochManager) TimeUntilNextRotation() time.Duration {
	em.mu.RLock()
	defer em.mu.RUnlock()

	if em.lastKeygenTime.IsZero() {
		return 0
	}

	elapsed := time.Since(em.lastKeygenTime)
	if elapsed >= MinEpochDuration {
		return 0
	}

	return MinEpochDuration - elapsed
}

// Stats returns current epoch statistics.
func (em *EpochManager) Stats() EpochStats {
	em.mu.RLock()
	defer em.mu.RUnlock()

	var validatorCount int
	var epochAge time.Duration
	if em.currentKeys != nil {
		validatorCount = len(em.currentKeys.ValidatorSet)
		epochAge = time.Since(em.currentKeys.CreatedAt)
	}

	return EpochStats{
		CurrentEpoch:      em.currentEpoch,
		EpochAge:          epochAge,
		ValidatorCount:    validatorCount,
		Threshold:         em.threshold,
		HistorySize:       len(em.epochHistory),
		LastKeygenTime:    em.lastKeygenTime,
		TimeUntilRotation: em.timeUntilNextRotationLocked(),
	}
}

// EpochStats provides statistics about the epoch manager.
type EpochStats struct {
	CurrentEpoch      uint64
	EpochAge          time.Duration
	ValidatorCount    int
	Threshold         int
	HistorySize       int
	LastKeygenTime    time.Time
	TimeUntilRotation time.Duration
}

// Internal helpers

func (em *EpochManager) generateEpochKeys(epoch uint64, validators []string) (*EpochKeys, error) {
	return em.generateEpochKeysWithThreshold(epoch, validators, em.threshold)
}

func (em *EpochManager) generateEpochKeysWithThreshold(epoch uint64, validators []string, threshold int) (*EpochKeys, error) {
	n := len(validators)

	// Generate Ringtail threshold keys
	shares, groupKey, err := ringtailThreshold.GenerateKeys(threshold, n, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Ringtail keys: %w", err)
	}

	now := time.Now()
	keys := &EpochKeys{
		Epoch:        epoch,
		CreatedAt:    now,
		ExpiresAt:    now.Add(MaxEpochDuration),
		ValidatorSet: validators,
		Threshold:    threshold,
		TotalParties: n,
		GroupKey:     groupKey,
		Shares:       make(map[string]*ringtailThreshold.KeyShare),
		Signers:      make(map[string]*ringtailThreshold.Signer),
	}

	// Map shares to validator IDs
	for i, validatorID := range validators {
		keys.Shares[validatorID] = shares[i]
		keys.Signers[validatorID] = ringtailThreshold.NewSigner(shares[i])
	}

	return keys, nil
}

func (em *EpochManager) validatorSetUnchanged(newValidators []string) bool {
	if len(newValidators) != len(em.currentValidators) {
		return false
	}

	// Create set of current validators
	current := make(map[string]bool, len(em.currentValidators))
	for _, v := range em.currentValidators {
		current[v] = true
	}

	// Check if all new validators exist in current set
	for _, v := range newValidators {
		if !current[v] {
			return false
		}
	}

	return true
}

func (em *EpochManager) pruneHistory() {
	if len(em.epochHistory) <= em.historyLimit {
		return
	}

	// Find the minimum epoch to keep
	minEpochToKeep := em.currentEpoch
	if uint64(em.historyLimit) < minEpochToKeep {
		minEpochToKeep = em.currentEpoch - uint64(em.historyLimit) + 1
	} else {
		minEpochToKeep = 0
	}

	// Remove old epochs
	for epoch := range em.epochHistory {
		if epoch < minEpochToKeep {
			delete(em.epochHistory, epoch)
		}
	}
}

func (em *EpochManager) timeUntilNextRotationLocked() time.Duration {
	if em.lastKeygenTime.IsZero() {
		return 0
	}

	elapsed := time.Since(em.lastKeygenTime)
	if elapsed >= MinEpochDuration {
		return 0
	}

	return MinEpochDuration - elapsed
}

// ============================================================================
// Quantum Block - Bundles BLS blocks into quantum-safe anchors
// ============================================================================
//
// Architecture (parallel execution):
//   BLS Layer:     [B1]--[B2]--[B3]--[B4]--[B5]--[B6]--[B7]--[B8]--[B9]--...
//                   |     500ms finality per block     |
//                   |_____________________________________|
//                                    |
//   Quantum Layer:              [QB1: Merkle(B1-B6)]--------[QB2: Merkle(B7-B12)]
//                                    |  3-second interval, async Ringtail signing
//
// NTT Ringtail benchmarks (IEEE S&P 2025):
//   - 0.6s online signing phase (2-round protocol)
//   - 2.5s total including offline prep across 5 continents
//   - Our 3-second interval provides comfortable margin

// QuantumBundle bundles multiple BLS-signed blocks into a quantum-safe anchor.
// BLS blocks continue at 500ms pace; quantum bundles form every 3 seconds
// containing a Merkle root of ~6 BLS block hashes.
// Note: This is distinct from core.go's QuantumBlock which represents per-block finality.
type QuantumBundle struct {
	Epoch        uint64     // Current key epoch
	Sequence     uint64     // Bundle sequence within epoch
	StartHeight  uint64     // First BLS block in this bundle
	EndHeight    uint64     // Last BLS block in this bundle
	BlockCount   int        // Number of BLS blocks bundled
	MerkleRoot   [32]byte   // Merkle root of BLS block hashes
	BlockHashes  [][32]byte // Individual block hashes (for Merkle proof)
	PreviousHash [32]byte   // Previous bundle hash (chain linkage)
	Timestamp    int64      // Unix timestamp

	// Ringtail threshold signature (post-quantum secure)
	Signature *ringtailThreshold.Signature
}

// Hash returns the hash of this bundle for chain linkage.
func (qb *QuantumBundle) Hash() [32]byte {
	h := sha256.New()
	buf := make([]byte, 8)

	// Epoch + sequence
	binary.BigEndian.PutUint64(buf, qb.Epoch)
	h.Write(buf)
	binary.BigEndian.PutUint64(buf, qb.Sequence)
	h.Write(buf)

	// Block range
	binary.BigEndian.PutUint64(buf, qb.StartHeight)
	h.Write(buf)
	binary.BigEndian.PutUint64(buf, qb.EndHeight)
	h.Write(buf)

	// Merkle root of bundled BLS blocks
	h.Write(qb.MerkleRoot[:])

	// Previous bundle hash (chain linkage)
	h.Write(qb.PreviousHash[:])

	// Timestamp
	binary.BigEndian.PutUint64(buf, uint64(qb.Timestamp))
	h.Write(buf)

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// SignableMessage returns the message for Ringtail signing.
func (qb *QuantumBundle) SignableMessage() string {
	hash := qb.Hash()
	return fmt.Sprintf("QUASAR-QB-v1:%x", hash)
}

// BundleSigner handles creating and verifying 3-second quantum bundles.
// Bundles accumulate multiple BLS blocks and sign them with Ringtail.
type BundleSigner struct {
	em         *EpochManager
	lastBundle *QuantumBundle
	sequence   uint64

	// Pending BLS blocks waiting to be bundled
	pendingBlocks [][32]byte
	pendingStart  uint64
	pendingEnd    uint64

	mu sync.Mutex
}

// NewBundleSigner creates a bundle signer for the epoch manager.
func NewBundleSigner(em *EpochManager) *BundleSigner {
	return &BundleSigner{em: em}
}

// AddBLSBlock adds a finalized BLS block hash to the pending bundle.
// Called whenever a BLS block achieves finality (~500ms).
func (bs *BundleSigner) AddBLSBlock(height uint64, hash [32]byte) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if len(bs.pendingBlocks) == 0 {
		bs.pendingStart = height
	}
	bs.pendingBlocks = append(bs.pendingBlocks, hash)
	bs.pendingEnd = height
}

// PendingCount returns the number of pending BLS blocks.
func (bs *BundleSigner) PendingCount() int {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return len(bs.pendingBlocks)
}

// CreateBundle bundles pending BLS blocks into a quantum bundle.
// Call this every 3 seconds (QuantumCheckpointInterval).
// Returns nil if no pending blocks.
func (bs *BundleSigner) CreateBundle() *QuantumBundle {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if len(bs.pendingBlocks) == 0 {
		return nil
	}

	// Compute Merkle root of bundled BLS blocks
	merkleRoot := ComputeMerkleRoot(bs.pendingBlocks)

	// Get previous bundle hash for chain linkage
	var prevHash [32]byte
	if bs.lastBundle != nil {
		prevHash = bs.lastBundle.Hash()
	}

	epoch := bs.em.GetCurrentEpoch()

	// Reset sequence on new epoch
	if bs.lastBundle != nil && bs.lastBundle.Epoch != epoch {
		bs.sequence = 0
	}

	qb := &QuantumBundle{
		Epoch:        epoch,
		Sequence:     bs.sequence,
		StartHeight:  bs.pendingStart,
		EndHeight:    bs.pendingEnd,
		BlockCount:   len(bs.pendingBlocks),
		MerkleRoot:   merkleRoot,
		BlockHashes:  bs.pendingBlocks,
		PreviousHash: prevHash,
		Timestamp:    time.Now().Unix(),
	}

	// Clear pending and advance sequence
	bs.pendingBlocks = nil
	bs.sequence++
	bs.lastBundle = qb

	return qb
}

// LastBundle returns the most recent bundle.
func (bs *BundleSigner) LastBundle() *QuantumBundle {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.lastBundle
}

// SignBundle performs the 2-round Ringtail signing for a bundle.
// This runs the full threshold signing protocol with participating validators.
func (bs *BundleSigner) SignBundle(
	bundle *QuantumBundle,
	sessionID int,
	prfKey []byte,
	participatingValidators []string,
) error {
	bs.em.mu.RLock()
	keys := bs.em.currentKeys
	bs.em.mu.RUnlock()

	if keys == nil {
		return errors.New("no current epoch keys")
	}

	// Build signer indices
	signerIndices := make([]int, 0, len(participatingValidators))
	for _, v := range participatingValidators {
		if share, ok := keys.Shares[v]; ok {
			signerIndices = append(signerIndices, share.Index)
		}
	}

	if len(signerIndices) < keys.Threshold {
		return fmt.Errorf("insufficient signers: %d < threshold %d", len(signerIndices), keys.Threshold)
	}

	message := bundle.SignableMessage()

	// Round 1: Collect commitments
	round1Data := make(map[int]*ringtailThreshold.Round1Data)
	for _, v := range participatingValidators {
		signer, exists := keys.Signers[v]
		if !exists {
			continue
		}
		r1 := signer.Round1(sessionID, prfKey, signerIndices)
		round1Data[keys.Shares[v].Index] = r1
	}

	// Round 2: Generate signature shares
	round2Data := make(map[int]*ringtailThreshold.Round2Data)
	for _, v := range participatingValidators {
		signer, exists := keys.Signers[v]
		if !exists {
			continue
		}
		r2, err := signer.Round2(sessionID, message, prfKey, signerIndices, round1Data)
		if err != nil {
			return fmt.Errorf("round2 failed for %s: %w", v, err)
		}
		round2Data[r2.PartyID] = r2
	}

	// Finalize
	firstValidator := participatingValidators[0]
	signer := keys.Signers[firstValidator]
	sig, err := signer.Finalize(round2Data)
	if err != nil {
		return fmt.Errorf("finalize failed: %w", err)
	}

	bundle.Signature = sig
	return nil
}

// VerifyBundle verifies a quantum bundle's Ringtail signature.
func (bs *BundleSigner) VerifyBundle(bundle *QuantumBundle) bool {
	if bundle.Signature == nil {
		return false
	}

	// Verify Merkle root matches block hashes
	expectedRoot := ComputeMerkleRoot(bundle.BlockHashes)
	if expectedRoot != bundle.MerkleRoot {
		return false
	}

	// Get keys for the bundle's epoch
	keys, err := bs.em.GetEpochKeys(bundle.Epoch)
	if err != nil {
		return false
	}

	message := bundle.SignableMessage()
	return ringtailThreshold.Verify(keys.GroupKey, message, bundle.Signature)
}

// ComputeMerkleRoot computes a Merkle root over block hashes.
func ComputeMerkleRoot(hashes [][32]byte) [32]byte {
	if len(hashes) == 0 {
		return [32]byte{}
	}
	if len(hashes) == 1 {
		return hashes[0]
	}

	// Pad to even length
	level := make([][32]byte, len(hashes))
	copy(level, hashes)
	if len(level)%2 != 0 {
		level = append(level, level[len(level)-1])
	}

	// Build tree
	for len(level) > 1 {
		nextLevel := make([][32]byte, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			combined := append(level[i][:], level[i+1][:]...)
			nextLevel[i/2] = sha256.Sum256(combined)
		}
		level = nextLevel
		if len(level) > 1 && len(level)%2 != 0 {
			level = append(level, level[len(level)-1])
		}
	}

	return level[0]
}
