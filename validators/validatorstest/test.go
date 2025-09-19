package validatorstest

import (
	"context"

	"github.com/luxfi/consensus/validators"
	"github.com/luxfi/ids"
)

// State is an alias for TestState for backward compatibility
type State = TestState

// TestState is a test implementation of validators.State
type TestState struct {
	validators map[ids.ID]validators.Set

	// Function fields for test customization
	GetCurrentHeightF      func(context.Context) (uint64, error)
	GetMinimumHeightF      func(context.Context) (uint64, error)
	GetValidatorSetF       func(context.Context, uint64, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error)
	GetChainIDF            func(ids.ID) (ids.ID, error)
	GetNetIDF              func(ids.ID) (ids.ID, error)
	GetSubnetIDF           func(ids.ID) (ids.ID, error)
	GetValidatorSetSimpleF func(uint64, ids.ID) (map[ids.NodeID]uint64, error)
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

// GetValidatorSet returns a validator set with detailed output (validators.State interface)
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

// GetCurrentHeight returns the current height for ValidatorState interface (no context)
func (s *TestState) GetCurrentHeightNoContext() (uint64, error) {
	if s.GetCurrentHeightF != nil {
		return s.GetCurrentHeightF(context.Background())
	}
	return 0, nil
}

// GetMinimumHeight returns the minimum height
func (s *TestState) GetMinimumHeight(ctx context.Context) (uint64, error) {
	if s.GetMinimumHeightF != nil {
		return s.GetMinimumHeightF(ctx)
	}
	return 0, nil
}

// GetChainID returns chain ID for ValidatorState interface
func (s *TestState) GetChainID(subnetID ids.ID) (ids.ID, error) {
	if s.GetChainIDF != nil {
		return s.GetChainIDF(subnetID)
	}
	return ids.Empty, nil
}

// GetNetID returns net ID for ValidatorState interface
func (s *TestState) GetNetID(chainID ids.ID) (ids.ID, error) {
	if s.GetNetIDF != nil {
		return s.GetNetIDF(chainID)
	}
	return ids.Empty, nil
}

// GetSubnetID returns subnet ID for ValidatorState interface
func (s *TestState) GetSubnetID(chainID ids.ID) (ids.ID, error) {
	if s.GetSubnetIDF != nil {
		return s.GetSubnetIDF(chainID)
	}
	return ids.Empty, nil
}

// GetValidatorSetSimple returns validator set for ValidatorState interface
func (s *TestState) GetValidatorSetSimple(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error) {
	if s.GetValidatorSetSimpleF != nil {
		return s.GetValidatorSetSimpleF(height, subnetID)
	}
	// Convert from detailed output to simple weights map
	if s.GetValidatorSetF != nil {
		detailed, err := s.GetValidatorSetF(context.Background(), height, subnetID)
		if err != nil {
			return nil, err
		}
		result := make(map[ids.NodeID]uint64, len(detailed))
		for nodeID, output := range detailed {
			result[nodeID] = output.Weight
		}
		return result, nil
	}
	return make(map[ids.NodeID]uint64), nil
}

// Consensus ValidatorState interface methods (without context)

// GetValidatorSetForConsensus returns validator set for ValidatorState interface (no context, returns simple weights)
func (s *TestState) GetValidatorSetForConsensus(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error) {
	return s.GetValidatorSetSimple(height, subnetID)
}
