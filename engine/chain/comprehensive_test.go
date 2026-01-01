// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// =============================================================================
// ChainConsensus Tests - AddBlock, ProcessVote, Poll, IsAccepted, IsRejected,
// Preference, GetBlock
// =============================================================================

// TestChainConsensusAddBlock tests the AddBlock method of ChainConsensus
func TestChainConsensusAddBlock(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Create a genesis-like block
	genesisID := ids.GenerateTestID()
	block := &Block{
		id:        genesisID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
		data:      []byte("genesis"),
	}

	// Add block should succeed
	err := consensus.AddBlock(ctx, block)
	require.NoError(err)

	// Block should be in the blocks map
	storedBlock, exists := consensus.GetBlock(genesisID)
	require.True(exists)
	require.Equal(genesisID, storedBlock.id)

	// Block should be a tip
	require.Contains(consensus.tips, genesisID)

	// Lux consensus should be initialized
	require.NotNil(storedBlock.luxConsensus)
}

// TestChainConsensusAddBlockDuplicate tests that adding duplicate blocks fails
func TestChainConsensusAddBlockDuplicate(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	blockID := ids.GenerateTestID()
	block := &Block{
		id:        blockID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}

	// First add should succeed
	err := consensus.AddBlock(ctx, block)
	require.NoError(err)

	// Second add should fail
	err = consensus.AddBlock(ctx, block)
	require.Error(err)
	require.Contains(err.Error(), "block already exists")
}

// TestChainConsensusAddBlockUpdatesTips tests that adding child blocks updates tips
func TestChainConsensusAddBlockUpdatesTips(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Add genesis block
	genesisID := ids.GenerateTestID()
	genesis := &Block{
		id:        genesisID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, genesis))
	require.Contains(consensus.tips, genesisID)

	// Add child block
	childID := ids.GenerateTestID()
	child := &Block{
		id:        childID,
		parentID:  genesisID,
		height:    1,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, child))

	// Genesis should no longer be a tip
	require.NotContains(consensus.tips, genesisID)

	// Child should be a tip
	require.Contains(consensus.tips, childID)
}

// TestChainConsensusProcessVote tests the ProcessVote method
func TestChainConsensusProcessVote(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Add a block
	blockID := ids.GenerateTestID()
	block := &Block{
		id:        blockID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, block))

	// Process accept vote
	err := consensus.ProcessVote(ctx, blockID, true)
	require.NoError(err)
}

// TestChainConsensusProcessVoteUnknownBlock tests voting for unknown block
func TestChainConsensusProcessVoteUnknownBlock(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Process vote for non-existent block
	unknownID := ids.GenerateTestID()
	err := consensus.ProcessVote(ctx, unknownID, true)
	require.Error(err)
	require.Contains(err.Error(), "block not found")
}

// TestChainConsensusProcessVoteNilConsensus tests voting when consensus not initialized
func TestChainConsensusProcessVoteNilConsensus(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Manually add block without initializing luxConsensus
	blockID := ids.GenerateTestID()
	block := &Block{
		id:           blockID,
		parentID:     ids.Empty,
		height:       0,
		luxConsensus: nil, // intentionally nil
	}
	consensus.blocks[blockID] = block

	// Process vote should fail
	err := consensus.ProcessVote(ctx, blockID, true)
	require.Error(err)
	require.Contains(err.Error(), "block not initialized for consensus")
}

// TestChainConsensusPoll tests the Poll method
func TestChainConsensusPoll(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(1, 1, 1)

	// Add a block
	blockID := ids.GenerateTestID()
	block := &Block{
		id:        blockID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, block))

	// Poll with votes
	responses := map[ids.ID]int{
		blockID: 5,
	}
	err := consensus.Poll(ctx, responses)
	require.NoError(err)
}

