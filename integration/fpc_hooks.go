// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package integration

import (
	"context"
	"sync/atomic"

	"github.com/luxfi/consensus/chain"
	"github.com/luxfi/consensus/config"
	// "github.com/luxfi/consensus/types" // TODO: Enable when TxRef is needed
	// "github.com/luxfi/consensus/wave/fpc" // TODO: Enable when FPC is integrated
	"github.com/luxfi/consensus/dag/witness"
	"github.com/luxfi/log"
)

// FPCIntegration manages FPC integration with the consensus engine
type FPCIntegration struct {
	cfg           config.FPCConfig
	// fpcEngine     *fpc.Engine // TODO: Uncomment when FPC engine is ready
	witnessCache  *witness.Cache
	epochClosing  atomic.Bool
	metricsPrefix string
}

// NewFPCIntegration creates a new FPC integration layer
func NewFPCIntegration(cfg config.FPCConfig, committeeSize int) *FPCIntegration {
	if !cfg.Enable {
		return nil
	}

	// Create FPC engine with quorum for validators
	// TODO: Create FPC engine when API is complete
	// fpcCfg := fpc.Config{
	// 	N:                 committeeSize,
	// 	F:                 (committeeSize - 1) / 3, // Byzantine fault tolerance
	// 	Epoch:             0,
	// 	VoteLimitPerBlock: cfg.VoteLimitPerBlock,
	// 	VotePrefix:        cfg.VotePrefix,
	// 	EnableFastPath:    true, // Always enable fast path
	// }

	// Create witness cache for Verkle with proper caps
	witnessCache := witness.NewCache(
		witness.Policy{
			Mode:     witness.RequireFull, // TODO: Update when Soft mode is available
			MaxBytes: 1 << 20,             // 1 MiB full witness cap
		},
		1000,    // node entries cap
		1<<24,   // 16 MiB node budget
	)

	return &FPCIntegration{
		cfg:           cfg,
		// TODO: Update FPC engine creation when API is complete
		// fpcEngine:     fpc.New(fpcCfg, classifier, dagTap, pqEngine, nodeID, peers),
		witnessCache:  witnessCache,
		metricsPrefix: "consensus_fpc_",
	}
}

// OnPropose is called when proposing a new block
func (f *FPCIntegration) OnPropose(ctx context.Context) ProposalData {
	if f == nil || !f.cfg.Enable {
		return ProposalData{}
	}

	// Get next batch of transactions to vote on
	// TODO: Enable when FPC engine is integrated
	// picks := f.fpcEngine.NextVotes(f.cfg.VoteLimitPerBlock)
	picks := [][32]byte{}

	// Convert to byte slices for embedding
	votes := make([][]byte, len(picks))
	for i, tx := range picks {
		voteCopy := make([]byte, 32)
		copy(voteCopy, tx[:])
		votes[i] = voteCopy
	}

	// Record metrics - simplified without metrics registry for now
	// TODO: Add proper metrics once registry is available

	return ProposalData{
		FPCVotes: votes,
		EpochBit: f.epochClosing.Load(),
	}
}

// OnBlockObserved is called when a block is observed (gossip/receive)
func (f *FPCIntegration) OnBlockObserved(ctx context.Context, blk chain.Block) {
	if f == nil || !f.cfg.Enable {
		return
	}

	// TODO: Enable when FPC engine and BlockRef are available
	// blockRef := &fpc.BlockRef{
	// 	ID:       types.BlockID(blk.ID()),
	// 	Round:    blk.Height(),
	// 	Author:   types.NodeID{}, // Would come from block metadata
	// 	Final:    false,
	// 	EpochBit: blk.EpochBit(),
	// 	FPCVotes: blk.FPCVotes(),
	// }
	// f.fpcEngine.OnBlockObserved(ctx, blockRef)
	_ = blk

	// Record metrics - simplified without metrics registry for now
	// TODO: Add proper metrics once registry is available
}

