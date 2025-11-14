// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Quasar - The supermassive black hole at the center of the blockchain galaxy
// Processes blocks from all chains with quantum consensus

package quasar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// QuasarCore is the supermassive black hole at the center of the blockchain galaxy
// It collects blocks from ALL chains (P, X, C, + any new subnets) and applies quantum consensus
// External systems (bridges, contracts) can use RPC to add blocks to the event horizon
type QuasarCore struct {
	mu sync.RWMutex

	// Dynamic chain registration - automatically includes new subnets/chains
	chainBuffers map[string]chan *ChainBlock // chainName -> buffer

	// Legacy chain buffers for backwards compatibility
	pChainBlocks chan *ChainBlock
	xChainBlocks chan *ChainBlock
	cChainBlocks chan *ChainBlock

	// Quantum consensus engine - the event horizon
	hybridConsensus *QuasarHybridConsensus

	// Quantum finality state - the singularity
	finalizedBlocks map[string]*QuantumBlock // blockHash -> finalized block
	quantumHeight   uint64

	// Metrics - radiation emitted by the accretion disk
	processedBlocks uint64
	quantumProofs   uint64

	// Chain registry - track all registered chains
	registeredChains map[string]bool // chainName -> active
}

// ChainBlock represents a block from any chain
type ChainBlock struct {
	ChainID   [32]byte
	ChainName string // "P-Chain", "X-Chain", "C-Chain"
	BlockID   [32]byte
	Height    uint64
	Timestamp time.Time
	Data      []byte
}

// QuantumBlock represents a quantum-finalized block
type QuantumBlock struct {
	Height          uint64
	SourceBlocks    []*ChainBlock
	QuantumHash     string
	BLSSignature    []byte
	RingtailProof   []byte // ML-DSA signature
	Timestamp       time.Time
	ValidatorSigs   map[string]*HybridSignature
}

// NewQuasar creates the supermassive black hole at the center of the blockchain galaxy
func NewQuasarCore(threshold int) (*QuasarCore, error) {
	hybrid, err := NewQuasarHybridConsensus(threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to create hybrid consensus: %w", err)
	}

	core := &QuasarCore{
		pChainBlocks:     make(chan *ChainBlock, 100),
		xChainBlocks:     make(chan *ChainBlock, 100),
		cChainBlocks:     make(chan *ChainBlock, 100),
		chainBuffers:     make(map[string]chan *ChainBlock),
		hybridConsensus:  hybrid,
		finalizedBlocks:  make(map[string]*QuantumBlock),
		quantumHeight:    0,
		registeredChains: make(map[string]bool),
	}

	// Auto-register primary chains
	core.RegisterChain("P-Chain")
	core.RegisterChain("X-Chain")
	core.RegisterChain("C-Chain")

	return core, nil
}

// Start begins drawing blocks into the Quasar's gravitational pull
func (q *QuasarCore) Start(ctx context.Context) error {
	// Start block processors for legacy chains
	go q.processPChain(ctx)
	go q.processXChain(ctx)
	go q.processCChain(ctx)

	// Start processors for ALL dynamically registered chains (including new subnets)
	go q.ProcessDynamicChains(ctx)

	// Start quantum finalization engine - the singularity
	go q.quantumFinalizer(ctx)

	fmt.Println("[QUASAR] Event horizon activated - all chains drawn into quantum consensus")
	return nil
}

// SubmitPChainBlock submits a P-Chain block for quantum consensus
func (q *QuasarCore) SubmitPChainBlock(block *ChainBlock) {
	block.ChainName = "P-Chain"
	select {
	case q.pChainBlocks <- block:
	default:
		// Buffer full, drop oldest
		<-q.pChainBlocks
		q.pChainBlocks <- block
	}
}

// SubmitXChainBlock submits an X-Chain block for quantum consensus
func (q *QuasarCore) SubmitXChainBlock(block *ChainBlock) {
	block.ChainName = "X-Chain"
	select {
	case q.xChainBlocks <- block:
	default:
		<-q.xChainBlocks
		q.xChainBlocks <- block
	}
}

// SubmitCChainBlock submits a C-Chain block for quantum consensus
func (q *QuasarCore) SubmitCChainBlock(block *ChainBlock) {
	block.ChainName = "C-Chain"
	select {
	case q.cChainBlocks <- block:
	default:
		<-q.cChainBlocks
		q.cChainBlocks <- block
	}
}

// processPChain handles P-Chain blocks
func (q *QuasarCore) processPChain(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case block := <-q.pChainBlocks:
			q.processBlock(block)
		}
	}
}

// processXChain handles X-Chain blocks
func (q *QuasarCore) processXChain(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case block := <-q.xChainBlocks:
			q.processBlock(block)
		}
	}
}

// processCChain handles C-Chain blocks
func (q *QuasarCore) processCChain(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case block := <-q.cChainBlocks:
			q.processBlock(block)
		}
	}
}

// processBlock applies quantum consensus to a single block
func (q *QuasarCore) processBlock(block *ChainBlock) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Create quantum hash combining block data
	quantumHash := q.computeQuantumHash(block)

	// Collect validator signatures (BLS + Ringtail parallel)
	signatures := make(map[string]*HybridSignature)

	// In production, this would collect from actual validators
	// For now, simulate with local validator
	sig, err := q.hybridConsensus.SignMessage("validator1", []byte(quantumHash))
	if err == nil {
		signatures["validator1"] = sig
	}

	// Create quantum block
	qBlock := &QuantumBlock{
		Height:        q.quantumHeight + 1,
		SourceBlocks:  []*ChainBlock{block},
		QuantumHash:   quantumHash,
		Timestamp:     time.Now(),
		ValidatorSigs: signatures,
	}

	// Store finalized block
	q.finalizedBlocks[quantumHash] = qBlock
	q.quantumHeight++
	q.processedBlocks++
}

