// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quantum

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// Pulsar implements the linear blockchain consensus layer.
// Like a cosmic pulsar that emits regular pulses with clockwork precision,
// this engine processes transactions sequentially in traditional blockchain fashion.
// Single-threaded but reliable, it provides the steady heartbeat of linear consensus.
type Pulsar struct {
	mu     sync.RWMutex
	params config.Parameters
	nodeID ids.NodeID

	// Chain state
	height      uint64
	lastBlock   *LinearBlock
	chain       []*LinearBlock
	pendingTxs  []Transaction

	// Pulse timing
	pulseInterval time.Duration    // Time between pulses
	lastPulse     time.Time        // Last emission time
	pulseStrength uint64           // Current pulse energy
	rotationRate  float64          // Blocks per second

	// Consensus state
	preferredBlock *LinearBlock
	confidence     int

	// Photon accumulator for this round
	photons []*Photon

	// Callbacks
	onPulse    func(*LinearBlock)
	onFinalize func(*LinearBlock)
}

// LinearBlock represents a block in the linear chain
type LinearBlock struct {
	// Block header
	Height       uint64
	Timestamp    time.Time
	ParentID     ids.ID
	BlockID      ids.ID
	ProposerID   ids.NodeID

	// Content
	Transactions []Transaction
	StateRoot    [32]byte

	// Consensus metadata
	PulseNumber  uint64      // Which pulse created this block
	PulseEnergy  uint64      // Energy of the creating pulse
	PhotonCount  uint64      // Number of photons in this block

	// Finalization
	Finalized    bool
	FinalizedAt  time.Time
}

// Transaction represents a transaction in the linear chain
type Transaction interface {
	ID() ids.ID
	Bytes() []byte
	Verify() error
}

// NewPulsar creates a new Pulsar linear consensus engine
func NewPulsar(params config.Parameters, nodeID ids.NodeID) *Pulsar {
	return &Pulsar{
		params:        params,
		nodeID:        nodeID,
		chain:         make([]*LinearBlock, 0),
		pendingTxs:    make([]Transaction, 0),
		photons:       make([]*Photon, 0),
		pulseInterval: time.Duration(params.MaxItemProcessingTime),
		lastPulse:     time.Now(),
		pulseStrength: 1,
		rotationRate:  1.0,
		confidence:    0,
	}
}

// Initialize starts the Pulsar engine
func (p *Pulsar) Initialize(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Start the pulse generator
	go p.runPulseGenerator(ctx)

	return nil
}

// AddTransaction adds a transaction to the pending pool
func (p *Pulsar) AddTransaction(tx Transaction) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Verify transaction
	if err := tx.Verify(); err != nil {
		return err
	}

	// Add to pending pool
	p.pendingTxs = append(p.pendingTxs, tx)

	return nil
}

// AddPhoton accumulates photons for the next pulse
func (p *Pulsar) AddPhoton(photon *Photon) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.photons = append(p.photons, photon)
	p.pulseStrength += photon.Energy
}

// RecordVote records a vote for a block
func (p *Pulsar) RecordVote(ctx context.Context, blockID ids.ID, vote bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Find the block
	var block *LinearBlock
	for _, b := range p.chain {
		if b.BlockID == blockID {
			block = b
			break
		}
	}

	if block == nil {
		return fmt.Errorf("block not found: %s", blockID)
	}

	// Update confidence
	if vote {
		p.confidence++
	} else {
		p.confidence--
	}

	// Check if we should update preferred block
	if p.confidence >= p.params.AlphaPreference {
		p.preferredBlock = block
	}

	// Check finalization threshold
	if p.confidence >= p.params.AlphaConfidence && !block.Finalized {
		p.finalizeBlock(ctx, block)
	}

	return nil
}

// runPulseGenerator generates regular pulses to create blocks
func (p *Pulsar) runPulseGenerator(ctx context.Context) {
	ticker := time.NewTicker(p.pulseInterval)
	defer ticker.Stop()

	pulseNumber := uint64(0)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.emitPulse(ctx, pulseNumber)
			pulseNumber++
		}
	}
}