// TestChainConsensusPollSkipsUnknownBlocks tests that Poll ignores unknown blocks
func TestChainConsensusPollSkipsUnknownBlocks(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Poll with votes for non-existent blocks
	unknownID := ids.GenerateTestID()
	responses := map[ids.ID]int{
		unknownID: 5,
	}
	err := consensus.Poll(ctx, responses)
	require.NoError(err) // Should not error, just skip
}

// TestChainConsensusPollFinalization tests block finalization through polling
func TestChainConsensusPollFinalization(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Beta=1 for quick finalization
	consensus := NewChainConsensus(1, 1, 1)

	// Add a block
	blockID := ids.GenerateTestID()
	block := &Block{
		id:        blockID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, block))

	// Initial state
	require.False(consensus.IsAccepted(blockID))

	// Poll multiple times to achieve finalization
	responses := map[ids.ID]int{blockID: 10}
	for i := 0; i < 5; i++ {
		err := consensus.Poll(ctx, responses)
		require.NoError(err)
	}

	// Check finalized tip
	require.Equal(blockID, consensus.finalizedTip)
}

// TestChainConsensusIsAccepted tests the IsAccepted method
func TestChainConsensusIsAccepted(t *testing.T) {
	require := require.New(t)

	consensus := NewChainConsensus(3, 2, 2)

	// Unknown block should not be accepted
	unknownID := ids.GenerateTestID()
	require.False(consensus.IsAccepted(unknownID))

	// Add and mark block as accepted
	blockID := ids.GenerateTestID()
	block := &Block{
		id:       blockID,
		accepted: true,
	}
	consensus.blocks[blockID] = block

	require.True(consensus.IsAccepted(blockID))
}

// TestChainConsensusIsRejected tests the IsRejected method
func TestChainConsensusIsRejected(t *testing.T) {
	require := require.New(t)

	consensus := NewChainConsensus(3, 2, 2)

	// Unknown block should not be rejected
	unknownID := ids.GenerateTestID()
	require.False(consensus.IsRejected(unknownID))

	// Add and mark block as rejected
	blockID := ids.GenerateTestID()
	block := &Block{
		id:       blockID,
		rejected: true,
	}
	consensus.blocks[blockID] = block

	require.True(consensus.IsRejected(blockID))
}

// TestChainConsensusPreference tests the Preference method
func TestChainConsensusPreference(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Empty consensus should return Empty preference
	require.Equal(ids.Empty, consensus.Preference())

	// Add a block as tip
	blockID := ids.GenerateTestID()
	block := &Block{
		id:        blockID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, block))

	// Should return the tip
	pref := consensus.Preference()
	require.Equal(blockID, pref)

	// Set finalized tip
	consensus.finalizedTip = blockID
	require.Equal(blockID, consensus.Preference())
}

// TestChainConsensusPreferenceFinalizedTip tests that finalized tip takes precedence
func TestChainConsensusPreferenceFinalizedTip(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Add multiple tips
	block1ID := ids.GenerateTestID()
	block1 := &Block{
		id:        block1ID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, block1))

	block2ID := ids.GenerateTestID()
	block2 := &Block{
		id:        block2ID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, block2))

	// Set finalized tip to block1
	consensus.finalizedTip = block1ID

	// Preference should be the finalized tip
	require.Equal(block1ID, consensus.Preference())
}

// TestChainConsensusGetBlock tests the GetBlock method
func TestChainConsensusGetBlock(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Non-existent block
	unknownID := ids.GenerateTestID()
	block, exists := consensus.GetBlock(unknownID)
	require.False(exists)
	require.Nil(block)

	// Add a block
	blockID := ids.GenerateTestID()
	expectedBlock := &Block{
		id:        blockID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, expectedBlock))

	// Get the block
	block, exists = consensus.GetBlock(blockID)
	require.True(exists)
	require.NotNil(block)
	require.Equal(blockID, block.id)
}

