// Package validatorsmock provides mock implementations for validator management
package validatorsmock

import (
	"context"
	"github.com/luxfi/consensus/types"
)

// MockValidatorSet provides a mock implementation for validator sets
type MockValidatorSet struct {
	validators []Validator
	weights    map[types.NodeID]uint64
}

// Validator represents a validator
type Validator struct {
	ID     types.NodeID
	Weight uint64
}

// NewMockValidatorSet creates a new mock validator set
func NewMockValidatorSet() *MockValidatorSet {
	return &MockValidatorSet{
		validators: make([]Validator, 0),
		weights:    make(map[types.NodeID]uint64),
	}
}

// AddValidator adds a validator to the set
func (m *MockValidatorSet) AddValidator(nodeID types.NodeID, weight uint64) {
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
func (m *MockValidatorSet) GetWeight(nodeID types.NodeID) uint64 {
	return m.weights[nodeID]
}

// Contains checks if a node is a validator
func (m *MockValidatorSet) Contains(nodeID types.NodeID) bool {
	_, exists := m.weights[nodeID]
	return exists
}

// Size returns the number of validators
func (m *MockValidatorSet) Size() int {
	return len(m.validators)
}