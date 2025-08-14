// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package dag2 implements MYSTICETI-style uncertified DAG consensus
// as an evolution of our existing DAG engine, optimized for DEX operations
package dag2

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/engine/dag"
	"github.com/luxfi/consensus/protocol/mysticeti"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
	"github.com/luxfi/metric"
)

// Engine implements MYSTICETI uncertified-DAG consensus
// This is an enhanced version of our DAG engine specifically optimized for:
// - X-Chain high-performance trading
// - Fast-path LUSD transfers  
// - DEX order matching with low latency
type Engine struct {
	// Embed base DAG functionality
	*dag.Engine
	
	// MYSTICETI-specific components
	params      mysticeti.Parameters
	roundClock  *RoundClock
	dagStore    *DAGStore
	decider     *Decider
	builder     *BlockBuilder
	fastPath    *FastPathManager
	epochMgr    *EpochManager
	reputation  *ReputationManager
	
	// Runtime state
	mu          sync.RWMutex
	ctx         *interfaces.Context
	log         log.Logger
	metrics     *Metrics
	
	// Consensus state
	currentRound uint64
	currentEpoch uint64
	validators   map[ids.NodeID]uint64 // nodeID -> stake weight
	
	// Channels for coordination
	proposeCh   chan struct{}
	decideCh    chan SlotDecision
	shutdownCh  chan struct{}
}

// New creates a new MYSTICETI DAG2 engine
func New(ctx *interfaces.Context, params mysticeti.Parameters) (*Engine, error) {
	// Calculate 2f+1 based on validator set
	validatorCount := len(ctx.ValidatorState.GetValidatorSet())
	minParents := uint32((2 * validatorCount) / 3 + 1)
	if params.MinParents == 0 {
		params.MinParents = minParents
	}
	
	// Create base DAG engine for compatibility
	baseDag, err := dag.New(ctx, dag.Parameters{
		K:               int(params.MinParents),
		AlphaPreference: int(params.MinParents),
		AlphaConfidence: int(params.MinParents),
		Beta:            int(params.WaveLength),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create base DAG: %w", err)
	}
	
	e := &Engine{
		Engine:      baseDag,
		params:      params,
		ctx:         ctx,
		log:         ctx.Log,
		validators:  make(map[ids.NodeID]uint64),
		proposeCh:   make(chan struct{}, 1),
		decideCh:    make(chan SlotDecision, 100),
		shutdownCh:  make(chan struct{}),
	}
	
	// Initialize MYSTICETI components
	e.roundClock = NewRoundClock(params.PrimaryTimeoutMS)
	e.dagStore = NewDAGStore()
	e.decider = NewDecider(e.dagStore, params)
	e.builder = NewBlockBuilder(e.dagStore, params)
	e.fastPath = NewFastPathManager(params.EnableFastPath)
	e.epochMgr = NewEpochManager()
	e.reputation = NewReputationManager()
	e.metrics = NewMetrics(ctx.Metrics)
	
	return e, nil
}

// Start begins the MYSTICETI consensus engine
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	// Start base DAG engine
	if err := e.Engine.Start(ctx); err != nil {
		return fmt.Errorf("failed to start base DAG: %w", err)
	}
	
	// Load validator set for current epoch
	if err := e.loadValidatorSet(ctx); err != nil {
		return fmt.Errorf("failed to load validators: %w", err)
	}
	
	// Start MYSTICETI components
	go e.roundClockLoop(ctx)
	go e.proposerLoop(ctx)
	go e.decisionLoop(ctx)
	
	e.log.Info("MYSTICETI DAG2 engine started",
		"epoch", e.currentEpoch,
		"round", e.currentRound,
		"validators", len(e.validators),
		"minParents", e.params.MinParents,
		"slotsPerRound", e.params.NumSlotsPerRound,
	)
	
	return nil
}

// Stop halts the engine
func (e *Engine) Stop(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	close(e.shutdownCh)
	
	// Stop base DAG
	if err := e.Engine.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop base DAG: %w", err)
	}
	
	e.log.Info("MYSTICETI DAG2 engine stopped")
	return nil
}

