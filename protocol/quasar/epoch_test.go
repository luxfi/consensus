// Copyright (C) 2025, Lux Industries Inc All rights reserved.

package quasar

import (
	"fmt"
	"testing"
	"time"

	ringtailThreshold "github.com/luxfi/ringtail/threshold"
	"github.com/stretchr/testify/require"
)

func TestEpochManager_Initialize(t *testing.T) {
	em := NewEpochManager(2, 3)

	validators := []string{"v0", "v1", "v2"}
	keys, err := em.InitializeEpoch(validators)
	require.NoError(t, err)
	require.NotNil(t, keys)

	require.Equal(t, uint64(0), keys.Epoch)
	require.Equal(t, 3, keys.TotalParties)
	require.Equal(t, 2, keys.Threshold)
	require.Equal(t, validators, keys.ValidatorSet)
	require.NotNil(t, keys.GroupKey)
	require.Len(t, keys.Shares, 3)
	require.Len(t, keys.Signers, 3)

	t.Logf("Epoch 0 initialized: %d validators, threshold %d", keys.TotalParties, keys.Threshold)
}

func TestEpochManager_Initialize_InvalidThreshold(t *testing.T) {
	em := NewEpochManager(3, 3) // threshold >= validators

	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidValidatorSet)
}

func TestEpochManager_Initialize_TooFewValidators(t *testing.T) {
	em := NewEpochManager(1, 3)

	validators := []string{"v0"} // only 1 validator
	_, err := em.InitializeEpoch(validators)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidValidatorSet)
}

func TestEpochManager_RotateEpoch_RateLimited(t *testing.T) {
	em := NewEpochManager(2, 3)

	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	// Try to rotate immediately - should fail
	newValidators := []string{"v0", "v1", "v3"} // v2 -> v3
	_, err = em.RotateEpoch(newValidators, false)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrEpochRateLimited)

	t.Log("Rotation correctly rate limited within 10 minutes")
}

func TestEpochManager_RotateEpoch_NoChange(t *testing.T) {
	em := NewEpochManager(2, 3)

	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	// Hack: bypass rate limit for testing
	em.mu.Lock()
	em.lastKeygenTime = time.Now().Add(-2 * time.Hour)
	em.mu.Unlock()

	// Try to rotate with same validator set - should fail
	_, err = em.RotateEpoch(validators, false)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNoValidatorChange)

	t.Log("Rotation correctly rejected when validator set unchanged")
}

func TestEpochManager_RotateEpoch_Force(t *testing.T) {
	em := NewEpochManager(2, 3)

	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	// Hack: bypass rate limit for testing
	em.mu.Lock()
	em.lastKeygenTime = time.Now().Add(-2 * time.Hour)
	em.mu.Unlock()

	// Force rotate even with same validator set
	keys, err := em.RotateEpoch(validators, true)
	require.NoError(t, err)
	require.NotNil(t, keys)

	require.Equal(t, uint64(1), keys.Epoch)
	require.Equal(t, uint64(1), em.GetCurrentEpoch())

	t.Log("Force rotation succeeded")
}

func TestEpochManager_RotateEpoch_ValidatorChange(t *testing.T) {
	em := NewEpochManager(2, 3)

	validators := []string{"v0", "v1", "v2"}
	keys0, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	// Hack: bypass rate limit for testing
	em.mu.Lock()
	em.lastKeygenTime = time.Now().Add(-2 * time.Hour)
	em.mu.Unlock()

	// Rotate with new validator (v2 replaced by v3)
	newValidators := []string{"v0", "v1", "v3"}
	keys1, err := em.RotateEpoch(newValidators, false)
	require.NoError(t, err)
	require.NotNil(t, keys1)

	require.Equal(t, uint64(1), keys1.Epoch)
	require.Contains(t, keys1.ValidatorSet, "v3")
	require.NotContains(t, keys1.ValidatorSet, "v2")

	// Old epoch should still be accessible for verification
	oldKeys, err := em.GetEpochKeys(0)
	require.NoError(t, err)
	require.Equal(t, keys0.GroupKey, oldKeys.GroupKey)

	t.Log("Validator rotation from v2 to v3 succeeded")
}

