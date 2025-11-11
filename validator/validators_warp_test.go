// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validators_test

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	validators "github.com/luxfi/consensus/validator"
	"github.com/luxfi/consensus/validator/validatorsmock"
)

func TestWarpValidatorTypes(t *testing.T) {
	require := require.New(t)

	nodeID := ids.GenerateTestNodeID()
	pubKey := []byte("bls-public-key")

	// Test WarpValidator
	warpVal := &validators.WarpValidator{
		NodeID:    nodeID,
		PublicKey: pubKey,
		Weight:    100,
	}
	require.Equal(nodeID, warpVal.NodeID)
	require.Equal(pubKey, warpVal.PublicKey)
	require.Equal(uint64(100), warpVal.Weight)

	// Test WarpSet
	warpSet := &validators.WarpSet{
		Height: 1000,
		Validators: map[ids.NodeID]*validators.WarpValidator{
			nodeID: warpVal,
		},
	}
	require.Equal(uint64(1000), warpSet.Height)
	require.Len(warpSet.Validators, 1)
	require.Equal(warpVal, warpSet.Validators[nodeID])
}

func TestGetWarpValidatorSet(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockState := validatorsmock.NewState(ctrl)
	ctx := context.Background()
	netID := ids.GenerateTestID()
	height := uint64(100)

	nodeID1 := ids.GenerateTestNodeID()
	nodeID2 := ids.GenerateTestNodeID()

	expectedSet := &validators.WarpSet{
		Height: height,
		Validators: map[ids.NodeID]*validators.WarpValidator{
			nodeID1: {
				NodeID:    nodeID1,
				PublicKey: []byte("bls-key-1"),
				Weight:    50,
			},
			nodeID2: {
				NodeID:    nodeID2,
				PublicKey: []byte("bls-key-2"),
				Weight:    50,
			},
		},
	}

	mockState.EXPECT().
		GetWarpValidatorSet(ctx, height, netID).
		Return(expectedSet, nil)

	result, err := mockState.GetWarpValidatorSet(ctx, height, netID)
	require.NoError(err)
	require.NotNil(result)
	require.Equal(height, result.Height)
	require.Len(result.Validators, 2)
	require.Equal(expectedSet.Validators[nodeID1], result.Validators[nodeID1])
	require.Equal(expectedSet.Validators[nodeID2], result.Validators[nodeID2])
}

func TestGetWarpValidatorSets(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockState := validatorsmock.NewState(ctrl)
	ctx := context.Background()

	netID1 := ids.GenerateTestID()
	netID2 := ids.GenerateTestID()
	heights := []uint64{100, 200}
	netIDs := []ids.ID{netID1, netID2}

	nodeID1 := ids.GenerateTestNodeID()
	nodeID2 := ids.GenerateTestNodeID()

	expectedSets := map[ids.ID]map[uint64]*validators.WarpSet{
		netID1: {
			100: {
				Height: 100,
				Validators: map[ids.NodeID]*validators.WarpValidator{
					nodeID1: {
						NodeID:    nodeID1,
						PublicKey: []byte("bls-key-1"),
						Weight:    50,
					},
				},
			},
			200: {
				Height: 200,
				Validators: map[ids.NodeID]*validators.WarpValidator{
					nodeID1: {
						NodeID:    nodeID1,
						PublicKey: []byte("bls-key-1"),
						Weight:    60,
					},
				},
			},
		},
		netID2: {
			100: {
				Height: 100,
				Validators: map[ids.NodeID]*validators.WarpValidator{
					nodeID2: {
						NodeID:    nodeID2,
						PublicKey: []byte("bls-key-2"),
						Weight:    40,
					},
				},
			},
			200: {
				Height: 200,
				Validators: map[ids.NodeID]*validators.WarpValidator{
					nodeID2: {
						NodeID:    nodeID2,
						PublicKey: []byte("bls-key-2"),
						Weight:    45,
					},
				},
			},
		},
	}

	mockState.EXPECT().
		GetWarpValidatorSets(ctx, heights, netIDs).
		Return(expectedSets, nil)

	result, err := mockState.GetWarpValidatorSets(ctx, heights, netIDs)
	require.NoError(err)
	require.NotNil(result)
	require.Len(result, 2)

	// Verify netID1 results
	require.Contains(result, netID1)
	require.Len(result[netID1], 2)
	require.Equal(uint64(100), result[netID1][100].Height)
	require.Equal(uint64(200), result[netID1][200].Height)
	require.Equal(uint64(50), result[netID1][100].Validators[nodeID1].Weight)
	require.Equal(uint64(60), result[netID1][200].Validators[nodeID1].Weight)

	// Verify netID2 results
	require.Contains(result, netID2)
	require.Len(result[netID2], 2)
	require.Equal(uint64(100), result[netID2][100].Height)
	require.Equal(uint64(200), result[netID2][200].Height)
	require.Equal(uint64(40), result[netID2][100].Validators[nodeID2].Weight)
	require.Equal(uint64(45), result[netID2][200].Validators[nodeID2].Weight)
}

func TestGetWarpValidatorSet_EmptySet(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockState := validatorsmock.NewState(ctrl)
	ctx := context.Background()
	netID := ids.GenerateTestID()
	height := uint64(100)

	emptySet := &validators.WarpSet{
		Height:     height,
		Validators: map[ids.NodeID]*validators.WarpValidator{},
	}

	mockState.EXPECT().
		GetWarpValidatorSet(ctx, height, netID).
		Return(emptySet, nil)

	result, err := mockState.GetWarpValidatorSet(ctx, height, netID)
	require.NoError(err)
	require.NotNil(result)
	require.Equal(height, result.Height)
	require.Empty(result.Validators)
}

func TestGetWarpValidatorSets_EmptySets(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockState := validatorsmock.NewState(ctrl)
	ctx := context.Background()

	heights := []uint64{}
	netIDs := []ids.ID{}

	emptySets := map[ids.ID]map[uint64]*validators.WarpSet{}

	mockState.EXPECT().
		GetWarpValidatorSets(ctx, heights, netIDs).
		Return(emptySets, nil)

	result, err := mockState.GetWarpValidatorSets(ctx, heights, netIDs)
	require.NoError(err)
	require.NotNil(result)
	require.Empty(result)
}
