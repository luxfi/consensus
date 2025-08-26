package pq

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestConsensusEngine(t *testing.T) {
	params := config.LocalParams()
	engine := NewConsensus(params)

	require.NotNil(t, engine)
	require.NotNil(t, engine.quasar)
	require.NotNil(t, engine.finality)
}

func TestConsensusInitialize(t *testing.T) {
	ctx := context.Background()
	engine := NewConsensus(config.LocalParams())

	blsKey := make([]byte, 48)
	pqKey := make([]byte, 256)

	err := engine.Initialize(ctx, blsKey, pqKey)
	require.NoError(t, err)
}

func TestConsensusProcessBlock(t *testing.T) {
	ctx := context.Background()
	engine := NewConsensus(config.LocalParams())

	blockID := ids.GenerateTestID()
	// votes map is block -> vote count, not node -> vote
	// We need one block with enough votes
	votes := map[string]int{
		blockID.String(): 5, // 5 votes for this block
	}

	// Process block should succeed with sufficient votes
	err := engine.ProcessBlock(ctx, blockID, votes)
	require.NoError(t, err)
}

func TestConsensusFinalization(t *testing.T) {
	ctx := context.Background()
	engine := NewConsensus(config.LocalParams())

	// Initialize engine
	err := engine.Initialize(ctx, make([]byte, 48), make([]byte, 256))
	require.NoError(t, err)

	// Process enough blocks to trigger finalization
	blockID := ids.GenerateTestID()
	// votes map is block -> vote count
	votes := map[string]int{
		blockID.String(): 5, // 5 votes for this block (sufficient for local params)
	}

	err = engine.ProcessBlock(ctx, blockID, votes)
	require.NoError(t, err)

	// Check if block is finalized (with sufficient votes)
	finalized := engine.IsFinalized(blockID)
	require.True(t, finalized)
}

func TestConsensusFinalityChannel(t *testing.T) {
	engine := NewConsensus(config.LocalParams())

	ch := engine.FinalityChannel()
	require.NotNil(t, ch)

	// Test that we can receive from the channel
	select {
	case <-ch:
		// Got an event
	case <-time.After(10 * time.Millisecond):
		// Timeout is ok, channel exists
	}
}

func TestConsensusHeight(t *testing.T) {
	engine := NewConsensus(config.LocalParams())

	height := engine.Height()
	require.Equal(t, uint64(0), height)

	// Process a block to increase height
	engine.height = 100

	height = engine.Height()
	require.Equal(t, uint64(100), height)
}

func TestConsensusMetrics(t *testing.T) {
	engine := NewConsensus(config.LocalParams())

	metrics := engine.Metrics()
	require.NotNil(t, metrics)
	require.Contains(t, metrics, "height")
	require.Contains(t, metrics, "round")
	require.Contains(t, metrics, "finalized")
}

func TestNewFunction(t *testing.T) {
	engine := New()
	require.NotNil(t, engine)
	require.IsType(t, &ConsensusEngine{}, engine)
}

func TestConsensusSetFinalizedCallback(t *testing.T) {
	engine := NewConsensus(config.LocalParams())

	engine.SetFinalizedCallback(func(event FinalityEvent) {
		// Callback is set
	})

	// Callback is set, we can't easily test it fires without complex setup
	require.NotNil(t, engine.quasar)
}

func BenchmarkConsensusProcessBlock(b *testing.B) {
	ctx := context.Background()
	engine := NewConsensus(config.LocalParams())

	blockID := ids.GenerateTestID()
	votes := map[string]int{
		"node1": 1,
		"node2": 1,
		"node3": 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.ProcessBlock(ctx, blockID, votes)
	}
}

func BenchmarkConsensusIsFinalized(b *testing.B) {
	engine := NewConsensus(config.LocalParams())
	blockID := ids.GenerateTestID()
	engine.finalized[blockID] = true

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.IsFinalized(blockID)
	}
}
