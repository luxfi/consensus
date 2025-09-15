// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensustest

import (
	"context"
	"testing"

	"github.com/luxfi/consensus"
	"github.com/luxfi/consensus/validators"
	"github.com/luxfi/ids"
)

// SimpleContext creates a simple test context for consensus testing
func SimpleContext(tb testing.TB) *consensus.Context {
	tb.Helper()
	
	ctx := &consensus.Context{
		NetworkID: 1,
		ChainID:   ids.GenerateTestID(),
		NodeID:    ids.GenerateTestNodeID(),
	}
	
	// Set up a simple validator state
	ctx.ValidatorState = &SimpleValidatorState{}
	
	return ctx
}

// SimpleValidatorState is a minimal validator state for testing
type SimpleValidatorState struct{}

func (s *SimpleValidatorState) GetValidatorSet(ctx context.Context, height uint64, subnetID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	return map[ids.NodeID]*validators.GetValidatorOutput{}, nil
}

func (s *SimpleValidatorState) GetCurrentValidators(ctx context.Context, subnetID ids.ID) ([]validators.GetValidatorOutput, error) {
	return []validators.GetValidatorOutput{}, nil
}

func (s *SimpleValidatorState) GetSubnetID(ctx context.Context, chainID ids.ID) (ids.ID, error) {
	return ids.Empty, nil
}

func (s *SimpleValidatorState) GetMinimumHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}