// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package consensustest

import (
	"context"
	"testing"

	"github.com/luxfi/consensus"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

var (
	PChainID = ids.GenerateTestID()
	XChainID = ids.GenerateTestID()
	CChainID = ids.GenerateTestID()
	XAssetID = ids.GenerateTestID()
)

// SimpleValidatorState is a minimal validator state for testing
type SimpleValidatorState struct{}

func (s *SimpleValidatorState) GetChainID(chainID ids.ID) (ids.ID, error) {
	return chainID, nil
}

func (s *SimpleValidatorState) GetNetID(chainID ids.ID) (ids.ID, error) {
	return ids.Empty, nil
}

func (s *SimpleValidatorState) GetSubnetID(chainID ids.ID) (ids.ID, error) {
	return ids.Empty, nil
}

func (s *SimpleValidatorState) GetValidatorSet(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error) {
	return map[ids.NodeID]uint64{}, nil
}

func (s *SimpleValidatorState) GetCurrentHeight() (uint64, error) {
	return 0, nil
}

func (s *SimpleValidatorState) GetMinimumHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}

// ConsensusContext updates a consensus context with default test values
func ConsensusContext(ctx *consensus.Context) *consensus.Context {
	// Simple consensus package doesn't have these fields, just return the context as-is
	return ctx
}

// NewContext creates a new consensus context for testing
// This is a compatibility function that creates a context with a generated chain ID
func NewContext(tb testing.TB) *consensus.Context {
	return Context(tb, ids.GenerateTestID())
}

// Context creates a new consensus context for testing with a specific chain ID
func Context(tb testing.TB, chainID ids.ID) *consensus.Context {
	tb.Helper()

	ctx := &consensus.Context{
		QuantumID:  1,
		NetworkID:  1,
		ChainID:    chainID,
		NodeID:     ids.GenerateTestNodeID(),
		NetID:      ids.Empty,
		XChainID:   XChainID,
		CChainID:   CChainID,
		XAssetID:   XAssetID,
		LUXAssetID: XAssetID, // Use XAssetID as default LUX asset
	}

	// Set up a simple validator state
	ctx.ValidatorState = &SimpleValidatorState{}

	// Set up a no-op logger for tests
	ctx.Log = log.NoLog{}

	return ctx
}
