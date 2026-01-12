// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package consensustest

import (
	"context"
	"testing"

	"github.com/luxfi/consensus"
	validators "github.com/luxfi/consensus/validator"
	"github.com/luxfi/ids"
	log "github.com/luxfi/log"
)

var (
	PChainID = ids.GenerateTestID()
	XChainID = ids.GenerateTestID()
	CChainID = ids.GenerateTestID()
	// Use fixed asset ID to match genesistest.LUXAssetID for UTXO consistency
	XAssetID = ids.ID{'l', 'u', 'x', ' ', 'a', 's', 's', 'e', 't', ' ', 'i', 'd'}
)

// SimpleValidatorState is a minimal validator state for testing
type SimpleValidatorState struct{}

// GetValidatorSet returns an empty validator set for testing
func (s *SimpleValidatorState) GetValidatorSet(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	return map[ids.NodeID]*validators.GetValidatorOutput{}, nil
}

// GetCurrentValidators returns an empty validator set for testing
func (s *SimpleValidatorState) GetCurrentValidators(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	return map[ids.NodeID]*validators.GetValidatorOutput{}, nil
}

// GetCurrentHeight returns height 0 for testing
func (s *SimpleValidatorState) GetCurrentHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}

// GetWarpValidatorSets returns empty warp validator sets for testing
func (s *SimpleValidatorState) GetWarpValidatorSets(ctx context.Context, heights []uint64, netIDs []ids.ID) (map[ids.ID]map[uint64]*validators.WarpSet, error) {
	return map[ids.ID]map[uint64]*validators.WarpSet{}, nil
}

// GetWarpValidatorSet returns an empty warp validator set for testing
func (s *SimpleValidatorState) GetWarpValidatorSet(ctx context.Context, height uint64, netID ids.ID) (*validators.WarpSet, error) {
	return &validators.WarpSet{
		Height:     height,
		Validators: map[ids.NodeID]*validators.WarpValidator{},
	}, nil
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
		NetworkID: 1, // Mainnet (primary network)
		ChainID:   chainID,
		NodeID:    ids.GenerateTestNodeID(),
		XChainID:  XChainID,
		CChainID:  CChainID,
		XAssetID:  XAssetID,
	}

	// Set up a simple validator state
	ctx.ValidatorState = &SimpleValidatorState{}

	// Set up a no-op logger for tests
	ctx.Log = log.NoLog{}

	return ctx
}
