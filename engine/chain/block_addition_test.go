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

// TestInitialize verifies Initialize() sets correct initial state:
// preference = genesis, numProcessing = 0, lastAccepted = genesis.
func TestInitialize(t *testing.T) {
	require := require.New(t)
	engine := NewTransitiveEngine()
	ctx := context.Background()

	params := TransitiveParams{
		K:               3,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            5,
	}

	genesisID := ids.GenerateTestID()
	genesisHeight := uint64(0)
	genesisTime := time.Now()

	require.NoError(engine.Initialize(ctx, params, genesisID, genesisHeight, genesisTime))

	// Verify initial state
	require.Equal(genesisID, engine.preference, "preference should be genesis")
	require.Equal(genesisID, engine.lastAccepted, "lastAccepted should be genesis")
	require.Zero(engine.NumProcessing(), "numProcessing should be 0")

	// Verify genesis block is in blocks map
	genesisBlock, exists := engine.blocks[genesisID]
	require.True(exists, "genesis block should be in blocks map")
	require.Equal(StatusAccepted, genesisBlock.Status(), "genesis should be accepted")

	// Verify genesis is finalized
	require.True(engine.finalized.Contains(genesisID), "genesis should be finalized")

	// Verify engine is bootstrapped
	require.True(engine.bootstrapped, "engine should be bootstrapped")

	// Verify params are set correctly
	require.Equal(params.K, engine.params.K)
	require.Equal(params.AlphaPreference, engine.params.AlphaPreference)
	require.Equal(params.AlphaConfidence, engine.params.AlphaConfidence)
	require.Equal(params.Beta, engine.params.Beta)
}

// TestAddToTail verifies that adding a block that extends the current preference
// UPDATES the preference to the new block.
func TestAddToTail(t *testing.T) {
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

	// Initial preference should be genesis
	require.Equal(genesisID, engine.preference)

	// Add a block that extends genesis (the current preference)
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	require.NoError(engine.Add(block1))

	// Preference should update to block1 since it extends the tail
	require.Equal(block1.ID(), engine.preference, "preference should update to block1")

	// Block should be in processing
	require.True(engine.processing.Contains(block1.ID()))
	require.Equal(1, engine.NumProcessing())

	// Add another block extending block1 (the new tail)
	block2 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block1.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}
	require.NoError(engine.Add(block2))

	// Preference should update to block2
	require.Equal(block2.ID(), engine.preference, "preference should update to block2")
	require.Equal(2, engine.NumProcessing())

	// Verify the chain: genesis -> block1 -> block2
	require.Equal(genesisID, block1.ParentID())
	require.Equal(block1.ID(), block2.ParentID())
}

// TestAddToNonTail verifies that adding a block that is a sibling
// (not extending current preference) does NOT change the preference.
func TestAddToNonTail(t *testing.T) {
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

	// Add block1 extending genesis
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	require.NoError(engine.Add(block1))

	// Preference should be block1
	require.Equal(block1.ID(), engine.preference)

	// Add block2 also extending genesis (sibling of block1, not extending the tail)
	block2 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	require.NoError(engine.Add(block2))

	// Preference should still be block1 (higher height takes precedence in shouldUpdatePreference,
	// but both are same height, so preference stays with block1)
	require.Equal(block1.ID(), engine.preference, "preference should remain block1")

	// Both blocks should be in processing
	require.True(engine.processing.Contains(block1.ID()))
	require.True(engine.processing.Contains(block2.ID()))
	require.Equal(2, engine.NumProcessing())

	// Add block3 extending block1 (the current preference tail)
	block3 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block1.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}
	require.NoError(engine.Add(block3))

	// Now preference should update to block3 (extends the tail and is higher)
	require.Equal(block3.ID(), engine.preference, "preference should update to block3")
	require.Equal(3, engine.NumProcessing())

	// Add block4 extending block2 (not the current preference)
	block4 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block2.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}
	require.NoError(engine.Add(block4))

	// Preference should still be block3 (same height as block4, but block3 was already preferred)
	require.Equal(block3.ID(), engine.preference, "preference should remain block3")
	require.Equal(4, engine.NumProcessing())
}

