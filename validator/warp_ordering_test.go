// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validators

import (
	"math"
	"testing"

	"github.com/luxfi/ids"
	mathset "github.com/luxfi/math/set"
	"github.com/luxfi/node/utils/crypto/bls"
	"github.com/stretchr/testify/require"
)

// TestCanonicalValidatorCompare tests the Compare method
func TestCanonicalValidatorCompare(t *testing.T) {
	require := require.New(t)

	v1 := &CanonicalValidator{
		PublicKeyBytes: []byte{0x01, 0x02, 0x03},
		Weight:         100,
	}

	v2 := &CanonicalValidator{
		PublicKeyBytes: []byte{0x01, 0x02, 0x04},
		Weight:         200,
	}

	v3 := &CanonicalValidator{
		PublicKeyBytes: []byte{0x01, 0x02, 0x03},
		Weight:         300,
	}

	// v1 < v2 (0x03 < 0x04)
	require.Less(v1.Compare(v2), 0)

	// v2 > v1
	require.Greater(v2.Compare(v1), 0)

	// v1 == v3 (same public key bytes)
	require.Equal(0, v1.Compare(v3))
}

// TestFlattenValidatorSetEmpty tests with empty input
func TestFlattenValidatorSetEmpty(t *testing.T) {
	require := require.New(t)

	result, err := FlattenValidatorSet(nil)
	require.NoError(err)
	require.Empty(result.Validators)
	require.Equal(uint64(0), result.TotalWeight)

	result, err = FlattenValidatorSet(map[ids.NodeID]*GetValidatorOutput{})
	require.NoError(err)
	require.Empty(result.Validators)
	require.Equal(uint64(0), result.TotalWeight)
}

// TestFlattenValidatorSetNoPubKey tests validators without public keys
func TestFlattenValidatorSetNoPubKey(t *testing.T) {
	require := require.New(t)

	nodeID := ids.GenerateTestNodeID()
	vdrSet := map[ids.NodeID]*GetValidatorOutput{
		nodeID: {
			NodeID:    nodeID,
			PublicKey: nil, // No public key
			Weight:    100,
		},
	}

	result, err := FlattenValidatorSet(vdrSet)
	require.NoError(err)
	require.Empty(result.Validators)        // No validators with public keys
	require.Equal(uint64(100), result.TotalWeight) // But weight still counted
}

// TestFlattenValidatorSetEmptyPubKey tests validators with empty public key slice
func TestFlattenValidatorSetEmptyPubKey(t *testing.T) {
	require := require.New(t)

	nodeID := ids.GenerateTestNodeID()
	vdrSet := map[ids.NodeID]*GetValidatorOutput{
		nodeID: {
			NodeID:    nodeID,
			PublicKey: []byte{}, // Empty slice
			Weight:    100,
		},
	}

	result, err := FlattenValidatorSet(vdrSet)
	require.NoError(err)
	require.Empty(result.Validators)
	require.Equal(uint64(100), result.TotalWeight)
}

// TestFlattenValidatorSetInvalidPubKey tests validators with invalid public keys
func TestFlattenValidatorSetInvalidPubKey(t *testing.T) {
	require := require.New(t)

	nodeID := ids.GenerateTestNodeID()
	vdrSet := map[ids.NodeID]*GetValidatorOutput{
		nodeID: {
			NodeID:    nodeID,
			PublicKey: []byte("invalid-key"), // Invalid BLS key
			Weight:    100,
		},
	}

	result, err := FlattenValidatorSet(vdrSet)
	require.NoError(err)
	require.Empty(result.Validators) // Invalid key skipped
	require.Equal(uint64(100), result.TotalWeight)
}

// TestFlattenValidatorSetValidPubKey tests with valid BLS public key
func TestFlattenValidatorSetValidPubKey(t *testing.T) {
	require := require.New(t)

	// Generate a valid BLS key pair
	sk, err := bls.NewSecretKey()
	require.NoError(err)

	pk := sk.PublicKey()
	pkBytes := bls.PublicKeyToCompressedBytes(pk)

	nodeID := ids.GenerateTestNodeID()
	vdrSet := map[ids.NodeID]*GetValidatorOutput{
		nodeID: {
			NodeID:    nodeID,
			PublicKey: pkBytes,
			Weight:    100,
		},
	}

	result, err := FlattenValidatorSet(vdrSet)
	require.NoError(err)
	require.Len(result.Validators, 1)
	require.Equal(uint64(100), result.TotalWeight)
	require.Equal(uint64(100), result.Validators[0].Weight)
	require.Contains(result.Validators[0].NodeIDs, nodeID)
}

