// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validatorstest

import (
	"context"
	"errors"
	"testing"

	validators "github.com/luxfi/consensus/validator"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// TestNewTestState tests the constructor
func TestNewTestState(t *testing.T) {
	require := require.New(t)

	state := NewTestState()
	require.NotNil(state)
	require.NotNil(state.validators)
}

// TestTestStateGetCurrentValidators tests GetCurrentValidators
func TestTestStateGetCurrentValidators(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	state := NewTestState()
	netID := ids.GenerateTestID()

	// Default returns empty map
	result, err := state.GetCurrentValidators(ctx, 100, netID)
	require.NoError(err)
	require.Empty(result)

	// With custom function
	nodeID := ids.GenerateTestNodeID()
	expectedValidators := map[ids.NodeID]*validators.GetValidatorOutput{
		nodeID: {
			NodeID: nodeID,
			Weight: 100,
		},
	}

	state.GetValidatorSetF = func(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
		return expectedValidators, nil
	}

	result, err = state.GetCurrentValidators(ctx, 100, netID)
	require.NoError(err)
	require.Equal(expectedValidators, result)
}

// TestTestStateGetValidatorSet tests GetValidatorSet
func TestTestStateGetValidatorSet(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	state := NewTestState()
	netID := ids.GenerateTestID()

	// Default returns empty map
	result, err := state.GetValidatorSet(ctx, 100, netID)
	require.NoError(err)
	require.Empty(result)

	// With custom function
	nodeID := ids.GenerateTestNodeID()
	expectedValidators := map[ids.NodeID]*validators.GetValidatorOutput{
		nodeID: {
			NodeID:    nodeID,
			PublicKey: []byte("key"),
			Weight:    200,
		},
	}

	state.GetValidatorSetF = func(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
		return expectedValidators, nil
	}

	result, err = state.GetValidatorSet(ctx, 100, netID)
	require.NoError(err)
	require.Equal(expectedValidators, result)

	// With error
	expectedErr := errors.New("test error")
	state.GetValidatorSetF = func(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
		return nil, expectedErr
	}

	_, err = state.GetValidatorSet(ctx, 100, netID)
	require.ErrorIs(err, expectedErr)
}

// TestTestStateGetCurrentHeight tests GetCurrentHeight
func TestTestStateGetCurrentHeight(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	state := NewTestState()

	// Default returns 0
	height, err := state.GetCurrentHeight(ctx)
	require.NoError(err)
	require.Equal(uint64(0), height)

	// With custom function
	state.GetCurrentHeightF = func(ctx context.Context) (uint64, error) {
		return 12345, nil
	}

	height, err = state.GetCurrentHeight(ctx)
	require.NoError(err)
	require.Equal(uint64(12345), height)

	// With error
	expectedErr := errors.New("height error")
	state.GetCurrentHeightF = func(ctx context.Context) (uint64, error) {
		return 0, expectedErr
	}

	_, err = state.GetCurrentHeight(ctx)
	require.ErrorIs(err, expectedErr)
}

// TestTestStateGetWarpValidatorSet tests GetWarpValidatorSet
func TestTestStateGetWarpValidatorSet(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	state := NewTestState()
	netID := ids.GenerateTestID()
	height := uint64(100)

	// Default returns empty WarpSet
	result, err := state.GetWarpValidatorSet(ctx, height, netID)
	require.NoError(err)
	require.NotNil(result)
	require.Equal(height, result.Height)
	require.Empty(result.Validators)

	// With custom function
	nodeID := ids.GenerateTestNodeID()
	expectedSet := &validators.WarpSet{
		Height: height,
		Validators: map[ids.NodeID]*validators.WarpValidator{
			nodeID: {
				NodeID:    nodeID,
				PublicKey: []byte("bls-key"),
				Weight:    300,
			},
		},
	}

	state.GetWarpValidatorSetF = func(ctx context.Context, h uint64, nID ids.ID) (*validators.WarpSet, error) {
		return expectedSet, nil
	}

	result, err = state.GetWarpValidatorSet(ctx, height, netID)
	require.NoError(err)
	require.Equal(expectedSet, result)

	// With error
	expectedErr := errors.New("warp error")
	state.GetWarpValidatorSetF = func(ctx context.Context, h uint64, nID ids.ID) (*validators.WarpSet, error) {
		return nil, expectedErr
	}

	_, err = state.GetWarpValidatorSet(ctx, height, netID)
	require.ErrorIs(err, expectedErr)
}

// TestTestStateGetWarpValidatorSets tests GetWarpValidatorSets
func TestTestStateGetWarpValidatorSets(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	state := NewTestState()
	heights := []uint64{100, 200}
	netIDs := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	// Default returns empty sets for each combination
	result, err := state.GetWarpValidatorSets(ctx, heights, netIDs)
	require.NoError(err)
	require.NotNil(result)
	require.Len(result, 2)
	for _, netID := range netIDs {
		require.Contains(result, netID)
		require.Len(result[netID], 2)
		for _, height := range heights {
			require.Contains(result[netID], height)
			require.Equal(height, result[netID][height].Height)
			require.Empty(result[netID][height].Validators)
		}
	}

	// With custom function
	nodeID := ids.GenerateTestNodeID()
	expectedSets := map[ids.ID]map[uint64]*validators.WarpSet{
		netIDs[0]: {
			100: {
				Height: 100,
				Validators: map[ids.NodeID]*validators.WarpValidator{
					nodeID: {NodeID: nodeID, Weight: 100},
				},
			},
		},
	}

	state.GetWarpValidatorSetsF = func(ctx context.Context, h []uint64, nIDs []ids.ID) (map[ids.ID]map[uint64]*validators.WarpSet, error) {
		return expectedSets, nil
	}

	result, err = state.GetWarpValidatorSets(ctx, heights, netIDs)
	require.NoError(err)
	require.Equal(expectedSets, result)

	// With error
	expectedErr := errors.New("warp sets error")
	state.GetWarpValidatorSetsF = func(ctx context.Context, h []uint64, nIDs []ids.ID) (map[ids.ID]map[uint64]*validators.WarpSet, error) {
		return nil, expectedErr
	}

	_, err = state.GetWarpValidatorSets(ctx, heights, netIDs)
	require.ErrorIs(err, expectedErr)
}

// TestTestStateGetWarpValidatorSetsEmptyInputs tests with empty inputs
func TestTestStateGetWarpValidatorSetsEmptyInputs(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	state := NewTestState()

	// Empty heights and netIDs
	result, err := state.GetWarpValidatorSets(ctx, []uint64{}, []ids.ID{})
	require.NoError(err)
	require.Empty(result)

	// Empty heights only
	netIDs := []ids.ID{ids.GenerateTestID()}
	result, err = state.GetWarpValidatorSets(ctx, []uint64{}, netIDs)
	require.NoError(err)
	require.Len(result, 1)
	require.Empty(result[netIDs[0]])

	// Empty netIDs only
	result, err = state.GetWarpValidatorSets(ctx, []uint64{100}, []ids.ID{})
	require.NoError(err)
	require.Empty(result)
}

// TestStateTypeAlias tests the State type alias
func TestStateTypeAlias(t *testing.T) {
	require := require.New(t)

	// State should be an alias for TestState
	state := NewTestState()
	require.NotNil(state)

	// State is a type alias for TestState
	var _ *State = state

	// Should have all the same methods
	ctx := context.Background()
	_, err := state.GetCurrentHeight(ctx)
	require.NoError(err)
}

// TestTestStateImplementsInterface tests that TestState implements validators.State
func TestTestStateImplementsInterface(t *testing.T) {
	var _ validators.State = (*TestState)(nil)
	var _ validators.State = NewTestState()
}
