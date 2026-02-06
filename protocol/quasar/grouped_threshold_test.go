// Copyright (C) 2025, Lux Industries Inc All rights reserved.

package quasar

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGroupedEpochManager_SmallSet(t *testing.T) {
	// Use group size 5 so 3 validators triggers small set fallback
	gem := NewGroupedEpochManager(5, 3, 3)

	// Small set (< groupSize) falls back to single group behavior
	validators := []string{"v0", "v1", "v2"}
	seed := sha256.Sum256([]byte("epoch-0"))

	err := gem.InitializeGroupedEpoch(validators, seed[:])
	require.NoError(t, err)

	// Should use regular epoch manager for small sets
	keys := gem.GetCurrentKeys()
	require.NotNil(t, keys)
	require.Equal(t, 3, len(keys.ValidatorSet))

	// Should have 1 group with all validators
	stats := gem.GroupedStats()
	require.Equal(t, 1, stats.NumGroups)
	require.Equal(t, 3, stats.TotalValidators)
}

func TestGroupedEpochManager_LargeSet(t *testing.T) {
	gem := NewGroupedEpochManager(3, 2, 6) // 2-of-3 groups (fast!)

	// Create 99 validators (should create 33 groups of 3)
	validators := make([]string, 99)
	for i := 0; i < 99; i++ {
		validators[i] = fmt.Sprintf("v%d", i)
	}

	seed := sha256.Sum256([]byte("epoch-0-seed"))
	start := time.Now()
	err := gem.InitializeGroupedEpoch(validators, seed[:])
	elapsed := time.Since(start)

	require.NoError(t, err)

	stats := gem.GroupedStats()
	require.Equal(t, 33, stats.NumGroups) // 99/3 = 33 groups
	require.Equal(t, 99, stats.TotalValidators)
	require.Equal(t, 3, stats.GroupSize)
	require.Equal(t, 2, stats.GroupThreshold)
	require.Equal(t, 22, stats.GroupQuorum) // 2/3 of 33 = 22

	t.Logf("99 validators in %d groups, keygen: %v", stats.NumGroups, elapsed)
}

func TestGroupedEpochManager_ParallelKeygen(t *testing.T) {
	gem := NewGroupedEpochManager(3, 2, 6) // 2-of-3 groups

	// 999 validators → 333 groups of 3
	validators := make([]string, 999)
	for i := 0; i < 999; i++ {
		validators[i] = fmt.Sprintf("validator-%04d", i)
	}

	seed := sha256.Sum256([]byte("epoch-999-validators"))
	start := time.Now()
	err := gem.InitializeGroupedEpoch(validators, seed[:])
	elapsed := time.Since(start)

	require.NoError(t, err)

	stats := gem.GroupedStats()
	t.Logf("999 validators → %d groups of 3, keygen: %v", stats.NumGroups, elapsed)
	t.Logf("Naive 999-validator keygen would be ~500ms+")

	// 333 groups × 3ms = ~1s sequential keygen
	// Skip timing check with race detector (adds 10-20x overhead)
	if !raceEnabled {
		require.Less(t, elapsed, 2*time.Second, "Grouped keygen should complete in reasonable time")
	} else {
		t.Logf("Skipping timing check with race detector enabled")
	}
}

func TestGroupedEpochManager_GroupAssignment(t *testing.T) {
	gem := NewGroupedEpochManager(3, 2, 3) // 2-of-3 groups

	validators := make([]string, 9) // Exactly 3 groups of 3
	for i := 0; i < 9; i++ {
		validators[i] = fmt.Sprintf("v%d", i)
	}

	seed := sha256.Sum256([]byte("deterministic-seed"))
	err := gem.InitializeGroupedEpoch(validators, seed[:])
	require.NoError(t, err)

	// Each validator should be in exactly one group
	groupCounts := make(map[int]int)
	for _, v := range validators {
		groupIdx, err := gem.GetValidatorGroup(v)
		require.NoError(t, err)
		require.GreaterOrEqual(t, groupIdx, 0)
		require.Less(t, groupIdx, 3)
		groupCounts[groupIdx]++
	}

	// Each group should have 3 validators
	for i := 0; i < 3; i++ {
		require.Equal(t, 3, groupCounts[i], "Group %d should have 3 validators", i)
	}

	t.Logf("3 groups with 3 validators each: %v", groupCounts)
}

func TestGroupedEpochManager_DeterministicAssignment(t *testing.T) {
	// Same seed should produce same group assignment
	validators := make([]string, 12)
	for i := 0; i < 12; i++ {
		validators[i] = fmt.Sprintf("v%d", i)
	}

	seed := sha256.Sum256([]byte("reproducible-seed"))

	gem1 := NewGroupedEpochManager(3, 2, 3)
	err := gem1.InitializeGroupedEpoch(validators, seed[:])
	require.NoError(t, err)

	gem2 := NewGroupedEpochManager(3, 2, 3)
	err = gem2.InitializeGroupedEpoch(validators, seed[:])
	require.NoError(t, err)

	// Same validator should be in same group
	for _, v := range validators {
		g1, _ := gem1.GetValidatorGroup(v)
		g2, _ := gem2.GetValidatorGroup(v)
		require.Equal(t, g1, g2, "Validator %s should be in same group", v)
	}

	t.Log("Group assignment is deterministic with same seed")
}

