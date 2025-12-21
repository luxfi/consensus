// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// Test BLS threshold signing for quantum-safe consensus.
// Note: Full Ringtail integration requires multi-round protocol coordination.

package quasar

import (
	"context"
	"testing"

	"github.com/luxfi/crypto/threshold"
	ringtailThreshold "github.com/luxfi/ringtail/threshold"
	"github.com/stretchr/testify/require"
)

// TestBLSThresholdSigningFlow tests BLS threshold signing via the Hybrid engine.
func TestBLSThresholdSigningFlow(t *testing.T) {
	// Generate BLS threshold keys for 2-of-3 threshold
	shares, groupKey, err := GenerateThresholdKeys(threshold.SchemeBLS, 2, 3)
	require.NoError(t, err, "Failed to generate BLS threshold keys")

	require.Len(t, shares, 3, "Should have 3 key shares")
	require.NotNil(t, groupKey, "Group key should not be nil")

	t.Logf("Generated BLS threshold keys: t=2, n=3")
	t.Logf("Group key size: %d bytes", len(groupKey.Bytes()))

	// Create key shares map
	keyShares := make(map[string]threshold.KeyShare)
	keyShares["v0"] = shares[0]
	keyShares["v1"] = shares[1]
	keyShares["v2"] = shares[2]

	// Create hybrid engine with BLS threshold signing
	config := ThresholdConfig{
		SchemeID:     threshold.SchemeBLS,
		Threshold:    2,
		TotalParties: 3,
		KeyShares:    keyShares,
		GroupKey:     groupKey,
	}

	h, err := NewHybridWithThresholdConfig(config)
	require.NoError(t, err, "Failed to create hybrid engine")

	// Verify threshold mode is enabled
	require.True(t, h.IsThresholdMode(), "Should be in threshold mode")

	// Sign a message with 2 validators (meeting 2-of-3 threshold)
	ctx := context.Background()
	message := []byte("consensus block for quantum security")

	// Get signature shares from validators
	share0, err := h.SignMessageThreshold(ctx, "v0", message)
	require.NoError(t, err, "Validator 0 signing failed")
	require.NotNil(t, share0, "Signature share should not be nil")

	share1, err := h.SignMessageThreshold(ctx, "v1", message)
	require.NoError(t, err, "Validator 1 signing failed")

	t.Logf("Share 0: %d bytes", len(share0.Bytes()))
	t.Logf("Share 1: %d bytes", len(share1.Bytes()))

	// Aggregate signatures (need t+1=3 shares for 2-of-3, but API expects >threshold)
	// Getting a third share
	share2, err := h.SignMessageThreshold(ctx, "v2", message)
	require.NoError(t, err, "Validator 2 signing failed")

	aggSig, err := h.AggregateThresholdSignatures(ctx, message, []threshold.SignatureShare{share0, share1, share2})
	require.NoError(t, err, "Signature aggregation failed")
	require.NotNil(t, aggSig, "Aggregated signature should not be nil")

	t.Logf("Aggregated signature: %d bytes", len(aggSig.Bytes()))

	// Verify the aggregated signature
	valid := h.VerifyThresholdSignature(message, aggSig)
	require.True(t, valid, "Aggregated signature should verify")

	// Also verify via bytes
	validBytes := h.VerifyThresholdSignatureBytes(message, aggSig.Bytes())
	require.True(t, validBytes, "Serialized signature should verify")

	t.Log("✓ BLS threshold signing flow complete")
}

