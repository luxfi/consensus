// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/types"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// TestNewChain tests the NewChain constructor
func TestNewChain(t *testing.T) {
	require := require.New(t)

	config := types.Config{
		Alpha:          15,
		K:              20,
		MaxOutstanding: 10,
	}

	chain := NewChain(config)
	require.NotNil(chain)
	require.NotNil(chain.blocks)
	require.NotNil(chain.votes)
	require.NotNil(chain.status)
	require.Equal(types.GenesisID, chain.lastAccepted)
	require.Equal(config, chain.config)
}

// TestChainAdd tests adding blocks to the chain
func TestChainAdd(t *testing.T) {
	require := require.New(t)

	config := types.Config{Alpha: 15, K: 20}
	chain := NewChain(config)

	block := &types.Block{
		ID:       ids.GenerateTestID(),
		ParentID: types.GenesisID,
		Height:   1,
		Time:     time.Now(),
	}

	err := chain.Add(context.Background(), block)
	require.NoError(err)

	// Verify block was stored
	chain.mu.RLock()
	stored, exists := chain.blocks[block.ID]
	status := chain.status[block.ID]
	votes := chain.votes[block.ID]
	chain.mu.RUnlock()

	require.True(exists)
	require.Equal(block, stored)
	require.Equal(types.StatusProcessing, status)
	require.NotNil(votes)
	require.Len(votes, 0)
}

// TestChainAddMultipleBlocks tests adding multiple blocks
func TestChainAddMultipleBlocks(t *testing.T) {
	require := require.New(t)

	config := types.Config{Alpha: 3, K: 5}
	chain := NewChain(config)

	// Add first block
	block1 := &types.Block{
		ID:       ids.GenerateTestID(),
		ParentID: types.GenesisID,
		Height:   1,
		Time:     time.Now(),
	}
	err := chain.Add(context.Background(), block1)
	require.NoError(err)

	// Add second block
	block2 := &types.Block{
		ID:       ids.GenerateTestID(),
		ParentID: block1.ID,
		Height:   2,
		Time:     time.Now(),
	}
	err = chain.Add(context.Background(), block2)
	require.NoError(err)

	require.Equal(2, len(chain.blocks))
}

// TestChainRecordVote tests recording votes
func TestChainRecordVote(t *testing.T) {
	require := require.New(t)

	config := types.Config{Alpha: 2, K: 3}
	chain := NewChain(config)

	block := &types.Block{
		ID:       ids.GenerateTestID(),
		ParentID: types.GenesisID,
		Height:   1,
		Time:     time.Now(),
	}

	err := chain.Add(context.Background(), block)
	require.NoError(err)

	// Record first vote - should not reach quorum yet
	vote1 := &types.Vote{
		BlockID:  block.ID,
		VoteType: types.VotePreference,
		Voter:    ids.GenerateTestNodeID(),
	}
	err = chain.RecordVote(context.Background(), vote1)
	require.NoError(err)
	require.False(chain.IsAccepted(block.ID))

	// Record second vote - should reach quorum (alpha=2)
	vote2 := &types.Vote{
		BlockID:  block.ID,
		VoteType: types.VotePreference,
		Voter:    ids.GenerateTestNodeID(),
	}
	err = chain.RecordVote(context.Background(), vote2)
	require.NoError(err)
	require.True(chain.IsAccepted(block.ID))
}

// TestChainRecordVoteBlockNotFound tests voting on non-existent block
func TestChainRecordVoteBlockNotFound(t *testing.T) {
	require := require.New(t)

	config := types.Config{Alpha: 2, K: 3}
	chain := NewChain(config)

	vote := &types.Vote{
		BlockID:  ids.GenerateTestID(),
		VoteType: types.VotePreference,
		Voter:    ids.GenerateTestNodeID(),
	}

	err := chain.RecordVote(context.Background(), vote)
	require.Error(err)
	require.Equal(types.ErrBlockNotFound, err)
}

