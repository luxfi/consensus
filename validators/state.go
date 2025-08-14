package validators

import "github.com/luxfi/ids"

// Validator represents a validator node
type Validator struct {
    NodeID    ids.NodeID
    PublicKey interface{} // *bls.PublicKey
    TxID      ids.ID
    Weight    uint64
}

// State provides validator state information
type State interface {
    GetCurrentHeight() (uint64, error)
    GetValidatorSet(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error)
}

// Manager manages validator sets
type Manager interface {
    GetValidators(subnetID ids.ID) ([]ids.NodeID, error)
    GetValidator(subnetID ids.ID, nodeID ids.NodeID) (*Validator, bool)
    GetWeight(subnetID ids.ID, nodeID ids.NodeID) (uint64, error)
    TotalWeight(subnetID ids.ID) (uint64, error)
    NumValidators(subnetID ids.ID) int
    RegisterSetCallbackListener(listener SetCallbackListener)
}
