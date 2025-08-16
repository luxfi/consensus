// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

import (
	"context"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// Context is a type alias for standard context - use this for cleaner call sites
type Context = context.Context

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

// IDs contains small immutable identity info carried in context
type IDs struct {
	NetworkID   uint32
	SubnetID    ids.ID
	ChainID     ids.ID
	NodeID      ids.NodeID
	PublicKey   *bls.PublicKey
	XAssetID    ids.ID // The asset ID of LUX on the X-chain
	LUXAssetID  ids.ID // The native LUX token asset ID
}

// Private typed keys to avoid collisions
type (
	idsKey  struct{}
	peerKey struct{}
	logKey  struct{}
)

// WithIDs sets the chain IDs in the context
func WithIDs(ctx context.Context, v IDs) context.Context {
	return context.WithValue(ctx, idsKey{}, v)
}

// MustIDs panics if IDs are missing (fail fast)
func MustIDs(ctx context.Context) IDs {
	v, ok := ctx.Value(idsKey{}).(IDs)
	if !ok {
		panic("consensus: IDs missing from context")
	}
	return v
}

// Short accessors for minimal typing at call sites
func NID(ctx context.Context) uint32        { return MustIDs(ctx).NetworkID }
func SID(ctx context.Context) ids.ID        { return MustIDs(ctx).SubnetID }
func CID(ctx context.Context) ids.ID        { return MustIDs(ctx).ChainID }
func Node(ctx context.Context) ids.NodeID   { return MustIDs(ctx).NodeID }
func PK(ctx context.Context) *bls.PublicKey { return MustIDs(ctx).PublicKey }

// WithPeerChainID attaches a peer chain ID for cross-chain ops
func WithPeerChainID(ctx context.Context, peer ids.ID) context.Context {
	return context.WithValue(ctx, peerKey{}, peer)
}

// MustPeerCID gets peer chain ID (panics if missing)
func MustPeerCID(ctx context.Context) ids.ID {
	v, ok := ctx.Value(peerKey{}).(ids.ID)
	if !ok {
		panic("peer chain ID missing")
	}
	return v
}

// GetChainID returns the ChainID
func GetChainID(ctx context.Context) ids.ID {
	if ids, ok := ctx.Value(idsKey{}).(IDs); ok {
		return ids.ChainID
	}
	return ids.Empty
}

// GetNodeID returns the NodeID
func GetNodeID(ctx context.Context) ids.NodeID {
	if ids, ok := ctx.Value(idsKey{}).(IDs); ok {
		return ids.NodeID
	}
	return ids.EmptyNodeID
}

// GetNetworkID returns the NetworkID
func GetNetworkID(ctx context.Context) uint32 {
	if ids, ok := ctx.Value(idsKey{}).(IDs); ok {
		return ids.NetworkID
	}
	return 0
}

// GetSubnetID returns the SubnetID from context
func GetSubnetID(ctx context.Context) ids.ID {
	if ids, ok := ctx.Value(idsKey{}).(IDs); ok {
		return ids.SubnetID
	}
	return ids.Empty
}

// GetXAssetID returns the LUX asset ID on the X-chain
func GetXAssetID(ctx context.Context) ids.ID {
	if ids, ok := ctx.Value(idsKey{}).(IDs); ok {
		return ids.XAssetID
	}
	return ids.Empty
}

// GetLUXAssetID returns the LUX asset ID (for backward compatibility, returns X-chain asset ID)
func GetLUXAssetID(ctx context.Context) ids.ID {
	if ids, ok := ctx.Value(idsKey{}).(IDs); ok {
		// For backward compatibility, return XAssetID when LUXAssetID is requested
		// since most code expects the X-chain asset ID
		if ids.XAssetID != (ids.ID{}) {
			return ids.XAssetID
		}
		return ids.LUXAssetID
	}
	return ids.Empty
}

// LuxAssetID is an alias for GetLUXAssetID for backward compatibility
func LuxAssetID(ctx context.Context) ids.ID {
	return GetLUXAssetID(ctx)
}

// GetLogger extracts the logger from context
func GetLogger(ctx context.Context) Logger {
	if logger, ok := ctx.Value(logKey{}).(Logger); ok {
		return logger
	}
	// Return a no-op logger if not found
	return NoOpLogger{}
}

// validatorStateKey is the key for ValidatorState in context
type validatorStateKey struct{}

// WithValidatorState attaches a ValidatorState to context (temporary during migration)
func WithValidatorState(ctx context.Context, vs ValidatorState) context.Context {
	return context.WithValue(ctx, validatorStateKey{}, vs)
}

// GetValidatorState retrieves ValidatorState from context
func GetValidatorState(ctx context.Context) ValidatorState {
	if vs, ok := ctx.Value(validatorStateKey{}).(ValidatorState); ok {
		return vs
	}
	return nil
}
