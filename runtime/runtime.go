// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package runtime provides chain wiring and runtime dependencies for VMs.
// This package is part of the consensus module and provides the Runtime struct
// that VMs use for chain configuration, logging, validators, etc.
//
// Use stdlib context.Context for cancellation/deadlines.
// Use *Runtime for chain wiring (IDs, logging, validators, etc.)
package runtime

import (
	"context"
	"sync"
	"time"

	validators "github.com/luxfi/consensus/validator"
	"github.com/luxfi/ids"
	"github.com/luxfi/metric"
	"github.com/luxfi/vm/chains/atomic"
	"github.com/luxfi/warp"
)

// ValidatorState is an alias to validators.State for convenience
type ValidatorState = validators.State

// Runtime provides chain wiring and runtime dependencies for VMs.
// This is separate from stdlib context.Context which handles cancellation/deadlines.
//
// Use context.Context for:
//   - Cancellation signals
//   - Request deadlines
//   - Request-scoped values (sparingly)
//
// Use *Runtime for:
//   - Chain IDs, network IDs
//   - Node identity (NodeID, PublicKey)
//   - Logging, metrics
//   - Database handles
//   - Validator state
//   - Upgrade configurations
type Runtime struct {
	// NetworkID is the numeric network identifier (1=mainnet, 2=testnet)
	NetworkID uint32 `json:"networkID"`

	// ChainID identifies the specific chain within the network
	ChainID ids.ID `json:"chainID"`

	// NodeID identifies this node
	NodeID ids.NodeID `json:"nodeID"`

	// PublicKey is the node's BLS public key bytes
	PublicKey []byte `json:"publicKey"`

	// XChainID is the X-Chain identifier
	XChainID ids.ID `json:"xChainID"`

	// CChainID is the C-Chain identifier
	CChainID ids.ID `json:"cChainID"`

	// XAssetID is the primary asset ID (X-chain native, typically LUX)
	XAssetID ids.ID `json:"xAssetID"`

	// ChainDataDir is the directory for chain-specific data
	ChainDataDir string `json:"chainDataDir"`

	// StartTime is when the node started
	StartTime time.Time `json:"startTime"`

	// ValidatorState provides validator information (uses consensus/validators.State)
	ValidatorState validators.State

	// Keystore provides key management
	Keystore Keystore

	// Metrics provides metrics tracking
	Metrics Metrics

	// Log provides logging
	Log Logger

	// SharedMemory provides cross-chain atomic operations
	SharedMemory SharedMemory

	// BCLookup provides blockchain alias lookup
	BCLookup BCLookup

	// WarpSigner provides BLS signing for Warp messages
	WarpSigner WarpSigner

	// NetworkUpgrades contains upgrade activation times
	NetworkUpgrades NetworkUpgrades

	// Lock for thread-safe access to runtime fields
	Lock sync.RWMutex
}

// BCLookup provides blockchain alias lookup
type BCLookup interface {
	Lookup(alias string) (ids.ID, error)
	PrimaryAlias(id ids.ID) (string, error)
	Aliases(id ids.ID) ([]string, error)
}

// Keystore provides key management
type Keystore interface {
	GetDatabase(username, password string) (interface{}, error)
	NewAccount(username, password string) error
}

// Logger provides logging functionality
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Fatal(msg string, fields ...interface{})
}

// Metrics provides metrics tracking.
// Matches api/metrics.MultiGatherer interface.
type Metrics interface {
	metric.Gatherer
	Register(name string, gatherer metric.Gatherer) error
}

// SharedMemory is the canonical interface for cross-chain atomic operations.
// Uses atomic.Requests and atomic.Element as the ONE canonical types.
type SharedMemory = atomic.SharedMemory

// WarpSigner provides BLS signing for Warp messages.
// Matches warp.Signer interface exactly.
type WarpSigner = warp.Signer

// NetworkUpgrades contains network upgrade activation times.
// Matches upgrade.Config methods exactly.
type NetworkUpgrades interface {
	IsApricotPhase1Activated(timestamp time.Time) bool
	IsApricotPhase2Activated(timestamp time.Time) bool
	IsApricotPhase3Activated(timestamp time.Time) bool
	IsApricotPhase4Activated(timestamp time.Time) bool
	IsApricotPhase5Activated(timestamp time.Time) bool
	IsApricotPhasePre6Activated(timestamp time.Time) bool
	IsApricotPhase6Activated(timestamp time.Time) bool
	IsApricotPhasePost6Activated(timestamp time.Time) bool
	IsBanffActivated(timestamp time.Time) bool
	IsCortinaActivated(timestamp time.Time) bool
	IsDurangoActivated(timestamp time.Time) bool
	IsEtnaActivated(timestamp time.Time) bool
	IsFortunaActivated(timestamp time.Time) bool
	IsGraniteActivated(timestamp time.Time) bool
}

