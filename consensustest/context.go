// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensustest

import (
	"context"
	"testing"

	"github.com/luxfi/consensus"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

var (
	// PChainID is the actual P-Chain ID (empty ID)
	PChainID = ids.Empty
	
	// CChainID is a test C-Chain ID
	CChainID = ids.GenerateTestID()

	// LUXAssetID is a test LUX asset ID
	LUXAssetID = ids.GenerateTestID()
)

// Context creates a test context with chain IDs
func Context(t testing.TB, chainID ids.ID) context.Context {
	ctx := context.Background()
	
	// Create IDs struct
	// Use PrimaryNetworkID (empty ID) for subnet so chains get indexed
	ids := consensus.IDs{
		NetworkID:  10001,
		SubnetID:   ids.Empty, // PrimaryNetworkID
		ChainID:    chainID,
		NodeID:     ids.GenerateTestNodeID(),
		PublicKey:  &bls.PublicKey{},
		XAssetID:   LUXAssetID,
		LUXAssetID: LUXAssetID,
	}
	
	// Add to context
	ctx = consensus.WithIDs(ctx, ids)
	ctx = consensus.WithLogger(ctx, consensus.NoOpLogger{})
	
	return ctx
}