func TestEpochManager_RotateEpoch_AddValidator(t *testing.T) {
	em := NewEpochManager(2, 3)

	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	// Hack: bypass rate limit for testing
	em.mu.Lock()
	em.lastKeygenTime = time.Now().Add(-2 * time.Hour)
	em.mu.Unlock()

	// Add a new validator
	newValidators := []string{"v0", "v1", "v2", "v3"}
	keys, err := em.RotateEpoch(newValidators, false)
	require.NoError(t, err)
	require.NotNil(t, keys)

	require.Equal(t, 4, keys.TotalParties)
	require.Equal(t, 2, keys.Threshold) // threshold stays the same

	t.Log("Added v3 to validator set")
}

func TestEpochManager_GetSigner(t *testing.T) {
	em := NewEpochManager(2, 3)

	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	signer, err := em.GetSigner("v0")
	require.NoError(t, err)
	require.NotNil(t, signer)

	_, err = em.GetSigner("v99") // non-existent
	require.Error(t, err)
}

func TestEpochManager_FullSigningFlow(t *testing.T) {
	em := NewEpochManager(2, 3)

	validators := []string{"v0", "v1", "v2"}
	keys, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	// Get signers
	signer0 := keys.Signers["v0"]
	signer1 := keys.Signers["v1"]
	signer2 := keys.Signers["v2"]
	require.NotNil(t, signer0)
	require.NotNil(t, signer1)
	require.NotNil(t, signer2)

	// Run 2-round Ringtail protocol
	sessionID := 1
	prfKey := []byte("prf-key-for-epoch-signing-test!")
	signerIDs := []int{0, 1, 2}
	message := "epoch 0 block hash"

	// Round 1
	round1Data := make(map[int]*ringtailThreshold.Round1Data)
	round1Data[0] = signer0.Round1(sessionID, prfKey, signerIDs)
	round1Data[1] = signer1.Round1(sessionID, prfKey, signerIDs)
	round1Data[2] = signer2.Round1(sessionID, prfKey, signerIDs)
	t.Log("Round 1 complete: D matrices computed")

	// Round 2
	round2Data := make(map[int]*ringtailThreshold.Round2Data)
	r2_0, err := signer0.Round2(sessionID, message, prfKey, signerIDs, round1Data)
	require.NoError(t, err)
	round2Data[0] = r2_0

	r2_1, err := signer1.Round2(sessionID, message, prfKey, signerIDs, round1Data)
	require.NoError(t, err)
	round2Data[1] = r2_1

	r2_2, err := signer2.Round2(sessionID, message, prfKey, signerIDs, round1Data)
	require.NoError(t, err)
	round2Data[2] = r2_2
	t.Log("Round 2 complete: z shares computed")

	// Finalize
	sig, err := signer0.Finalize(round2Data)
	require.NoError(t, err)
	require.NotNil(t, sig)
	t.Logf("Signature finalized: Z=%d, Delta=%d", len(sig.Z), len(sig.Delta))

	// Verify using epoch manager
	valid := em.VerifySignatureForEpoch(message, sig, 0)
	require.True(t, valid)
	t.Log("Signature verified for epoch 0")
}

