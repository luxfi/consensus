package validators

import (
	"sync"

	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
)

// NewManager creates a new validator manager
func NewManager() *manager {
	return &manager{
		validators: make(map[ids.ID]map[ids.NodeID]*GetValidatorOutput),
		mu:         &sync.RWMutex{},
		listeners:  make([]ManagerCallbackListener, 0),
	}
}

type manager struct {
	validators map[ids.ID]map[ids.NodeID]*GetValidatorOutput
	mu         *sync.RWMutex
	listeners  []ManagerCallbackListener
}

// AddStaker adds a validator to the set
func (m *manager) AddStaker(netID ids.ID, nodeID ids.NodeID, publicKey []byte, txID ids.ID, light uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.validators[netID] == nil {
		m.validators[netID] = make(map[ids.NodeID]*GetValidatorOutput)
	}

	m.validators[netID][nodeID] = &GetValidatorOutput{
		NodeID:    nodeID,
		PublicKey: publicKey,
		Light:     light,
		Weight:    light,
		TxID:      txID,
	}

	// Notify all listeners
	for _, listener := range m.listeners {
		listener.OnValidatorAdded(netID, nodeID, light)
	}
	return nil
}

// AddWeight adds weight to an existing validator
func (m *manager) AddWeight(netID ids.ID, nodeID ids.NodeID, light uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.validators[netID] == nil {
		m.validators[netID] = make(map[ids.NodeID]*GetValidatorOutput)
	}

	val, exists := m.validators[netID][nodeID]
	if !exists {
		return nil // Validator doesn't exist, nothing to add
	}

	val.Light += light
	val.Weight += light
	return nil
}

// RemoveWeight removes weight from an existing validator
func (m *manager) RemoveWeight(netID ids.ID, nodeID ids.NodeID, light uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.validators[netID] == nil {
		return nil
	}

	val, exists := m.validators[netID][nodeID]
	if !exists {
		return nil // Validator doesn't exist, nothing to remove
	}

	if val.Light >= light {
		val.Light -= light
		val.Weight -= light
	} else {
		val.Light = 0
		val.Weight = 0
	}

	// Remove validator if weight is 0
	if val.Light == 0 {
		delete(m.validators[netID], nodeID)
		if len(m.validators[netID]) == 0 {
			delete(m.validators, netID)
		}
	}

	return nil
}

// NumNets returns the number of networks with validators
func (m *manager) NumNets() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.validators)
}

func (m *manager) GetValidators(netID ids.ID) (Set, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if validators, ok := m.validators[netID]; ok {
		return &validatorSet{validators: validators}, nil
	}
	return &emptySet{}, nil
}

func (m *manager) GetValidator(netID ids.ID, nodeID ids.NodeID) (*GetValidatorOutput, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

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

// Count returns the number of validators in a network
func (m *manager) Count(netID ids.ID) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if subnet, ok := m.validators[netID]; ok {
		return len(subnet)
	}
	return 0
}

// NumValidators is an alias for Count
func (m *manager) NumValidators(netID ids.ID) int {
	return m.Count(netID)
}

// Sample returns a sample of validator node IDs
func (m *manager) Sample(netID ids.ID, size int) ([]ids.NodeID, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodeIDs := make([]ids.NodeID, 0, size)
	if subnet, ok := m.validators[netID]; ok {
		for nodeID := range subnet {
			if len(nodeIDs) >= size {
				break
			}
			nodeIDs = append(nodeIDs, nodeID)
		}
	}
	return nodeIDs, nil
}

// GetValidatorIDs returns all validator node IDs for a network
func (m *manager) GetValidatorIDs(netID ids.ID) []ids.NodeID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if subnet, ok := m.validators[netID]; ok {
		nodeIDs := make([]ids.NodeID, 0, len(subnet))
		for nodeID := range subnet {
			nodeIDs = append(nodeIDs, nodeID)
		}
		return nodeIDs
	}
	return nil
}

// SubsetWeight returns the total weight of a subset of validators
func (m *manager) SubsetWeight(netID ids.ID, nodeIDs set.Set[ids.NodeID]) (uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var totalWeight uint64
	if subnet, ok := m.validators[netID]; ok {
		for nodeID := range nodeIDs {
			if vdr, ok := subnet[nodeID]; ok {
				totalWeight += vdr.Weight
			}
		}
	}
	return totalWeight, nil
}

// GetMap returns a copy of the validator map for a network
func (m *manager) GetMap(netID ids.ID) map[ids.NodeID]*GetValidatorOutput {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if subnet, ok := m.validators[netID]; ok {
		// Return a copy
		result := make(map[ids.NodeID]*GetValidatorOutput, len(subnet))
		for k, v := range subnet {
			result[k] = v
		}
		return result
	}
	return make(map[ids.NodeID]*GetValidatorOutput)
}

// RegisterCallbackListener registers a callback listener
func (m *manager) RegisterCallbackListener(listener ManagerCallbackListener) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.listeners = append(m.listeners, listener)

	// Notify listener of all existing validators
	for netID, validators := range m.validators {
		for nodeID, val := range validators {
			listener.OnValidatorAdded(netID, nodeID, val.Light)
		}
	}
}

// RegisterSetCallbackListener registers a set callback listener (no-op for now)
func (m *manager) RegisterSetCallbackListener(netID ids.ID, listener SetCallbackListener) {
	// No-op for now - can be implemented later if needed
}