// TestChainIsAccepted tests the IsAccepted method
func TestChainIsAccepted(t *testing.T) {
	require := require.New(t)

	config := types.Config{Alpha: 1, K: 1}
	chain := NewChain(config)

	blockID := ids.GenerateTestID()

	// Non-existent block should not be accepted
	require.False(chain.IsAccepted(blockID))

	// Add block
	block := &types.Block{
		ID:       blockID,
		ParentID: types.GenesisID,
		Height:   1,
		Time:     time.Now(),
	}
	err := chain.Add(context.Background(), block)
	require.NoError(err)

	// Not yet accepted
	require.False(chain.IsAccepted(blockID))

	// Vote to accept
	vote := &types.Vote{
		BlockID:  blockID,
		VoteType: types.VotePreference,
		Voter:    ids.GenerateTestNodeID(),
	}
	err = chain.RecordVote(context.Background(), vote)
	require.NoError(err)

	// Now accepted
	require.True(chain.IsAccepted(blockID))
}

// TestChainGetStatus tests the GetStatus method
func TestChainGetStatus(t *testing.T) {
	require := require.New(t)

	config := types.Config{Alpha: 1, K: 1}
	chain := NewChain(config)

	// Unknown block
	unknownID := ids.GenerateTestID()
	require.Equal(types.StatusUnknown, chain.GetStatus(unknownID))

	// Add block
	block := &types.Block{
		ID:       ids.GenerateTestID(),
		ParentID: types.GenesisID,
		Height:   1,
		Time:     time.Now(),
	}
	err := chain.Add(context.Background(), block)
	require.NoError(err)

	// Processing status
	require.Equal(types.StatusProcessing, chain.GetStatus(block.ID))

	// Vote to accept
	vote := &types.Vote{
		BlockID:  block.ID,
		VoteType: types.VotePreference,
		Voter:    ids.GenerateTestNodeID(),
	}
	err = chain.RecordVote(context.Background(), vote)
	require.NoError(err)

	// Accepted status
	require.Equal(types.StatusAccepted, chain.GetStatus(block.ID))
}

// TestChainStart tests the Start method
func TestChainStart(t *testing.T) {
	require := require.New(t)

	config := types.Config{Alpha: 15, K: 20}
	chain := NewChain(config)

	err := chain.Start(context.Background())
	require.NoError(err)

	// Verify genesis block was initialized
	chain.mu.RLock()
	genesis, exists := chain.blocks[types.GenesisID]
	status := chain.status[types.GenesisID]
	lastAccepted := chain.lastAccepted
	chain.mu.RUnlock()

	require.True(exists)
	require.NotNil(genesis)
	require.Equal(uint64(0), genesis.Height)
	require.Equal(types.StatusAccepted, status)
	require.Equal(types.GenesisID, lastAccepted)
}

// TestChainStop tests the Stop method
func TestChainStop(t *testing.T) {
	require := require.New(t)

	config := types.Config{Alpha: 15, K: 20}
	chain := NewChain(config)

	err := chain.Start(context.Background())
	require.NoError(err)

	err = chain.Stop()
	require.NoError(err)
}