// ProcessBlock handles incoming blocks
func (e *Engine) ProcessBlock(ctx context.Context, block *mysticeti.Block) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	// Validate block
	if err := e.validateBlock(block); err != nil {
		e.metrics.InvalidBlocks.Inc()
		return fmt.Errorf("invalid block: %w", err)
	}
	
	// Add to DAG store
	if err := e.dagStore.AddBlock(block); err != nil {
		return fmt.Errorf("failed to store block: %w", err)
	}
	
	// Process fast-path votes if enabled
	if e.params.EnableFastPath {
		e.processFastPathVotes(block)
	}
	
	// Try to make decisions
	e.tryDecide()
	
	// Update metrics
	e.metrics.BlocksReceived.Inc()
	e.metrics.CurrentRound.Set(float64(block.Header.Round))
	
	return nil
}

// BuildBlock creates a new block proposal
func (e *Engine) BuildBlock(ctx context.Context) (*mysticeti.Block, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	round := e.roundClock.CurrentRound()
	slotIndex := e.getMySlotIndex(round)
	
	// Select parents (≥ 2f+1 from previous round)
	parents, err := e.builder.SelectParents(round-1, e.ctx.NodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to select parents: %w", err)
	}
	
	// Collect transactions
	txs := e.collectTransactions()
	
	// Collect fast-path votes
	votes := e.collectFastPathVotes()
	
	// Determine flags
	flags := mysticeti.BlockFlags{
		EpochChangeBit: e.epochMgr.ShouldSetEpochChangeBit(round),
	}
	
	// Build block
	block := &mysticeti.Block{
		Header: mysticeti.BlockHeader{
			Version:      1,
			Epoch:        e.currentEpoch,
			Round:        round,
			SlotIndex:    slotIndex,
			Proposer:     e.ctx.NodeID,
			ParentIDs:    parents,
			TxRoot:       e.computeTxRoot(txs),
			FPVoteRoot:   e.computeVoteRoot(votes),
			Flags:        flags,
			WeightCommit: e.getWeightCommit(),
			Timestamp:    time.Now().Unix(),
		},
		Transactions:  txs,
		FastPathVotes: votes,
	}
	
	// Sign block
	if err := e.signBlock(block); err != nil {
		return nil, fmt.Errorf("failed to sign block: %w", err)
	}
	
	e.metrics.BlocksBuilt.Inc()
	return block, nil
}

// validateBlock performs validity checks
func (e *Engine) validateBlock(block *mysticeti.Block) error {
	// 1. Verify signature
	if err := e.verifySignature(block); err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}
	
	// 2. Check author is in current epoch set
	if _, ok := e.validators[block.Header.Proposer]; !ok {
		return fmt.Errorf("proposer not in validator set")
	}
	
	// 3. Verify parent requirements
	if len(block.Header.ParentIDs) < int(e.params.MinParents) {
		return fmt.Errorf("insufficient parents: %d < %d", 
			len(block.Header.ParentIDs), e.params.MinParents)
	}
	
	// 4. Check round progression
	for _, parentID := range block.Header.ParentIDs {
		parent, err := e.dagStore.GetBlock(parentID)
		if err != nil {
			return fmt.Errorf("parent not found: %s", parentID)
		}
		if parent.Header.Round != block.Header.Round-1 {
			return fmt.Errorf("invalid parent round: %d != %d-1", 
				parent.Header.Round, block.Header.Round)
		}
	}
	
	// 5. Check first parent is proposer's previous block
	if len(block.Header.ParentIDs) > 0 {
		firstParent, _ := e.dagStore.GetBlock(block.Header.ParentIDs[0])
		if firstParent != nil && firstParent.Header.Proposer != block.Header.Proposer {
			// Find proposer's previous block
			prevBlock := e.dagStore.GetProposerPreviousBlock(block.Header.Proposer, block.Header.Round-1)
			if prevBlock != nil && prevBlock.ID() != block.Header.ParentIDs[0] {
				return fmt.Errorf("first parent must be proposer's previous block")
			}
		}
	}
	
	// 6. No equivocation
	if existing := e.dagStore.GetBlockByAuthorRound(block.Header.Proposer, block.Header.Round); existing != nil {
		if existing.ID() != block.ID() {
			return fmt.Errorf("equivocation detected for %s at round %d", 
				block.Header.Proposer, block.Header.Round)
		}
	}
	
	return nil
}

// tryDecide attempts to make consensus decisions
func (e *Engine) tryDecide() {
	decisions := e.decider.TryDecide(e.currentRound)
	for _, decision := range decisions {
		select {
		case e.decideCh <- decision:
		default:
			// Channel full, log and continue
			e.log.Warn("decision channel full", "slot", decision.Slot)
		}
	}
}