// TestChainConsensusStats tests the Stats method with various states
func TestChainConsensusStats(t *testing.T) {
	require := require.New(t)

	consensus := NewChainConsensus(3, 2, 2)

	// Add blocks with different states
	acceptedID := ids.GenerateTestID()
	consensus.blocks[acceptedID] = &Block{id: acceptedID, accepted: true}

	rejectedID := ids.GenerateTestID()
	consensus.blocks[rejectedID] = &Block{id: rejectedID, rejected: true}

	pendingID := ids.GenerateTestID()
	consensus.blocks[pendingID] = &Block{id: pendingID}

	consensus.tips[pendingID] = true
	consensus.finalizedTip = acceptedID

	stats := consensus.Stats()

	require.Equal(3, stats["total_blocks"])
	require.Equal(1, stats["accepted"])
	require.Equal(1, stats["rejected"])
	require.Equal(1, stats["pending"])
	require.Equal(1, stats["tips"])
	require.Equal(acceptedID.String(), stats["finalized_tip"])
}

// =============================================================================
// Transitive Engine Tests - AddBlock, ProcessVote, Poll, IsAccepted, Preference,
// SetVM, Notify, buildBlocksLocked, PendingBuildBlocks
// =============================================================================

// TestTransitiveAddBlock tests the Transitive AddBlock method
func TestTransitiveAddBlock(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()
	require.NoError(engine.Start(ctx, true))

	// Create a block
	blockID := ids.GenerateTestID()
	block := &Block{
		id:        blockID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}

	// Add block
	err := engine.AddBlock(ctx, block)
	require.NoError(err)
}