// VMContext is an interface that VM contexts must implement
// This allows different context types (node, plugin, etc.) to be used interchangeably
type VMContext interface {
	GetNetworkID() uint32
	GetChainID() ids.ID
	GetNodeID() ids.NodeID
	GetPublicKey() []byte
	GetXChainID() ids.ID
	GetCChainID() ids.ID
	GetAssetID() ids.ID
	GetChainDataDir() string
	GetLog() interface{}
	GetSharedMemory() interface{}
	GetMetrics() interface{}
	GetValidatorState() interface{}
	GetBCLookup() interface{}
	GetWarpSigner() interface{}
	GetNetworkUpgrades() interface{}
}

// runtimeKeyType is the context key type for storing Runtime
type runtimeKeyType struct{}

var runtimeKey = runtimeKeyType{}

// WithRuntime adds Runtime to a context.Context
func WithRuntime(ctx context.Context, rt *Runtime) context.Context {
	return context.WithValue(ctx, runtimeKey, rt)
}

// FromContext extracts Runtime from a context.Context
func FromContext(ctx context.Context) *Runtime {
	if rt, ok := ctx.Value(runtimeKey).(*Runtime); ok {
		return rt
	}
	return nil
}

// GetChainID gets the chain ID from context
func GetChainID(ctx context.Context) ids.ID {
	if rt := FromContext(ctx); rt != nil {
		return rt.ChainID
	}
	return ids.Empty
}

// GetNetworkID gets the numeric network ID from context
func GetNetworkID(ctx context.Context) uint32 {
	if rt := FromContext(ctx); rt != nil {
		return rt.NetworkID
	}
	return 0
}

// GetNodeID gets the node ID from context
func GetNodeID(ctx context.Context) ids.NodeID {
	if rt := FromContext(ctx); rt != nil {
		return rt.NodeID
	}
	return ids.EmptyNodeID
}

// IsPrimaryNetwork checks if the network is the primary network
func IsPrimaryNetwork(ctx context.Context) bool {
	if rt := FromContext(ctx); rt != nil {
		return rt.NetworkID == 1
	}
	return false
}

// GetValidatorState gets the validator state from context
func GetValidatorState(ctx context.Context) validators.State {
	if rt := FromContext(ctx); rt != nil {
		return rt.ValidatorState
	}
	return nil
}

// GetWarpSigner gets the warp signer from context
func GetWarpSigner(ctx context.Context) WarpSigner {
	if rt := FromContext(ctx); rt != nil {
		return rt.WarpSigner
	}
	return nil
}

// IDs holds the IDs for runtime context
type IDs struct {
	NetworkID    uint32
	ChainID      ids.ID
	NodeID       ids.NodeID
	PublicKey    []byte
	XAssetID     ids.ID
	ChainDataDir string `json:"chainDataDir"`
}

// WithIDs adds IDs to the context via Runtime
func WithIDs(ctx context.Context, id IDs) context.Context {
	rt := FromContext(ctx)
	if rt == nil {
		rt = &Runtime{}
	}
	rt.NetworkID = id.NetworkID
	rt.ChainID = id.ChainID
	rt.NodeID = id.NodeID
	rt.PublicKey = id.PublicKey
	rt.XAssetID = id.XAssetID
	rt.ChainDataDir = id.ChainDataDir
	return WithRuntime(ctx, rt)
}

// WithValidatorState adds validator state to the context
func WithValidatorState(ctx context.Context, vs validators.State) context.Context {
	rt := FromContext(ctx)
	if rt == nil {
		rt = &Runtime{}
	}
	rt.ValidatorState = vs
	return WithRuntime(ctx, rt)
}

// GetTimestamp returns the current timestamp
func GetTimestamp() int64 {
	return time.Now().Unix()
}

// AsBCLookup returns the BCLookup interface for blockchain alias lookup.
// This method is used by VMs (particularly coreth) to resolve chain aliases.
func (r *Runtime) AsBCLookup() BCLookup {
	if r == nil {
		return nil
	}
	return r.BCLookup
}

// GetValidatorOutput is re-exported from validator package for convenience
type GetValidatorOutput = validators.GetValidatorOutput
