// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Grouped threshold signatures for scaling to 10,000+ validators.
// Uses probabilistic consensus to parallelize Ringtail signing.
//
// PERFORMANCE MODEL:
//   - BLS signs every block → 500ms finality (metastable consensus)
//   - Ringtail signs epoch checkpoints only → every 10 minutes
//   - Epoch checkpoint = hash of all block hashes in epoch
//   - Provides quantum-safe anchor without slowing block production

package quasar

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	ringtailThreshold "github.com/luxfi/ringtail/threshold"
)

const (
	// DefaultGroupSize is the optimal group size for Ringtail threshold signing.
	// Ringtail scales O(n²), so small groups are critical for performance:
	//   n=3: 243ms, n=10: 1.1s, n=21: 3.4s, n=100: 53s
	// Groups of 3 with 2-of-3 threshold provides 243ms signing.
	DefaultGroupSize = 3

	// DefaultGroupThreshold is the threshold within each group.
	// 2-of-3 = majority required within each group.
	DefaultGroupThreshold = 2

	// DefaultGroupQuorum is the fraction of groups that must sign.
	// 2/3 of groups must produce valid signatures for consensus.
	DefaultGroupQuorum = 2 // numerator (2/3)
	GroupQuorumDenom   = 3 // denominator
)

var (
	ErrInsufficientGroups    = errors.New("insufficient groups signed")
	ErrGroupSignatureFailed  = errors.New("group signature failed")
	ErrInvalidGroupAssignment = errors.New("invalid group assignment")
)

// GroupedEpochManager extends EpochManager with grouped threshold signing.
// Instead of one global threshold key, validators are split into groups,
// each with their own Ringtail keys. This enables parallel signing and
// scales to 10,000+ validators with constant signing time.
type GroupedEpochManager struct {
	*EpochManager

	mu sync.RWMutex

	// Group configuration
	groupSize      int
	groupThreshold int
	groupQuorum    int // Number of groups required (out of total)

	// Current epoch group state
	groups       []*ValidatorGroup
	groupByValidator map[string]int // validator -> group index

	// Epoch randomness for group assignment (VRF-based)
	epochSeed []byte
}

// ValidatorGroup holds the Ringtail keys for a single group of validators.
type ValidatorGroup struct {
	Index       int
	Validators  []string
	Threshold   int
	GroupKey    *ringtailThreshold.GroupKey
	Shares      map[string]*ringtailThreshold.KeyShare
	Signers     map[string]*ringtailThreshold.Signer
}

// GroupedSignature holds signatures from multiple groups.
type GroupedSignature struct {
	Epoch           uint64
	Message         string
	GroupSignatures map[int]*ringtailThreshold.Signature // group index -> signature
	SignedGroups    []int
}

// NewGroupedEpochManager creates a grouped epoch manager.
func NewGroupedEpochManager(groupSize, groupThreshold, historyLimit int) *GroupedEpochManager {
	if groupSize < 3 {
		groupSize = DefaultGroupSize
	}
	if groupThreshold < 2 || groupThreshold >= groupSize {
		groupThreshold = (groupSize * 2) / 3 // default 2/3
	}

	return &GroupedEpochManager{
		EpochManager:     NewEpochManager(groupThreshold, historyLimit),
		groupSize:        groupSize,
		groupThreshold:   groupThreshold,
		groupByValidator: make(map[string]int),
	}
}

// InitializeGroupedEpoch creates the first epoch with grouped validator assignment.
func (gem *GroupedEpochManager) InitializeGroupedEpoch(validators []string, epochSeed []byte) error {
	gem.mu.Lock()
	defer gem.mu.Unlock()

	if len(validators) < gem.groupSize {
		// Fall back to single group for small validator sets
		// Use appropriate threshold for small sets
		smallThreshold := len(validators) - 1
		if smallThreshold < 1 {
			smallThreshold = 1
		}
		smallEM := NewEpochManager(smallThreshold, gem.historyLimit)
		keys, err := smallEM.InitializeEpoch(validators)
		if err != nil {
			return err
		}
		gem.EpochManager = smallEM

		// Set up single group
		gem.groups = []*ValidatorGroup{{
			Index:      0,
			Validators: validators,
			Threshold:  smallThreshold,
			GroupKey:   keys.GroupKey,
			Shares:     keys.Shares,
			Signers:    keys.Signers,
		}}
		gem.groupQuorum = 1
		for _, v := range validators {
			gem.groupByValidator[v] = 0
		}
		return nil
	}

	gem.epochSeed = epochSeed

	// Assign validators to groups using deterministic shuffle
	groups := gem.assignToGroups(validators, epochSeed)
	gem.groups = make([]*ValidatorGroup, len(groups))
	gem.groupByValidator = make(map[string]int)

	// Calculate quorum (2/3 of groups must sign)
	gem.groupQuorum = (len(groups) * DefaultGroupQuorum) / GroupQuorumDenom
	if gem.groupQuorum < 1 {
		gem.groupQuorum = 1
	}

	// Generate keys for each group
	// Note: Ringtail keygen uses global random state, so we run sequentially
	// The keygen is fast enough (~3ms per group) that this is acceptable
	for i, groupValidators := range groups {
		group, err := gem.generateGroupKeys(i, groupValidators)
		if err != nil {
			return fmt.Errorf("group %d: %w", i, err)
		}

		gem.groups[i] = group
		for _, v := range groupValidators {
			gem.groupByValidator[v] = i
		}
	}

	return nil
}