// TestAddOnUnknownParent verifies that adding a block with an unknown/missing
// parent returns an error.
func TestAddOnUnknownParent(t *testing.T) {
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

	// Create a block with an unknown parent ID
	unknownParentID := ids.GenerateTestID()
	orphanBlock := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    unknownParentID,
		TimestampV: time.Now(),
		StatusV:    StatusProcessing,
	}

	// Adding a block with unknown parent should return an error
	err := engine.Add(orphanBlock)
	require.Error(err, "should return error for unknown parent")
	require.Contains(err.Error(), "unknown parent", "error should mention unknown parent")

	// Block should NOT be added to processing set
	require.False(engine.processing.Contains(orphanBlock.ID()))
	require.Zero(engine.NumProcessing())

	// Preference should remain genesis
	require.Equal(genesisID, engine.preference)
}

// TestLastAccepted verifies that LastAccepted() correctly tracks the most
// recently accepted block through multiple accepts.
func TestLastAccepted(t *testing.T) {
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

	// Initially, lastAccepted should be genesis
	require.Equal(genesisID, engine.lastAccepted, "initially lastAccepted should be genesis")

	// Add and accept block1
	block1 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    1,
		ParentV:    genesisID,
		TimestampV: genesisTime.Add(time.Second),
	}
	require.NoError(engine.Add(block1))

	votes1 := NewBag[ids.ID]()
	votes1.Add(block1.ID())
	require.NoError(engine.RecordPoll(ctx, votes1))

	// LastAccepted should now be block1
	require.Equal(block1.ID(), engine.lastAccepted, "lastAccepted should be block1 after acceptance")
	require.Equal(StatusAccepted, block1.Status())
	require.Zero(engine.NumProcessing())

	// Add and accept block2
	block2 := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    2,
		ParentV:    block1.ID(),
		TimestampV: genesisTime.Add(2 * time.Second),
	}
	require.NoError(engine.Add(block2))

	votes2 := NewBag[ids.ID]()
	votes2.Add(block2.ID())
	require.NoError(engine.RecordPoll(ctx, votes2))

	// LastAccepted should now be block2
	require.Equal(block2.ID(), engine.lastAccepted, "lastAccepted should be block2 after acceptance")
	require.Equal(StatusAccepted, block2.Status())
	require.Zero(engine.NumProcessing())

	// Add competing blocks at height 3
	block3a := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    3,
		ParentV:    block2.ID(),
		TimestampV: genesisTime.Add(3 * time.Second),
	}
	block3b := &TestBlock{
		IDV:        ids.GenerateTestID(),
		HeightV:    3,
		ParentV:    block2.ID(),
		TimestampV: genesisTime.Add(3 * time.Second),
	}

	require.NoError(engine.Add(block3a))
	require.NoError(engine.Add(block3b))
	require.Equal(2, engine.NumProcessing())

	// Accept block3a
	votes3a := NewBag[ids.ID]()
	votes3a.Add(block3a.ID())
	require.NoError(engine.RecordPoll(ctx, votes3a))

	// LastAccepted should now be block3a
	require.Equal(block3a.ID(), engine.lastAccepted, "lastAccepted should be block3a after acceptance")
	require.Equal(StatusAccepted, block3a.Status())

	// Block3b should be rejected
	require.Equal(StatusRejected, block3b.Status())
	require.Zero(engine.NumProcessing())

	// Verify the acceptance chain: genesis -> block1 -> block2 -> block3a
	require.Equal(genesisID, block1.ParentID())
	require.Equal(block1.ID(), block2.ParentID())
	require.Equal(block2.ID(), block3a.ParentID())
}
