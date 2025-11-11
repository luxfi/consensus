// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"fmt"
	"sync"

	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/ids"
)

// Block represents a block in the chain
type Block struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp int64
	data      []byte

	// Consensus state - using Lux consensus instead of Snowball
	luxConsensus *engine.LuxConsensus
	accepted     bool
	rejected     bool
}

// ChainConsensus implements real Lux consensus for linear chains using Photon → Wave → Focus
type ChainConsensus struct {
	mu sync.RWMutex

	// Parameters
	k     int // Sample size
	alpha int // Quorum size
	beta  int // Decision threshold

	// State
	blocks map[ids.ID]*Block
	tips   map[ids.ID]bool // Current chain tips

	// Consensus tracking
	bootstrapped bool
	finalizedTip ids.ID
}

// NewChainConsensus creates a real consensus engine
func NewChainConsensus(k, alpha, beta int) *ChainConsensus {
	return &ChainConsensus{
		k:      k,
		alpha:  alpha,
		beta:   beta,
		blocks: make(map[ids.ID]*Block),
		tips:   make(map[ids.ID]bool),
	}
}

// AddBlock adds a block to consensus
func (c *ChainConsensus) AddBlock(ctx context.Context, block *Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if block already exists
	if _, exists := c.blocks[block.id]; exists {
		return fmt.Errorf("block already exists: %s", block.id)
	}

	// Initialize Lux consensus for this block using Photon → Wave → Focus
	block.luxConsensus = engine.NewLuxConsensus(c.k, c.alpha, c.beta)

	// Add to blocks map
	c.blocks[block.id] = block

	// Update tips
	if block.parentID != ids.Empty {
		// Remove parent from tips (no longer a tip)
		delete(c.tips, block.parentID)
	}
	c.tips[block.id] = true

	return nil
}

// ProcessVote processes a vote for a block
func (c *ChainConsensus) ProcessVote(ctx context.Context, blockID ids.ID, accept bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	block, exists := c.blocks[blockID]
	if !exists {
		return fmt.Errorf("block not found: %s", blockID)
	}

	if block.luxConsensus == nil {
		return fmt.Errorf("block not initialized for consensus")
	}

	if accept {
		block.luxConsensus.RecordVote(blockID)
	}

	return nil
}

// Poll conducts a consensus poll
func (c *ChainConsensus) Poll(ctx context.Context, responses map[ids.ID]int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Poll each block's Lux consensus instance using Wave → Focus protocols
	for blockID, votes := range responses {
		block, exists := c.blocks[blockID]
		if !exists {
			continue
		}

		if block.luxConsensus != nil {
			blockResponses := map[ids.ID]int{blockID: votes}
			shouldContinue := block.luxConsensus.Poll(blockResponses)

			// Check if block reached finality through Focus convergence
			if !shouldContinue && block.luxConsensus.Decided() {
				block.accepted = true
				c.finalizedTip = blockID
			}
		}
	}

	return nil
}

// IsAccepted checks if a block is accepted
func (c *ChainConsensus) IsAccepted(blockID ids.ID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	block, exists := c.blocks[blockID]
	if !exists {
		return false
	}

	return block.accepted
}

// IsRejected checks if a block is rejected
func (c *ChainConsensus) IsRejected(blockID ids.ID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	block, exists := c.blocks[blockID]
	if !exists {
		return false
	}

	return block.rejected
}

// Preference returns current preferred block
func (c *ChainConsensus) Preference() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return the finalized tip if available
	if c.finalizedTip != ids.Empty {
		return c.finalizedTip
	}

	// Otherwise return latest tip
	for tip := range c.tips {
		return tip
	}

	return ids.Empty
}

// GetBlock returns a block by ID
func (c *ChainConsensus) GetBlock(blockID ids.ID) (*Block, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	block, exists := c.blocks[blockID]
	return block, exists
}

// Stats returns consensus statistics
func (c *ChainConsensus) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	accepted := 0
	rejected := 0
	pending := 0

	for _, block := range c.blocks {
		if block.accepted {
			accepted++
		} else if block.rejected {
			rejected++
		} else {
			pending++
		}
	}

	return map[string]interface{}{
		"total_blocks": len(c.blocks),
		"accepted":     accepted,
		"rejected":     rejected,
		"pending":      pending,
		"tips":         len(c.tips),
		"finalized_tip": c.finalizedTip.String(),
	}
}
