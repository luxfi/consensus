package validators

import (
	"fmt"

	"github.com/luxfi/consensus/utils/set"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// NewManager creates a new validator manager
func NewManager() *manager {
	return &manager{
		validators: make(map[ids.ID]map[ids.NodeID]*GetValidatorOutput),
		callbacks:  []ManagerCallbackListener{},
		setCallbacks: make(map[ids.ID][]SetCallbackListener),
	}
}

type manager struct {
	validators map[ids.ID]map[ids.NodeID]*GetValidatorOutput
	callbacks  []ManagerCallbackListener
	setCallbacks map[ids.ID][]SetCallbackListener
}

// AddStaker adds a validator to the set
func (m *manager) AddStaker(subnetID ids.ID, nodeID ids.NodeID, pk *bls.PublicKey, txID ids.ID, weight uint64) error {
	if m.validators[subnetID] == nil {
		m.validators[subnetID] = make(map[ids.NodeID]*GetValidatorOutput)
	}

	// Convert PublicKey to bytes if not nil
	var pkBytes []byte
	if pk != nil {
		pkBytes = bls.PublicKeyToCompressedBytes(pk)
	}

	m.validators[subnetID][nodeID] = &GetValidatorOutput{
		NodeID:    nodeID,
		PublicKey: pkBytes,
		Light:     weight,
		Weight:    weight,
		TxID:      txID,
	}
	return nil
}

func (m *manager) GetValidators(netID ids.ID) (Set, error) {
	if validators, ok := m.validators[netID]; ok {
		return &validatorSet{validators: validators}, nil
	}
	return &emptySet{}, nil
}

func (m *manager) GetValidator(netID ids.ID, nodeID ids.NodeID) (*GetValidatorOutput, bool) {
	if validators, ok := m.validators[netID]; ok {
		if val, exists := validators[nodeID]; exists {
			return val, true
		}
	}
	return nil, false
}

func (m *manager) GetLight(netID ids.ID, nodeID ids.NodeID) uint64 {
	if val, ok := m.GetValidator(netID, nodeID); ok {
		return val.Light
	}
	return 0
}

func (m *manager) GetWeight(netID ids.ID, nodeID ids.NodeID) uint64 {
	return m.GetLight(netID, nodeID)
}

func (m *manager) TotalLight(netID ids.ID) (uint64, error) {
	set, err := m.GetValidators(netID)
	if err != nil {
		return 0, err
	}
	return set.Light(), nil
}

func (m *manager) TotalWeight(netID ids.ID) (uint64, error) {
	return m.TotalLight(netID)
}

// AddWeight adds weight to an existing validator
func (m *manager) AddWeight(subnetID ids.ID, nodeID ids.NodeID, weight uint64) error {
	if m.validators[subnetID] == nil {
		return fmt.Errorf("subnet %s not found", subnetID)
	}

	val, exists := m.validators[subnetID][nodeID]
	if !exists {
		return fmt.Errorf("validator %s not found in subnet %s", nodeID, subnetID)
	}

	val.Weight += weight
	val.Light += weight
	return nil
}

// RemoveWeight removes weight from a validator
func (m *manager) RemoveWeight(subnetID ids.ID, nodeID ids.NodeID, weight uint64) error {
	if m.validators[subnetID] == nil {
		return fmt.Errorf("subnet %s not found", subnetID)
	}

	val, exists := m.validators[subnetID][nodeID]
	if !exists {
		return fmt.Errorf("validator %s not found in subnet %s", nodeID, subnetID)
	}

	if val.Weight < weight {
		return fmt.Errorf("validator %s weight %d is less than weight to remove %d", nodeID, val.Weight, weight)
	}

	val.Weight -= weight
	val.Light -= weight

	// Remove validator if weight becomes 0
	if val.Weight == 0 {
		delete(m.validators[subnetID], nodeID)
		if len(m.validators[subnetID]) == 0 {
			delete(m.validators, subnetID)
		}
	}

	return nil
}

// validatorSet represents a validator set
type validatorSet struct {
	validators map[ids.NodeID]*GetValidatorOutput
}

func (s *validatorSet) Has(nodeID ids.NodeID) bool {
	_, ok := s.validators[nodeID]
	return ok
}

func (s *validatorSet) Len() int {
	return len(s.validators)
}

func (s *validatorSet) List() []Validator {
	vals := make([]Validator, 0, len(s.validators))
	for _, v := range s.validators {
		vals = append(vals, &ValidatorImpl{
			NodeID:   v.NodeID,
			LightVal: v.Light,
		})
	}
	return vals
}

func (s *validatorSet) Light() uint64 {
	var total uint64
	for _, v := range s.validators {
		total += v.Light
	}
	return total
}

func (s *validatorSet) Sample(size int) ([]ids.NodeID, error) {
	nodeIDs := make([]ids.NodeID, 0, size)
	for nodeID := range s.validators {
		if len(nodeIDs) >= size {
			break
		}
		nodeIDs = append(nodeIDs, nodeID)
	}
	return nodeIDs, nil
}

// emptySet represents an empty validator set
type emptySet struct{}

func (s *emptySet) Has(ids.NodeID) bool { return false }
func (s *emptySet) Len() int            { return 0 }
func (s *emptySet) List() []Validator   { return nil }
func (s *emptySet) Light() uint64       { return 0 }
func (s *emptySet) Sample(size int) ([]ids.NodeID, error) {
	return nil, nil
}

// GetValidatorIDs returns all validator node IDs for a subnet
func (m *manager) GetValidatorIDs(netID ids.ID) []ids.NodeID {
	if validators, ok := m.validators[netID]; ok {
		nodeIDs := make([]ids.NodeID, 0, len(validators))
		for nodeID := range validators {
			nodeIDs = append(nodeIDs, nodeID)
		}
		return nodeIDs
	}
	return nil
}

// GetMap returns all validators for a subnet as a map
func (m *manager) GetMap(netID ids.ID) map[ids.NodeID]*GetValidatorOutput {
	if validators, ok := m.validators[netID]; ok {
		// Return a copy to avoid external modification
		result := make(map[ids.NodeID]*GetValidatorOutput, len(validators))
		for k, v := range validators {
			result[k] = v
		}
		return result
	}
	return make(map[ids.NodeID]*GetValidatorOutput)
}

// Sample returns a sample of validators from a subnet
func (m *manager) Sample(netID ids.ID, size int) ([]ids.NodeID, error) {
	if validators, ok := m.validators[netID]; ok {
		nodeIDs := make([]ids.NodeID, 0, size)
		for nodeID := range validators {
			if len(nodeIDs) >= size {
				break
			}
			nodeIDs = append(nodeIDs, nodeID)
		}
		return nodeIDs, nil
	}
	return nil, nil
}

// SubsetWeight returns the total weight of a subset of validators
func (m *manager) SubsetWeight(netID ids.ID, nodeIDs set.Set[ids.NodeID]) (uint64, error) {
	var totalWeight uint64
	if validators, ok := m.validators[netID]; ok {
		for nodeID := range nodeIDs {
			if validator, exists := validators[nodeID]; exists {
				totalWeight += validator.Weight
			}
		}
	}
	return totalWeight, nil
}

// Count returns the number of validators for a subnet
func (m *manager) Count(netID ids.ID) int {
	if validators, ok := m.validators[netID]; ok {
		return len(validators)
	}
	return 0
}

// NumSubnets returns the number of subnets
func (m *manager) NumSubnets() int {
	return len(m.validators)
}

// NumValidators returns the number of validators in a subnet
func (m *manager) NumValidators(netID ids.ID) int {
	return m.Count(netID)
}

// RegisterCallbackListener registers a manager callback listener
func (m *manager) RegisterCallbackListener(listener ManagerCallbackListener) {
	m.callbacks = append(m.callbacks, listener)
}

// RegisterSetCallbackListener registers a set callback listener for a subnet
func (m *manager) RegisterSetCallbackListener(netID ids.ID, listener SetCallbackListener) {
	if m.setCallbacks[netID] == nil {
		m.setCallbacks[netID] = []SetCallbackListener{}
	}
	m.setCallbacks[netID] = append(m.setCallbacks[netID], listener)
}
