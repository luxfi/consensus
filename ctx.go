// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

import (
	"context"
	
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// contextKey is the type for consensus context keys
type contextKey int

const (
	chainIDKey contextKey = iota
	networkIDKey
	subnetIDKey
	validatorStateKey
	nodeIDKey
	idsKey
	loggerKey
	sharedMemoryKey
	bcLookupKey
	warpSignerKey
	chainDataDirKey
)

// ValidatorState provides validator information
type ValidatorState interface {
	GetCurrentHeight() (uint64, error)
	GetMinimumHeight(ctx context.Context) (uint64, error)
	GetValidatorSet(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error)
	GetSubnetID(chainID ids.ID) (ids.ID, error)
}

// WarpSigner is an interface for signing warp messages
type WarpSigner interface {
	Sign(msg interface{}) ([]byte, error)
}

// Logger interface for consensus logging
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// NoOpLogger is a logger that discards all messages
type NoOpLogger struct{}

func (NoOpLogger) Debug(string, ...interface{}) {}
func (NoOpLogger) Info(string, ...interface{})  {}
func (NoOpLogger) Warn(string, ...interface{})  {}
func (NoOpLogger) Error(string, ...interface{}) {}

// GetChainID retrieves the chain ID from context
func GetChainID(ctx context.Context) ids.ID {
	if v := ctx.Value(chainIDKey); v != nil {
		if id, ok := v.(ids.ID); ok {
			return id
		}
	}
	return ids.Empty
}

// GetNetworkID retrieves the network ID from context
func GetNetworkID(ctx context.Context) uint32 {
	if v := ctx.Value(networkIDKey); v != nil {
		if id, ok := v.(uint32); ok {
			return id
		}
	}
	return 0
}

// GetSubnetID retrieves the subnet ID from context
func GetSubnetID(ctx context.Context) ids.ID {
	if v := ctx.Value(subnetIDKey); v != nil {
		if id, ok := v.(ids.ID); ok {
			return id
		}
	}
	return ids.Empty
}

// GetValidatorState retrieves the validator state from context
func GetValidatorState(ctx context.Context) ValidatorState {
	if v := ctx.Value(validatorStateKey); v != nil {
		if vs, ok := v.(ValidatorState); ok {
			return vs
		}
	}
	return nil
}

// WithChainID adds chain ID to context
func WithChainID(ctx context.Context, chainID ids.ID) context.Context {
	return context.WithValue(ctx, chainIDKey, chainID)
}

// WithNetworkID adds network ID to context
func WithNetworkID(ctx context.Context, networkID uint32) context.Context {
	return context.WithValue(ctx, networkIDKey, networkID)
}

// WithSubnetID adds subnet ID to context
func WithSubnetID(ctx context.Context, subnetID ids.ID) context.Context {
	return context.WithValue(ctx, subnetIDKey, subnetID)
}

// WithValidatorState adds validator state to context
func WithValidatorState(ctx context.Context, vs ValidatorState) context.Context {
	return context.WithValue(ctx, validatorStateKey, vs)
}

// GetNodeID retrieves the node ID from context
func GetNodeID(ctx context.Context) ids.NodeID {
	if v := ctx.Value(nodeIDKey); v != nil {
		if id, ok := v.(ids.NodeID); ok {
			return id
		}
	}
	return ids.EmptyNodeID
}

// WithNodeID adds node ID to context
func WithNodeID(ctx context.Context, nodeID ids.NodeID) context.Context {
	return context.WithValue(ctx, nodeIDKey, nodeID)
}

// IDs represents a collection of chain IDs
type IDs struct {
	NetworkID   uint32
	ChainID     ids.ID
	SubnetID    ids.ID
	NodeID      ids.NodeID
	PublicKey   *bls.PublicKey
	XAssetID    ids.ID
	LUXAssetID  ids.ID
}

// WithIDs adds chain IDs to context
func WithIDs(ctx context.Context, ids IDs) context.Context {
	ctx = WithNetworkID(ctx, ids.NetworkID)
	ctx = WithChainID(ctx, ids.ChainID)
	ctx = WithSubnetID(ctx, ids.SubnetID)
	ctx = WithNodeID(ctx, ids.NodeID)
	return context.WithValue(ctx, idsKey, ids)
}

// LuxAssetID retrieves the LUX asset ID from context
func LuxAssetID(ctx context.Context) ids.ID {
	if v := ctx.Value(idsKey); v != nil {
		if ctxIDs, ok := v.(IDs); ok {
			if ctxIDs.LUXAssetID != ids.Empty {
				return ctxIDs.LUXAssetID
			}
			return ctxIDs.XAssetID
		}
	}
	return ids.Empty
}

// Short accessors for common fields

// CID returns the chain ID from context (shorthand)
func CID(ctx context.Context) ids.ID {
	return GetChainID(ctx)
}

// SID returns the subnet ID from context (shorthand) 
func SID(ctx context.Context) ids.ID {
	return GetSubnetID(ctx)
}

// PK returns the public key from context
func PK(ctx context.Context) *bls.PublicKey {
	if v := ctx.Value(idsKey); v != nil {
		if ctxIDs, ok := v.(IDs); ok {
			return ctxIDs.PublicKey
		}
	}
	return nil
}

// MustIDs retrieves the IDs from context or panics
func MustIDs(ctx context.Context) IDs {
	if v := ctx.Value(idsKey); v != nil {
		if ctxIDs, ok := v.(IDs); ok {
			return ctxIDs
		}
	}
	panic("consensus: IDs not found in context")
}

// GetXAssetID retrieves the X-chain asset ID from context
func GetXAssetID(ctx context.Context) ids.ID {
	if v := ctx.Value(idsKey); v != nil {
		if ctxIDs, ok := v.(IDs); ok {
			return ctxIDs.XAssetID
		}
	}
	return ids.Empty
}

// GetLUXAssetID retrieves the LUX asset ID from context (for backward compatibility)
func GetLUXAssetID(ctx context.Context) ids.ID {
	// For backward compatibility, return XAssetID since most code expects the X-chain asset ID
	return GetXAssetID(ctx)
}

// GetLogger retrieves the logger from context
func GetLogger(ctx context.Context) Logger {
	if v := ctx.Value(loggerKey); v != nil {
		if logger, ok := v.(Logger); ok {
			return logger
		}
	}
	// Return a no-op logger if not found
	return NoOpLogger{}
}

// WithLogger adds a logger to the context
func WithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// GetSharedMemory retrieves shared memory from context
func GetSharedMemory(ctx context.Context) interface{} {
	return ctx.Value(sharedMemoryKey)
}

// WithSharedMemory adds shared memory to context
func WithSharedMemory(ctx context.Context, sharedMemory interface{}) context.Context {
	return context.WithValue(ctx, sharedMemoryKey, sharedMemory)
}

// GetBCLookup retrieves the blockchain lookup from context
func GetBCLookup(ctx context.Context) interface{} {
	return ctx.Value(bcLookupKey)
}

// WithBCLookup adds blockchain lookup to context
func WithBCLookup(ctx context.Context, bcLookup interface{}) context.Context {
	return context.WithValue(ctx, bcLookupKey, bcLookup)
}

// GetWarpSigner retrieves the warp signer from context  
func GetWarpSigner(ctx context.Context) interface{} {
	return ctx.Value(warpSignerKey)
}

// WithWarpSigner adds warp signer to context
func WithWarpSigner(ctx context.Context, warpSigner interface{}) context.Context {
	return context.WithValue(ctx, warpSignerKey, warpSigner)
}

// GetChainDataDir retrieves the chain data directory from context
func GetChainDataDir(ctx context.Context) string {
	if v := ctx.Value(chainDataDirKey); v != nil {
		if dir, ok := v.(string); ok {
			return dir
		}
	}
	return ""
}

// WithChainDataDir adds chain data directory to context
func WithChainDataDir(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, chainDataDirKey, dir)
}