// OnBlockAccepted is called when a block is consensus-committed
func (f *FPCIntegration) OnBlockAccepted(ctx context.Context, blk chain.Block) {
	if f == nil || !f.cfg.Enable {
		return
	}

	// TODO: Enable when FPC engine and BlockRef are available
	// blockRef := &fpc.BlockRef{
	// 	ID:       types.BlockID(blk.ID()),
	// 	Round:    blk.Height(),
	// 	Author:   types.NodeID{}, // Would come from block metadata
	// 	Final:    true,
	// 	EpochBit: blk.EpochBit(),
	// 	FPCVotes: blk.FPCVotes(),
	// }
	// f.fpcEngine.OnBlockAccepted(ctx, blockRef)
	_ = blk

	// Store committed root for witness caching
	if stateRoot := extractStateRoot(blk); stateRoot != nil {
		// Convert stateRoot to [32]byte
		var root [32]byte
		copy(root[:], stateRoot)
		f.witnessCache.PutCommittedRoot(blk.ID(), root)
	}

	// Record metrics - simplified without metrics registry for now
	// TODO: Add proper metrics once registry is available
}

// ValidateWitness checks if witness size is acceptable
// TODO: Update when witness.Header is available
// func (f *FPCIntegration) ValidateWitness(ctx context.Context, header witness.Header, witnessBytes []byte) bool {
// 	if f == nil || f.witnessCache == nil {
// 		return true // No witness validation if not configured
// 	}
//
// 	ok, wsize, _ := f.witnessCache.Validate(header, witnessBytes)
//
// 	// Record witness size metrics - simplified without metrics registry for now
// 	// TODO: Add proper metrics once registry is available
// 	_ = wsize // Avoid unused variable warning
//
// 	return ok
// }

// ExecuteOwned executes owned transactions that reached Executable status
func (f *FPCIntegration) ExecuteOwned(ctx context.Context, executor TxExecutor) error {
	if f == nil || !f.cfg.Enable || !f.cfg.ExecuteOwned {
		return nil
	}

	// TODO: Enable when FPC engine is integrated
	// ownedTxs := f.fpcEngine.ExecutableOwned()
	ownedTxs := [][32]byte{}

	for _, tx := range ownedTxs {
		if err := executor.Execute(ctx, tx[:]); err != nil {
			log.Warn("failed to execute owned tx", "tx", tx, "error", err)
			continue
		}

		// Record execution metrics - simplified without metrics registry for now
		// TODO: Add proper metrics once registry is available
	}

	return nil
}

// ShouldExecuteMixed checks if a mixed transaction should execute
func (f *FPCIntegration) ShouldExecuteMixed(txID []byte) bool {
	if f == nil || !f.cfg.Enable || !f.cfg.ExecuteMixedOnFinal {
		return false // Default to not executing if FPC disabled
	}

	if len(txID) != 32 {
		return false
	}

	var tx [32]byte
	copy(tx[:], txID)

	// TODO: Enable when FPC engine is integrated
	// status, _ := f.fpcEngine.Status(tx)
	// return status == fpc.StatusFinal
	_ = tx
	return false
}

// SetEpochClosing marks that epoch is closing (enables epoch fence)
func (f *FPCIntegration) SetEpochClosing(closing bool) {
	if f != nil {
		f.epochClosing.Store(closing)
	}
}

// EnqueueTransaction adds a transaction to FPC queue
func (f *FPCIntegration) EnqueueTransaction(txID []byte) {
	if f == nil || !f.cfg.Enable || len(txID) != 32 {
		return
	}

	// Transaction enqueueing would be handled through NextVotes mechanism
	// This is a placeholder for future implementation
}

// ProposalData contains data to include in block proposal
type ProposalData struct {
	FPCVotes [][]byte
	EpochBit bool
}

// TxExecutor executes transactions
type TxExecutor interface {
	Execute(ctx context.Context, txID []byte) error
}

// extractStateRoot extracts state root from block (implementation-specific)
func extractStateRoot(blk chain.Block) []byte {
	// In production, this would extract the actual state root from block
	// For now, return nil to indicate no state root available
	return nil
}
