package validators

import "github.com/luxfi/ids"

// State provides validator state information
type State interface {
    GetCurrentHeight() (uint64, error)
    GetValidatorSet(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error)
}

// Manager manages validator sets
type Manager interface {
    GetValidators(subnetID ids.ID) ([]ids.NodeID, error)
    GetWeight(subnetID ids.ID, nodeID ids.NodeID) (uint64, error)
    TotalWeight(subnetID ids.ID) (uint64, error)
}