// TestTransitiveProcessVote tests the Transitive ProcessVote method
func TestTransitiveProcessVote(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()
	require.NoError(engine.Start(ctx, true))

	// Add a block first
	blockID := ids.GenerateTestID()
	block := &Block{
		id:        blockID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(engine.AddBlock(ctx, block))

	// Process vote
	err := engine.ProcessVote(ctx, blockID, true)
	require.NoError(err)
}

// TestTransitivePoll tests the Transitive Poll method
func TestTransitivePoll(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()
	require.NoError(engine.Start(ctx, true))

	// Add a block first
	blockID := ids.GenerateTestID()
	block := &Block{
		id:        blockID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(engine.AddBlock(ctx, block))

	// Poll with responses
	responses := map[ids.ID]int{
		blockID: 5,
	}
	err := engine.Poll(ctx, responses)
	require.NoError(err)
}

// TestTransitiveIsAccepted tests the Transitive IsAccepted method
func TestTransitiveIsAccepted(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()
	require.NoError(engine.Start(ctx, true))

	// Unknown block should not be accepted
	unknownID := ids.GenerateTestID()
	require.False(engine.IsAccepted(unknownID))
}

// TestTransitivePreference tests the Transitive Preference method
func TestTransitivePreference(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()
	require.NoError(engine.Start(ctx, true))

	// Initially should return empty preference
	pref := engine.Preference()
	require.Equal(ids.Empty, pref)
}

// TestTransitiveSetVM tests the SetVM method
func TestTransitiveSetVM(t *testing.T) {
	require := require.New(t)

	engine := New()

	// Initially no VM
	require.Nil(engine.vm)

	// Create a mock VM
	mockVM := &mockBlockBuilder{}

	// Set VM
	engine.SetVM(mockVM)

	// VM should be set
	require.NotNil(engine.vm)
	require.Equal(mockVM, engine.vm)
}

// TestTransitiveNotifyPendingTxs tests Notify with PendingTxs message
func TestTransitiveNotifyPendingTxs(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()
	require.NoError(engine.Start(ctx, true))

	// Initially no pending builds
	require.Zero(engine.PendingBuildBlocks())

	// Send PendingTxs notification without VM
	msg := Message{Type: PendingTxs}
	err := engine.Notify(ctx, msg)
	require.NoError(err)

	// Without VM, pending builds accumulate (buildBlocksLocked returns early)
	require.Equal(1, engine.PendingBuildBlocks())
}

// TestTransitiveNotifyPendingTxsWithVM tests Notify with PendingTxs and a VM set
func TestTransitiveNotifyPendingTxsWithVM(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()
	require.NoError(engine.Start(ctx, true))

	// Set a mock VM that succeeds
	mockVM := &mockBlockBuilder{
		buildFunc: func(ctx context.Context) (block.Block, error) {
			return &testBlock{id: ids.GenerateTestID(), height: 1}, nil
		},
	}
	engine.SetVM(mockVM)

	// Send PendingTxs notification
	msg := Message{Type: PendingTxs}
	err := engine.Notify(ctx, msg)
	require.NoError(err)

	// Pending builds should be consumed
	require.Zero(engine.PendingBuildBlocks())
}

// TestTransitiveNotifyPendingTxsWithVMError tests Notify when VM returns error
func TestTransitiveNotifyPendingTxsWithVMError(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()
	require.NoError(engine.Start(ctx, true))

	// Set a mock VM that returns error
	mockVM := &mockBlockBuilder{
		buildFunc: func(ctx context.Context) (block.Block, error) {
			return nil, errors.New("no transactions")
		},
	}
	engine.SetVM(mockVM)

	// Send PendingTxs notification
	msg := Message{Type: PendingTxs}
	err := engine.Notify(ctx, msg)
	require.NoError(err) // Errors are swallowed, not fatal

	// Pending builds should be cleared after attempt
	require.Zero(engine.PendingBuildBlocks())
}

// TestTransitiveNotifyStateSyncDone tests Notify with StateSyncDone message
func TestTransitiveNotifyStateSyncDone(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()
	require.NoError(engine.Start(ctx, true))

	// Send StateSyncDone notification
	msg := Message{Type: StateSyncDone}
	err := engine.Notify(ctx, msg)
	require.NoError(err)
}

// TestTransitiveNotifyUnknownType tests Notify with unknown message type
func TestTransitiveNotifyUnknownType(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()
	require.NoError(engine.Start(ctx, true))

	// Send unknown notification type
	msg := Message{Type: 999}
	err := engine.Notify(ctx, msg)
	require.NoError(err) // Unknown types are ignored
}

// TestTransitivePendingBuildBlocks tests the PendingBuildBlocks getter
func TestTransitivePendingBuildBlocks(t *testing.T) {
	require := require.New(t)

	engine := New()

	// Initially zero
	require.Zero(engine.PendingBuildBlocks())

	// Manually set pending
	engine.mu.Lock()
	engine.pendingBuildBlocks = 5
	engine.mu.Unlock()

	require.Equal(5, engine.PendingBuildBlocks())
}

// TestTransitiveBuildBlocksLockedNilVM tests buildBlocksLocked with nil VM
func TestTransitiveBuildBlocksLockedNilVM(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()

	// Set pending builds
	engine.pendingBuildBlocks = 3

	// buildBlocksLocked should exit early with nil VM
	err := engine.buildBlocksLocked(ctx)
	require.NoError(err)

	// Pending builds should remain unchanged
	require.Equal(3, engine.pendingBuildBlocks)
}

// TestTransitiveBuildBlocksLockedMultiple tests building multiple blocks
func TestTransitiveBuildBlocksLockedMultiple(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()

	// Track build calls
	buildCount := 0
	mockVM := &mockBlockBuilder{
		buildFunc: func(ctx context.Context) (interface{}, error) {
			buildCount++
			return "block", nil
		},
	}
	engine.vm = mockVM
	engine.pendingBuildBlocks = 3

	// Build should be called for each pending block
	err := engine.buildBlocksLocked(ctx)
	require.NoError(err)
	require.Equal(3, buildCount)
	require.Zero(engine.pendingBuildBlocks)
}

// TestTransitiveBuildBlocksLockedPartialFailure tests partial build failures
func TestTransitiveBuildBlocksLockedPartialFailure(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()

	// Fail after first build
	buildCount := 0
	mockVM := &mockBlockBuilder{
		buildFunc: func(ctx context.Context) (interface{}, error) {
			buildCount++
			if buildCount > 1 {
				return nil, errors.New("no more transactions")
			}
			return "block", nil
		},
	}
	engine.vm = mockVM
	engine.pendingBuildBlocks = 3

	// Should stop after error (not fatal)
	err := engine.buildBlocksLocked(ctx)
	require.NoError(err)
	require.Equal(2, buildCount) // Called twice (success, then fail)
}

// =============================================================================
// NewWithParams Tests
// =============================================================================

// TestNewWithParams tests creating engine with custom parameters
func TestNewWithParams(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               5,
		AlphaPreference: 4,
		AlphaConfidence: 4,
		Beta:            10,
	}

	engine := NewWithParams(params)

	require.NotNil(engine)
	require.NotNil(engine.consensus)
	require.Equal(params.K, engine.params.K)
	require.Equal(params.AlphaPreference, engine.params.AlphaPreference)
	require.Equal(params.Beta, engine.params.Beta)
	require.False(engine.IsBootstrapped())
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

// TestChainConsensusConcurrentAddBlock tests concurrent block additions
func TestChainConsensusConcurrentAddBlock(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Add blocks concurrently
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			blockID := ids.GenerateTestID()
			block := &Block{
				id:        blockID,
				parentID:  ids.Empty,
				height:    uint64(idx),
				timestamp: time.Now().UnixNano(),
			}
			_ = consensus.AddBlock(ctx, block)
		}(i)
	}
	wg.Wait()

	// Should have added blocks (some may fail due to duplicates)
	stats := consensus.Stats()
	require.Greater(stats["total_blocks"].(int), 0)
}

