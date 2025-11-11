// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/ids"
)

// ============================================================================
// Phase 2 Tests: Poll Edge Cases (5 tests - HIGH PRIORITY)
// ============================================================================

// TestRecordPollSplitVote verifies that split votes (no supermajority) don't
// change preference or finalize blocks. Tests exact threshold boundary behavior.
func TestRecordPollSplitVote(t *testing.T) {
	require := require.New(t)
	engine := NewTransitiveEngine()
	ctx := context.Background()

	// K=5, Alpha=3 means we need 3 votes to reach threshold
	params := TransitiveParams{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            1,
	}

	genesisID := ids.GenerateTestID()
	genesisHeight := uint64(0)
	genesisTime := time.Now()

	require.NoError(engine.Initialize(ctx, params, genesisID, genesisHeight, genesisTime))

	// Add two competing blocks at same height
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block2 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}

	require.NoError(engine.Add(block1))
	require.NoError(engine.Add(block2))

	// Record split vote: K=5 total, 2 for block1, 2 for block2, 1 abstain
	// Neither reaches alpha threshold of 3
	votes := NewBag[ids.ID]()
	votes.AddCount(block1.ID(), 2)
	votes.AddCount(block2.ID(), 2)
	require.NoError(engine.RecordPoll(ctx, votes))

	// Preference should remain unchanged (or switch based on vote count, but not finalize)
	// Both blocks should still be processing (neither finalized)
	require.Equal(StatusProcessing, block1.Status())
	require.Equal(StatusProcessing, block2.Status())
	require.Equal(2, engine.NumProcessing())

	// Confidence should be reset to 0 for both since neither reached threshold
	require.Zero(engine.confidence[block1.ID()])
	require.Zero(engine.confidence[block2.ID()])

	// Verify no block was finalized
	require.False(engine.finalized.Contains(block1.ID()))
	require.False(engine.finalized.Contains(block2.ID()))

	// One more poll that still splits (2 vs 2)
	votes2 := NewBag[ids.ID]()
	votes2.AddCount(block1.ID(), 2)
	votes2.AddCount(block2.ID(), 2)
	require.NoError(engine.RecordPoll(ctx, votes2))

	// Still no finalization
	require.Equal(2, engine.NumProcessing())
	require.Equal(StatusProcessing, block1.Status())
	require.Equal(StatusProcessing, block2.Status())
}

// TestRecordPollWhenFinalized verifies that polling after finalization is
// safe and acts as a no-op without changing state.
func TestRecordPollWhenFinalized(t *testing.T) {
	require := require.New(t)
	engine := NewTransitiveEngine()
	ctx := context.Background()

	params := TransitiveParams{
		K:               1,
		AlphaPreference: 1,
		AlphaConfidence: 1,
		Beta:            1,
	}

	genesisID := ids.GenerateTestID()
	genesisHeight := uint64(0)
	genesisTime := time.Now()

	require.NoError(engine.Initialize(ctx, params, genesisID, genesisHeight, genesisTime))

	// Add and finalize a block
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	require.NoError(engine.Add(block1))

	// Vote to accept block1
	votes := NewBag[ids.ID]()
	votes.Add(block1.ID())
	require.NoError(engine.RecordPoll(ctx, votes))

	// Block should be finalized
	require.Equal(StatusAccepted, block1.Status())
	require.True(engine.finalized.Contains(block1.ID()))
	require.Zero(engine.NumProcessing())
	require.Equal(block1.ID(), engine.lastAccepted)

	// Record another poll after finalization (should be no-op)
	votes2 := NewBag[ids.ID]()
	votes2.Add(block1.ID())
	err := engine.RecordPoll(ctx, votes2)

	// Should not error
	require.NoError(err)

	// State should remain unchanged
	require.Equal(StatusAccepted, block1.Status())
	require.True(engine.finalized.Contains(block1.ID()))
	require.Zero(engine.NumProcessing())
	require.Equal(block1.ID(), engine.lastAccepted)
	require.Equal(block1.ID(), engine.preference)

	// Try voting for already finalized block multiple times
	for i := 0; i < 5; i++ {
		require.NoError(engine.RecordPoll(ctx, votes2))
	}

	// Still no change
	require.Equal(StatusAccepted, block1.Status())
	require.Zero(engine.NumProcessing())
}

