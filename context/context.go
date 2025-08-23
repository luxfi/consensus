// Package context provides consensus context for VMs
package context

import (
	"context"
	"time"

	"github.com/luxfi/ids"
)

// Context provides consensus context for VMs
type Context struct {
	NetworkID   uint32        `json:"networkID"`
	SubnetID    ids.ID        `json:"subnetID"`
	ChainID     ids.ID        `json:"chainID"`
	NodeID      ids.NodeID    `json:"nodeID"`
	PublicKey   []byte        `json:"publicKey"`
	XChainID    ids.ID        `json:"xChainID"`
	CChainID    ids.ID        `json:"cChainID"`
	AVAXAssetID ids.ID        `json:"avaxAssetID"`
	
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
	GetSubnetID(ids.ID) (ids.ID, error)
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

// GetSubnetID gets the subnet ID from context
func GetSubnetID(ctx context.Context) ids.ID {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.SubnetID
	}
	return ids.Empty
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
	NetworkID uint32
	SubnetID  ids.ID
	ChainID   ids.ID
	NodeID    ids.NodeID
	PublicKey []byte
}

// WithIDs adds IDs to the context
func WithIDs(ctx context.Context, ids IDs) context.Context {
	c := FromContext(ctx)
	if c == nil {
		c = &Context{}
	}
	c.NetworkID = ids.NetworkID
	c.SubnetID = ids.SubnetID
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

type contextKeyType struct{}

var contextKey = contextKeyType{}