func TestEpochManager_CrossEpochVerification(t *testing.T) {
	em := NewEpochManager(2, 3)

	validators := []string{"v0", "v1", "v2"}
	keys0, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	// Sign with epoch 0 keys
	sessionID := 1
	prfKey := []byte("prf-key-for-epoch-signing-test!")
	signerIDs := []int{0, 1, 2}
	message := "message signed in epoch 0"

	signer0 := keys0.Signers["v0"]
	signer1 := keys0.Signers["v1"]
	signer2 := keys0.Signers["v2"]

	// Complete 2-round protocol
	round1Data := make(map[int]*ringtailThreshold.Round1Data)
	round1Data[0] = signer0.Round1(sessionID, prfKey, signerIDs)
	round1Data[1] = signer1.Round1(sessionID, prfKey, signerIDs)
	round1Data[2] = signer2.Round1(sessionID, prfKey, signerIDs)

	round2Data := make(map[int]*ringtailThreshold.Round2Data)
	r2_0, _ := signer0.Round2(sessionID, message, prfKey, signerIDs, round1Data)
	r2_1, _ := signer1.Round2(sessionID, message, prfKey, signerIDs, round1Data)
	r2_2, _ := signer2.Round2(sessionID, message, prfKey, signerIDs, round1Data)
	round2Data[0] = r2_0
	round2Data[1] = r2_1
	round2Data[2] = r2_2

	sig0, err := signer0.Finalize(round2Data)
	require.NoError(t, err)

	// Rotate to epoch 1
	em.mu.Lock()
	em.lastKeygenTime = time.Now().Add(-2 * time.Hour)
	em.mu.Unlock()

	newValidators := []string{"v0", "v1", "v3"} // v2 replaced
	_, err = em.RotateEpoch(newValidators, false)
	require.NoError(t, err)

	require.Equal(t, uint64(1), em.GetCurrentEpoch())

	// Signature from epoch 0 should still verify against epoch 0 keys
	valid := em.VerifySignatureForEpoch(message, sig0, 0)
	require.True(t, valid, "Epoch 0 signature should verify with epoch 0 keys")

	// Should NOT verify against epoch 1 keys (different group key)
	valid = em.VerifySignatureForEpoch(message, sig0, 1)
	require.False(t, valid, "Epoch 0 signature should NOT verify with epoch 1 keys")

	t.Log("Cross-epoch verification working correctly")
}

func TestEpochManager_PruneHistory(t *testing.T) {
	em := NewEpochManager(2, 2) // Keep only 2 epochs in history

	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	// Create several epochs
	for i := 1; i <= 5; i++ {
		em.mu.Lock()
		em.lastKeygenTime = time.Now().Add(-2 * time.Hour)
		em.mu.Unlock()

		newValidators := []string{"v0", "v1", "v2"}
		_, err := em.RotateEpoch(newValidators, true)
		require.NoError(t, err)
	}

	require.Equal(t, uint64(5), em.GetCurrentEpoch())

	// Only recent epochs should be in history
	stats := em.Stats()
	require.LessOrEqual(t, stats.HistorySize, 3) // historyLimit + 1 for current

	// Epoch 0 should be pruned
	_, err = em.GetEpochKeys(0)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrEpochNotFound)

	// Recent epochs should still exist
	_, err = em.GetEpochKeys(5)
	require.NoError(t, err)

	t.Logf("History pruned: %d epochs retained", stats.HistorySize)
}

func TestEpochManager_Stats(t *testing.T) {
	em := NewEpochManager(2, 3)

	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	stats := em.Stats()

	require.Equal(t, uint64(0), stats.CurrentEpoch)
	require.Equal(t, 3, stats.ValidatorCount)
	require.Equal(t, 2, stats.Threshold)
	require.Equal(t, 1, stats.HistorySize)
	require.NotZero(t, stats.LastKeygenTime)
	require.Greater(t, stats.TimeUntilRotation, time.Duration(0))

	t.Logf("Stats: epoch=%d, validators=%d, threshold=%d, next rotation in %v",
		stats.CurrentEpoch, stats.ValidatorCount, stats.Threshold,
		stats.TimeUntilRotation.Round(time.Second))
}

func TestEpochManager_TimeUntilNextRotation(t *testing.T) {
	em := NewEpochManager(2, 3)

	// Before initialization, should be 0
	remaining := em.TimeUntilNextRotation()
	require.Equal(t, time.Duration(0), remaining)

	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	// Immediately after keygen, should be ~10 minutes
	remaining = em.TimeUntilNextRotation()
	require.Greater(t, remaining, 9*time.Minute)
	require.LessOrEqual(t, remaining, MinEpochDuration)

	// After waiting, should decrease
	em.mu.Lock()
	em.lastKeygenTime = time.Now().Add(-5 * time.Minute)
	em.mu.Unlock()

	remaining = em.TimeUntilNextRotation()
	require.Less(t, remaining, 6*time.Minute)
	require.Greater(t, remaining, 4*time.Minute)

	// After 10 minutes, should be 0
	em.mu.Lock()
	em.lastKeygenTime = time.Now().Add(-11 * time.Minute)
	em.mu.Unlock()

	remaining = em.TimeUntilNextRotation()
	require.Equal(t, time.Duration(0), remaining)
}

