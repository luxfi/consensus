// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

import (
	consensusctx "github.com/luxfi/consensus/context"
)

// ContextInitializable can be initialized with context
type ContextInitializable interface {
	InitCtx(*consensusctx.Context)
}

// Contextualizable can be contextualized
type Contextualizable interface {
	InitializeContext(*consensusctx.Context) error
}

// XAssetID returns the ID of the X-Chain native asset
func XAssetID(ctx *consensusctx.Context) interface{} {
	if ctx != nil {
		return ctx.XAssetID
	}
	return nil
}

// QuantumNetworkID returns the quantum network ID from context
func QuantumNetworkID(ctx *consensusctx.Context) uint32 {
	if ctx != nil {
		return ctx.QuantumID
	}
	return 0
}

// GetQuantumIDs retrieves QuantumIDs from context
func GetQuantumIDs(ctx *consensusctx.Context) *QuantumIDs {
	if ctx != nil {
		return &QuantumIDs{
			QuantumID: ctx.QuantumID,
			NetworkID: ctx.NetworkID,
		}
	}
	return nil
}