// TestChainConsensusConcurrentPoll tests concurrent polling
func TestChainConsensusConcurrentPoll(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Add a block
	blockID := ids.GenerateTestID()
	block := &Block{
		id:        blockID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, block))

	// Poll concurrently
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses := map[ids.ID]int{blockID: 5}
			_ = consensus.Poll(ctx, responses)
		}()
	}
	wg.Wait()

	// Should not panic or deadlock
}

// TestChainConsensusConcurrentStats tests concurrent Stats access
func TestChainConsensusConcurrentStats(t *testing.T) {
	consensus := NewChainConsensus(3, 2, 2)

	// Add some blocks
	for i := 0; i < 10; i++ {
		blockID := ids.GenerateTestID()
		consensus.blocks[blockID] = &Block{
			id:       blockID,
			accepted: i%3 == 0,
			rejected: i%3 == 1,
		}
	}

	// Access Stats concurrently
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = consensus.Stats()
		}()
	}
	wg.Wait()
}

// TestTransitiveConcurrentNotify tests concurrent Notify calls
func TestTransitiveConcurrentNotify(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()
	require.NoError(engine.Start(ctx, true))

	// Set a simple VM
	engine.SetVM(&mockBlockBuilder{
		buildFunc: func(ctx context.Context) (interface{}, error) {
			return "block", nil
		},
	})

	// Send notifications concurrently
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := Message{Type: PendingTxs}
			_ = engine.Notify(ctx, msg)
		}()
	}
	wg.Wait()
}

// =============================================================================
// Edge Cases
// =============================================================================

// TestChainConsensusEmptyPoll tests polling with empty responses
func TestChainConsensusEmptyPoll(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Poll with empty responses
	responses := map[ids.ID]int{}
	err := consensus.Poll(ctx, responses)
	require.NoError(err)
}

// TestChainConsensusPollNilConsensus tests polling block with nil consensus
func TestChainConsensusPollNilConsensus(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Add block without consensus
	blockID := ids.GenerateTestID()
	block := &Block{
		id:           blockID,
		luxConsensus: nil,
	}
	consensus.blocks[blockID] = block

	// Poll should handle nil consensus gracefully
	responses := map[ids.ID]int{blockID: 5}
	err := consensus.Poll(ctx, responses)
	require.NoError(err)
}