func TestEpochManager_Constants(t *testing.T) {
	// Verify constants are sensible
	require.Equal(t, 10*time.Minute, MinEpochDuration, "Min epoch duration should be 10 minutes")
	require.Equal(t, time.Hour, MaxEpochDuration, "Max epoch duration should be 1 hour")
	require.Equal(t, 6, DefaultHistoryLimit, "Default history should be 6 epochs (1 hour)")

	t.Logf("Key rotation rate: min=%v, max=%v, history=%d epochs", MinEpochDuration, MaxEpochDuration, DefaultHistoryLimit)
}

// ============================================================================
// Integrated Quasar + Epoch Manager Tests
// ============================================================================

func TestQuasar_InitializeValidators(t *testing.T) {
	q, err := NewQuasar(2)
	require.NoError(t, err)

	validators := []string{"v0", "v1", "v2"}
	err = q.InitializeValidators(validators)
	require.NoError(t, err)

	// Verify BLS validators were added
	require.Equal(t, 3, q.GetActiveValidatorCount())

	// Verify Ringtail epoch was initialized
	require.Equal(t, uint64(0), q.GetCurrentEpoch())

	stats := q.GetEpochStats()
	require.Equal(t, uint64(0), stats.CurrentEpoch)
	require.Equal(t, 3, stats.ValidatorCount)
	require.Equal(t, 2, stats.Threshold)

	t.Logf("Initialized %d validators with BLS + Ringtail", stats.ValidatorCount)
}

func TestQuasar_AddValidator_WithRotation(t *testing.T) {
	q, err := NewQuasar(2)
	require.NoError(t, err)

	validators := []string{"v0", "v1", "v2"}
	err = q.InitializeValidators(validators)
	require.NoError(t, err)

	// Bypass rate limit for testing
	q.epochManager.mu.Lock()
	q.epochManager.lastKeygenTime = time.Now().Add(-2 * time.Hour)
	q.epochManager.mu.Unlock()

	// Add new validator
	rotated, err := q.AddValidator("v3", 1)
	require.NoError(t, err)
	require.True(t, rotated, "Should have rotated RT keys")

	require.Equal(t, uint64(1), q.GetCurrentEpoch())
	require.Equal(t, 4, q.GetActiveValidatorCount())

	t.Log("Added v3 and rotated to epoch 1")
}

func TestQuasar_AddValidator_RateLimited(t *testing.T) {
	q, err := NewQuasar(2)
	require.NoError(t, err)

	validators := []string{"v0", "v1", "v2"}
	err = q.InitializeValidators(validators)
	require.NoError(t, err)

	// Don't bypass rate limit - rotation should be blocked
	rotated, err := q.AddValidator("v3", 1)
	require.NoError(t, err)
	require.False(t, rotated, "Should NOT have rotated RT keys (rate limited)")

	// BLS validator is still added
	require.Equal(t, 4, q.GetActiveValidatorCount())
	// But epoch stays at 0
	require.Equal(t, uint64(0), q.GetCurrentEpoch())

	t.Log("Added v3 to BLS only (RT rotation rate-limited)")
}

func TestQuasar_RemoveValidator_WithRotation(t *testing.T) {
	q, err := NewQuasar(2)
	require.NoError(t, err)

	validators := []string{"v0", "v1", "v2", "v3"}
	err = q.InitializeValidators(validators)
	require.NoError(t, err)

	// Bypass rate limit
	q.epochManager.mu.Lock()
	q.epochManager.lastKeygenTime = time.Now().Add(-2 * time.Hour)
	q.epochManager.mu.Unlock()

	// Remove validator
	rotated, err := q.RemoveValidator("v3")
	require.NoError(t, err)
	require.True(t, rotated, "Should have rotated RT keys")

	require.Equal(t, uint64(1), q.GetCurrentEpoch())
	require.Equal(t, 3, q.GetActiveValidatorCount())

	t.Log("Removed v3 and rotated to epoch 1")
}

