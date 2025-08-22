package validators

import (
    "github.com/luxfi/ids"
)

// Manager manages validators
type Manager interface {
    // GetValidators returns validators for a subnet
    GetValidators(subnetID ids.ID) (map[ids.NodeID]*Validator, error)
    
    // GetValidator returns a specific validator
    GetValidator(subnetID ids.ID, nodeID ids.NodeID) (*Validator, bool)
    
    // GetWeight returns the weight of a validator
    GetWeight(subnetID ids.ID, nodeID ids.NodeID) (uint64, bool)
    
    // SubsetWeight returns total weight of a subset
    SubsetWeight(subnetID ids.ID, nodeIDs []ids.NodeID) (uint64, error)
    
    // TotalWeight returns total weight
    TotalWeight(subnetID ids.ID) (uint64, error)
}

// Validator represents a validator
type Validator struct {
    NodeID    ids.NodeID
    PublicKey []byte
    Weight    uint64
}

// Set is a set of validators
type Set interface {
    // Add adds a validator
    Add(nodeID ids.NodeID, pk []byte, weight uint64) error
    
    // RemoveWeight removes weight from a validator
    RemoveWeight(nodeID ids.NodeID, weight uint64) error
    
    // Get returns a validator
    Get(nodeID ids.NodeID) (*Validator, bool)
    
    // Len returns the number of validators
    Len() int
    
    // Weight returns total weight
    Weight() uint64
    
    // Sample samples validators
    Sample(n int) ([]ids.NodeID, error)
}

// State manages validator state
type State interface {
    // GetCurrentValidators returns current validators
    GetCurrentValidators(subnetID ids.ID) (map[ids.NodeID]*Validator, error)
    
    // GetValidatorSet returns a validator set
    GetValidatorSet(subnetID ids.ID) (Set, error)
}