// assignToGroups deterministically assigns validators to groups using epoch seed.
func (gem *GroupedEpochManager) assignToGroups(validators []string, seed []byte) [][]string {
	// Create deterministic shuffle using seed
	shuffled := make([]string, len(validators))
	copy(shuffled, validators)

	// Fisher-Yates shuffle with deterministic randomness from seed
	for i := len(shuffled) - 1; i > 0; i-- {
		// Create index-specific seed
		indexBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(indexBytes, uint64(i))
		h := sha256.Sum256(append(seed, indexBytes...))
		// Use unsigned modulo to avoid negative indices
		j := int(binary.BigEndian.Uint64(h[:8]) % uint64(i+1))
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	// Split into groups
	numGroups := len(shuffled) / gem.groupSize
	if len(shuffled)%gem.groupSize != 0 {
		numGroups++ // Handle remainder
	}

	groups := make([][]string, numGroups)
	for i := 0; i < numGroups; i++ {
		start := i * gem.groupSize
		end := start + gem.groupSize
		if end > len(shuffled) {
			end = len(shuffled)
		}
		groups[i] = shuffled[start:end]
	}

	return groups
}

// generateGroupKeys creates Ringtail keys for a single group.
func (gem *GroupedEpochManager) generateGroupKeys(index int, validators []string) (*ValidatorGroup, error) {
	n := len(validators)
	t := gem.groupThreshold
	if t >= n {
		t = n - 1
	}

	shares, groupKey, err := ringtailThreshold.GenerateKeys(t, n, nil)
	if err != nil {
		return nil, err
	}

	group := &ValidatorGroup{
		Index:      index,
		Validators: validators,
		Threshold:  t,
		GroupKey:   groupKey,
		Shares:     make(map[string]*ringtailThreshold.KeyShare),
		Signers:    make(map[string]*ringtailThreshold.Signer),
	}

	for i, v := range validators {
		group.Shares[v] = shares[i]
		group.Signers[v] = ringtailThreshold.NewSigner(shares[i])
	}

	return group, nil
}

// GetValidatorGroup returns the group index for a validator.
func (gem *GroupedEpochManager) GetValidatorGroup(validatorID string) (int, error) {
	gem.mu.RLock()
	defer gem.mu.RUnlock()

	idx, exists := gem.groupByValidator[validatorID]
	if !exists {
		return -1, ErrInvalidGroupAssignment
	}
	return idx, nil
}

// GetGroupSigner returns the Ringtail signer for a validator in their group.
func (gem *GroupedEpochManager) GetGroupSigner(validatorID string) (*ringtailThreshold.Signer, int, error) {
	gem.mu.RLock()
	defer gem.mu.RUnlock()

	idx, exists := gem.groupByValidator[validatorID]
	if !exists {
		return nil, -1, ErrInvalidGroupAssignment
	}

	group := gem.groups[idx]
	signer, exists := group.Signers[validatorID]
	if !exists {
		return nil, -1, fmt.Errorf("validator %s not in group %d", validatorID, idx)
	}

	return signer, idx, nil
}

// GetGroupValidators returns the validators in a specific group.
func (gem *GroupedEpochManager) GetGroupValidators(groupIndex int) ([]string, error) {
	gem.mu.RLock()
	defer gem.mu.RUnlock()

	if groupIndex < 0 || groupIndex >= len(gem.groups) {
		return nil, fmt.Errorf("invalid group index: %d", groupIndex)
	}

	return gem.groups[groupIndex].Validators, nil
}

// GetGroupKey returns the group's public key.
func (gem *GroupedEpochManager) GetGroupKey(groupIndex int) (*ringtailThreshold.GroupKey, error) {
	gem.mu.RLock()
	defer gem.mu.RUnlock()

	if groupIndex < 0 || groupIndex >= len(gem.groups) {
		return nil, fmt.Errorf("invalid group index: %d", groupIndex)
	}

	return gem.groups[groupIndex].GroupKey, nil
}

// VerifyGroupedSignature verifies that enough groups signed the message.
func (gem *GroupedEpochManager) VerifyGroupedSignature(gs *GroupedSignature) (bool, error) {
	gem.mu.RLock()
	defer gem.mu.RUnlock()

	if len(gs.GroupSignatures) < gem.groupQuorum {
		return false, fmt.Errorf("%w: got %d, need %d", ErrInsufficientGroups,
			len(gs.GroupSignatures), gem.groupQuorum)
	}

	// Verify each group signature
	validGroups := 0
	for groupIdx, sig := range gs.GroupSignatures {
		if groupIdx < 0 || groupIdx >= len(gem.groups) {
			continue
		}

		group := gem.groups[groupIdx]
		if ringtailThreshold.Verify(group.GroupKey, gs.Message, sig) {
			validGroups++
		}
	}

	if validGroups < gem.groupQuorum {
		return false, fmt.Errorf("%w: only %d valid, need %d",
			ErrInsufficientGroups, validGroups, gem.groupQuorum)
	}

	return true, nil
}

// Stats returns grouped epoch statistics.
func (gem *GroupedEpochManager) GroupedStats() GroupedEpochStats {
	gem.mu.RLock()
	defer gem.mu.RUnlock()

	totalValidators := 0
	for _, g := range gem.groups {
		totalValidators += len(g.Validators)
	}

	return GroupedEpochStats{
		EpochStats:      gem.EpochManager.Stats(),
		NumGroups:       len(gem.groups),
		GroupSize:       gem.groupSize,
		GroupThreshold:  gem.groupThreshold,
		GroupQuorum:     gem.groupQuorum,
		TotalValidators: totalValidators,
	}
}

// GroupedEpochStats extends EpochStats with group information.
type GroupedEpochStats struct {
	EpochStats
	NumGroups       int
	GroupSize       int
	GroupThreshold  int
	GroupQuorum     int
	TotalValidators int
}

// ParallelGroupSign runs threshold signing for all groups in parallel.
// Returns signatures from each group. Caller aggregates into GroupedSignature.
func (gem *GroupedEpochManager) ParallelGroupSign(
	sessionID int,
	message string,
	prfKey []byte,
	signersByGroup map[int][]string, // group -> participating validators
) (map[int]*ringtailThreshold.Signature, error) {
	gem.mu.RLock()
	defer gem.mu.RUnlock()

	results := make(map[int]*ringtailThreshold.Signature)
	var resultsMu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(signersByGroup))

	for groupIdx, signers := range signersByGroup {
		if groupIdx < 0 || groupIdx >= len(gem.groups) {
			continue
		}

		wg.Add(1)
		go func(gIdx int, validators []string) {
			defer wg.Done()

			sig, err := gem.signWithGroup(gIdx, sessionID, message, prfKey, validators)
			if err != nil {
				errChan <- fmt.Errorf("group %d: %w", gIdx, err)
				return
			}

			resultsMu.Lock()
			results[gIdx] = sig
			resultsMu.Unlock()
		}(groupIdx, signers)
	}

	wg.Wait()
	close(errChan)

	// Collect errors (non-fatal - some groups may fail)
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	// Check if we have quorum
	if len(results) < gem.groupQuorum {
		return results, fmt.Errorf("%w: %d groups signed, need %d (errors: %v)",
			ErrInsufficientGroups, len(results), gem.groupQuorum, errs)
	}

	return results, nil
}

