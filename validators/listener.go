package validators

import "github.com/luxfi/ids"

// SetCallbackListener provides callbacks for validator set changes
type SetCallbackListener interface {
	OnValidatorAdded(nodeID ids.NodeID, weight uint64)
	OnValidatorRemoved(nodeID ids.NodeID, weight uint64)
	OnValidatorWeightChanged(nodeID ids.NodeID, oldWeight, newWeight uint64)
}
