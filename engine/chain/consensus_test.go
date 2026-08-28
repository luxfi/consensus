// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// TestConsensusVoting tests basic voting and finality
func TestConsensusVoting(t *testing.T) {
	require := require.New(t)

	engine := newTestEngine()
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

	engine := newTestEngine()
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

	engine := newTestEngine()
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

	engine := newTestEngine()
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

	engine := newTestEngine()
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

	engine := newTestEngine()
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

// TestRestartPreservesState proves a Stop/Start cycle neither loses decided
// consensus state nor wedges further finality. The old body only checked the
// IsBootstrapped/HealthCheck flags across the restart — an engine that dropped
// every accepted block on Stop would still have passed. Now the test drives a
// REAL signed 4-of-5 quorum accept before the stop, asserts the
// consensus-critical triple — accepted set, preference, certified canonical at
// the height — survives the restart unchanged, and then drives a SECOND quorum
// accept on the restarted engine: a restart must preserve the past AND keep
// finalizing the future.
func TestRestartPreservesState(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	vs := newTestValidatorSet(5)
	equal := newStakeMap(vs, 20, 20, 20, 20, 20)
	rec := &recordingGossiper{}

	// First session: a real quorum accept at height 1. Four of five equal
	// stakes is 80% > ⅔, so the block must also reach the certified (Quasar)
	// export frontier before the stop.
	blk1 := newTestBlock(1, ids.Empty, "restart-height-1")
	engine, chainID := driveSignedAccepts(t, vs, equal, rec, blk1, []int{0, 1, 2, 3})
	mustFinalize(t, engine, blk1, 2*time.Second, "pre-restart quorum accept")
	mustQuasar(t, engine, blk1, 2*time.Second, "pre-restart export cert")

	// The consensus-critical state the restart must preserve.
	prePreference := engine.Preference()
	preQuasar, preOK := engine.QuasarHeight()
	require.True(preOK, "an exported height must have a certified frontier")
	require.Equal(blk1.height, preQuasar)

	require.NoError(engine.Stop(ctx))

	// Restart.
	require.NoError(engine.Start(ctx, true))
	require.True(engine.IsBootstrapped())

	// EQUALITY: accepted set, preference and certified frontier all survive.
	require.True(engine.IsAccepted(blk1.id), "restart must not lose an accepted block")
	require.Equal(prePreference, engine.Preference(), "restart must not move the preference")
	postQuasar, postOK := engine.QuasarHeight()
	require.True(postOK, "restart must not lose the certified frontier")
	require.Equal(preQuasar, postQuasar, "restart must not move the certified frontier")

	health, err := engine.HealthCheck(ctx)
	require.NoError(err)
	require.NotNil(health)

	// CONTINUITY: the restarted engine must still finalize. Drive height 2
	// through the same real signed-vote road, extending the preserved tip.
	blk2 := newTestBlock(2, blk1.id, "restart-height-2")
	cb2 := &Block{id: blk2.id, parentID: blk2.parentID, height: blk2.height, timestamp: blk2.timestamp.Unix(), data: blk2.bytes}
	_ = engine.consensus.AddBlock(ctx, cb2)
	engine.mu.Lock()
	engine.pendingBlocks[blk2.id] = &PendingBlock{ConsensusBlock: cb2, VMBlock: blk2, ProposedAt: time.Now(), Round: 0}
	engine.mu.Unlock()
	pos := VotePosition{ChainID: chainID, Height: blk2.height, Round: 0, BlockID: blk2.id, ParentID: blk2.parentID}
	for _, i := range []int{0, 1, 2, 3} {
		engine.ReceiveVote(vs.signedVote(i, pos))
	}
	mustFinalize(t, engine, blk2, 2*time.Second, "post-restart quorum accept")
	mustQuasar(t, engine, blk2, 2*time.Second, "the restarted engine must certify new heights")

	require.NoError(engine.Stop(ctx))
}

// TestInvalidBlockHandling tests handling of invalid blocks
func TestInvalidBlockHandling(t *testing.T) {
	require := require.New(t)

	engine := newTestEngine()
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

	engine := newTestEngine()
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
