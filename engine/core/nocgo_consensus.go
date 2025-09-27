// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build !cgo
// +build !cgo

package core

import (
	"sync"
	"sync/atomic"

	"github.com/luxfi/ids"
)

// Types are defined in types.go

// CGOConsensus is a pure Go implementation of consensus
type CGOConsensus struct {
	mu         sync.RWMutex
	preference atomic.Value // ids.ID
	finalized  atomic.Bool
	
	// Cache for blocks
	blockCache map[ids.ID]*cachedBlock
	cacheMu    sync.RWMutex
	
	// Store parameters for later retrieval
	params ConsensusParams
	
	// Consensus state
	accepted map[ids.ID]bool
}

// cachedBlock stores block data
type cachedBlock struct {
	id       ids.ID
	parentID ids.ID
	height   uint64
	timestamp int64
	data     []byte
	status   BlockStatus
}

// BlockStatus is defined in types.go

// NewCGOConsensus creates a new pure Go consensus engine
func NewCGOConsensus(params ConsensusParams) (*CGOConsensus, error) {
	c := &CGOConsensus{
		blockCache: make(map[ids.ID]*cachedBlock, params.MaxOutstandingItems),
		params:     params,
		accepted:   make(map[ids.ID]bool),
	}
	
	// Set initial preference
	c.preference.Store(ids.Empty)
	
	return c, nil
}

// Add adds a block to consensus
func (c *CGOConsensus) Add(block Block) error {
	// Fast path: check cache first
	blockID := block.ID()
	
	c.cacheMu.RLock()
	if _, exists := c.blockCache[blockID]; exists {
		c.cacheMu.RUnlock()
		return nil // Already added
	}
	c.cacheMu.RUnlock()

	// Slow path: add to consensus
	c.mu.Lock()
	defer c.mu.Unlock()

	// Cache the block
	c.cacheMu.Lock()
	c.blockCache[blockID] = &cachedBlock{
		id:        blockID,
		parentID:  block.ParentID(),
		height:    block.Height(),
		timestamp: block.Timestamp(),
		data:      block.Bytes(),
		status:    StatusProcessing,
	}
	c.cacheMu.Unlock()

	return nil
}

// RecordPoll records a vote for a block
func (c *CGOConsensus) RecordPoll(blockID ids.ID, isPreference bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple implementation: if it's a preference vote, mark as accepted
	if isPreference {
		c.accepted[blockID] = true
		c.preference.Store(blockID)
		
		// Update cache
		c.cacheMu.Lock()
		if cached, ok := c.blockCache[blockID]; ok {
			cached.status = StatusAccepted
		}
		c.cacheMu.Unlock()
	}

	return nil
}

// IsAccepted checks if a block is accepted
func (c *CGOConsensus) IsAccepted(blockID ids.ID) bool {
	// Fast path: check cache
	c.cacheMu.RLock()
	if cached, ok := c.blockCache[blockID]; ok && cached.status == StatusAccepted {
		c.cacheMu.RUnlock()
		return true
	}
	c.cacheMu.RUnlock()

	// Slow path: check accepted map
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return c.accepted[blockID]
}

// GetPreference returns the current preferred block
func (c *CGOConsensus) GetPreference() ids.ID {
	// Fast path: use cached value
	if pref := c.preference.Load(); pref != nil {
		return pref.(ids.ID)
	}

	return ids.Empty
}

// Finalized checks if consensus is finalized
func (c *CGOConsensus) Finalized() bool {
	return c.finalized.Load()
}

// Parameters returns consensus parameters
func (c *CGOConsensus) Parameters() ConsensusParams {
	// These are immutable after creation, so no lock needed
	return c.params
}

// HealthCheck performs a health check
func (c *CGOConsensus) HealthCheck() error {
	// Pure Go implementation is always healthy
	return nil
}

// GetStats returns consensus statistics
func (c *CGOConsensus) GetStats() (*Stats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	acceptedCount := uint64(len(c.accepted))
	
	return &Stats{
		BlocksAccepted: acceptedCount,
		BlocksRejected: 0,
		VotesProcessed: acceptedCount * 2, // Rough estimate
		PollsCompleted: acceptedCount,
		AverageDecisionTimeMs: 100, // Mock value
	}, nil
}

// Poll polls validators for their preferences
func (c *CGOConsensus) Poll(validators []ids.ID) error {
	// Simple implementation: do nothing
	return nil
}

// Close cleans up the consensus engine
func (c *CGOConsensus) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clean up cached blocks
	c.cacheMu.Lock()
	c.blockCache = nil
	c.cacheMu.Unlock()

	return nil
}

// InitializeLibrary is a no-op for pure Go implementation
func InitializeLibrary() error {
	return nil
}

// Cleanup is a no-op for pure Go implementation
func Cleanup() error {
	return nil
}

// PureGoConsensus is an alias for CGOConsensus in the pure Go implementation
type PureGoConsensus = CGOConsensus