// emitPulse creates a new block with clockwork precision
func (p *Pulsar) emitPulse(ctx context.Context, pulseNumber uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if we have enough energy to pulse
	if p.pulseStrength < 1 {
		return
	}

	// Gather transactions for this pulse
	txCount := min(len(p.pendingTxs), int(p.pulseStrength))
	if txCount == 0 && len(p.photons) == 0 {
		return // Nothing to emit
	}

	// Create block transactions
	blockTxs := make([]Transaction, txCount)
	copy(blockTxs, p.pendingTxs[:txCount])
	p.pendingTxs = p.pendingTxs[txCount:]

	// Calculate parent
	var parentID ids.ID
	if p.lastBlock != nil {
		parentID = p.lastBlock.BlockID
	}

	// Create new block
	block := &LinearBlock{
		Height:       p.height,
		Timestamp:    time.Now(),
		ParentID:     parentID,
		ProposerID:   p.nodeID,
		Transactions: blockTxs,
		PulseNumber:  pulseNumber,
		PulseEnergy:  p.pulseStrength,
		PhotonCount:  uint64(len(p.photons)),
	}

	// Compute block ID
	block.BlockID = p.computeBlockID(block)

	// Add to chain
	p.chain = append(p.chain, block)
	p.lastBlock = block
	p.height++

	// Update pulse timing
	p.lastPulse = time.Now()
	duration := p.lastPulse.Sub(block.Timestamp).Seconds()
	if duration > 0 {
		p.rotationRate = 1.0 / duration
	}

	// Reset pulse energy and photons
	p.pulseStrength = 1
	p.photons = make([]*Photon, 0)

	// Emit pulse callback
	if p.onPulse != nil {
		p.onPulse(block)
	}

	// Update confidence for own block
	p.confidence = 1
	p.preferredBlock = block
}

// finalizeBlock marks a block as finalized
func (p *Pulsar) finalizeBlock(ctx context.Context, block *LinearBlock) {
	block.Finalized = true
	block.FinalizedAt = time.Now()

	// Callback
	if p.onFinalize != nil {
		p.onFinalize(block)
	}

	// Reset confidence
	p.confidence = 0
}

// computeBlockID computes the ID of a block
func (p *Pulsar) computeBlockID(block *LinearBlock) ids.ID {
	// Simple implementation - would hash all block contents
	return ids.GenerateTestID()
}

// GetHeight returns the current chain height
func (p *Pulsar) GetHeight() uint64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.height
}

// GetLastBlock returns the most recent block
func (p *Pulsar) GetLastBlock() (*LinearBlock, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if p.lastBlock != nil {
		return p.lastBlock, true
	}
	return nil, false
}

// GetPreferredBlock returns the current preferred block
func (p *Pulsar) GetPreferredBlock() (*LinearBlock, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if p.preferredBlock != nil {
		return p.preferredBlock, true
	}
	return nil, false
}

// GetChain returns the full chain
func (p *Pulsar) GetChain() []*LinearBlock {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	chain := make([]*LinearBlock, len(p.chain))
	copy(chain, p.chain)
	return chain
}

// SetPulseCallback sets the callback for when a pulse creates a block
func (p *Pulsar) SetPulseCallback(cb func(*LinearBlock)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onPulse = cb
}

// SetFinalizeCallback sets the callback for when a block is finalized
func (p *Pulsar) SetFinalizeCallback(cb func(*LinearBlock)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onFinalize = cb
}

// PulsarStats provides statistics about the Pulsar engine
type PulsarStats struct {
	Height          uint64
	ChainLength     int
	PendingTxCount  int
	PulseStrength   uint64
	RotationRate    float64
	LastPulseTime   time.Time
	PhotonCount     int
	Confidence      int
	FinalizedBlocks int
}

// GetStats returns current Pulsar statistics
func (p *Pulsar) GetStats() PulsarStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := PulsarStats{
		Height:         p.height,
		ChainLength:    len(p.chain),
		PendingTxCount: len(p.pendingTxs),
		PulseStrength:  p.pulseStrength,
		RotationRate:   p.rotationRate,
		LastPulseTime:  p.lastPulse,
		PhotonCount:    len(p.photons),
		Confidence:     p.confidence,
	}

	// Count finalized blocks
	for _, block := range p.chain {
		if block.Finalized {
			stats.FinalizedBlocks++
		}
	}

	return stats
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}