func TestQuasar_UpdateValidatorSet(t *testing.T) {
	q, err := NewQuasar(2)
	require.NoError(t, err)

	validators := []string{"v0", "v1", "v2"}
	err = q.InitializeValidators(validators)
	require.NoError(t, err)

	// Bypass rate limit
	q.epochManager.mu.Lock()
	q.epochManager.lastKeygenTime = time.Now().Add(-2 * time.Hour)
	q.epochManager.mu.Unlock()

	// Update entire validator set (v2 replaced by v3, v4 added)
	newValidators := []string{"v0", "v1", "v3", "v4"}
	rotated, err := q.UpdateValidatorSet(newValidators)
	require.NoError(t, err)
	require.True(t, rotated)

	require.Equal(t, uint64(1), q.GetCurrentEpoch())
	require.Equal(t, 4, q.GetActiveValidatorCount())

	// v2 should be deactivated (proven by active count being 4 for v0,v1,v3,v4)
	// If v2 were still active, count would be 5

	t.Log("Updated validator set: v2->inactive, added v3, v4")
}

func TestQuasar_ValidatorSetSync_BLSAndRingtail(t *testing.T) {
	q, err := NewQuasar(2)
	require.NoError(t, err)

	validators := []string{"v0", "v1", "v2"}
	err = q.InitializeValidators(validators)
	require.NoError(t, err)

	// Verify both BLS and Ringtail have same validator count
	blsCount := q.GetActiveValidatorCount()
	epochKeys := q.epochManager.GetCurrentKeys()
	rtCount := len(epochKeys.ValidatorSet)

	require.Equal(t, blsCount, rtCount, "BLS and Ringtail should have same validator count")
	require.Equal(t, 3, blsCount)

	// Verify threshold is synchronized
	require.Equal(t, q.GetThreshold(), epochKeys.Threshold)

	t.Logf("BLS validators=%d, RT validators=%d, threshold=%d",
		blsCount, rtCount, epochKeys.Threshold)
}

func TestQuasar_EpochSigningAfterRotation(t *testing.T) {
	q, err := NewQuasar(2)
	require.NoError(t, err)

	validators := []string{"v0", "v1", "v2"}
	err = q.InitializeValidators(validators)
	require.NoError(t, err)

	// Get epoch 0 keys and sign
	keys0 := q.epochManager.GetCurrentKeys()
	require.NotNil(t, keys0)

	// Sign with epoch 0
	sessionID := 1
	prfKey := []byte("prf-key-for-quasar-epoch-test!!!")
	signerIDs := []int{0, 1, 2}
	message := "block signed in epoch 0"

	// 2-round protocol
	round1Data := make(map[int]*ringtailThreshold.Round1Data)
	for _, vid := range validators {
		signer := keys0.Signers[vid]
		round1Data[keys0.Shares[vid].Index] = signer.Round1(sessionID, prfKey, signerIDs)
	}

	round2Data := make(map[int]*ringtailThreshold.Round2Data)
	for _, vid := range validators {
		signer := keys0.Signers[vid]
		r2, err := signer.Round2(sessionID, message, prfKey, signerIDs, round1Data)
		require.NoError(t, err)
		round2Data[r2.PartyID] = r2
	}

	sig0, err := keys0.Signers["v0"].Finalize(round2Data)
	require.NoError(t, err)

	// Verify with epoch 0
	valid := q.epochManager.VerifySignatureForEpoch(message, sig0, 0)
	require.True(t, valid, "Epoch 0 signature should verify")

	// Rotate to epoch 1
	q.epochManager.mu.Lock()
	q.epochManager.lastKeygenTime = time.Now().Add(-2 * time.Hour)
	q.epochManager.mu.Unlock()

	_, err = q.UpdateValidatorSet([]string{"v0", "v1", "v3"}) // v2 -> v3
	require.NoError(t, err)
	require.Equal(t, uint64(1), q.GetCurrentEpoch())

	// Epoch 0 signature should still verify with epoch 0 keys
	valid = q.epochManager.VerifySignatureForEpoch(message, sig0, 0)
	require.True(t, valid, "Epoch 0 signature should still verify after rotation")

	// But not with epoch 1 keys
	valid = q.epochManager.VerifySignatureForEpoch(message, sig0, 1)
	require.False(t, valid, "Epoch 0 signature should NOT verify with epoch 1 keys")

	t.Log("Signing and verification across epochs working correctly")
}

