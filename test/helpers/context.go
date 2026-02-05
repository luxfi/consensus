// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package consensustest

import (
	"context"
	"testing"

	consensusruntime "github.com/luxfi/consensus/runtime"
	validators "github.com/luxfi/consensus/validator"
	"github.com/luxfi/ids"
	log "github.com/luxfi/log"
	"github.com/luxfi/runtime"
	luxvalidators "github.com/luxfi/validators"
)

var (
	PChainID = ids.GenerateTestID()
	XChainID = ids.GenerateTestID()
	CChainID = ids.GenerateTestID()
	// Use fixed asset ID to match genesistest.LUXAssetID for UTXO consistency
	XAssetID = ids.ID{'l', 'u', 'x', ' ', 'a', 's', 's', 'e', 't', ' ', 'i', 'd'}
)

// SimpleValidatorState is a minimal validator state for testing
// Implements validators.State interface
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

// GetMinimumHeight returns height 0 for testing
func (s *SimpleValidatorState) GetMinimumHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}

// GetChainID returns the provided ID as the chain ID for testing
func (s *SimpleValidatorState) GetChainID(netID ids.ID) (ids.ID, error) {
	return netID, nil
}

// GetNetworkID returns the primary network ID for testing.
// All standard chains (P-Chain, X-Chain, C-Chain) belong to the primary network.
func (s *SimpleValidatorState) GetNetworkID(chainID ids.ID) (ids.ID, error) {
	return ids.Empty, nil
}

// Verify SimpleValidatorState implements validators.State
var _ validators.State = (*SimpleValidatorState)(nil)

// RuntimeValidatorState implements luxvalidators.State for the base runtime package.
// This is used for tests that need *runtime.Runtime instead of *consensusruntime.Runtime.
type RuntimeValidatorState struct{}

// GetValidatorSet returns an empty validator set for testing
func (s *RuntimeValidatorState) GetValidatorSet(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*luxvalidators.GetValidatorOutput, error) {
	return map[ids.NodeID]*luxvalidators.GetValidatorOutput{}, nil
}

// GetCurrentValidators returns an empty validator set for testing
func (s *RuntimeValidatorState) GetCurrentValidators(ctx context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*luxvalidators.GetValidatorOutput, error) {
	return map[ids.NodeID]*luxvalidators.GetValidatorOutput{}, nil
}

// GetCurrentHeight returns height 0 for testing
func (s *RuntimeValidatorState) GetCurrentHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}

// GetMinimumHeight returns height 0 for testing
func (s *RuntimeValidatorState) GetMinimumHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}

// GetChainID returns the provided ID for testing
func (s *RuntimeValidatorState) GetChainID(netID ids.ID) (ids.ID, error) {
	return netID, nil
}

// GetNetworkID returns the primary network ID for testing.
// All standard chains (P-Chain, X-Chain, C-Chain) belong to the primary network.
func (s *RuntimeValidatorState) GetNetworkID(chainID ids.ID) (ids.ID, error) {
	return ids.Empty, nil
}

// GetWarpValidatorSets returns empty Warp validator sets for testing
func (s *RuntimeValidatorState) GetWarpValidatorSets(ctx context.Context, heights []uint64, netIDs []ids.ID) (map[ids.ID]map[uint64]*luxvalidators.WarpSet, error) {
	return map[ids.ID]map[uint64]*luxvalidators.WarpSet{}, nil
}

// GetWarpValidatorSet returns an empty Warp validator set for testing
func (s *RuntimeValidatorState) GetWarpValidatorSet(ctx context.Context, height uint64, netID ids.ID) (*luxvalidators.WarpSet, error) {
	return &luxvalidators.WarpSet{
		Height:     height,
		Validators: map[ids.NodeID]*luxvalidators.WarpValidator{},
	}, nil
}

// Verify RuntimeValidatorState implements luxvalidators.State
var _ luxvalidators.State = (*RuntimeValidatorState)(nil)

// ConsensusContext updates a consensus context with default test values
func ConsensusContext(ctx *consensusruntime.Runtime) *consensusruntime.Runtime {
	// Simple consensus package doesn't have these fields, just return the context as-is
	return ctx
}

// NewContext creates a new consensus context for testing
// This is a compatibility function that creates a context with a generated chain ID
func NewContext(tb testing.TB) *consensusruntime.Runtime {
	return Context(tb, ids.GenerateTestID())
}

// Context creates a new consensus runtime for testing with a specific chain ID.
// This returns the consensus-specific runtime type for internal consensus testing.
func Context(tb testing.TB, chainID ids.ID) *consensusruntime.Runtime {
	tb.Helper()

	ctx := &consensusruntime.Runtime{
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

// Runtime creates a base runtime.Runtime for testing with a specific chain ID.
// This returns the base runtime type that VMs expect.
func Runtime(tb testing.TB, chainID ids.ID) *runtime.Runtime {
	tb.Helper()

	rt := &runtime.Runtime{
		NetworkID: 1, // Mainnet (primary network)
		ChainID:   chainID,
		NodeID:    ids.GenerateTestNodeID(),
		XChainID:  XChainID,
		CChainID:  CChainID,
		XAssetID:  XAssetID,
	}

	// Set up a simple validator state
	rt.ValidatorState = &RuntimeValidatorState{}

	// Set up a no-op logger for tests
	rt.Log = log.NoLog{}

	return rt
}

// NewRuntime creates a new runtime.Runtime for testing with a generated chain ID.
func NewRuntime(tb testing.TB) *runtime.Runtime {
	return Runtime(tb, ids.GenerateTestID())
}
