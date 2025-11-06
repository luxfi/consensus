package validatorstest

import (
	"context"

	"github.com/luxfi/consensus/validator"
	"github.com/luxfi/ids"
)

// State is an alias for TestState for backward compatibility
type State = TestState

// TestState is a test implementation of validators.State
type TestState struct {
	validators map[ids.ID]validators.Set

	// Function fields for test customization
	GetCurrentHeightF func(context.Context) (uint64, error)
	GetValidatorSetF  func(context.Context, uint64, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error)
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
