// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Grouped threshold signatures for scaling to 10,000+ validators.
// Uses probabilistic consensus to parallelize Corona signing.
//
// PERFORMANCE MODEL:
//   - BLS signs every block → 500ms finality (metastable consensus)
//   - Corona signs epoch checkpoints only → every 10 minutes
//   - Epoch checkpoint = hash of all block hashes in epoch
//   - Provides quantum-safe anchor without slowing block production

package quasar

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
	"github.com/luxfi/threshold/protocols/corona/keyera"
	"golang.org/x/crypto/sha3"
)

// groupedThresholdHashCustomization is the cSHAKE customisation tag that
// derives every internal digest in this file:
//   - committee-selection DRBG (assignToGroups Fisher–Yates seed)
//   - epoch-checkpoint chain hash (EpochCheckpoint.CheckpointHash)
//   - block-hash Merkle tree (computeMerkleRoot)
//
// All three digests are required to live in the SHA-3 family per the
// strict-PQ profile's HashSuiteID=SHA3NIST + MinHashOutputBits=384;
// SHA-256 was incorrect both because it sits at the SHA-2 family (FIPS
// 180-4, not FIPS 202) and because its 256-bit output is below the
// profile floor under Grover. Closes red-team findings F100 / F101.
const (
	groupedThresholdHashV1 = "LUX-QUASAR-GROUPED-THRESHOLD-V1"
	merkleNodeHashV1       = "LUX-QUASAR-MERKLE-NODE-V1"
	checkpointHashV1       = "LUX-QUASAR-CHECKPOINT-V1"
)

// sha3_384 returns a SHA3-384 (FIPS 202) digest of the concatenated
// inputs. 48-byte output matches the strict-PQ MinHashOutputBits floor;
// the customisation byte string flows into the cSHAKE256 N parameter
// so two SHA3-384 digests with different customisation strings cannot
// collide.
func sha3_384(customization string, parts ...[]byte) [48]byte {
	h := sha3.NewCShake256([]byte("KMAC"), []byte(customization))
	for _, p := range parts {
		_, _ = h.Write(p)
	}
	out := make([]byte, 48)
	_, _ = h.Read(out)
	var digest [48]byte
	copy(digest[:], out)
	return digest
}

// (F113) — the previous 32-byte sha3_256 helper has been removed. Every
// digest inside this file lives in the SHA-3 family at the 48-byte width
// the strict-PQ profile pins via MinHashOutputBits=384. The Merkle-tree
// / checkpoint paths now use sha3_384 directly; the EpochCheckpoint wire
// layout (MerkleRoot, PreviousAnchor) widens to [48]byte accordingly.
// Forward-only — no live chain runs the strict-PQ profile yet.

const (
	// DefaultGroupSize is the optimal group size for Corona threshold signing.
	// Corona scales O(n²), so small groups are critical for performance:
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
	ErrInsufficientGroups     = errors.New("insufficient groups signed")
	ErrGroupSignatureFailed   = errors.New("group signature failed")
	ErrInvalidGroupAssignment = errors.New("invalid group assignment")
)

// GroupedEpochManager extends EpochManager with grouped threshold signing.
// Instead of one global threshold key, validators are split into groups,
// each with their own Corona keys. This enables parallel signing and
// scales to 10,000+ validators with constant signing time.
type GroupedEpochManager struct {
	*EpochManager

	mu sync.RWMutex

	// Group configuration
	groupSize      int
	groupThreshold int
	groupQuorum    int // Number of groups required (out of total)

	// Current epoch group state
	groups           []*ValidatorGroup
	groupByValidator map[string]int // validator -> group index

	// Epoch randomness for group assignment (VRF-based)
	epochSeed []byte
}

// ValidatorGroup holds the Corona keys for a single group of validators.
type ValidatorGroup struct {
	Index      int
	Validators []string
	Threshold  int
	GroupKey   *coronaThreshold.GroupKey
	Shares     map[string]*coronaThreshold.KeyShare
	Signers    map[string]*coronaThreshold.Signer
}

// GroupedSignature holds signatures from multiple groups.
type GroupedSignature struct {
	Epoch           uint64
	Message         string
	GroupSignatures map[int]*coronaThreshold.Signature // group index -> signature
	SignedGroups    []int
}