// TestRecordPollInvalidVote verifies that votes for unknown/non-existent
// block IDs are safely ignored without causing errors.
func TestRecordPollInvalidVote(t *testing.T) {
	require := require.New(t)
	engine := NewTransitiveEngine()
	ctx := context.Background()

	params := TransitiveParams{
		K:               2,
		AlphaPreference: 2,
		AlphaConfidence: 2,
		Beta:            1,
	}

	genesisID := ids.GenerateTestID()
	genesisHeight := uint64(0)
	genesisTime := time.Now()

	require.NoError(engine.Initialize(ctx, params, genesisID, genesisHeight, genesisTime))

	// Add a valid block
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	require.NoError(engine.Add(block1))

	// Create votes with both valid and invalid block IDs
	unknownBlock1 := ids.GenerateTestID()
	unknownBlock2 := ids.GenerateTestID()

	votes := NewBag[ids.ID]()
	votes.Add(unknownBlock1)           // Invalid - should be ignored
	votes.Add(unknownBlock2)           // Invalid - should be ignored
	votes.AddCount(block1.ID(), 2)     // Valid - should be processed

	// Record poll with mixed valid/invalid votes
	err := engine.RecordPoll(ctx, votes)

	// Should not error - invalid votes are silently ignored
	require.NoError(err)

	// Valid votes should still be processed correctly
	require.Equal(StatusAccepted, block1.Status())
	require.Zero(engine.NumProcessing())

	// Verify unknown blocks are not in the engine
	require.NotContains(engine.blocks, unknownBlock1)
	require.NotContains(engine.blocks, unknownBlock2)

	// Test with only invalid votes (should be safe no-op)
	votesAllInvalid := NewBag[ids.ID]()
	votesAllInvalid.Add(ids.GenerateTestID())
	votesAllInvalid.Add(ids.GenerateTestID())

	err = engine.RecordPoll(ctx, votesAllInvalid)
	require.NoError(err)

	// State unchanged
	require.Equal(StatusAccepted, block1.Status())
	require.Zero(engine.NumProcessing())
}

