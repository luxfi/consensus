// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensustest

import (
	"testing"

	"github.com/luxfi/consensus"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

var (
	// PChainID is a test P-Chain ID
	PChainID = ids.GenerateTestID()

	// LUXAssetID is a test LUX asset ID
	LUXAssetID = ids.GenerateTestID()
)

// Context creates a test context with chain IDs
func Context(t testing.TB, chainID ids.ID) *consensus.Context {
	return &consensus.Context{
		NetworkID:  10001,
		SubnetID:   ids.GenerateTestID(),
		ChainID:    chainID,
		NodeID:     ids.GenerateTestNodeID(),
		PublicKey:  &bls.PublicKey{},
		XAssetID:   LUXAssetID,
		LUXAssetID: LUXAssetID,
		Log:        consensus.NoOpLogger{},
	}
}