// signWithGroup runs the 2-round Ringtail protocol for a single group.
func (gem *GroupedEpochManager) signWithGroup(
	groupIdx int,
	sessionID int,
	message string,
	prfKey []byte,
	signerIDs []string,
) (*ringtailThreshold.Signature, error) {
	group := gem.groups[groupIdx]

	// Build signer index list
	signerIndices := make([]int, len(signerIDs))
	for i, vid := range signerIDs {
		share, exists := group.Shares[vid]
		if !exists {
			return nil, fmt.Errorf("validator %s not in group", vid)
		}
		signerIndices[i] = share.Index
	}
	sort.Ints(signerIndices)

	// Round 1: Collect D matrices
	round1Data := make(map[int]*ringtailThreshold.Round1Data)
	for _, vid := range signerIDs {
		signer := group.Signers[vid]
		r1 := signer.Round1(sessionID, prfKey, signerIndices)
		round1Data[group.Shares[vid].Index] = r1
	}

	// Round 2: Compute z shares
	round2Data := make(map[int]*ringtailThreshold.Round2Data)
	for _, vid := range signerIDs {
		signer := group.Signers[vid]
		r2, err := signer.Round2(sessionID, message, prfKey, signerIndices, round1Data)
		if err != nil {
			return nil, err
		}
		round2Data[r2.PartyID] = r2
	}

	// Finalize
	firstSigner := group.Signers[signerIDs[0]]
	sig, err := firstSigner.Finalize(round2Data)
	if err != nil {
		return nil, err
	}

	return sig, nil
}

