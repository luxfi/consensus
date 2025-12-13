// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cgo
// +build cgo

package engine

import (
	"sync"
	"sync/atomic"

	"github.com/luxfi/ids"
)

// CGOConsensus is a CGO-based implementation of consensus
// For now, it's the same as the pure Go implementation
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
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp int64
	data      []byte
	status    BlockStatus
}

// NewCGOConsensus creates a new CGO consensus engine
func NewCGOConsensus(params ConsensusParams) (*CGOConsensus, error) {
	c := &CGOConsensus{
		blockCache: make(map[ids.ID]*cachedBlock, params.MaxOutstandingItems),
		params:     params,
		accepted:   make(map[ids.ID]bool),
	}

	// Set initial preference to empty ID
	c.preference.Store(ids.Empty)

	return c, nil
}

// Add adds a block to consensus
func (c *CGOConsensus) Add(block Block) error {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	blockID := block.ID()

	// Cache the block
	c.blockCache[blockID] = &cachedBlock{
		id:        blockID,
		parentID:  block.ParentID(),
		height:    block.Height(),
		timestamp: block.Timestamp(),
		data:      block.Bytes(),
		status:    StatusProcessing,
	}

	// Update preference to latest block
	c.preference.Store(blockID)

	return nil
}

// RecordPoll records a poll result
func (c *CGOConsensus) RecordPoll(blockID ids.ID, accept bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if accept {
		c.accepted[blockID] = true
	}

	return nil
}

// IsAccepted checks if a block is accepted
func (c *CGOConsensus) IsAccepted(blockID ids.ID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.accepted[blockID]
}

// GetPreference returns the current preference
func (c *CGOConsensus) GetPreference() ids.ID {
	return c.preference.Load().(ids.ID)
}

// Finalized checks if consensus is finalized
func (c *CGOConsensus) Finalized() bool {
	return c.finalized.Load()
}

// Parameters returns consensus parameters
func (c *CGOConsensus) Parameters() ConsensusParams {
	return c.params
}

// HealthCheck performs a health check
func (c *CGOConsensus) HealthCheck() error {
	return nil
}
