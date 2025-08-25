package validators

import (
	"context"
	"github.com/luxfi/ids"
	"github.com/luxfi/node/version"
)

// State provides validator state management
type State interface {
	GetValidatorSet(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*GetValidatorOutput, error)
	GetCurrentValidators(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*GetValidatorOutput, error)
	GetCurrentHeight(ctx context.Context) (uint64, error)
}

// GetValidatorOutput provides validator information
type GetValidatorOutput struct {
	NodeID    ids.NodeID
	PublicKey []byte
	Light     uint64
	Weight    uint64 // Alias for Light for backward compatibility
	TxID      ids.ID // Transaction ID that added this validator
}

// Set represents a set of validators
type Set interface {
	Has(ids.NodeID) bool
	Len() int
	List() []Validator
	Light() uint64
	Sample(size int) ([]ids.NodeID, error)
}

// Validator represents a validator
type Validator interface {
	ID() ids.NodeID
	Light() uint64
}

// ValidatorImpl is a concrete implementation of Validator
type ValidatorImpl struct {
	NodeID   ids.NodeID
	LightVal uint64
}

// ID returns the node ID
func (v *ValidatorImpl) ID() ids.NodeID {
	return v.NodeID
}

// Light returns the validator light
func (v *ValidatorImpl) Light() uint64 {
	return v.LightVal
}

// Manager manages validator sets
type Manager interface {
	GetValidators(netID ids.ID) (Set, error)
	GetValidator(netID ids.ID, nodeID ids.NodeID) (*GetValidatorOutput, bool)
	GetLight(netID ids.ID, nodeID ids.NodeID) uint64
	GetWeight(netID ids.ID, nodeID ids.NodeID) uint64 // Deprecated: use GetLight
	TotalLight(netID ids.ID) (uint64, error)
	TotalWeight(netID ids.ID) (uint64, error) // Deprecated: use TotalLight
}

// SetCallbackListener listens to validator set changes
type SetCallbackListener interface {
	OnValidatorAdded(nodeID ids.NodeID, light uint64)
	OnValidatorRemoved(nodeID ids.NodeID, light uint64)
	OnValidatorLightChanged(nodeID ids.NodeID, oldLight, newLight uint64)
}

// ManagerCallbackListener listens to manager changes
type ManagerCallbackListener interface {
	OnValidatorAdded(netID ids.ID, nodeID ids.NodeID, light uint64)
	OnValidatorRemoved(netID ids.ID, nodeID ids.NodeID, light uint64)
	OnValidatorLightChanged(netID ids.ID, nodeID ids.NodeID, oldLight, newLight uint64)
}

// Connector handles validator connections
type Connector interface {
	Connected(ctx context.Context, nodeID ids.NodeID, nodeVersion *version.Application) error
	Disconnected(ctx context.Context, nodeID ids.NodeID) error
}