// ============================================================================
// Epoch Checkpoint - Quantum-safe anchors for block ranges
// ============================================================================

// EpochCheckpoint represents a quantum-safe anchor for a range of blocks.
// Created every epoch (10 min), contains Ringtail signature over block hashes.
// Normal blocks use BLS for 500ms finality; checkpoints add quantum security.
type EpochCheckpoint struct {
	Epoch          uint64   // Epoch number
	StartHeight    uint64   // First block in epoch
	EndHeight      uint64   // Last block in epoch
	BlockCount     int      // Number of blocks
	MerkleRoot     [32]byte // Merkle root of block hashes
	PreviousAnchor [32]byte // Previous checkpoint hash (chain of anchors)
	Timestamp      int64    // Unix timestamp

	// Quantum-safe signature (Ringtail grouped threshold)
	Signature      *GroupedSignature
}

// CheckpointHash returns the hash of this checkpoint (for chaining).
func (ec *EpochCheckpoint) CheckpointHash() [32]byte {
	data := make([]byte, 0, 128)

	// Epoch + heights
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, ec.Epoch)
	data = append(data, buf...)
	binary.BigEndian.PutUint64(buf, ec.StartHeight)
	data = append(data, buf...)
	binary.BigEndian.PutUint64(buf, ec.EndHeight)
	data = append(data, buf...)

	// Merkle root + previous anchor
	data = append(data, ec.MerkleRoot[:]...)
	data = append(data, ec.PreviousAnchor[:]...)

	// Timestamp
	binary.BigEndian.PutUint64(buf, uint64(ec.Timestamp))
	data = append(data, buf...)

	return sha256.Sum256(data)
}

// SignableMessage returns the message to be signed by Ringtail.
func (ec *EpochCheckpoint) SignableMessage() string {
	hash := ec.CheckpointHash()
	return fmt.Sprintf("QUASAR-CHECKPOINT-v1:%x", hash)
}

// CreateEpochCheckpoint creates a new checkpoint for a range of blocks.
// blockHashes should be ordered from StartHeight to EndHeight.
func CreateEpochCheckpoint(
	epoch uint64,
	startHeight, endHeight uint64,
	blockHashes [][32]byte,
	previousAnchor [32]byte,
) *EpochCheckpoint {
	// Build Merkle root of block hashes
	merkleRoot := computeMerkleRoot(blockHashes)

	return &EpochCheckpoint{
		Epoch:          epoch,
		StartHeight:    startHeight,
		EndHeight:      endHeight,
		BlockCount:     len(blockHashes),
		MerkleRoot:     merkleRoot,
		PreviousAnchor: previousAnchor,
		Timestamp:      time.Now().Unix(),
	}
}

// computeMerkleRoot computes a simple Merkle root over block hashes.
func computeMerkleRoot(hashes [][32]byte) [32]byte {
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

// SignCheckpoint signs an epoch checkpoint using grouped Ringtail.
// This runs asynchronously - doesn't block normal block production.
func (gem *GroupedEpochManager) SignCheckpoint(
	checkpoint *EpochCheckpoint,
	sessionID int,
	prfKey []byte,
	signersByGroup map[int][]string,
) error {
	message := checkpoint.SignableMessage()

	sigs, err := gem.ParallelGroupSign(sessionID, message, prfKey, signersByGroup)
	if err != nil {
		return err
	}

	checkpoint.Signature = &GroupedSignature{
		Epoch:           checkpoint.Epoch,
		Message:         message,
		GroupSignatures: sigs,
	}

	return nil
}

// VerifyCheckpoint verifies a checkpoint's Ringtail signature.
func (gem *GroupedEpochManager) VerifyCheckpoint(checkpoint *EpochCheckpoint) (bool, error) {
	if checkpoint.Signature == nil {
		return false, errors.New("checkpoint has no signature")
	}

	// Verify message matches checkpoint
	expectedMessage := checkpoint.SignableMessage()
	if checkpoint.Signature.Message != expectedMessage {
		return false, errors.New("signature message mismatch")
	}

	return gem.VerifyGroupedSignature(checkpoint.Signature)
}
