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

// TestStatusOrProcessingAccepted verifies that blocks with Accepted status (status=2)
// are NOT in the processing set and ARE marked as preferred at their height.
func TestStatusOrProcessingAccepted(t *testing.T) {
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

	// Genesis is already accepted
	genesis := engine.blocks[genesisID]
	require.Equal(StatusAccepted, genesis.Status())

	// Accepted blocks should NOT be in processing set
	require.False(engine.processing.Contains(genesisID))

	// Genesis should be the preference (last accepted block)
	require.Equal(genesisID, engine.preference)

	// Add a child block and accept it
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	require.NoError(engine.Add(block1))

	// Block 1 should be in processing initially
	require.True(engine.processing.Contains(block1.ID()))
	require.Equal(StatusProcessing, block1.Status())

	// Vote to accept block1
	votes := NewBag[ids.ID]()
	votes.Add(block1.ID())
	require.NoError(engine.RecordPoll(ctx, votes))

	// Block 1 should now be accepted
	require.Equal(StatusAccepted, block1.Status())

	// Accepted block should NOT be in processing set
	require.False(engine.processing.Contains(block1.ID()))

	// Block 1 should be the new preference
	require.Equal(block1.ID(), engine.preference)
	require.Equal(block1.ID(), engine.lastAccepted)
}

// TestStatusOrProcessingRejected verifies that blocks with Rejected status (status=3)
// are NOT in the processing set and are NOT marked as preferred.
func TestStatusOrProcessingRejected(t *testing.T) {
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

	// Build two competing blocks
	block0 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}

	require.NoError(engine.Add(block0))
	require.NoError(engine.Add(block1))

	// Both should be in processing initially
	require.True(engine.processing.Contains(block0.ID()))
	require.True(engine.processing.Contains(block1.ID()))

	// Vote to accept block0 (which will reject block1)
	votes := NewBag[ids.ID]()
	votes.Add(block0.ID())
	require.NoError(engine.RecordPoll(ctx, votes))

	// Block 0 should be accepted, block 1 should be rejected
	require.Equal(StatusAccepted, block0.Status())
	require.Equal(StatusRejected, block1.Status())

	// Rejected block should NOT be in processing set
	require.False(engine.processing.Contains(block1.ID()))

	// Rejected block should NOT be the preference
	require.NotEqual(block1.ID(), engine.preference)
	require.Equal(block0.ID(), engine.preference)
}

// TestStatusOrProcessingUnissued verifies that blocks that haven't been added
// to consensus (unissued) are NOT in the processing set and are NOT marked as preferred.
func TestStatusOrProcessingUnissued(t *testing.T) {
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

	// Build a block but don't add it to the engine
	unissuedBlock := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}

	// Unissued block should NOT be in processing set
	require.False(engine.processing.Contains(unissuedBlock.ID()))

	// Unissued block should NOT be the preference
	require.NotEqual(unissuedBlock.ID(), engine.preference)

	// Preference should still be genesis
	require.Equal(genesisID, engine.preference)

	// Number of processing blocks should be 0
	require.Zero(engine.NumProcessing())
}

// TestStatusOrProcessingIssued verifies that blocks added via Add() ARE in the
// processing set and ARE marked as preferred (if they extend the preferred chain).
func TestStatusOrProcessingIssued(t *testing.T) {
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

	// Build and add a block that extends genesis
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	require.NoError(engine.Add(block1))

	// Issued block should be in processing set
	require.True(engine.processing.Contains(block1.ID()))

	// Status should be Processing
	require.Equal(StatusProcessing, block1.Status())

	// Since block1 extends the current preference (genesis), it should become the new preference
	require.Equal(block1.ID(), engine.preference)

	// Number of processing blocks should be 1
	require.Equal(1, engine.NumProcessing())

	// Add another block extending block1
	block2 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block1.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}
	require.NoError(engine.Add(block2))

	// Block2 should also be in processing
	require.True(engine.processing.Contains(block2.ID()))

	// Block2 should be the new preference (extends current preference)
	require.Equal(block2.ID(), engine.preference)

	// Number of processing blocks should be 2
	require.Equal(2, engine.NumProcessing())
}
