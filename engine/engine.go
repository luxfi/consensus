// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"context"
	"sync"
	"time"

	"github.com/luxfi/consensus/types"
	"github.com/luxfi/ids"
)

// Engine is the main consensus engine interface
type Engine interface {
	// Add a new block to the consensus
	Add(ctx context.Context, block *types.Block) error

	// RecordVote records a vote for a block
	RecordVote(ctx context.Context, vote *types.Vote) error

	// IsAccepted returns whether a block has been accepted
	IsAccepted(id types.ID) bool

	// GetStatus returns the status of a block
	GetStatus(id types.ID) types.Status

	// Start the consensus engine
	Start(ctx context.Context) error

	// Stop the consensus engine
	Stop() error
}

// Chain represents a linear blockchain consensus engine
type Chain struct {
	mu sync.RWMutex

	// Configuration
	config types.Config

	// Block storage
	blocks map[types.ID]*types.Block
	votes  map[types.ID][]types.Vote
	status map[types.ID]types.Status

	// Consensus state
	lastAccepted types.ID
	height       uint64

	// Network
	validators []types.NodeID
}

// NewChain creates a new chain consensus engine
func NewChain(config types.Config) *Chain {
	return &Chain{
		config: config,
		blocks: make(map[types.ID]*types.Block),
		votes:  make(map[types.ID][]types.Vote),
		status: make(map[types.ID]types.Status),
		lastAccepted: types.GenesisID,
	}
}

// Add adds a new block to the chain
func (c *Chain) Add(ctx context.Context, block *types.Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Store the block
	c.blocks[block.ID] = block
	c.status[block.ID] = types.StatusProcessing
	
	// Initialize vote tracking
	if c.votes[block.ID] == nil {
		c.votes[block.ID] = []types.Vote{}
	}

	return nil
}

// RecordVote records a vote for a block
func (c *Chain) RecordVote(ctx context.Context, vote *types.Vote) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if block exists
	if _, exists := c.blocks[vote.BlockID]; !exists {
		return types.ErrBlockNotFound
	}

	// Add vote
	c.votes[vote.BlockID] = append(c.votes[vote.BlockID], *vote)

	// Check if we have quorum
	if len(c.votes[vote.BlockID]) >= c.config.Alpha {
		c.acceptBlock(vote.BlockID)
	}

	return nil
}

// IsAccepted returns whether a block has been accepted
func (c *Chain) IsAccepted(id types.ID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.status[id] == types.StatusAccepted
}

// GetStatus returns the status of a block
func (c *Chain) GetStatus(id types.ID) types.Status {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status, exists := c.status[id]
	if !exists {
		return types.StatusUnknown
	}
	return status
}

// Start starts the consensus engine
func (c *Chain) Start(ctx context.Context) error {
	// Initialize genesis block
	genesis := &types.Block{
		ID:       types.GenesisID,
		ParentID: ids.Empty,
		Height:   0,
		Time:     time.Now(),
	}
	
	c.mu.Lock()
	c.blocks[genesis.ID] = genesis
	c.status[genesis.ID] = types.StatusAccepted
	c.lastAccepted = genesis.ID
	c.mu.Unlock()

	return nil
}

// Stop stops the consensus engine
func (c *Chain) Stop() error {
	// Cleanup resources if needed
	return nil
}

// acceptBlock marks a block as accepted
func (c *Chain) acceptBlock(id types.ID) {
	c.status[id] = types.StatusAccepted
	
	if block, exists := c.blocks[id]; exists {
		if block.Height > c.height {
			c.height = block.Height
			c.lastAccepted = id
		}
	}
}

// DefaultConfig returns the default chain configuration
func DefaultConfig() types.Config {
	return types.DefaultConfig()
}