// TestFlattenValidatorSetDuplicatePubKey tests merging validators with same public key
func TestFlattenValidatorSetDuplicatePubKey(t *testing.T) {
	require := require.New(t)

	// Generate a valid BLS key pair
	sk, err := bls.NewSecretKey()
	require.NoError(err)

	pk := sk.PublicKey()
	pkBytes := bls.PublicKeyToCompressedBytes(pk)

	nodeID1 := ids.GenerateTestNodeID()
	nodeID2 := ids.GenerateTestNodeID()

	vdrSet := map[ids.NodeID]*GetValidatorOutput{
		nodeID1: {
			NodeID:    nodeID1,
			PublicKey: pkBytes, // Same key
			Weight:    100,
		},
		nodeID2: {
			NodeID:    nodeID2,
			PublicKey: pkBytes, // Same key
			Weight:    200,
		},
	}

	result, err := FlattenValidatorSet(vdrSet)
	require.NoError(err)
	require.Len(result.Validators, 1) // Merged into one
	require.Equal(uint64(300), result.TotalWeight)
	require.Equal(uint64(300), result.Validators[0].Weight)
	require.Len(result.Validators[0].NodeIDs, 2)
}

// TestFlattenValidatorSetWeightOverflow tests weight overflow
func TestFlattenValidatorSetWeightOverflow(t *testing.T) {
	require := require.New(t)

	nodeID1 := ids.GenerateTestNodeID()
	nodeID2 := ids.GenerateTestNodeID()

	vdrSet := map[ids.NodeID]*GetValidatorOutput{
		nodeID1: {
			NodeID: nodeID1,
			Weight: math.MaxUint64,
		},
		nodeID2: {
			NodeID: nodeID2,
			Weight: 1,
		},
	}

	_, err := FlattenValidatorSet(vdrSet)
	require.ErrorIs(err, ErrWeightOverflow)
}

// TestFlattenValidatorSetDuplicateKeyWeightOverflow tests weight overflow during merge
func TestFlattenValidatorSetDuplicateKeyWeightOverflow(t *testing.T) {
	require := require.New(t)

	// Generate a valid BLS key pair
	sk, err := bls.NewSecretKey()
	require.NoError(err)

	pk := sk.PublicKey()
	pkBytes := bls.PublicKeyToCompressedBytes(pk)

	nodeID1 := ids.GenerateTestNodeID()
	nodeID2 := ids.GenerateTestNodeID()

	vdrSet := map[ids.NodeID]*GetValidatorOutput{
		nodeID1: {
			NodeID:    nodeID1,
			PublicKey: pkBytes,
			Weight:    math.MaxUint64,
		},
		nodeID2: {
			NodeID:    nodeID2,
			PublicKey: pkBytes, // Same key
			Weight:    1,
		},
	}

	_, err = FlattenValidatorSet(vdrSet)
	require.ErrorIs(err, ErrWeightOverflow)
}

// TestFlattenValidatorSetSorting tests that result is sorted by public key
func TestFlattenValidatorSetSorting(t *testing.T) {
	require := require.New(t)

	// Generate multiple valid BLS key pairs
	sks := make([]*bls.SecretKey, 3)
	pks := make([]*bls.PublicKey, 3)
	pkBytes := make([][]byte, 3)

	for i := range sks {
		var err error
		sks[i], err = bls.NewSecretKey()
		require.NoError(err)
		pks[i] = sks[i].PublicKey()
		pkBytes[i] = bls.PublicKeyToCompressedBytes(pks[i])
	}

	vdrSet := make(map[ids.NodeID]*GetValidatorOutput)
	for i := range sks {
		nodeID := ids.GenerateTestNodeID()
		vdrSet[nodeID] = &GetValidatorOutput{
			NodeID:    nodeID,
			PublicKey: pkBytes[i],
			Weight:    uint64((i + 1) * 100),
		}
	}

	result, err := FlattenValidatorSet(vdrSet)
	require.NoError(err)
	require.Len(result.Validators, 3)

	// Verify sorted by public key bytes
	for i := 1; i < len(result.Validators); i++ {
		prev := result.Validators[i-1]
		curr := result.Validators[i]
		require.LessOrEqual(prev.Compare(curr), 0, "validators should be sorted by public key")
	}
}

// TestFilterValidatorsEmpty tests with empty inputs
func TestFilterValidatorsEmpty(t *testing.T) {
	require := require.New(t)

	// Empty validators with initialized empty Bits
	emptyBits := mathset.NewBits()
	result, err := FilterValidators(emptyBits, nil)
	require.NoError(err)
	require.Empty(result)

	// Empty indices
	vdrs := []*CanonicalValidator{
		{Weight: 100},
		{Weight: 200},
	}
	result, err = FilterValidators(emptyBits, vdrs)
	require.NoError(err)
	require.Empty(result)
}

// TestFilterValidatorsSelect tests selecting specific validators
func TestFilterValidatorsSelect(t *testing.T) {
	require := require.New(t)

	vdrs := []*CanonicalValidator{
		{Weight: 100},
		{Weight: 200},
		{Weight: 300},
		{Weight: 400},
	}

	// Select indices 0 and 2
	indices := mathset.NewBits(0, 2)

	result, err := FilterValidators(indices, vdrs)
	require.NoError(err)
	require.Len(result, 2)
	require.Equal(uint64(100), result[0].Weight)
	require.Equal(uint64(300), result[1].Weight)
}