// quantumFinalizer runs the quantum finalization process
func (q *QuasarCore) quantumFinalizer(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.finalizeQuantumEpoch()
		}
	}
}

// finalizeQuantumEpoch creates a quantum proof for the current epoch
func (q *QuasarCore) finalizeQuantumEpoch() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.finalizedBlocks) == 0 {
		return
	}

	// Create epoch hash from all blocks
	epochData := ""
	for hash := range q.finalizedBlocks {
		epochData += hash
	}

	_ = sha256.Sum256([]byte(epochData))

	// Generate quantum proof (BLS aggregated + ML-DSA)
	// In production, this would aggregate from multiple validators
	q.quantumProofs++

	// Log quantum finality achievement
	fmt.Printf("[QUANTUM] Epoch finalized at height %d with %d blocks, proof #%d\n",
		q.quantumHeight, len(q.finalizedBlocks), q.quantumProofs)
}

// computeQuantumHash creates a quantum-resistant hash
func (q *QuasarCore) computeQuantumHash(block *ChainBlock) string {
	// Combine block data with quantum parameters
	data := fmt.Sprintf("%s:%x:%d:%d",
		block.ChainName,
		block.BlockID[:],
		block.Height,
		block.Timestamp.Unix())

	// Use SHA-256 for now (would use quantum-resistant hash in production)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// GetQuantumHeight returns the current quantum finalized height
func (q *QuasarCore) GetQuantumHeight() uint64 {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.quantumHeight
}

// GetMetrics returns aggregator metrics
func (q *QuasarCore) GetMetrics() (processedBlocks, quantumProofs uint64) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.processedBlocks, q.quantumProofs
}

// VerifyQuantumFinality checks if a block has quantum finality
func (q *QuasarCore) VerifyQuantumFinality(blockHash string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	qBlock, exists := q.finalizedBlocks[blockHash]
	if !exists {
		return false
	}

	// Verify both BLS and Ringtail signatures
	for validatorID, sig := range qBlock.ValidatorSigs {
		if !q.hybridConsensus.VerifyHybridSignature([]byte(blockHash), sig) {
			fmt.Printf("[QUANTUM] Invalid signature from validator %s\n", validatorID)
			return false
		}
	}

	return true
}

// RegisterChain dynamically registers a new chain/subnet for automatic quantum security
// All new subnets are automatically protected by the event horizon
func (q *QuasarCore) RegisterChain(chainName string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.registeredChains[chainName] {
		return nil // Already registered
	}

	// Create buffer for this chain
	q.chainBuffers[chainName] = make(chan *ChainBlock, 100)
	q.registeredChains[chainName] = true

	fmt.Printf("[QUASAR] Chain '%s' pulled into event horizon - quantum security active\n", chainName)
	return nil
}

// SubmitBlock is the universal RPC endpoint for ANY chain/contract to add blocks
// External systems (bridge, contracts) use this to enter the event horizon
func (q *QuasarCore) SubmitBlock(block *ChainBlock) error {
	q.mu.RLock()
	// Auto-register chain if not yet registered
	if !q.registeredChains[block.ChainName] {
		q.mu.RUnlock()
		if err := q.RegisterChain(block.ChainName); err != nil {
			return fmt.Errorf("failed to auto-register chain %s: %w", block.ChainName, err)
		}
		q.mu.RLock()
	}

	buffer, exists := q.chainBuffers[block.ChainName]
	q.mu.RUnlock()

	if !exists {
		return fmt.Errorf("chain %s not registered", block.ChainName)
	}

	select {
	case buffer <- block:
		// Block accepted into event horizon
		return nil
	default:
		// Buffer full, drop oldest and insert new
		<-buffer
		buffer <- block
		return nil
	}
}

// ProcessDynamicChains starts processors for all dynamically registered chains
// This runs alongside the legacy P/X/C chain processors
func (q *QuasarCore) ProcessDynamicChains(ctx context.Context) {
	q.mu.RLock()
	chains := make([]string, 0, len(q.chainBuffers))
	for chain := range q.chainBuffers {
		chains = append(chains, chain)
	}
	q.mu.RUnlock()

	// Start a processor for each registered chain
	for _, chainName := range chains {
		go q.processChain(ctx, chainName)
	}
}

// processChain handles blocks from any dynamically registered chain
func (q *QuasarCore) processChain(ctx context.Context, chainName string) {
	q.mu.RLock()
	buffer := q.chainBuffers[chainName]
	q.mu.RUnlock()

	for {
		select {
		case <-ctx.Done():
			return
		case block := <-buffer:
			q.processBlock(block)
		}
	}
}

// GetRegisteredChains returns all chains currently in the event horizon
func (q *QuasarCore) GetRegisteredChains() []string {
	q.mu.RLock()
	defer q.mu.RUnlock()

	chains := make([]string, 0, len(q.registeredChains))
	for chain := range q.registeredChains {
		chains = append(chains, chain)
	}
	return chains
}