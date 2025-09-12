package validators

import (
	"github.com/luxfi/ids"
)

// NewManager creates a new validator manager
func NewManager() *manager {
	return &manager{
		validators: make(map[ids.ID]map[ids.NodeID]*GetValidatorOutput),
	}
}

type manager struct {
	validators map[ids.ID]map[ids.NodeID]*GetValidatorOutput
}

// AddStaker adds a validator to the set
func (m *manager) AddStaker(netID ids.ID, nodeID ids.NodeID, publicKey []byte, txID ids.ID, light uint64) error {
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

func (s *emptySet) Has(ids.NodeID) bool               { return false }
func (s *emptySet) Len() int                           { return 0 }
func (s *emptySet) List() []Validator                  { return nil }
func (s *emptySet) Light() uint64                      { return 0 }
func (s *emptySet) Sample(size int) ([]ids.NodeID, error) { 
	return nil, nil 
}