// TestBLSThresholdInsufficientShares tests that aggregation fails without enough shares.
func TestBLSThresholdInsufficientShares(t *testing.T) {
	shares, groupKey, err := GenerateThresholdKeys(threshold.SchemeBLS, 2, 3)
	require.NoError(t, err)

	keyShares := make(map[string]threshold.KeyShare)
	keyShares["v0"] = shares[0]
	keyShares["v1"] = shares[1]
	keyShares["v2"] = shares[2]

	h, err := NewHybridWithThresholdConfig(ThresholdConfig{
		SchemeID:     threshold.SchemeBLS,
		Threshold:    2,
		TotalParties: 3,
		KeyShares:    keyShares,
		GroupKey:     groupKey,
	})
	require.NoError(t, err)

	ctx := context.Background()
	message := []byte("need more signatures")

	// Sign with only 2 validators (need 3 for t+1 with threshold=2)
	share0, _ := h.SignMessageThreshold(ctx, "v0", message)
	share1, _ := h.SignMessageThreshold(ctx, "v1", message)

	// Aggregation should fail with insufficient signatures
	_, err = h.AggregateThresholdSignatures(ctx, message, []threshold.SignatureShare{share0, share1})
	require.Error(t, err, "Should fail with insufficient signatures")
}

// TestBLSThresholdWrongMessage tests that verification fails for wrong message.
func TestBLSThresholdWrongMessage(t *testing.T) {
	shares, groupKey, err := GenerateThresholdKeys(threshold.SchemeBLS, 2, 3)
	require.NoError(t, err)

	keyShares := make(map[string]threshold.KeyShare)
	keyShares["v0"] = shares[0]
	keyShares["v1"] = shares[1]
	keyShares["v2"] = shares[2]

	h, err := NewHybridWithThresholdConfig(ThresholdConfig{
		SchemeID:     threshold.SchemeBLS,
		Threshold:    2,
		TotalParties: 3,
		KeyShares:    keyShares,
		GroupKey:     groupKey,
	})
	require.NoError(t, err)

	ctx := context.Background()
	message := []byte("original message")
	wrongMessage := []byte("tampered message")

	share0, _ := h.SignMessageThreshold(ctx, "v0", message)
	share1, _ := h.SignMessageThreshold(ctx, "v1", message)
	share2, _ := h.SignMessageThreshold(ctx, "v2", message)

	aggSig, err := h.AggregateThresholdSignatures(ctx, message, []threshold.SignatureShare{share0, share1, share2})
	require.NoError(t, err)

	// Verify with wrong message should fail
	valid := h.VerifyThresholdSignature(wrongMessage, aggSig)
	require.False(t, valid, "Verification should fail for wrong message")
}

// TestDualThresholdKeyGeneration tests that dual key generation works.
func TestDualThresholdKeyGeneration(t *testing.T) {
	config, err := GenerateDualThresholdKeys(2, 3)
	require.NoError(t, err, "Failed to generate dual threshold keys")

	// Verify both BLS and Ringtail keys were generated
	require.NotNil(t, config.BLSGroupKey, "BLS group key should not be nil")
	require.NotNil(t, config.RingtailGroupKey, "Ringtail group key should not be nil")
	require.Len(t, config.BLSKeyShares, 3, "Should have 3 BLS key shares")
	require.Len(t, config.RingtailShares, 3, "Should have 3 Ringtail key shares")

	t.Logf("BLS group key: %d bytes", len(config.BLSGroupKey.Bytes()))
	t.Logf("Ringtail group key: %d bytes", len(config.RingtailGroupKey.Bytes()))

	t.Log("✓ Dual threshold key generation works")
}

