// Package validatorsmock provides mock implementations for validator management
package validatorsmock

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// MockValidatorSet provides a mock implementation for validator sets
type MockValidatorSet struct {
	validators []Validator
	weights    map[ids.NodeID]uint64
}

// Validator represents a validator
type Validator struct {
	ID     ids.NodeID
	Weight uint64
}

// NewMockValidatorSet creates a new mock validator set
func NewMockValidatorSet() *MockValidatorSet {
	return &MockValidatorSet{
		validators: make([]Validator, 0),
		weights:    make(map[ids.NodeID]uint64),
	}
}

// AddValidator adds a validator to the set
func (m *MockValidatorSet) AddValidator(nodeID ids.NodeID, weight uint64) {
	m.validators = append(m.validators, Validator{
		ID:     nodeID,
		Weight: weight,
	})
	m.weights[nodeID] = weight
}

// GetValidators returns all validators
func (m *MockValidatorSet) GetValidators(ctx context.Context) []Validator {
	return m.validators
}

// GetWeight returns the weight of a validator
func (m *MockValidatorSet) GetWeight(nodeID ids.NodeID) uint64 {
	return m.weights[nodeID]
}

// Contains checks if a node is a validator
func (m *MockValidatorSet) Contains(nodeID ids.NodeID) bool {
	_, exists := m.weights[nodeID]
	return exists
}

// Size returns the number of validators
func (m *MockValidatorSet) Size() int {
	return len(m.validators)
}

// State provides a mock implementation for validator state
type State struct {
	T             *testing.T
	currentHeight uint64
	minimumHeight uint64
	validators    map[uint64][]Validator
	chainID       ids.ID

	// Function fields that can be overridden
	GetNetIDF         func(context.Context, ids.ID) (ids.ID, error)
	GetCurrentHeightF func(context.Context) (uint64, error)
	GetMinimumHeightF func(context.Context) (uint64, error)
	GetValidatorSetF  func(context.Context, uint64, ids.ID) ([]Validator, error)
	GetChainIDF       func(ids.ID) (ids.ID, error)
}

// NewState creates a new mock validator state
func NewState(t *testing.T) *State {
	return &State{
		T:             t,
		currentHeight: 0,
		minimumHeight: 0,
		validators:    make(map[uint64][]Validator),
		chainID:       ids.Empty,
	}
}

// GetCurrentHeight returns the current height
func (s *State) GetCurrentHeight(ctx context.Context) (uint64, error) {
	if s.GetCurrentHeightF != nil {
		return s.GetCurrentHeightF(ctx)
	}
	return s.currentHeight, nil
}

// GetMinimumHeight returns the minimum height
func (s *State) GetMinimumHeight(ctx context.Context) (uint64, error) {
	if s.GetMinimumHeightF != nil {
		return s.GetMinimumHeightF(ctx)
	}
	return s.minimumHeight, nil
}

// GetValidatorSet returns the validator set at a given height and subnet
func (s *State) GetValidatorSet(ctx context.Context, height uint64, subnetID ids.ID) ([]Validator, error) {
	if s.GetValidatorSetF != nil {
		return s.GetValidatorSetF(ctx, height, subnetID)
	}
	if validators, ok := s.validators[height]; ok {
		return validators, nil
	}
	return []Validator{}, nil
}

// GetChainID returns the chain ID for a given chain
func (s *State) GetChainID(chainID ids.ID) (ids.ID, error) {
	if s.GetChainIDF != nil {
		return s.GetChainIDF(chainID)
	}
	return s.chainID, nil
}

// SetHeight sets the current height
func (s *State) SetHeight(height uint64) {
	s.currentHeight = height
}

// SetMinimumHeight sets the minimum height
func (s *State) SetMinimumHeight(height uint64) {
	s.minimumHeight = height
}

// SetValidatorSet sets the validator set at a given height
func (s *State) SetValidatorSet(height uint64, validators []Validator) {
	s.validators[height] = validators
}

// SetChainID sets the chain ID
func (s *State) SetChainID(chainID ids.ID) {
	s.chainID = chainID
}

// GetNetID returns the network ID for a given chain
func (s *State) GetNetID(ctx context.Context, chainID ids.ID) (ids.ID, error) {
	if s.GetNetIDF != nil {
		return s.GetNetIDF(ctx, chainID)
	}
	// Simple mock: return a fixed network ID
	return ids.GenerateTestID(), nil
}
