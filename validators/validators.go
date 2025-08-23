package validators

import (
    "context"
    
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

// GetValidatorOutput represents validator output
type GetValidatorOutput struct {
    NodeID    ids.NodeID
    PublicKey []byte
    Weight    uint64
}

// GetCurrentValidatorOutput represents current validator output
type GetCurrentValidatorOutput struct {
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
    // GetCurrentHeight returns the current P-chain height
    GetCurrentHeight() (uint64, error)
    
    // GetMinimumHeight returns the minimum height available
    GetMinimumHeight(ctx context.Context) (uint64, error)
    
    // GetCurrentValidators returns current validators
    GetCurrentValidators(subnetID ids.ID) (map[ids.NodeID]*Validator, error)
    
    // GetValidatorSet returns a validator set at a given height
    GetValidatorSet(ctx context.Context, height uint64, subnetID ids.ID) (map[ids.NodeID]*GetValidatorOutput, error)
    
    // GetSubnetID returns the subnet ID of a chain
    GetSubnetID(chainID ids.ID) (ids.ID, error)
}

// SetCallbackListener listens for validator set changes
type SetCallbackListener interface {
    // OnValidatorAdded is called when a validator is added
    OnValidatorAdded(nodeID ids.NodeID, pk []byte, weight uint64)
    
    // OnValidatorRemoved is called when a validator is removed
    OnValidatorRemoved(nodeID ids.NodeID, weight uint64)
    
    // OnValidatorWeightChanged is called when a validator's weight changes
    OnValidatorWeightChanged(nodeID ids.NodeID, oldWeight, newWeight uint64)
}

// ManagerCallbackListener listens for manager events
type ManagerCallbackListener interface {
    SetCallbackListener
    
    // OnValidatorManagerInitialized is called when the manager is initialized
    OnValidatorManagerInitialized()
}