// TestDualSigningFlow tests the full BLS + Ringtail parallel signing flow.
// This is how validators sign blocks in quantum-safe consensus.
func TestDualSigningFlow(t *testing.T) {
	// Generate dual keys (would be done at epoch start in production)
	config, err := GenerateDualKeys(2, 3)
	require.NoError(t, err, "Failed to generate dual keys")

	// Create hybrid engine with dual threshold signing
	h, err := NewHybridWithDualThreshold(*config)
	require.NoError(t, err, "Failed to create hybrid engine")

	require.True(t, h.IsDualThresholdMode(), "Should be in dual threshold mode")

	ctx := context.Background()
	message := []byte("block hash for quantum-safe consensus")
	sessionID := 1
	prfKey := []byte("prf-key-for-signing-session-!!")

	validatorIDs := []string{"v0", "v1", "v2"}

	// === ROUND 1: All validators compute BLS share + Ringtail D+MACs in parallel ===
	t.Log("=== Round 1: BLS signing + Ringtail D matrices ===")

	blsSigs := make([]*HybridSignature, 3)
	allRound1 := make(map[int]*ringtailThreshold.Round1Data)

	for i, vid := range validatorIDs {
		blsSig, rtRound1, err := h.DualSignRound1(ctx, vid, message, sessionID, prfKey)
		require.NoError(t, err, "Round1 failed for %s", vid)
		require.NotNil(t, blsSig, "BLS signature should not be nil")
		require.NotNil(t, rtRound1, "Ringtail Round1 data should not be nil")

		blsSigs[i] = blsSig
		allRound1[rtRound1.PartyID] = rtRound1
		t.Logf("  %s: BLS share=%d bytes, Ringtail D=%dx%d", vid, len(blsSig.BLS), len(rtRound1.D), len(rtRound1.D[0]))
	}

	// === BLS IS DONE: Can aggregate immediately ===
	t.Log("=== BLS Aggregation (single round) ===")
	blsAggSig, err := h.AggregateSignatures(message, blsSigs)
	require.NoError(t, err, "BLS aggregation failed")
	require.NotNil(t, blsAggSig, "BLS aggregated signature should not be nil")
	t.Logf("  BLS aggregated: %d bytes", len(blsAggSig.BLSAggregated))

	// Verify BLS immediately
	blsValid := h.VerifyAggregatedSignature(message, blsAggSig)
	require.True(t, blsValid, "BLS signature verification failed")
	t.Log("  BLS: ✓ verified")

	// === Ringtail uses the native package directly for full 2-round flow ===
	// This demonstrates the protocol but in production would use consensus messages
	t.Log("=== Ringtail 2-Round Protocol (via native package) ===")

	// Use the native ringtail threshold package directly
	rtShares := config.RingtailShares
	rtGroupKey := config.RingtailGroupKey

	signers := make([]*ringtailThreshold.Signer, 3)
	for i := 0; i < 3; i++ {
		signers[i] = ringtailThreshold.NewSigner(rtShares[validatorIDs[i]])
	}

	signerIDs := []int{0, 1, 2}
	messageStr := string(message)

	// Round 1: All parties compute D + MACs
	rtRound1Data := make(map[int]*ringtailThreshold.Round1Data)
	for _, signer := range signers {
		data := signer.Round1(sessionID, prfKey, signerIDs)
		rtRound1Data[data.PartyID] = data
	}
	t.Log("  Round1: D matrices computed")

	// Round 2: All parties compute z shares
	rtRound2Data := make(map[int]*ringtailThreshold.Round2Data)
	for _, signer := range signers {
		data, err := signer.Round2(sessionID, messageStr, prfKey, signerIDs, rtRound1Data)
		require.NoError(t, err, "Ringtail Round2 failed")
		rtRound2Data[data.PartyID] = data
	}
	t.Log("  Round2: z shares computed")

	// Finalize: Any signer aggregates
	rtSig, err := signers[0].Finalize(rtRound2Data)
	require.NoError(t, err, "Ringtail finalization failed")
	require.NotNil(t, rtSig, "Ringtail signature should not be nil")
	t.Logf("  Finalize: Z=%d, Delta=%d", len(rtSig.Z), len(rtSig.Delta))

	// Verify Ringtail
	rtValid := ringtailThreshold.Verify(rtGroupKey, messageStr, rtSig)
	require.True(t, rtValid, "Ringtail signature verification failed")
	t.Log("  Ringtail: ✓ verified")

	t.Log("✓ Dual BLS + Ringtail signing flow complete")
	t.Log("  - BLS: 1 round, aggregated, verified")
	t.Log("  - Ringtail: 2 rounds, aggregated, verified (post-quantum)")
}
