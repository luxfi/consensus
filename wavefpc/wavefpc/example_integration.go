// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"context"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// ExampleEngineIntegration shows how to wire WaveFPC into your consensus engine
type ExampleEngineIntegration struct {
	// Your existing engine fields...

	// Add FPC integration
	fpc *Integration
}

// InitializeFPC shows how to initialize the FPC subsystem
func (e *ExampleEngineIntegration) InitializeFPC(
	params config.Parameters,
	nodeIDs []ids.NodeID,
	myNodeID ids.NodeID,
) {
	if !params.EnableFPC {
		return
	}

	// Find my validator index
	var myIndex ValidatorIndex
	found := false
	for i, nodeID := range nodeIDs {
		if nodeID == myNodeID {
			myIndex = ValidatorIndex(i)
			found = true
			break
		}
	}

	if !found {
		// Not a validator, can't participate in FPC
		return
	}

	// Create committee adapter
	committee := NewValidatorCommitteeAdapter(nodeIDs)

	// Create classifier (you provide actual implementation)
	classifier := &ExampleClassifier{}

	// Create DAG tap (you provide actual implementation)
	dagTap := NewSimpleDAGTap(e.getBlock)

	// Create FPC config
	fpcConfig := &Config{
		Quorum:            Quorum{N: len(nodeIDs), F: (len(nodeIDs) - 1) / 3},
		Epoch:             0, // Set current epoch
		VoteLimitPerBlock: params.FPCVoteLimit,
		VotePrefix:        params.FPCVotePrefix,
	}

	// Create integration
	e.fpc = NewIntegration(
		fpcConfig,
		committee,
		myIndex,
		classifier,
		dagTap,
		nil, // PQ engine (optional)
		nil, // Candidate source (optional)
	)
}

// OnBuildBlock shows how to add votes when building a block
func (e *ExampleEngineIntegration) OnBuildBlock(ctx context.Context) {
	if e.fpc == nil || !e.fpc.IsEnabled() {
		return
	}

	// Get votes for this block
	votes := e.fpc.NextVotes(256) // or params.FPCVoteLimit
	_ = votes                     // Use votes in your block building

	// Add to block payload (pseudo-code)
	// block.Payload.FPCVotes = FromTxRefs(votes)
	// block.Payload.EpochBit = e.fpc.IsEpochClosing()
}

// OnBlockGossiped shows how to process observed blocks
func (e *ExampleEngineIntegration) OnBlockGossiped(blk BlockWithFPC) {
	if e.fpc == nil {
		return
	}

	// Process votes in the block
	e.fpc.OnBlockObserved(blk)

	// Your existing gossip handling...
}

// OnBlockAccepted shows how to process accepted blocks
func (e *ExampleEngineIntegration) OnBlockAccepted(blk BlockWithFPC) {
	if e.fpc == nil {
		return
	}

	// Update FPC state for accepted block
	e.fpc.OnBlockAccepted(blk)

	// Your existing accept handling...
}

// CheckExecutable shows how to check if a transaction can execute
func (e *ExampleEngineIntegration) CheckExecutable(tx TxRef, isMixed bool) bool {
	if e.fpc == nil || !e.fpc.IsEnabled() {
		// Fall back to regular consensus if FPC disabled
		return e.regularConsensusCheck(tx)
	}

	status, _ := e.fpc.Status(tx)

	if isMixed {
		// Mixed transactions need Final status
		e.fpc.MarkMixed(tx)
		return status == Final
	}

	// Owned-only transactions can execute at Executable status
	return status >= Executable
}

// Helper methods (you implement these)
func (e *ExampleEngineIntegration) getBlock(ctx context.Context, id ids.ID) (BlockWithFPC, error) {
	// Your block fetching logic
	return nil, nil
}

func (e *ExampleEngineIntegration) regularConsensusCheck(tx TxRef) bool {
	// Your existing consensus check
	return false
}

// ExampleClassifier shows what a real classifier might look like
type ExampleClassifier struct {
	// Your transaction/object model
}

func (c *ExampleClassifier) OwnedInputs(tx TxRef) []ObjectID {
	// Parse transaction, return owned object IDs
	// Example: for UTXO, return UTXOs controlled by single key
	// Example: for account model, return account IDs with single owner
	return nil
}

func (c *ExampleClassifier) Conflicts(a, b TxRef) bool {
	// Return true if transactions conflict
	// Example: spend same UTXO, modify same account, etc.
	return false
}
