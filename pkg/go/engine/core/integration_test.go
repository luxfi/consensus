package core

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// TestConsensusIntegration verifies the consensus engine works correctly
func TestConsensusIntegration(t *testing.T) {
	require := require.New(t)

	// Create consensus parameters
	params := ConsensusParams{
		K:                     20,
		AlphaPreference:      15,
		AlphaConfidence:      15,
		Beta:                 20,
		ConcurrentPolls:      10,
		OptimalProcessing:    10,
		MaxOutstandingItems:  1000,
		MaxItemProcessingTime: 30 * time.Second,
	}

	// Create consensus engine
	consensus, err := NewCGOConsensus(params)
	require.NoError(err)
	require.NotNil(consensus)

	// Verify parameters
	retrievedParams := consensus.Parameters()
	require.Equal(params.K, retrievedParams.K)
	require.Equal(params.AlphaPreference, retrievedParams.AlphaPreference)

	// Test preference
	testID := ids.GenerateTestID()
	consensus.preference.Store(testID)
	require.Equal(testID, consensus.GetPreference())

	// Test acceptance
	blockID := ids.GenerateTestID()
	err = consensus.RecordPoll(blockID, true)
	require.NoError(err)
	require.True(consensus.IsAccepted(blockID))

	// Test health check
	err = consensus.HealthCheck()
	require.NoError(err)

	// Test finalized state
	require.False(consensus.Finalized())
}

// MockBlock implements the Block interface for testing
type MockBlock struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp int64
	data      []byte
}

func (b *MockBlock) ID() ids.ID          { return b.id }
func (b *MockBlock) ParentID() ids.ID    { return b.parentID }
func (b *MockBlock) Height() uint64      { return b.height }
func (b *MockBlock) Timestamp() int64    { return b.timestamp }
func (b *MockBlock) Bytes() []byte       { return b.data }
func (b *MockBlock) Verify(context.Context) error  { return nil }
func (b *MockBlock) Accept(context.Context) error  { return nil }
func (b *MockBlock) Reject(context.Context) error  { return nil }

// TestConsensusWithBlocks tests the consensus engine with mock blocks
func TestConsensusWithBlocks(t *testing.T) {
	require := require.New(t)

	params := ConsensusParams{
		K:                     5,
		AlphaPreference:      3,
		AlphaConfidence:      3,
		Beta:                 5,
		ConcurrentPolls:      1,
		OptimalProcessing:    1,
		MaxOutstandingItems:  100,
		MaxItemProcessingTime: 10 * time.Second,
	}

	consensus, err := NewCGOConsensus(params)
	require.NoError(err)

	// Create and add a mock block
	block := &MockBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now().Unix(),
		data:      []byte("test block"),
	}

	err = consensus.Add(block)
	require.NoError(err)

	// Verify the block was cached
	consensus.cacheMu.RLock()
	cached, exists := consensus.blockCache[block.ID()]
	consensus.cacheMu.RUnlock()
	require.True(exists)
	require.Equal(block.ID(), cached.id)
	require.Equal(StatusProcessing, cached.status)

	// Verify preference was updated
	require.Equal(block.ID(), consensus.GetPreference())

	// Simulate voting to accept the block
	for i := 0; i < params.K; i++ {
		err = consensus.RecordPoll(block.ID(), true)
		require.NoError(err)
	}

	// Verify the block is accepted
	require.True(consensus.IsAccepted(block.ID()))
}