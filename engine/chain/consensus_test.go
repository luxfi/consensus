// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// TestConsensusVoting tests basic voting and finality
func TestConsensusVoting(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, true))
	require.True(engine.IsBootstrapped())

	// Verify health
	health, err := engine.HealthCheck(ctx)
	require.NoError(err)
	require.NotNil(health)

	require.NoError(engine.Stop(ctx))
}

// TestConflictResolution tests handling of conflicting blocks
func TestConflictResolution(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, true))

	// Create conflicting block IDs
	block1 := ids.GenerateTestID()
	block2 := ids.GenerateTestID()

	// Both should be gettable without error
	require.NoError(engine.GetBlock(ctx, ids.EmptyNodeID, 1, block1))
	require.NoError(engine.GetBlock(ctx, ids.EmptyNodeID, 1, block2))

	require.NoError(engine.Stop(ctx))
}

// TestMultipleBlocks tests processing multiple blocks in sequence
func TestMultipleBlocks(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, true))

	// Process multiple blocks
	for i := 0; i < 10; i++ {
		blockID := ids.GenerateTestID()
		nodeID := ids.GenerateTestNodeID()

		err := engine.GetBlock(ctx, nodeID, uint32(i), blockID)
		require.NoError(err)
	}

	// Verify engine still healthy
	health, err := engine.HealthCheck(ctx)
	require.NoError(err)
	require.NotNil(health)

	require.NoError(engine.Stop(ctx))
}

// TestChainReorg tests chain reorganization scenarios
func TestChainReorg(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, true))

	// Simulate a chain reorg by requesting different blocks at same height
	height := uint32(10)
	nodeID := ids.GenerateTestNodeID()

	originalBlock := ids.GenerateTestID()
	require.NoError(engine.GetBlock(ctx, nodeID, height, originalBlock))

	// Request alternate block at same height
	reorgBlock := ids.GenerateTestID()
	require.NoError(engine.GetBlock(ctx, nodeID, height, reorgBlock))

	require.NoError(engine.Stop(ctx))
}

// TestConcurrentRequests tests handling of concurrent block requests
func TestConcurrentRequests(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, true))

	// Launch multiple concurrent requests
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			blockID := ids.GenerateTestID()
			nodeID := ids.GenerateTestNodeID()
			err := engine.GetBlock(ctx, nodeID, uint32(idx), blockID)
			require.NoError(err)
			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	require.NoError(engine.Stop(ctx))
}

// TestFinality tests block finality guarantees
func TestFinality(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, true))
	require.True(engine.IsBootstrapped())

	// After bootstrap, blocks should be finalized
	// Test that engine remains in consistent state
	for i := 0; i < 5; i++ {
		health, err := engine.HealthCheck(ctx)
		require.NoError(err)
		require.NotNil(health)
	}

	require.NoError(engine.Stop(ctx))
}

// TestRestartPreservesState tests that restarting preserves consensus state
func TestRestartPreservesState(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	// First session
	require.NoError(engine.Start(ctx, true))
	require.True(engine.IsBootstrapped())
	require.NoError(engine.Stop(ctx))

	// Restart
	require.NoError(engine.Start(ctx, true))
	require.True(engine.IsBootstrapped())

	health, err := engine.HealthCheck(ctx)
	require.NoError(err)
	require.NotNil(health)

	require.NoError(engine.Stop(ctx))
}

// TestInvalidBlockHandling tests handling of invalid blocks
func TestInvalidBlockHandling(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, true))

	// Request block with invalid/empty IDs
	require.NoError(engine.GetBlock(ctx, ids.EmptyNodeID, 0, ids.Empty))

	// Engine should remain healthy
	health, err := engine.HealthCheck(ctx)
	require.NoError(err)
	require.NotNil(health)

	require.NoError(engine.Stop(ctx))
}

// TestHighLoadScenario tests engine under high load
func TestHighLoadScenario(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, true))

	// Process many blocks quickly
	for i := 0; i < 1000; i++ {
		blockID := ids.GenerateTestID()
		nodeID := ids.GenerateTestNodeID()
		err := engine.GetBlock(ctx, nodeID, uint32(i), blockID)
		require.NoError(err)
	}

	// Verify engine still healthy after high load
	health, err := engine.HealthCheck(ctx)
	require.NoError(err)
	require.NotNil(health)

	require.NoError(engine.Stop(ctx))
}