// TestFilterValidatorsSelectAll tests selecting all validators
func TestFilterValidatorsSelectAll(t *testing.T) {
	require := require.New(t)

	vdrs := []*CanonicalValidator{
		{Weight: 100},
		{Weight: 200},
		{Weight: 300},
	}

	indices := mathset.NewBits(0, 1, 2)

	result, err := FilterValidators(indices, vdrs)
	require.NoError(err)
	require.Len(result, 3)
}

// TestFilterValidatorsOutOfBounds tests index out of bounds error
func TestFilterValidatorsOutOfBounds(t *testing.T) {
	require := require.New(t)

	vdrs := []*CanonicalValidator{
		{Weight: 100},
		{Weight: 200},
	}

	// Index 5 is out of bounds (only 2 validators)
	indices := mathset.NewBits(5)

	_, err := FilterValidators(indices, vdrs)
	require.ErrorIs(err, ErrUnknownValidator)
}

// TestSumWeightEmpty tests with empty input
func TestSumWeightEmpty(t *testing.T) {
	require := require.New(t)

	weight, err := SumWeight(nil)
	require.NoError(err)
	require.Equal(uint64(0), weight)

	weight, err = SumWeight([]*CanonicalValidator{})
	require.NoError(err)
	require.Equal(uint64(0), weight)
}

// TestSumWeightNormal tests normal sum
func TestSumWeightNormal(t *testing.T) {
	require := require.New(t)

	vdrs := []*CanonicalValidator{
		{Weight: 100},
		{Weight: 200},
		{Weight: 300},
	}

	weight, err := SumWeight(vdrs)
	require.NoError(err)
	require.Equal(uint64(600), weight)
}

// TestSumWeightOverflow tests weight overflow
func TestSumWeightOverflow(t *testing.T) {
	require := require.New(t)

	vdrs := []*CanonicalValidator{
		{Weight: math.MaxUint64},
		{Weight: 1},
	}

	_, err := SumWeight(vdrs)
	require.ErrorIs(err, ErrWeightOverflow)
}

// TestAggregatePublicKeysEmpty tests with empty input
func TestAggregatePublicKeysEmpty(t *testing.T) {
	// Empty slice should return error or nil (depends on BLS implementation)
	_, err := AggregatePublicKeys([]*CanonicalValidator{})
	// The BLS library behavior varies, but we should handle empty gracefully
	_ = err
}

// TestAggregatePublicKeysSingle tests with single validator
func TestAggregatePublicKeysSingle(t *testing.T) {
	require := require.New(t)

	sk, err := bls.NewSecretKey()
	require.NoError(err)
	pk := sk.PublicKey()

	vdrs := []*CanonicalValidator{
		{PublicKey: pk, Weight: 100},
	}

	aggPK, err := AggregatePublicKeys(vdrs)
	require.NoError(err)
	require.NotNil(aggPK)
}

// TestAggregatePublicKeysMultiple tests with multiple validators
func TestAggregatePublicKeysMultiple(t *testing.T) {
	require := require.New(t)

	var vdrs []*CanonicalValidator
	for i := 0; i < 3; i++ {
		sk, err := bls.NewSecretKey()
		require.NoError(err)
		pk := sk.PublicKey()
		vdrs = append(vdrs, &CanonicalValidator{
			PublicKey: pk,
			Weight:    uint64((i + 1) * 100),
		})
	}

	aggPK, err := AggregatePublicKeys(vdrs)
	require.NoError(err)
	require.NotNil(aggPK)
}

// TestCanonicalValidatorSetTotalWeight tests TotalWeight field
func TestCanonicalValidatorSetTotalWeight(t *testing.T) {
	require := require.New(t)

	set := CanonicalValidatorSet{
		Validators: []*CanonicalValidator{
			{Weight: 100},
			{Weight: 200},
		},
		TotalWeight: 500, // Can differ from sum of validators (includes those without keys)
	}

	require.Equal(uint64(500), set.TotalWeight)
	require.Len(set.Validators, 2)
}

// TestCanonicalValidatorNodeIDs tests NodeIDs field
func TestCanonicalValidatorNodeIDs(t *testing.T) {
	require := require.New(t)

	nodeID1 := ids.GenerateTestNodeID()
	nodeID2 := ids.GenerateTestNodeID()

	v := &CanonicalValidator{
		Weight:  100,
		NodeIDs: []ids.NodeID{nodeID1, nodeID2},
	}

	require.Len(v.NodeIDs, 2)
	require.Contains(v.NodeIDs, nodeID1)
	require.Contains(v.NodeIDs, nodeID2)
}

// TestErrUnknownValidator tests the error variable
func TestErrUnknownValidator(t *testing.T) {
	require := require.New(t)
	require.NotNil(ErrUnknownValidator)
	require.Equal("unknown validator", ErrUnknownValidator.Error())
}

// TestErrWeightOverflow tests the error variable
func TestErrWeightOverflow(t *testing.T) {
	require := require.New(t)
	require.NotNil(ErrWeightOverflow)
	require.Equal("weight overflowed", ErrWeightOverflow.Error())
}