// TestRecordPollChangePreferredChain verifies that polling can switch preference
// between competing chains and correctly build confidence for the new chain while
// resetting confidence for the old chain.
func TestRecordPollChangePreferredChain(t *testing.T) {
	require := require.New(t)
	engine := NewTransitiveEngine()
	ctx := context.Background()

	// Beta=3 requires 3 rounds of voting to finalize
	params := TransitiveParams{
		K:               2,
		AlphaPreference: 2,
		AlphaConfidence: 2,
		Beta:            3,
	}

	genesisID := ids.GenerateTestID()
	genesisHeight := uint64(0)
	genesisTime := time.Now()

	require.NoError(engine.Initialize(ctx, params, genesisID, genesisHeight, genesisTime))

	// Build two competing chains:
	// Chain A: genesis -> A1 -> A2
	// Chain B: genesis -> B1 -> B2
	blockA1 := &TestBlock{
		IDV:        ids.ID{0xA1},
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	blockA2 := &TestBlock{
		IDV:        ids.ID{0xA2},
		HeightV:    2,
		ParentV:    blockA1.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}
	blockB1 := &TestBlock{
		IDV:        ids.ID{0xB1},
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	blockB2 := &TestBlock{
		IDV:        ids.ID{0xB2},
		HeightV:    2,
		ParentV:    blockB1.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}

	require.NoError(engine.Add(blockA1))
	require.NoError(engine.Add(blockA2))
	require.NoError(engine.Add(blockB1))
	require.NoError(engine.Add(blockB2))

	// Initially prefer chain A
	votesA := NewBag[ids.ID]()
	votesA.AddCount(blockA2.ID(), 2)
	require.NoError(engine.RecordPoll(ctx, votesA))

	// Preference should be A2
	require.Equal(blockA2.ID(), engine.preference)
	require.Equal(1, engine.confidence[blockA2.ID()])

	// Build more confidence for A
	require.NoError(engine.RecordPoll(ctx, votesA))
	require.Equal(2, engine.confidence[blockA2.ID()])

	// Now switch to chain B
	votesB := NewBag[ids.ID]()
	votesB.AddCount(blockB2.ID(), 2)
	require.NoError(engine.RecordPoll(ctx, votesB))

	// Preference should switch to B2
	require.Equal(blockB2.ID(), engine.preference)

	// Confidence for A2 should reset to 0
	require.Zero(engine.confidence[blockA2.ID()])

	// Confidence for B2 should be 1
	require.Equal(1, engine.confidence[blockB2.ID()])

	// Continue voting for B to finalize
	require.NoError(engine.RecordPoll(ctx, votesB))
	require.NoError(engine.RecordPoll(ctx, votesB))

	// Chain B should be accepted, chain A rejected
	require.Equal(StatusRejected, blockA1.Status())
	require.Equal(StatusRejected, blockA2.Status())
	require.Equal(StatusAccepted, blockB1.Status())
	require.Equal(StatusAccepted, blockB2.Status())
	require.Zero(engine.NumProcessing())
}

// TestRecordPollWithDefaultParams tests consensus with production-like parameters
// to ensure the engine works correctly with realistic values.
func TestRecordPollWithDefaultParams(t *testing.T) {
	require := require.New(t)
	engine := NewTransitiveEngine()
	ctx := context.Background()

	// Production-like parameters: K=20, Alpha=15 (75% quorum), Beta=20
	params := TransitiveParams{
		K:               20,
		AlphaPreference: 15,
		AlphaConfidence: 15,
		Beta:            20,
	}

	genesisID := ids.GenerateTestID()
	genesisHeight := uint64(0)
	genesisTime := time.Now()

	require.NoError(engine.Initialize(ctx, params, genesisID, genesisHeight, genesisTime))

	// Add multiple competing blocks
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block2 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}

	require.NoError(engine.Add(block1))
	require.NoError(engine.Add(block2))

	// Simulate realistic voting where block1 gets supermajority (15/20 = 75%)
	votes := NewBag[ids.ID]()
	votes.AddCount(block1.ID(), 15)
	votes.AddCount(block2.ID(), 5)

	// Vote repeatedly to build confidence (need Beta=20 rounds)
	for round := 0; round < params.Beta; round++ {
		require.NoError(engine.RecordPoll(ctx, votes))

		// Check intermediate state
		if round < params.Beta-1 {
			// Before Beta rounds, should still be processing
			require.Equal(2, engine.NumProcessing())
			require.Equal(StatusProcessing, block1.Status())
			require.Equal(StatusProcessing, block2.Status())
			require.Equal(round+1, engine.confidence[block1.ID()])
		}
	}

	// After Beta rounds with supermajority, block1 should be accepted
	require.Equal(StatusAccepted, block1.Status())
	require.Equal(StatusRejected, block2.Status())
	require.Zero(engine.NumProcessing())
	require.Equal(block1.ID(), engine.lastAccepted)

	// Test that additional votes after finalization are safe
	require.NoError(engine.RecordPoll(ctx, votes))
	require.Equal(StatusAccepted, block1.Status())
	require.Zero(engine.NumProcessing())

	// Add another block extending the accepted chain
	block3 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block1.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}
	require.NoError(engine.Add(block3))
	require.Equal(1, engine.NumProcessing())

	// Vote to finalize block3 with supermajority
	votes2 := NewBag[ids.ID]()
	votes2.AddCount(block3.ID(), 16) // >75% quorum

	for round := 0; round < params.Beta; round++ {
		require.NoError(engine.RecordPoll(ctx, votes2))
	}

	// Block3 should now be accepted
	require.Equal(StatusAccepted, block3.Status())
	require.Zero(engine.NumProcessing())
	require.Equal(block3.ID(), engine.lastAccepted)
}

// ============================================================================
// Phase 3 Tests: Advanced Scenarios (2 tests)
// ============================================================================

// TestRecordPollDivergedVoting simulates a complex scenario where multiple
// nodes have different initial preferences and tests whether the network
// converges to a single choice through repeated polling.
func TestRecordPollDivergedVoting(t *testing.T) {
	require := require.New(t)
	engine := NewTransitiveEngine()
	ctx := context.Background()

	params := TransitiveParams{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            3,
	}

	genesisID := ids.GenerateTestID()
	genesisHeight := uint64(0)
	genesisTime := time.Now()

	require.NoError(engine.Initialize(ctx, params, genesisID, genesisHeight, genesisTime))

	// Create 3 competing blocks to simulate high divergence
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block2 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block3 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}

	require.NoError(engine.Add(block1))
	require.NoError(engine.Add(block2))
	require.NoError(engine.Add(block3))

	// Round 1: Highly diverged votes (2-2-1)
	round1 := NewBag[ids.ID]()
	round1.AddCount(block1.ID(), 2)
	round1.AddCount(block2.ID(), 2)
	round1.AddCount(block3.ID(), 1)
	require.NoError(engine.RecordPoll(ctx, round1))

	// No block reaches threshold yet
	require.Equal(3, engine.NumProcessing())

	// Round 2: Slight convergence (3-1-1)
	round2 := NewBag[ids.ID]()
	round2.AddCount(block1.ID(), 3)
	round2.AddCount(block2.ID(), 1)
	round2.AddCount(block3.ID(), 1)
	require.NoError(engine.RecordPoll(ctx, round2))

	// Block1 reaches threshold, gains 1 confidence
	require.Equal(1, engine.confidence[block1.ID()])
	require.Equal(3, engine.NumProcessing())

	// Round 3: More nodes converge (4-1-0)
	round3 := NewBag[ids.ID]()
	round3.AddCount(block1.ID(), 4)
	round3.AddCount(block2.ID(), 1)
	require.NoError(engine.RecordPoll(ctx, round3))

	// Block1 gains more confidence
	require.Equal(2, engine.confidence[block1.ID()])

	// Round 4: Strong convergence (5-0-0)
	round4 := NewBag[ids.ID]()
	round4.AddCount(block1.ID(), 5)
	require.NoError(engine.RecordPoll(ctx, round4))

	// Block1 should now be finalized (confidence >= Beta)
	require.Equal(StatusAccepted, block1.Status())
	require.Equal(StatusRejected, block2.Status())
	require.Equal(StatusRejected, block3.Status())
	require.Zero(engine.NumProcessing())

	// Verify convergence achieved
	require.Equal(block1.ID(), engine.lastAccepted)
	require.Equal(block1.ID(), engine.preference)
}

