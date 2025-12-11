package validators

//go:generate go run go.uber.org/mock/mockgen -package=validatorsmock -destination=validatorsmock/state.go -mock_names=State=State . State

import (
	"context"

	"github.com/luxfi/consensus/version"
	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
)

// State provides validator state management
type State interface {
	GetValidatorSet(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*GetValidatorOutput, error)
	GetCurrentValidators(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*GetValidatorOutput, error)
	GetCurrentHeight(ctx context.Context) (uint64, error)

	// GetWarpValidatorSets returns Warp validator sets for the requested heights and netIDs.
	// Returns a map of netID -> height -> WarpSet containing BLS-enabled validators.
	GetWarpValidatorSets(ctx context.Context, heights []uint64, netIDs []ids.ID) (map[ids.ID]map[uint64]*WarpSet, error)

	// GetWarpValidatorSet returns the Warp validator set for a specific height and netID.
	// Returns a WarpSet containing validators with BLS public keys for Warp signing.
	GetWarpValidatorSet(ctx context.Context, height uint64, netID ids.ID) (*WarpSet, error)
}

// GetValidatorOutput provides validator information
type GetValidatorOutput struct {
	NodeID         ids.NodeID
	PublicKey      []byte // BLS public key (classical)
	RingtailPubKey []byte // Ringtail public key (post-quantum)
	Light          uint64
	Weight         uint64 // Alias for Light for backward compatibility
	TxID           ids.ID // Transaction ID that added this validator
}

// WarpValidator represents a Warp validator with BLS and Ringtail keys
type WarpValidator struct {
	NodeID         ids.NodeID
	PublicKey      []byte // BLS public key for Warp signing (classical)
	RingtailPubKey []byte // Ringtail public key (post-quantum)
	Weight         uint64
}

// WarpSet represents a set of Warp validators at a specific height
type WarpSet struct {
	Height     uint64
	Validators map[ids.NodeID]*WarpValidator
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

	// Mutable operations
	AddStaker(netID ids.ID, nodeID ids.NodeID, publicKey []byte, txID ids.ID, light uint64) error
	AddWeight(netID ids.ID, nodeID ids.NodeID, light uint64) error
	RemoveWeight(netID ids.ID, nodeID ids.NodeID, light uint64) error
	NumNets() int

	// Additional utility methods
	Count(netID ids.ID) int
	NumValidators(netID ids.ID) int // Alias for Count
	Sample(netID ids.ID, size int) ([]ids.NodeID, error)
	GetValidatorIDs(netID ids.ID) []ids.NodeID
	SubsetWeight(netID ids.ID, nodeIDs set.Set[ids.NodeID]) (uint64, error)
	GetMap(netID ids.ID) map[ids.NodeID]*GetValidatorOutput
	RegisterCallbackListener(listener ManagerCallbackListener)
	RegisterSetCallbackListener(netID ids.ID, listener SetCallbackListener)
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