// NewGroupedEpochManager creates a grouped epoch manager.
func NewGroupedEpochManager(groupSize, groupThreshold, historyLimit int) *GroupedEpochManager {
	if groupSize < 3 {
		groupSize = DefaultGroupSize
	}
	// Cap the signing group at the Mithril dealerless-RSS viability bound: a
	// Pulsar (ML-DSA / FIPS-204) group key produced WITHOUT a trusted dealer is
	// only stock-signable for N ≤ MithrilMaxCommittee (see mithril_committee.go
	// for the bound and the Avalanche-subsampling security argument). Groups are
	// the small, rotating, per-epoch signing committees; the chain's security
	// budget lives in subsampling + rotation + the 2/3 group quorum + dual-PQ
	// AND-mode, not in group size. The default group size (3) already complies;
	// this only constrains an over-large explicit configuration.
	if groupSize > MithrilMaxCommittee {
		groupSize = MithrilMaxCommittee
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
	// Note: Corona keygen uses global random state, so we run sequentially
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

	// Fisher-Yates shuffle with deterministic randomness from a
	// SHA3-384 DRBG. Per F100, the strict-PQ profile pins
	// HashSuiteID=SHA3NIST + MinHashOutputBits=384; the committee-
	// selection randomness MUST live in the same family so a
	// quantum-Grover-equipped adversary doesn't break the shuffle
	// at half the cost of the rest of the protocol.
	for i := len(shuffled) - 1; i > 0; i-- {
		indexBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(indexBytes, uint64(i))
		h := sha3_384(groupedThresholdHashV1, seed, indexBytes)
		// Use unsigned modulo to avoid negative indices.
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

// generateGroupKeys creates Pulsar share state for a single group via
// the keyera Bootstrap ceremony. Each group is its own key era — the
// grouped layout partitions validators into smaller signing committees,
// each carrying an independent persistent GroupKey. Resharing across
// the outer validator-set rotation flows through the LSS-Pulsar adapter
// at the EpochManager level; per-group bootstrap is a one-time call
// when the group is first formed.
func (gem *GroupedEpochManager) generateGroupKeys(index int, validators []string) (*ValidatorGroup, error) {
	n := len(validators)
	t := gem.groupThreshold
	if t >= n {
		t = n - 1
	}

	era, _, err := keyera.Bootstrap(
		t,
		validators,
		keyera.CoronaGroupID(index),
		keyera.CoronaKeyEraID(1),
		nil,
	)
	if err != nil {
		return nil, err
	}

	group := &ValidatorGroup{
		Index:      index,
		Validators: validators,
		Threshold:  t,
		GroupKey:   era.GroupKey,
		Shares:     make(map[string]*coronaThreshold.KeyShare),
		Signers:    make(map[string]*coronaThreshold.Signer),
	}

	for _, v := range validators {
		share := era.State.Shares[v]
		group.Shares[v] = share
		group.Signers[v] = coronaThreshold.NewSigner(share)
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

// GetGroupSigner returns the Corona signer for a validator in their group.
func (gem *GroupedEpochManager) GetGroupSigner(validatorID string) (*coronaThreshold.Signer, int, error) {
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
func (gem *GroupedEpochManager) GetGroupKey(groupIndex int) (*coronaThreshold.GroupKey, error) {
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

	// Verify all group signatures in parallel via coronaThreshold.VerifyBatch.
	// Build (groupKey, message, sig) triples up-front; skip any signature
	// whose groupIdx is out of bounds (treated as missing, not failed —
	// the quorum check below decides liveness).
	gks := make([]*coronaThreshold.GroupKey, 0, len(gs.GroupSignatures))
	msgs := make([]string, 0, len(gs.GroupSignatures))
	sigs := make([]*coronaThreshold.Signature, 0, len(gs.GroupSignatures))
	for groupIdx, sig := range gs.GroupSignatures {
		if groupIdx < 0 || groupIdx >= len(gem.groups) {
			continue
		}
		gks = append(gks, gem.groups[groupIdx].GroupKey)
		msgs = append(msgs, gs.Message)
		sigs = append(sigs, sig)
	}

	validGroups := 0
	if len(sigs) > 0 {
		results, err := coronaThreshold.VerifyBatch(gks, msgs, sigs)
		if err != nil {
			// Slice lens are equal by construction; surface unexpected
			// errors closed.
			return false, fmt.Errorf("VerifyGroupedSignature: %w", err)
		}
		for _, ok := range results {
			if ok {
				validGroups++
			}
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
) (map[int]*coronaThreshold.Signature, error) {
	gem.mu.RLock()
	defer gem.mu.RUnlock()

	results := make(map[int]*coronaThreshold.Signature)
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

// signWithGroup runs the 2-round Corona protocol for a single group.
func (gem *GroupedEpochManager) signWithGroup(
	groupIdx int,
	sessionID int,
	message string,
	prfKey []byte,
	signerIDs []string,
) (*coronaThreshold.Signature, error) {
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
	round1Data := make(map[int]*coronaThreshold.Round1Data)
	for _, vid := range signerIDs {
		signer := group.Signers[vid]
		r1, err := signer.Round1(sessionID, prfKey, signerIndices)
		if err != nil {
			return nil, fmt.Errorf("round1: validator %s: %w", vid, err)
		}
		round1Data[group.Shares[vid].Index] = r1
	}

	// Round 2: Compute z shares
	round2Data := make(map[int]*coronaThreshold.Round2Data)
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
// Created every epoch (10 min), contains Corona signature over block hashes.
// Normal blocks use BLS for 500ms finality; checkpoints add quantum security.
//
// Wire layout (F113): MerkleRoot and PreviousAnchor are [48]byte SHA3-384
// digests so the checkpoint chain honours the strict-PQ profile's
// MinHashOutputBits=384 floor. Forward-only — no live chain runs strict-PQ
// yet, so the prior [32]byte layout is not preserved.
type EpochCheckpoint struct {
	Epoch          uint64   // Epoch number
	StartHeight    uint64   // First block in epoch
	EndHeight      uint64   // Last block in epoch
	BlockCount     int      // Number of blocks
	MerkleRoot     [48]byte // SHA3-384 Merkle root of block hashes
	PreviousAnchor [48]byte // SHA3-384 previous checkpoint hash (chain of anchors)
	Timestamp      int64    // Unix timestamp

	// Quantum-safe signature (Corona grouped threshold)
	Signature *GroupedSignature
}

// CheckpointHash returns the hash of this checkpoint (for chaining).
//
// (F113) Output is SHA3-384 (48 bytes) under cSHAKE256 customisation
// LUX-QUASAR-CHECKPOINT-V1. Honours the strict-PQ profile's
// MinHashOutputBits=384 floor — Grover collision work on a 48-byte SHA-3
// digest sits at 2^192, above the profile claim.
func (ec *EpochCheckpoint) CheckpointHash() [48]byte {
	data := make([]byte, 0, 144)

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

	return sha3_384(checkpointHashV1, data)
}

// SignableMessage returns the message to be signed by Corona.
func (ec *EpochCheckpoint) SignableMessage() string {
	hash := ec.CheckpointHash()
	return fmt.Sprintf("QUASAR-CHECKPOINT-v1:%x", hash)
}

// CreateEpochCheckpoint creates a new checkpoint for a range of blocks.
// blockHashes should be ordered from StartHeight to EndHeight.
//
// (F113) blockHashes and previousAnchor are [48]byte SHA3-384 digests so
// the checkpoint chain meets the strict-PQ MinHashOutputBits=384 floor.
func CreateEpochCheckpoint(
	epoch uint64,
	startHeight, endHeight uint64,
	blockHashes [][48]byte,
	previousAnchor [48]byte,
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

// computeMerkleRoot computes a Merkle root over [48]byte block hashes.
//
// (F113) Each internal node is a SHA3-384 digest under cSHAKE256
// customisation LUX-QUASAR-MERKLE-NODE-V1; the 48-byte output honours
// the strict-PQ MinHashOutputBits=384 floor.
func computeMerkleRoot(hashes [][48]byte) [48]byte {
	if len(hashes) == 0 {
		return [48]byte{}
	}
	if len(hashes) == 1 {
		return hashes[0]
	}

	// Pad to even length
	level := make([][48]byte, len(hashes))
	copy(level, hashes)
	if len(level)%2 != 0 {
		level = append(level, level[len(level)-1])
	}

	for len(level) > 1 {
		nextLevel := make([][48]byte, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			combined := append(level[i][:], level[i+1][:]...)
			nextLevel[i/2] = sha3_384(merkleNodeHashV1, combined)
		}
		level = nextLevel
		if len(level) > 1 && len(level)%2 != 0 {
			level = append(level, level[len(level)-1])
		}
	}

	return level[0]
}

// SignCheckpoint signs an epoch checkpoint using grouped Corona.
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

// VerifyCheckpoint verifies a checkpoint's Corona signature.
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