// TestRecordPollRegressionIndegree is a regression test for indegree calculation
// bugs. Ensures that when counting votes transitively, each child block is only
// counted once per parent (no double-counting).
func TestRecordPollRegressionIndegree(t *testing.T) {
	require := require.New(t)
	engine := NewTransitiveEngine()
	ctx := context.Background()

	params := TransitiveParams{
		K:               3,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            1,
	}

	genesisID := ids.GenerateTestID()
	genesisHeight := uint64(0)
	genesisTime := time.Now()

	require.NoError(engine.Initialize(ctx, params, genesisID, genesisHeight, genesisTime))

	// Create a tree structure to test indegree calculation:
	//        genesis
	//          |
	//        block0
	//        /    \
	//    block1  block2
	//       |      |
	//    block3  block4

	block0 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block0.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}
	block2 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block0.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}
	block3 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    3,
		ParentV:    block1.ID(),
		TimestampV: genesisTime.Add(3 * time.Second),
	}
	block4 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    3,
		ParentV:    block2.ID(),
		TimestampV: genesisTime.Add(3 * time.Second),
	}

	require.NoError(engine.Add(block0))
	require.NoError(engine.Add(block1))
	require.NoError(engine.Add(block2))
	require.NoError(engine.Add(block3))
	require.NoError(engine.Add(block4))

	// Vote for both leaf blocks (block3 and block4)
	// These votes should transitively apply to block0 through different paths
	// Bug to test: ensure block0 doesn't get double-counted
	votes := NewBag[ids.ID]()
	votes.Add(block3.ID()) // Votes: block3 -> block1 -> block0
	votes.Add(block4.ID()) // Votes: block4 -> block2 -> block0

	require.NoError(engine.RecordPoll(ctx, votes))

	// With transitive voting:
	// - block3: 1 direct vote
	// - block4: 1 direct vote
	// - block1: 1 transitive vote (from block3)
	// - block2: 1 transitive vote (from block4)
	// - block0: 2 transitive votes (from block1 and block2)

	// Verify transitive vote counts
	require.Equal(1, engine.votes[block3.ID()])
	require.Equal(1, engine.votes[block4.ID()])
	require.Equal(1, engine.votes[block1.ID()])
	require.Equal(1, engine.votes[block2.ID()])
	require.Equal(2, engine.votes[block0.ID()]) // Should be exactly 2, not 4

	// None should be finalized yet (only 2 votes, need 3 for alpha)
	require.Equal(5, engine.NumProcessing())

	// Add one more vote to push block0 over threshold
	votes2 := NewBag[ids.ID]()
	votes2.Add(block3.ID()) // Another vote for block3
	require.NoError(engine.RecordPoll(ctx, votes2))

	// Now block0 should have 2 votes again (1 from block3 directly)
	// This poll only has 1 vote, so nothing finalizes
	require.Equal(5, engine.NumProcessing())

	// Vote with 3 votes for block3 to finalize the chain
	votes3 := NewBag[ids.ID]()
	votes3.AddCount(block3.ID(), 3)
	require.NoError(engine.RecordPoll(ctx, votes3))

	// Now the chain through block3 should be accepted
	require.Equal(StatusAccepted, block0.Status())
	require.Equal(StatusAccepted, block1.Status())
	require.Equal(StatusAccepted, block3.Status())

	// The competing chain should be rejected
	require.Equal(StatusRejected, block2.Status())
	require.Equal(StatusRejected, block4.Status())

	require.Zero(engine.NumProcessing())
}