// TestTransitiveStartStop tests multiple start/stop cycles
func TestTransitiveStartStop(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()

	// Multiple start/stop cycles
	for i := 0; i < 5; i++ {
		err := engine.Start(ctx, true)
		require.NoError(err)
		require.True(engine.IsBootstrapped())

		err = engine.Stop(ctx)
		require.NoError(err)
		require.False(engine.IsBootstrapped())
	}
}

// TestTransitiveStopWithoutStart tests stopping engine that was never started
func TestTransitiveStopWithoutStart(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()

	// Stop should be safe even without start
	err := engine.Stop(ctx)
	require.NoError(err)
}

// TestTransitiveHealthCheckBeforeStart tests health check before start
func TestTransitiveHealthCheckBeforeStart(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()

	// Health check should work even before start
	health, err := engine.HealthCheck(ctx)
	require.NoError(err)
	require.NotNil(health)

	m, ok := health.(map[string]interface{})
	require.True(ok)
	require.False(m["bootstrapped"].(bool))
}

// TestTransitiveHealthCheckAfterStart tests health check after start
func TestTransitiveHealthCheckAfterStart(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()
	require.NoError(engine.Start(ctx, true))

	health, err := engine.HealthCheck(ctx)
	require.NoError(err)

	m, ok := health.(map[string]interface{})
	require.True(ok)
	require.True(m["bootstrapped"].(bool))
}

// TestChainConsensusPreferenceEmptyTips tests Preference with no tips
func TestChainConsensusPreferenceEmptyTips(t *testing.T) {
	require := require.New(t)

	consensus := NewChainConsensus(3, 2, 2)

	// Clear tips
	consensus.tips = make(map[ids.ID]bool)
	consensus.finalizedTip = ids.Empty

	// Should return Empty
	require.Equal(ids.Empty, consensus.Preference())
}

// TestChainConsensusBlockWithEmptyParent tests adding block with Empty parent
func TestChainConsensusBlockWithEmptyParent(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Block with Empty parent
	blockID := ids.GenerateTestID()
	block := &Block{
		id:       blockID,
		parentID: ids.Empty,
		height:   0,
	}

	err := consensus.AddBlock(ctx, block)
	require.NoError(err)

	// Should be a tip
	require.Contains(consensus.tips, blockID)
}

// =============================================================================
// Mock Types
// =============================================================================

// testBlock implements block.Block for comprehensive testing
type testBlock struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp time.Time
	status    uint8
	bytes     []byte
}

func (b *testBlock) ID() ids.ID            { return b.id }
func (b *testBlock) Parent() ids.ID        { return b.parentID }
func (b *testBlock) ParentID() ids.ID      { return b.parentID }
func (b *testBlock) Height() uint64        { return b.height }
func (b *testBlock) Timestamp() time.Time  { return b.timestamp }
func (b *testBlock) Status() uint8         { return b.status }
func (b *testBlock) Verify(context.Context) error { return nil }
func (b *testBlock) Accept(context.Context) error { return nil }
func (b *testBlock) Reject(context.Context) error { return nil }
func (b *testBlock) Bytes() []byte         { return b.bytes }

// mockBlockBuilder implements BlockBuilder for testing
type mockBlockBuilder struct {
	buildFunc      func(ctx context.Context) (block.Block, error)
	blocks         map[ids.ID]block.Block
	lastAcceptedID ids.ID
}

func (m *mockBlockBuilder) BuildBlock(ctx context.Context) (block.Block, error) {
	if m.buildFunc != nil {
		return m.buildFunc(ctx)
	}
	return nil, nil
}

func (m *mockBlockBuilder) GetBlock(ctx context.Context, id ids.ID) (block.Block, error) {
	if m.blocks != nil {
		if blk, ok := m.blocks[id]; ok {
			return blk, nil
		}
	}
	return nil, errors.New("block not found")
}

func (m *mockBlockBuilder) ParseBlock(ctx context.Context, bytes []byte) (block.Block, error) {
	return &testBlock{bytes: bytes}, nil
}

