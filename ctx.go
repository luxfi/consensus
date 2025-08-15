// Package consensus provides context helpers for consensus operations
package consensus

import (
	"context"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// Context is a type alias for standard context.Context
// This allows call sites to use consensus.Context for clarity
type Context = context.Context

// IDs holds the small immutable identity data carried in context
type IDs struct {
	NetworkID uint32
	SubnetID  ids.ID
	ChainID   ids.ID
	NodeID    ids.NodeID
	PublicKey *bls.PublicKey // optional, keep immutable
	XChainID  ids.ID         // X-Chain ID for asset settlement
	CChainID  ids.ID         // C-Chain ID for contracts
	XAssetID  ids.ID         // Native asset ID on X-Chain
}

// Private typed key to avoid context collisions
type idsKey struct{}

var k idsKey

// WithIDs sets the consensus IDs in the context.
// Call this once at VM init or RPC boundary.
func WithIDs(ctx context.Context, v IDs) context.Context {
	return context.WithValue(ctx, k, v)
}

// idsFrom is an internal accessor for IDs
func idsFrom(ctx context.Context) (IDs, bool) {
	v, ok := ctx.Value(k).(IDs)
	return v, ok
}

// MustIDs retrieves IDs from context, panics if missing.
// Use this in internal code where IDs must be present.
func MustIDs(ctx context.Context) IDs {
	v, ok := idsFrom(ctx)
	if !ok {
		panic("consensus: IDs missing from context")
	}
	return v
}

// GetIDs retrieves IDs from context, returns zero value if missing.
// Use this at boundaries where IDs might not be set yet.
func GetIDs(ctx context.Context) (IDs, bool) {
	return idsFrom(ctx)
}

// Minimal typing getters - panic if IDs missing (use in internal code)

// NID returns the NetworkID from context
func NID(ctx context.Context) uint32 { return MustIDs(ctx).NetworkID }

// SID returns the SubnetID from context
func SID(ctx context.Context) ids.ID { return MustIDs(ctx).SubnetID }

// CID returns the ChainID from context
func CID(ctx context.Context) ids.ID { return MustIDs(ctx).ChainID }

// Node returns the NodeID from context
func Node(ctx context.Context) ids.NodeID { return MustIDs(ctx).NodeID }

// PK returns the PublicKey from context
func PK(ctx context.Context) *bls.PublicKey { return MustIDs(ctx).PublicKey }

// XChain returns the X-Chain ID from context
func XChain(ctx context.Context) ids.ID { return MustIDs(ctx).XChainID }

// CChain returns the C-Chain ID from context
func CChain(ctx context.Context) ids.ID { return MustIDs(ctx).CChainID }

// XAsset returns the native asset ID from context
func XAsset(ctx context.Context) ids.ID { return MustIDs(ctx).XAssetID }

// LogFields returns structured logging fields from context IDs.
// Usage: log.Info("built block", append(LogFields(ctx), "txs", len(txs))...)
func LogFields(ctx context.Context) []any {
	ids, ok := GetIDs(ctx)
	if !ok {
		return nil
	}
	return []any{
		"net", ids.NetworkID,
		"subnet", ids.SubnetID,
		"chain", ids.ChainID,
		"node", ids.NodeID,
	}
}

// Derive creates a new context with updated IDs.
// Use this when you need to modify specific fields.
func Derive(ctx context.Context, modifier func(*IDs)) context.Context {
	ids := MustIDs(ctx)
	modifier(&ids)
	return WithIDs(ctx, ids)
}