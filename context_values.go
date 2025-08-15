package consensus

import (
    "context"
    "sync"
    
    "github.com/luxfi/crypto/bls"
    "github.com/luxfi/database"
    "github.com/luxfi/ids"
    "github.com/luxfi/log"
    "github.com/luxfi/metric"
    "github.com/luxfi/consensus/core/interfaces"
)

// contextKey is the type for context keys
type contextKey string

// Context keys for storing runtime values
const (
    runtimeKey contextKey = "consensus.runtime"
    stateKey   contextKey = "consensus.state"
)

// WithRuntime adds runtime configuration to context
func WithRuntime(ctx context.Context, rt *interfaces.Runtime) context.Context {
    return context.WithValue(ctx, runtimeKey, rt)
}

// GetRuntime retrieves runtime configuration from context
func GetRuntime(ctx context.Context) *interfaces.Runtime {
    if v := ctx.Value(runtimeKey); v != nil {
        if rt, ok := v.(*interfaces.Runtime); ok {
            return rt
        }
    }
    return nil
}

// WithState adds state to context
func WithState(ctx context.Context, state *interfaces.StateHolder) context.Context {
    return context.WithValue(ctx, stateKey, state)
}

// GetState retrieves state from context
func GetState(ctx context.Context) *interfaces.StateHolder {
    if v := ctx.Value(stateKey); v != nil {
        if s, ok := v.(*interfaces.StateHolder); ok {
            return s
        }
    }
    return nil
}

// ChainContext wraps context.Context with consensus-specific fields
// This provides backward compatibility while using context.Context
type ChainContext struct {
    context.Context
    
    // Core fields
    NetworkID    uint32
    SubnetID     ids.ID
    ChainID      ids.ID
    NodeID       ids.NodeID
    PublicKey    *bls.PublicKey
    
    // Additional chain IDs
    XChainID     ids.ID
    CChainID     ids.ID
    LUXAssetID   ids.ID
    
    // Resources
    ChainDataDir string
    Lock         sync.RWMutex
    Log          log.Logger
    Keystore     interface{} // Keystore interface
    SharedMemory database.Database
    BCLookup     AliasLookup
    SNLookup     interface{} // SubnetLookup interface
    Metrics      metric.Registry
    
    // Validators and state
    ValidatorState ValidatorState
    WarpSigner     WarpSigner
    State          *interfaces.StateHolder
}

// NewChainContext creates a new chain context
func NewChainContext(ctx context.Context) *ChainContext {
    return &ChainContext{
        Context: ctx,
    }
}

// WithChainContext adds chain context to context
func WithChainContext(ctx context.Context, cc *ChainContext) context.Context {
    return context.WithValue(ctx, runtimeKey, cc)
}

// GetChainContext retrieves chain context from context
func GetChainContext(ctx context.Context) *ChainContext {
    if v := ctx.Value(runtimeKey); v != nil {
        if cc, ok := v.(*ChainContext); ok {
            return cc
        }
    }
    return nil
}