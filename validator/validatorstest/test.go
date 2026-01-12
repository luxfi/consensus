package validatorstest

import (
	"context"

	validators "github.com/luxfi/consensus/validator"
	"github.com/luxfi/ids"
)

// State is an alias for TestState for backward compatibility
type State = TestState

// TestState is a test implementation of validators.State
type TestState struct {
	validators map[ids.ID]validators.Set

	// Function fields for test customization
	GetCurrentHeightF     func(context.Context) (uint64, error)
	GetValidatorSetF      func(context.Context, uint64, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error)
	GetWarpValidatorSetF  func(context.Context, uint64, ids.ID) (*validators.WarpSet, error)
	GetWarpValidatorSetsF func(context.Context, []uint64, []ids.ID) (map[ids.ID]map[uint64]*validators.WarpSet, error)
}

// NewTestState creates a new test state
func NewTestState() *TestState {
	return &TestState{
		validators: make(map[ids.ID]validators.Set),
	}
}

// GetCurrentValidators returns current validators
func (s *TestState) GetCurrentValidators(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	return s.GetValidatorSet(ctx, height, netID)
}

// GetValidatorSet returns a validator set
func (s *TestState) GetValidatorSet(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	if s.GetValidatorSetF != nil {
		return s.GetValidatorSetF(ctx, height, netID)
	}
	return make(map[ids.NodeID]*validators.GetValidatorOutput), nil
}

// GetCurrentHeight returns the current height
func (s *TestState) GetCurrentHeight(ctx context.Context) (uint64, error) {
	if s.GetCurrentHeightF != nil {
		return s.GetCurrentHeightF(ctx)
	}
	return 0, nil
}

// GetWarpValidatorSet returns the Warp validator set for a specific height and netID
func (s *TestState) GetWarpValidatorSet(ctx context.Context, height uint64, netID ids.ID) (*validators.WarpSet, error) {
	if s.GetWarpValidatorSetF != nil {
		return s.GetWarpValidatorSetF(ctx, height, netID)
	}
	return &validators.WarpSet{
		Height:     height,
		Validators: make(map[ids.NodeID]*validators.WarpValidator),
	}, nil
}

// GetWarpValidatorSets returns Warp validator sets for the requested heights and netIDs
func (s *TestState) GetWarpValidatorSets(ctx context.Context, heights []uint64, netIDs []ids.ID) (map[ids.ID]map[uint64]*validators.WarpSet, error) {
	if s.GetWarpValidatorSetsF != nil {
		return s.GetWarpValidatorSetsF(ctx, heights, netIDs)
	}
	result := make(map[ids.ID]map[uint64]*validators.WarpSet)
	for _, netID := range netIDs {
		result[netID] = make(map[uint64]*validators.WarpSet)
		for _, height := range heights {
			result[netID][height] = &validators.WarpSet{
				Height:     height,
				Validators: make(map[ids.NodeID]*validators.WarpValidator),
			}
		}
	}
	return result, nil
}

// GetMinimumHeight returns the minimum acceptable height
func (s *TestState) GetMinimumHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}

// GetChainID returns the chain ID for a given network ID
func (s *TestState) GetChainID(netID ids.ID) (ids.ID, error) {
	return netID, nil
}

// GetNetworkID returns the network ID for a given chain ID
func (s *TestState) GetNetworkID(chainID ids.ID) (ids.ID, error) {
	return chainID, nil
}
