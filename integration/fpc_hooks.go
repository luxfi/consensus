// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package integration

import (
	"context"
	"sync/atomic"

	"github.com/luxfi/consensus/chain"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/wave/fpc"
	"github.com/luxfi/consensus/witness"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
	"github.com/luxfi/metrics"
)

// FPCIntegration manages FPC integration with the consensus engine
type FPCIntegration struct {
	cfg           config.FPCConfig
	fpcEngine     fpc.Engine
	witnessCache  *witness.Cache
	epochClosing  atomic.Bool
	metricsPrefix string
}

// NewFPCIntegration creates a new FPC integration layer
func NewFPCIntegration(cfg config.FPCConfig, preset config.Preset) *FPCIntegration {
	if !cfg.Enable {
		return nil
	}

	// Create FPC engine with quorum for 21 validators (f=7, need 2f+1=15 for quorum)
	quorum := fpc.Quorum{
		N: preset.CommitteeSize,
		F: (preset.CommitteeSize - 1) / 3, // Byzantine fault tolerance
	}

	fpcCfg := fpc.Config{
		Quorum:            quorum,
		Epoch:             0,
		VoteLimitPerBlock: cfg.VoteLimitPerBlock,
		VotePrefix:        cfg.VotePrefix,
	}

	// Create witness cache for Verkle
	witnessCfg := witness.Policy{
		Mode:     witness.Soft, // Start in soft mode
		MaxBytes: 1 << 20,      // 1 MiB full witness cap
		MaxDelta: 256 << 10,    // 256 KiB delta cap
	}

	witnessCache := witness.NewCache(
		witnessCfg,
		8192,    // Node entries cap
		128<<20, // 128 MiB node budget
	)

	return &FPCIntegration{
		cfg:           cfg,
		fpcEngine:     fpc.New(fpcCfg, fpc.SimpleClassifier{}),
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
	picks := f.fpcEngine.NextVotes(f.cfg.VoteLimitPerBlock)

	// Convert to byte slices for embedding
	votes := make([][]byte, len(picks))
	for i, tx := range picks {
		voteCopy := make([]byte, 32)
		copy(voteCopy, tx[:])
		votes[i] = voteCopy
	}

	// Record metrics
	metrics.GetOrRegisterCounter(f.metricsPrefix+"votes_embedded_total", nil).Inc(int64(len(votes)))

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

	blockRef := fpc.BlockRef{
		ID:       blk.ID(),
		Round:    blk.Height(),
		Author:   "", // Would come from block metadata
		Final:    false,
		EpochBit: blk.EpochBit(),
		FPCVotes: blk.FPCVotes(),
	}

	f.fpcEngine.OnBlockObserved(blockRef)

	// Record metrics
	metrics.GetOrRegisterCounter(f.metricsPrefix+"blocks_observed_total", nil).Inc(1)
}

// OnBlockAccepted is called when a block is consensus-committed
func (f *FPCIntegration) OnBlockAccepted(ctx context.Context, blk chain.Block) {
	if f == nil || !f.cfg.Enable {
		return
	}

	blockRef := fpc.BlockRef{
		ID:       blk.ID(),
		Round:    blk.Height(),
		Author:   "", // Would come from block metadata
		Final:    true,
		EpochBit: blk.EpochBit(),
		FPCVotes: blk.FPCVotes(),
	}

	f.fpcEngine.OnBlockAccepted(blockRef)

	// Store committed root for witness caching
	if stateRoot := extractStateRoot(blk); stateRoot != nil {
		f.witnessCache.PutCommittedRoot(blk.ID(), stateRoot)
	}

	// Record metrics
	metrics.GetOrRegisterCounter(f.metricsPrefix+"blocks_accepted_total", nil).Inc(1)
}

// ValidateWitness checks if witness size is acceptable
func (f *FPCIntegration) ValidateWitness(ctx context.Context, header witness.Header, witnessBytes []byte) bool {
	if f == nil || f.witnessCache == nil {
		return true // No witness validation if not configured
	}

	ok, wsize, _ := f.witnessCache.Validate(header, witnessBytes)

	// Record witness size metrics
	metrics.GetOrRegisterHistogram(f.metricsPrefix+"witness_bytes", nil).Update(int64(wsize))

	return ok
}

// ExecuteOwned executes owned transactions that reached Executable status
func (f *FPCIntegration) ExecuteOwned(ctx context.Context, executor TxExecutor) error {
	if f == nil || !f.cfg.Enable || !f.cfg.ExecuteOwned {
		return nil
	}

	ownedTxs := f.fpcEngine.ExecutableOwned()

	for _, tx := range ownedTxs {
		if err := executor.Execute(ctx, tx[:]); err != nil {
			log.Warn("failed to execute owned tx", "tx", tx, "error", err)
			continue
		}

		// Record execution metrics
		metrics.GetOrRegisterCounter(f.metricsPrefix+"owned_executed_total", nil).Inc(1)
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

	var tx fpc.TxRef
	copy(tx[:], txID)

	status := f.fpcEngine.Status(tx)
	return status == fpc.StatusFinal
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

	var tx fpc.TxRef
	copy(tx[:], txID)
	f.fpcEngine.Enqueue(tx)
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
