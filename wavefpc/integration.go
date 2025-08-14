// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// Integration provides minimal hooks to wire WaveFPC into existing consensus
// This is all you need to add to your existing block builder and consensus engine
type Integration struct {
	fpc    WaveFPC
	log    log.Logger
	nodeID ids.NodeID
	
	// Feature flags
	enabled     bool
	votingEnabled bool
	executionEnabled bool
}

// NewIntegration creates a new FPC integration
func NewIntegration(ctx *interfaces.Context, cfg Config, cls Classifier) *Integration {
	// Get validators from context
	validators := make([]ids.NodeID, 0)
	// TODO: Extract from ctx.ValidatorState
	
	// Create FPC engine
	fpc := New(cfg, cls, nil, nil, ctx.NodeID, validators)
	
	return &Integration{
		fpc:              fpc,
		log:              ctx.Log,
		nodeID:           ctx.NodeID,
		enabled:          false, // Start disabled
		votingEnabled:    false,
		executionEnabled: false,
	}
}

// Enable turns on WaveFPC (can be done via feature flag)
func (i *Integration) Enable() {
	i.enabled = true
	i.log.Info("WaveFPC enabled")
}

// EnableVoting turns on vote inclusion in blocks
func (i *Integration) EnableVoting() {
	i.votingEnabled = true
	i.log.Info("WaveFPC voting enabled")
}

// EnableExecution turns on fast-path execution
func (i *Integration) EnableExecution() {
	i.executionEnabled = true
	i.log.Info("WaveFPC fast execution enabled")
}

// ==== Minimal Integration Points ====

// OnBuildBlock - Call this when building a block
// Adds FPC votes to the block payload
func (i *Integration) OnBuildBlock(block interface{}) {
	if !i.enabled || !i.votingEnabled {
		return
	}
	
	// Get votes to include
	votes := i.fpc.NextVotes(256) // Configurable limit
	
	// Convert to byte slices
	voteBytes := make([][]byte, len(votes))
	for idx, vote := range votes {
		voteBytes[idx] = vote[:]
	}
	
	// Add to block
	// You'll cast block to your actual block type and set:
	// block.Payload.FPCVotes = voteBytes
	// block.Payload.EpochBit = i.isEpochClosing()
}

// OnBlockReceived - Call this when receiving a block via gossip
// Processes FPC votes from the block
func (i *Integration) OnBlockReceived(block interface{}) {
	if !i.enabled {
		return
	}
	
	// Convert to WaveFPC block format
	// You'll extract from your actual block type:
	// b := &Block{
	//     ID:      block.ID(),
	//     Author:  block.Author(),
	//     Round:   block.Round(),
	//     Payload: BlockPayload{
	//         FPCVotes: block.Payload.FPCVotes,
	//         EpochBit: block.Payload.EpochBit,
	//     },
	// }
	// i.fpc.OnBlockObserved(b)
}

// OnBlockAccepted - Call this when a block is accepted by consensus
// Anchors executable transactions to make them final
func (i *Integration) OnBlockAccepted(block interface{}) {
	if !i.enabled {
		return
	}
	
	// Convert and process
	// b := convertBlock(block)
	// i.fpc.OnBlockAccepted(b)
}

// CanExecute - Call this before executing a transaction
// Returns true if the transaction can be executed via fast-path
func (i *Integration) CanExecute(txRef TxRef) bool {
	if !i.enabled || !i.executionEnabled {
		return false
	}
	
	status, _ := i.fpc.Status(txRef)
	return status == Executable || status == Final
}

// MustWaitForFinal - Call this for mixed transactions
// Returns true if the transaction must wait for Final status
func (i *Integration) MustWaitForFinal(txRef TxRef) bool {
	if !i.enabled {
		return true // Conservative: wait for normal consensus
	}
	
	status, _ := i.fpc.Status(txRef)
	return status == Mixed || status == Pending
}

// ==== Epoch Management ====

// StartEpochClose - Call when starting epoch transition
func (i *Integration) StartEpochClose() {
	if i.enabled {
		i.fpc.OnEpochCloseStart()
	}
}

// CompleteEpochClose - Call when epoch transition completes
func (i *Integration) CompleteEpochClose() {
	if i.enabled {
		i.fpc.OnEpochClosed()
	}
}

// ==== Status and Metrics ====

// GetStatus returns the FPC status of a transaction
func (i *Integration) GetStatus(txRef TxRef) (Status, Proof) {
	if !i.enabled {
		return Pending, Proof{}
	}
	
	return i.fpc.Status(txRef)
}

// GetMetrics returns FPC performance metrics
func (i *Integration) GetMetrics() Metrics {
	if !i.enabled {
		return Metrics{}
	}
	
	return i.fpc.GetMetrics()
}

// ==== Example Usage in Your Consensus Engine ====
/*

// In your block builder:
func (e *Engine) BuildBlock() *Block {
    block := &Block{
        // ... your existing fields ...
    }
    
    // Add FPC votes (one line!)
    e.fpcIntegration.OnBuildBlock(block)
    
    return block
}

// In your gossip handler:
func (e *Engine) OnGossipBlock(block *Block) {
    // Your existing validation...
    
    // Process FPC votes (one line!)
    e.fpcIntegration.OnBlockReceived(block)
    
    // Your existing processing...
}

// In your consensus accept:
func (e *Engine) AcceptBlock(block *Block) {
    // Your existing accept logic...
    
    // Anchor FPC transactions (one line!)
    e.fpcIntegration.OnBlockAccepted(block)
}

// In your VM execution:
func (vm *VM) ExecuteTransaction(tx *Transaction) error {
    txRef := tx.Ref()
    
    // Check fast-path eligibility (one line!)
    if vm.fpcIntegration.CanExecute(txRef) {
        return vm.executeOwned(tx)
    }
    
    // Check if must wait for final
    if vm.fpcIntegration.MustWaitForFinal(txRef) {
        return ErrWaitingForConsensus
    }
    
    // Normal execution path
    return vm.executeNormal(tx)
}

*/