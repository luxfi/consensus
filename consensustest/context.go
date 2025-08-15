// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensustest

import (
	"context"
	"testing"

	"github.com/luxfi/consensus"
	"github.com/luxfi/ids"
)

// Context returns a test consensus context with IDs set
func Context(t testing.TB, chainID ids.ID) context.Context {
	ctx := context.Background()
	return consensus.WithIDs(ctx, consensus.IDs{
		NetworkID: 1,
		SubnetID:  ids.Empty,
		ChainID:   chainID,
		NodeID:    ids.GenerateTestNodeID(),
		XAssetID:  ids.GenerateTestID(),
		XChainID:  ids.GenerateTestID(),
		CChainID:  ids.GenerateTestID(),
	})
}