func TestGroupedEpochManager_ParallelGroupSign(t *testing.T) {
	gem := NewGroupedEpochManager(3, 2, 3) // 2-of-3 groups (fast!)

	// Create 9 validators (3 groups of 3)
	validators := make([]string, 9)
	for i := 0; i < 9; i++ {
		validators[i] = fmt.Sprintf("v%d", i)
	}

	seed := sha256.Sum256([]byte("sign-test-seed"))
	err := gem.InitializeGroupedEpoch(validators, seed[:])
	require.NoError(t, err)

	// Build signers for each group (all validators participate)
	signersByGroup := make(map[int][]string)
	for _, v := range validators {
		groupIdx, _ := gem.GetValidatorGroup(v)
		signersByGroup[groupIdx] = append(signersByGroup[groupIdx], v)
	}

	// Run parallel signing
	sessionID := 1
	message := "block-hash-to-sign"
	prfKey := []byte("prf-key-for-grouped-signing!!!!")

	start := time.Now()
	sigs, err := gem.ParallelGroupSign(sessionID, message, prfKey, signersByGroup)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Len(t, sigs, 3, "All 3 groups should produce signatures")

	t.Logf("Parallel group signing (3 groups of 3): %v", elapsed)
	t.Logf("Target: <500ms to match BLS consensus speed")

	// With groups of 3, should be ~243ms × 3 sequential = ~729ms
	// But with parallel, should be ~243ms
	// Skip timing check with race detector (adds 10-20x overhead)
	if !raceEnabled {
		require.Less(t, elapsed, 2*time.Second, "Signing should be reasonably fast")
	} else {
		t.Logf("Skipping timing check with race detector enabled")
	}

	// Verify grouped signature
	gs := &GroupedSignature{
		Epoch:           0,
		Message:         message,
		GroupSignatures: sigs,
	}

	valid, err := gem.VerifyGroupedSignature(gs)
	require.NoError(t, err)
	require.True(t, valid)

	t.Log("Grouped signature verified successfully")
}

func TestGroupedEpochManager_QuorumEnforcement(t *testing.T) {
	gem := NewGroupedEpochManager(3, 2, 3) // 2-of-3 groups

	validators := make([]string, 9) // 3 groups of 3
	for i := 0; i < 9; i++ {
		validators[i] = fmt.Sprintf("v%d", i)
	}

	seed := sha256.Sum256([]byte("quorum-test"))
	err := gem.InitializeGroupedEpoch(validators, seed[:])
	require.NoError(t, err)

	stats := gem.GroupedStats()
	require.Equal(t, 2, stats.GroupQuorum) // 2/3 of 3 = 2

	// Get validators from only group 0 (need to ensure we only have one group)
	group0Validators, err := gem.GetGroupValidators(0)
	require.NoError(t, err)

	// Only sign with 1 group (insufficient for quorum of 2)
	signersByGroup := make(map[int][]string)
	signersByGroup[0] = group0Validators

	sessionID := 1
	message := "test-message"
	prfKey := []byte("prf-key-for-quorum-test!!!!!!!!")

	sigs, err := gem.ParallelGroupSign(sessionID, message, prfKey, signersByGroup)
	require.Error(t, err, "Should fail with insufficient groups")
	require.ErrorIs(t, err, ErrInsufficientGroups)

	t.Logf("Correctly rejected: only %d groups signed, need %d", len(sigs), stats.GroupQuorum)
}

func BenchmarkGroupedKeygen(b *testing.B) {
	sizes := []int{99, 300, 999, 3000} // Divisible by 3 for clean groups

	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			validators := make([]string, n)
			for i := 0; i < n; i++ {
				validators[i] = fmt.Sprintf("v%d", i)
			}
			seed := sha256.Sum256([]byte("benchmark-seed"))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				gem := NewGroupedEpochManager(3, 2, 3) // 2-of-3 groups for speed
				gem.InitializeGroupedEpoch(validators, seed[:])
			}
		})
	}
}

func BenchmarkGroupedSigning(b *testing.B) {
	gem := NewGroupedEpochManager(3, 2, 3) // 2-of-3 groups for speed

	validators := make([]string, 30) // 10 groups of 3
	for i := 0; i < 30; i++ {
		validators[i] = fmt.Sprintf("v%d", i)
	}

	seed := sha256.Sum256([]byte("bench-sign-seed"))
	gem.InitializeGroupedEpoch(validators, seed[:])

	// All validators participate
	signersByGroup := make(map[int][]string)
	for _, v := range validators {
		groupIdx, _ := gem.GetValidatorGroup(v)
		signersByGroup[groupIdx] = append(signersByGroup[groupIdx], v)
	}

	prfKey := []byte("prf-key-for-benchmark-signing!!!")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		message := fmt.Sprintf("block-%d", i)
		gem.ParallelGroupSign(i, message, prfKey, signersByGroup)
	}
}