// ============================================================================
// Quantum Checkpoint Tests - 3-second quantum-safe anchors
// ============================================================================

func TestQuantumBundle_Constants(t *testing.T) {
	require.Equal(t, 3*time.Second, QuantumCheckpointInterval, "Quantum checkpoint interval should be 3 seconds")
	t.Logf("Quantum bundles every %v", QuantumCheckpointInterval)
}

func TestQuantumBundle_Create(t *testing.T) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	bs := NewBundleSigner(em)

	// Add BLS blocks and create first bundle
	for i := 0; i < 6; i++ {
		bs.AddBLSBlock(uint64(100+i), [32]byte{byte(i + 1)})
	}
	qb := bs.CreateBundle()

	require.Equal(t, uint64(0), qb.Epoch)
	require.Equal(t, uint64(0), qb.Sequence)
	require.Equal(t, uint64(100), qb.StartHeight)
	require.Equal(t, uint64(105), qb.EndHeight)
	require.Equal(t, 6, qb.BlockCount)
	require.Equal(t, [32]byte{}, qb.PreviousHash) // First bundle has no previous

	// Create second bundle
	for i := 0; i < 4; i++ {
		bs.AddBLSBlock(uint64(106+i), [32]byte{byte(10 + i)})
	}
	qb2 := bs.CreateBundle()

	require.Equal(t, uint64(1), qb2.Sequence)
	require.Equal(t, qb.Hash(), qb2.PreviousHash) // Chained

	t.Log("Bundle creation and chaining working correctly")
}

func TestQuantumBundle_SignAndVerify(t *testing.T) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	bs := NewBundleSigner(em)

	// Add BLS blocks and create bundle
	for i := 0; i < 6; i++ {
		bs.AddBLSBlock(uint64(i), [32]byte{byte(i + 1)})
	}
	qb := bs.CreateBundle()

	// Sign with all validators
	sessionID := 1
	prfKey := []byte("prf-key-for-quantum-bundle-test!")
	start := time.Now()
	err = bs.SignBundle(qb, sessionID, prfKey, validators)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, qb.Signature)

	t.Logf("Bundle signing with 3 validators: %v", elapsed)

	// Verify
	valid := bs.VerifyBundle(qb)
	require.True(t, valid)

	t.Log("Bundle signed and verified successfully")
}

func TestQuantumBundle_SignWithThreshold(t *testing.T) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	bs := NewBundleSigner(em)

	// Add BLS blocks and create bundle
	for i := 0; i < 6; i++ {
		bs.AddBLSBlock(uint64(200+i), [32]byte{byte(i)})
	}
	qb := bs.CreateBundle()

	// Sign with only 2 validators (threshold)
	sessionID := 2
	prfKey := []byte("prf-key-threshold-signing-test!!")
	err = bs.SignBundle(qb, sessionID, prfKey, []string{"v0", "v1"})

	require.NoError(t, err)
	require.NotNil(t, qb.Signature)

	// Verify
	valid := bs.VerifyBundle(qb)
	require.True(t, valid)

	t.Log("Threshold signing (2-of-3) working correctly")
}