func (m *mockBlockBuilder) LastAccepted(ctx context.Context) (ids.ID, error) {
	return m.lastAcceptedID, nil
}

// =============================================================================
// Integration Tests
// =============================================================================

// TestFullConsensusFlow tests a complete consensus flow from block addition to finalization
func TestFullConsensusFlow(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create engine with minimal params for quick finalization
	params := config.Parameters{
		K:               1,
		AlphaPreference: 1,
		AlphaConfidence: 1,
		Beta:            1,
	}
	engine := NewWithParams(params)
	require.NoError(engine.Start(ctx, true))

	// Add genesis block
	genesisID := ids.GenerateTestID()
	genesis := &Block{
		id:        genesisID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(engine.AddBlock(ctx, genesis))

	// Add child block
	childID := ids.GenerateTestID()
	child := &Block{
		id:        childID,
		parentID:  genesisID,
		height:    1,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(engine.AddBlock(ctx, child))

	// Poll to build consensus
	responses := map[ids.ID]int{
		childID: 5,
	}
	for i := 0; i < 5; i++ {
		err := engine.Poll(ctx, responses)
		require.NoError(err)
	}

	// Health check
	health, err := engine.HealthCheck(ctx)
	require.NoError(err)
	require.NotNil(health)

	// Stop engine
	require.NoError(engine.Stop(ctx))
	require.False(engine.IsBootstrapped())
}

// TestConsensusWithMultipleBranches tests consensus with competing branches
func TestConsensusWithMultipleBranches(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Create genesis
	genesisID := ids.GenerateTestID()
	genesis := &Block{
		id:        genesisID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, genesis))

	// Create two competing branches from genesis
	branch1ID := ids.GenerateTestID()
	branch1 := &Block{
		id:        branch1ID,
		parentID:  genesisID,
		height:    1,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, branch1))

	branch2ID := ids.GenerateTestID()
	branch2 := &Block{
		id:        branch2ID,
		parentID:  genesisID,
		height:    1,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, branch2))

	// Both branches should be tips (since genesis had its tip status removed)
	require.Contains(consensus.tips, branch1ID)
	require.Contains(consensus.tips, branch2ID)

	// Genesis should no longer be a tip
	require.NotContains(consensus.tips, genesisID)

	// Stats should show all blocks
	stats := consensus.Stats()
	require.Equal(3, stats["total_blocks"].(int))
	require.Equal(2, stats["tips"].(int))
}

// TestNotifyMultiplePendingTxs tests multiple PendingTxs notifications
func TestNotifyMultiplePendingTxs(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New()
	require.NoError(engine.Start(ctx, true))

	buildCount := 0
	mockVM := &mockBlockBuilder{
		buildFunc: func(ctx context.Context) (interface{}, error) {
			buildCount++
			return "block", nil
		},
	}
	engine.SetVM(mockVM)

	// Send multiple PendingTxs notifications
	for i := 0; i < 5; i++ {
		msg := Message{Type: PendingTxs}
		err := engine.Notify(ctx, msg)
		require.NoError(err)
	}

	// Should have built 5 blocks
	require.Equal(5, buildCount)
}

// TestMessageTypeConstants verifies message type re-exports
func TestMessageTypeConstants(t *testing.T) {
	require := require.New(t)

	// Verify message types match core definitions
	require.Equal(core.PendingTxs, PendingTxs)
	require.Equal(core.StateSyncDone, StateSyncDone)
}

// TestProcessVoteRejectPath tests ProcessVote with reject=false
func TestProcessVoteRejectPath(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	consensus := NewChainConsensus(3, 2, 2)

	// Add a block
	blockID := ids.GenerateTestID()
	block := &Block{
		id:        blockID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: time.Now().UnixNano(),
	}
	require.NoError(consensus.AddBlock(ctx, block))

	// Process reject vote (accept=false)
	err := consensus.ProcessVote(ctx, blockID, false)
	require.NoError(err)
	// Should not record a vote when accept=false
}