// processFastPathVotes handles fast-path votes in a block
func (e *Engine) processFastPathVotes(block *mysticeti.Block) {
	for _, vote := range block.FastPathVotes {
		e.fastPath.AddVote(block.Header.Proposer, vote)
		
		// Check if we can execute speculatively
		if e.fastPath.CanExecute(vote.TxID) {
			e.metrics.FastPathExecutions.Inc()
			// Notify VM for speculative execution
			// e.vm.ExecuteOwnedFastPath(vote.TxID)
		}
	}
}

// roundClockLoop manages round progression
func (e *Engine) roundClockLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(e.params.PrimaryTimeoutMS) * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.shutdownCh:
			return
		case <-ticker.C:
			e.mu.Lock()
			newRound := e.roundClock.Tick()
			if newRound > e.currentRound {
				e.currentRound = newRound
				e.metrics.CurrentRound.Set(float64(newRound))
				// Trigger block proposal
				select {
				case e.proposeCh <- struct{}{}:
				default:
				}
			}
			e.mu.Unlock()
		}
	}
}

// proposerLoop handles block proposals
func (e *Engine) proposerLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.shutdownCh:
			return
		case <-e.proposeCh:
			if e.shouldPropose() {
				block, err := e.BuildBlock(ctx)
				if err != nil {
					e.log.Error("failed to build block", "error", err)
					continue
				}
				if err := e.ProcessBlock(ctx, block); err != nil {
					e.log.Error("failed to process own block", "error", err)
				}
				// Broadcast block
				e.broadcastBlock(block)
			}
		}
	}
}

// decisionLoop handles consensus decisions
func (e *Engine) decisionLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.shutdownCh:
			return
		case decision := <-e.decideCh:
			e.handleDecision(decision)
		}
	}
}

// Helper methods

func (e *Engine) loadValidatorSet(ctx context.Context) error {
	// Load from validator state
	// This would integrate with the existing validator manager
	// For now, using placeholder
	e.validators[e.ctx.NodeID] = 1000 // Example weight
	return nil
}

func (e *Engine) getMySlotIndex(round uint64) uint16 {
	// Determine our slot index in this round
	// Uses reputation manager for ordering
	return e.reputation.GetSlotIndex(e.ctx.NodeID, round, e.params.NumSlotsPerRound)
}

func (e *Engine) shouldPropose() bool {
	// Check if it's our turn to propose
	round := e.currentRound
	slotIndex := e.getMySlotIndex(round)
	return e.reputation.IsMyTurn(e.ctx.NodeID, round, slotIndex)
}

func (e *Engine) collectTransactions() []mysticeti.Transaction {
	// Collect transactions from mempool
	// Prioritize by type and fee
	return nil // TODO: Implement
}

func (e *Engine) collectFastPathVotes() []mysticeti.FPVote {
	// Collect votes for owned-object transactions
	return e.fastPath.CollectVotes()
}

func (e *Engine) computeTxRoot(txs []mysticeti.Transaction) ids.ID {
	// Compute Merkle root of transactions
	return ids.Empty // TODO: Implement
}

func (e *Engine) computeVoteRoot(votes []mysticeti.FPVote) ids.ID {
	// Compute Merkle root of votes
	return ids.Empty // TODO: Implement
}

func (e *Engine) getWeightCommit() uint64 {
	// Get total stake weight for epoch
	var total uint64
	for _, weight := range e.validators {
		total += weight
	}
	return total
}

func (e *Engine) signBlock(block *mysticeti.Block) error {
	// Sign with node's private key
	// block.Signature = e.signer.Sign(block.Bytes())
	return nil // TODO: Implement
}

func (e *Engine) verifySignature(block *mysticeti.Block) error {
	// Verify block signature
	return nil // TODO: Implement
}

func (e *Engine) broadcastBlock(block *mysticeti.Block) {
	// Broadcast to network
	// This would use the existing network layer
}

func (e *Engine) handleDecision(decision SlotDecision) {
	switch decision.Decision {
	case mysticeti.Commit:
		e.log.Info("committed block", 
			"slot", decision.Slot,
			"block", decision.BlockID)
		e.metrics.CommitCount.Inc()
		// Apply to VM
		// e.vm.ApplyBlock(decision.Block)
	case mysticeti.Skip:
		e.log.Debug("skipped slot", "slot", decision.Slot)
		e.metrics.SkipCount.Inc()
	}
}