func TestQuantumBundle_InsufficientSigners(t *testing.T) {
	em := NewEpochManager(2, 3) // threshold = 2
	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	bs := NewBundleSigner(em)

	// Add BLS blocks and create bundle
	for i := 0; i < 3; i++ {
		bs.AddBLSBlock(uint64(300+i), [32]byte{byte(i)})
	}
	qb := bs.CreateBundle()

	// Try to sign with only 1 validator (below threshold)
	sessionID := 3
	prfKey := []byte("prf-key-insufficient-signers!!!!")
	err = bs.SignBundle(qb, sessionID, prfKey, []string{"v0"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient signers")

	t.Log("Correctly rejected signing with insufficient signers")
}

func TestQuantumBundle_ChainIntegrity(t *testing.T) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	bs := NewBundleSigner(em)
	prfKey := []byte("prf-key-for-chain-integrity!!!!!")

	// Create chain of 5 bundles
	var bundles []*QuantumBundle
	for b := 0; b < 5; b++ {
		for i := 0; i < 6; i++ {
			bs.AddBLSBlock(uint64(b*6+i), [32]byte{byte(b*6 + i + 1)})
		}
		qb := bs.CreateBundle()

		err := bs.SignBundle(qb, b+1, prfKey, validators)
		require.NoError(t, err)

		bundles = append(bundles, qb)
	}

	// Verify chain linkage
	for i := 1; i < len(bundles); i++ {
		require.Equal(t, bundles[i-1].Hash(), bundles[i].PreviousHash,
			"Bundle %d should reference bundle %d", i, i-1)
	}

	// Verify all bundles
	for i, qb := range bundles {
		valid := bs.VerifyBundle(qb)
		require.True(t, valid, "Bundle %d should verify", i)
	}

	t.Logf("Chain of %d bundles created and verified", len(bundles))
}

func TestQuantumBundle_EpochSequenceReset(t *testing.T) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	_, err := em.InitializeEpoch(validators)
	require.NoError(t, err)

	bs := NewBundleSigner(em)

	// Create bundles in epoch 0
	for i := 0; i < 6; i++ {
		bs.AddBLSBlock(uint64(i), [32]byte{byte(i)})
	}
	qb1 := bs.CreateBundle()

	for i := 0; i < 6; i++ {
		bs.AddBLSBlock(uint64(6+i), [32]byte{byte(6 + i)})
	}
	qb2 := bs.CreateBundle()

	require.Equal(t, uint64(0), qb1.Sequence)
	require.Equal(t, uint64(1), qb2.Sequence)

	// Rotate to epoch 1
	em.mu.Lock()
	em.lastKeygenTime = time.Now().Add(-2 * time.Hour)
	em.mu.Unlock()
	_, err = em.RotateEpoch(validators, true)
	require.NoError(t, err)

	// Create bundle in new epoch - sequence should reset
	for i := 0; i < 6; i++ {
		bs.AddBLSBlock(uint64(12+i), [32]byte{byte(12 + i)})
	}
	qb3 := bs.CreateBundle()

	require.Equal(t, uint64(1), qb3.Epoch)
	require.Equal(t, uint64(0), qb3.Sequence) // Reset on new epoch

	t.Log("Sequence resets correctly on epoch rotation")
}

func BenchmarkQuantumBundle_3Validators(b *testing.B) {
	em := NewEpochManager(2, 3)
	validators := []string{"v0", "v1", "v2"}
	em.InitializeEpoch(validators)

	bs := NewBundleSigner(em)
	prfKey := []byte("prf-key-for-benchmark-signing!!!")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 6; j++ {
			bs.AddBLSBlock(uint64(i*6+j), [32]byte{byte(i*6 + j)})
		}
		qb := bs.CreateBundle()
		bs.SignBundle(qb, i, prfKey, validators)
	}
}

func BenchmarkQuantumBundle_21Validators(b *testing.B) {
	em := NewEpochManager(14, 6)
	validators := make([]string, 21)
	for i := 0; i < 21; i++ {
		validators[i] = fmt.Sprintf("v%d", i)
	}
	em.InitializeEpoch(validators)

	bs := NewBundleSigner(em)
	prfKey := []byte("prf-key-for-21-validator-bench!!")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 6; j++ {
			bs.AddBLSBlock(uint64(i*6+j), [32]byte{byte(i*6 + j)})
		}
		qb := bs.CreateBundle()
		bs.SignBundle(qb, i, prfKey, validators)
	}
}
