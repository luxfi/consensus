package validatorstest

import (
	"github.com/luxfi/consensus/validators"
	"github.com/luxfi/ids"
)

// TestState is a test implementation of validators.State
type TestState struct {
	validators map[ids.ID]validators.Set
}

// NewTestState creates a new test state
func NewTestState() *TestState {
	return &TestState{
		validators: make(map[ids.ID]validators.Set),
	}
}

// GetCurrentValidators returns current validators
func (s *TestState) GetCurrentValidators(netID ids.ID) (map[ids.NodeID]*validators.Validator, error) {
	return make(map[ids.NodeID]*validators.Validator), nil
}

// GetValidatorSet returns a validator set
func (s *TestState) GetValidatorSet(netID ids.ID) (validators.Set, error) {
	if set, ok := s.validators[netID]; ok {
		return set, nil
	}
	return nil, nil
}