// TestChainAcceptBlock tests the acceptBlock internal method
func TestChainAcceptBlock(t *testing.T) {
	require := require.New(t)

	config := types.Config{Alpha: 1, K: 1}
	chain := NewChain(config)

	err := chain.Start(context.Background())
	require.NoError(err)

	// Add a block at height 1
	block1 := &types.Block{
		ID:       ids.GenerateTestID(),
		ParentID: types.GenesisID,
		Height:   1,
		Time:     time.Now(),
	}
	err = chain.Add(context.Background(), block1)
	require.NoError(err)

	// Vote to accept
	vote := &types.Vote{
		BlockID:  block1.ID,
		VoteType: types.VotePreference,
		Voter:    ids.GenerateTestNodeID(),
	}
	err = chain.RecordVote(context.Background(), vote)
	require.NoError(err)

	// Verify height and lastAccepted were updated
	chain.mu.RLock()
	height := chain.height
	lastAccepted := chain.lastAccepted
	chain.mu.RUnlock()

	require.Equal(uint64(1), height)
	require.Equal(block1.ID, lastAccepted)

	// Add and accept a block at height 2
	block2 := &types.Block{
		ID:       ids.GenerateTestID(),
		ParentID: block1.ID,
		Height:   2,
		Time:     time.Now(),
	}
	err = chain.Add(context.Background(), block2)
	require.NoError(err)

	vote2 := &types.Vote{
		BlockID:  block2.ID,
		VoteType: types.VotePreference,
		Voter:    ids.GenerateTestNodeID(),
	}
	err = chain.RecordVote(context.Background(), vote2)
	require.NoError(err)

	chain.mu.RLock()
	height = chain.height
	lastAccepted = chain.lastAccepted
	chain.mu.RUnlock()

	require.Equal(uint64(2), height)
	require.Equal(block2.ID, lastAccepted)
}

// TestChainAcceptBlockLowerHeight tests accepting a block with lower height
func TestChainAcceptBlockLowerHeight(t *testing.T) {
	require := require.New(t)

	config := types.Config{Alpha: 1, K: 1}
	chain := NewChain(config)

	err := chain.Start(context.Background())
	require.NoError(err)

	// Add and accept block at height 5
	block5 := &types.Block{
		ID:       ids.GenerateTestID(),
		ParentID: types.GenesisID,
		Height:   5,
		Time:     time.Now(),
	}
	err = chain.Add(context.Background(), block5)
	require.NoError(err)

	vote5 := &types.Vote{
		BlockID:  block5.ID,
		VoteType: types.VotePreference,
		Voter:    ids.GenerateTestNodeID(),
	}
	err = chain.RecordVote(context.Background(), vote5)
	require.NoError(err)

	chain.mu.RLock()
	height := chain.height
	chain.mu.RUnlock()
	require.Equal(uint64(5), height)

	// Add and accept block at height 3 (should not update height)
	block3 := &types.Block{
		ID:       ids.GenerateTestID(),
		ParentID: types.GenesisID,
		Height:   3,
		Time:     time.Now(),
	}
	err = chain.Add(context.Background(), block3)
	require.NoError(err)

	vote3 := &types.Vote{
		BlockID:  block3.ID,
		VoteType: types.VotePreference,
		Voter:    ids.GenerateTestNodeID(),
	}
	err = chain.RecordVote(context.Background(), vote3)
	require.NoError(err)

	chain.mu.RLock()
	height = chain.height
	lastAccepted := chain.lastAccepted
	chain.mu.RUnlock()

	// Height should still be 5
	require.Equal(uint64(5), height)
	require.Equal(block5.ID, lastAccepted)
}

// TestDefaultConfig tests the DefaultConfig function
func TestDefaultConfig(t *testing.T) {
	require := require.New(t)

	config := DefaultConfig()
	require.Equal(20, config.Alpha)
	require.Equal(20, config.K)
}

// TestChainConcurrentAccess tests concurrent access to the chain
func TestChainConcurrentAccess(t *testing.T) {
	require := require.New(t)

	config := types.Config{Alpha: 5, K: 10}
	chain := NewChain(config)

	err := chain.Start(context.Background())
	require.NoError(err)

	// Create blocks
	blocks := make([]*types.Block, 10)
	for i := 0; i < 10; i++ {
		blocks[i] = &types.Block{
			ID:       ids.GenerateTestID(),
			ParentID: types.GenesisID,
			Height:   uint64(i + 1),
			Time:     time.Now(),
		}
	}

	// Concurrent adds
	done := make(chan bool)
	for _, block := range blocks {
		go func(b *types.Block) {
			err := chain.Add(context.Background(), b)
			require.NoError(err)
			done <- true
		}(block)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all blocks were added
	chain.mu.RLock()
	count := len(chain.blocks)
	chain.mu.RUnlock()

	// 10 blocks + genesis
	require.Equal(11, count)
}
