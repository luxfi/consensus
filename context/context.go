// Package context provides consensus context for VMs
package context

import (
	"context"
	"time"

	"github.com/luxfi/ids"
)

// Context provides consensus context for VMs
type Context struct {
	// QuantumID is the root quantum network identifier
	QuantumID uint32 `json:"quantumID"`
	// NetID identifies the specific network/subnet within the quantum network
	NetID ids.ID `json:"netID"`
	// ChainID identifies the specific chain within the network
	ChainID     ids.ID     `json:"chainID"`
	NodeID      ids.NodeID `json:"nodeID"`
	PublicKey   []byte     `json:"publicKey"`
	XChainID    ids.ID     `json:"xChainID"`
	CChainID    ids.ID     `json:"cChainID"`
	AVAXAssetID ids.ID     `json:"avaxAssetID"`
	LUXAssetID  ids.ID     `json:"luxAssetID"`

	// Timing
	StartTime time.Time `json:"startTime"`

	// Additional fields for consensus
	ValidatorState ValidatorState
	Keystore       Keystore
	BCLookup       BlockchainIDLookup
	Metrics        Metrics
}

// ValidatorState provides validator information
type ValidatorState interface {
	GetChainID(ids.ID) (ids.ID, error)
	GetNetID(ids.ID) (ids.ID, error)
	GetSubnetID(chainID ids.ID) (ids.ID, error)
	GetValidatorSet(uint64, ids.ID) (map[ids.NodeID]uint64, error)
	GetCurrentHeight() (uint64, error)
	GetMinimumHeight(context.Context) (uint64, error)
}

// GetValidatorOutput contains validator information
type GetValidatorOutput struct {
	NodeID    ids.NodeID
	PublicKey []byte
	Weight    uint64
}

// Keystore provides key management
type Keystore interface {
	GetDatabase(username, password string) (interface{}, error)
	NewAccount(username, password string) error
}

// BlockchainIDLookup provides blockchain ID lookup
type BlockchainIDLookup interface {
	Lookup(alias string) (ids.ID, error)
}

// Metrics provides metrics tracking
type Metrics interface {
	Register(namespace string, registerer interface{}) error
}

// GetTimestamp returns the current timestamp
func GetTimestamp() int64 {
	return time.Now().Unix()
}

// GetChainID gets the chain ID from context
func GetChainID(ctx context.Context) ids.ID {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.ChainID
	}
	return ids.Empty
}

// GetNetID gets the network ID from context
func GetNetID(ctx context.Context) ids.ID {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.NetID
	}
	return ids.Empty
}

// Deprecated: GetSubnetID is deprecated, use GetNetID instead
func GetSubnetID(ctx context.Context) ids.ID {
	// Direct implementation to avoid calling deprecated functions
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.NetID
	}
	return ids.Empty
}

// GetNetworkID gets the network ID from context
func GetNetworkID(ctx context.Context) uint32 {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.QuantumID
	}
	return 0
}

// GetValidatorState gets the validator state from context
func GetValidatorState(ctx context.Context) ValidatorState {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.ValidatorState
	}
	return nil
}

// WithContext adds consensus context to a context
func WithContext(ctx context.Context, cc *Context) context.Context {
	return context.WithValue(ctx, contextKey, cc)
}

// FromContext extracts consensus context from a context
func FromContext(ctx context.Context) *Context {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c
	}
	return nil
}

// GetNodeID gets the node ID from context
func GetNodeID(ctx context.Context) ids.NodeID {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.NodeID
	}
	return ids.EmptyNodeID
}

// IDs holds the IDs for consensus context
type IDs struct {
	// NetworkID is the network identifier
	NetworkID uint32
	// QuantumID is the root quantum network identifier
	QuantumID uint32
	// NetID identifies the network within the quantum network
	NetID ids.ID
	// ChainID identifies the chain within the network
	ChainID   ids.ID
	NodeID    ids.NodeID
	PublicKey []byte
	// LUXAssetID is the asset ID for LUX
	LUXAssetID ids.ID
}

// WithIDs adds IDs to the context
func WithIDs(ctx context.Context, ids IDs) context.Context {
	c := FromContext(ctx)
	if c == nil {
		c = &Context{}
	}
	c.QuantumID = ids.QuantumID
	c.NetID = ids.NetID
	c.ChainID = ids.ChainID
	c.NodeID = ids.NodeID
	c.PublicKey = ids.PublicKey
	return WithContext(ctx, c)
}

// WithValidatorState adds validator state to the context
func WithValidatorState(ctx context.Context, vs ValidatorState) context.Context {
	c := FromContext(ctx)
	if c == nil {
		c = &Context{}
	}
	c.ValidatorState = vs
	return WithContext(ctx, c)
}

// GetLUXAssetID returns the LUX asset ID from the context
func GetLUXAssetID(ctx context.Context) ids.ID {
	c := FromContext(ctx)
	if c == nil {
		return ids.Empty
	}
	return c.LUXAssetID
}

type contextKeyType struct{}

var contextKey = contextKeyType{}

// ValidatorStateAdapter adapts validators.State to consensus context ValidatorState
type ValidatorStateAdapter struct {
	validators interface {
		GetCurrentHeight(ctx context.Context) (uint64, error)
		GetValidatorSet(ctx context.Context, height uint64, subnetID ids.ID) (map[ids.NodeID]*interface{}, error)
	}
}

// GetCurrentHeight implements ValidatorState
func (a *ValidatorStateAdapter) GetCurrentHeight() (uint64, error) {
	return a.validators.GetCurrentHeight(context.Background())
}

// GetMinimumHeight implements ValidatorState
func (a *ValidatorStateAdapter) GetMinimumHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}

// GetChainID implements ValidatorState
func (a *ValidatorStateAdapter) GetChainID(chainID ids.ID) (ids.ID, error) {
	return chainID, nil
}

// GetNetID implements ValidatorState
func (a *ValidatorStateAdapter) GetNetID(chainID ids.ID) (ids.ID, error) {
	return ids.Empty, nil
}

// GetSubnetID implements ValidatorState
func (a *ValidatorStateAdapter) GetSubnetID(chainID ids.ID) (ids.ID, error) {
	return ids.Empty, nil
}

// GetValidatorSet implements ValidatorState
func (a *ValidatorStateAdapter) GetValidatorSet(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error) {
	return make(map[ids.NodeID]uint64), nil
}
