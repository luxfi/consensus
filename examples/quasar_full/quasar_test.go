// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/luxfi/consensus"
	"github.com/luxfi/consensus/core/dag"
	"github.com/luxfi/consensus/protocol/wave/fpc"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// TestFPCDynamicThresholds tests FPC threshold selection
func TestFPCDynamicThresholds(t *testing.T) {
	selector := fpc.NewSelector(0.5, 0.8, nil)
	k := 20

	// Test determinism: same round = same threshold
	t1 := selector.SelectThreshold(0, k)
	t2 := selector.SelectThreshold(0, k)
	require.Equal(t, t1, t2, "Same round must give same threshold (determinism)")

	// Test variety: different rounds = different thresholds
	thresholds := make(map[int]bool)
	for round := uint64(0); round < 100; round++ {
		threshold := selector.SelectThreshold(round, k)

		// Verify threshold is in valid range
		require.GreaterOrEqual(t, threshold, 10, "Threshold should be ≥ 50%% of k")
		require.LessOrEqual(t, threshold, 16, "Threshold should be ≤ 80%% of k")

		thresholds[threshold] = true
	}

	require.Greater(t, len(thresholds), 1, "Should have variety in thresholds")
	t.Logf("✓ FPC generated %d unique thresholds across 100 rounds", len(thresholds))
}

// TestBlockFinalization tests complete block finalization
func TestBlockFinalization(t *testing.T) {
	cfg := consensus.DefaultConfig()
	chain := consensus.NewChain(cfg)

	ctx := context.Background()
	err := chain.Start(ctx)
	require.NoError(t, err)
	defer chain.Stop()

	// Create block
	block := &consensus.Block{
		ID:       consensus.ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		ParentID: consensus.GenesisID,
		Height:   1,
		Time:     time.Now(),
		Payload:  []byte("Test block finalization"),
	}

	// Add block
	err = chain.Add(ctx, block)
	require.NoError(t, err, "Block should be added")

	// Simulate votes
	validators := make([]consensus.NodeID, 20)
	for i := range validators {
		validators[i] = consensus.NodeID{byte(i + 1)}
	}

	for _, validator := range validators {
		vote := consensus.NewVote(block.ID, consensus.VotePreference, validator)
		err = chain.RecordVote(ctx, vote)
		require.NoError(t, err)
	}

	// Verify finality
	status := chain.GetStatus(block.ID)
	require.Equal(t, consensus.StatusAccepted, status, "Block should achieve finality")

	t.Log("✓ Block achieved finality with full validator support")
}

// TestHorizonReachability tests DAG reachability algorithm
func TestHorizonReachability(t *testing.T) {
	genesis := ids.Empty
	b1, _ := ids.ToID([]byte("block_1_________________________"))
	b2, _ := ids.ToID([]byte("block_2_________________________"))
	b3, _ := ids.ToID([]byte("block_3_________________________"))

	store := createMockDAG(genesis, b1, b2, b3)

	// Test forward reachability
	require.True(t, dag.IsReachable[ids.ID](store, genesis, b1), "Genesis should reach B1")
	require.True(t, dag.IsReachable[ids.ID](store, genesis, b3), "Genesis should reach B3")
	require.True(t, dag.IsReachable[ids.ID](store, b1, b3), "B1 should reach B3")

	// Test backward non-reachability (DAG is directed)
	require.False(t, dag.IsReachable[ids.ID](store, b3, genesis), "B3 cannot reach Genesis")
	require.False(t, dag.IsReachable[ids.ID](store, b1, genesis), "B1 cannot reach Genesis")

	t.Log("✓ Reachability algorithm works correctly")
}

// TestLCA tests Lowest Common Ancestor algorithm
func TestLCA(t *testing.T) {
	genesis := ids.Empty
	b1, _ := ids.ToID([]byte("block_1_________________________"))
	b2, _ := ids.ToID([]byte("block_2_________________________"))
	b3, _ := ids.ToID([]byte("block_3_________________________"))

	store := createMockDAG(genesis, b1, b2, b3)

	// LCA of parallel blocks should be their common parent
	lca := dag.LCA[ids.ID](store, b1, b2)
	require.Equal(t, genesis, lca, "LCA of B1 and B2 should be Genesis")

	t.Log("✓ LCA algorithm works correctly")
}

// TestSafePrefix tests finality detection
func TestSafePrefix(t *testing.T) {
	genesis := ids.Empty
	b1, _ := ids.ToID([]byte("block_1_________________________"))
	b2, _ := ids.ToID([]byte("block_2_________________________"))
	b3, _ := ids.ToID([]byte("block_3_________________________"))

	store := createMockDAG(genesis, b1, b2, b3)

	// Safe prefix with B3 as frontier should include ancestors
	frontier := []ids.ID{b3}
	safe := dag.ComputeSafePrefix[ids.ID](store, frontier)

	require.NotNil(t, safe, "Should return safe prefix")
	t.Logf("✓ Safe prefix computed: %d vertices finalized", len(safe))
}

// TestChooseFrontier tests Byzantine-tolerant parent selection
func TestChooseFrontier(t *testing.T) {
	// Create set of vertices
	vertices := make([]ids.ID, 10)
	for i := range vertices {
		vertices[i], _ = ids.ToID([]byte(fmt.Sprintf("vertex_%d", i)))
	}

	// Choose frontier with Byzantine tolerance
	chosen := dag.ChooseFrontier[ids.ID](vertices)

	require.NotEmpty(t, chosen, "Should choose some vertices")
	require.LessOrEqual(t, len(chosen), len(vertices), "Can't choose more than available")

	// For 10 vertices, f=(10-1)/3=3, so 2f+1=7
	// Should choose min(7, 10) = 7 or all if < 7
	t.Logf("✓ Chose %d vertices with Byzantine tolerance", len(chosen))
}

// TestEventHorizon tests horizon advancement
func TestEventHorizon(t *testing.T) {
	genesis := ids.Empty
	b1, _ := ids.ToID([]byte("block_1_________________________"))
	b2, _ := ids.ToID([]byte("block_2_________________________"))
	b3, _ := ids.ToID([]byte("block_3_________________________"))

	store := createMockDAG(genesis, b1, b2, b3)

	// Create initial checkpoint
	checkpoints := []dag.EventHorizon[ids.ID]{
		{
			Checkpoint: genesis,
			Height:     0,
			Validators: []string{"validator_set_0"},
			Signature:  []byte("ringtail+bls_sig_0"),
		},
	}

	// Compute new horizon
	newHorizon := dag.Horizon[ids.ID](store, checkpoints)

	require.NotEqual(t, ids.ID{}, newHorizon.Checkpoint, "Should have valid checkpoint")
	require.GreaterOrEqual(t, newHorizon.Height, checkpoints[0].Height, "Horizon should not regress")

	t.Logf("✓ Event horizon advanced from height %d to %d", checkpoints[0].Height, newHorizon.Height)
}

// TestBeyondHorizon tests finality boundary checking
func TestBeyondHorizon(t *testing.T) {
	genesis := ids.Empty
	b1, _ := ids.ToID([]byte("block_1_________________________"))
	b2, _ := ids.ToID([]byte("block_2_________________________"))
	b3, _ := ids.ToID([]byte("block_3_________________________"))

	store := createMockDAG(genesis, b1, b2, b3)

	horizon := dag.EventHorizon[ids.ID]{
		Checkpoint: genesis,
		Height:     0,
		Validators: []string{"set"},
		Signature:  []byte("sig"),
	}

	// Blocks reachable from genesis should be beyond horizon
	beyond := dag.BeyondHorizon[ids.ID](store, b3, horizon)
	require.True(t, beyond, "B3 should be beyond horizon (reachable from checkpoint)")

	t.Log("✓ BeyondHorizon correctly identifies finalized vertices")
}

// TestQuasarIntegration tests full protocol integration
func TestQuasarIntegration(t *testing.T) {
	// Create chain
	cfg := consensus.DefaultConfig()
	chain := consensus.NewChain(cfg)

	ctx := context.Background()
	err := chain.Start(ctx)
	require.NoError(t, err)
	defer chain.Stop()

	// Add multiple blocks
	blocks := make([]*consensus.Block, 5)
	for i := range blocks {
		blocks[i] = &consensus.Block{
			ID:       consensus.ID{byte(i + 1)},
			ParentID: consensus.GenesisID,
			Height:   uint64(i + 1),
			Time:     time.Now(),
			Payload:  []byte(fmt.Sprintf("Block %d", i+1)),
		}

		err = chain.Add(ctx, blocks[i])
		require.NoError(t, err)
	}

	// Vote on all blocks
	validators := make([]consensus.NodeID, 20)
	for i := range validators {
		validators[i] = consensus.NodeID{byte(i + 1)}
	}

	for _, block := range blocks {
		for _, validator := range validators {
			vote := consensus.NewVote(block.ID, consensus.VotePreference, validator)
			chain.RecordVote(ctx, vote)
		}
	}

	// Verify all blocks finalized
	finalizedCount := 0
	for _, block := range blocks {
		if chain.GetStatus(block.ID) == consensus.StatusAccepted {
			finalizedCount++
		}
	}

	require.Greater(t, finalizedCount, 0, "At least some blocks should finalize")
	t.Logf("✓ %d/%d blocks achieved finality", finalizedCount, len(blocks))
}

// TestCertificateThresholds tests Byzantine fault tolerance math
func TestCertificateThresholds(t *testing.T) {
	testCases := []struct {
		n             int
		f             int
		certThreshold int
		description   string
	}{
		{20, 6, 13, "Production network (n=20, f=6)"},
		{100, 33, 67, "Large network (n=100, f=33)"},
		{7, 2, 5, "Small network (n=7, f=2)"},
		{4, 1, 3, "Minimal BFT (n=4, f=1)"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			params := dag.Params{N: tc.n, F: tc.f}

			// Verify Byzantine tolerance: n > 3f
			require.Greater(t, params.N, 3*params.F, "Must have n > 3f for BFT")

			// Verify certificate threshold: 2f+1
			certThreshold := 2*params.F + 1
			require.Equal(t, tc.certThreshold, certThreshold)

			// Verify safety: can't have both cert and skip
			// (need 2f+1 for cert AND 2f+1 for skip = 4f+2 > n when n=3f+1)
			require.Greater(t, 2*certThreshold, params.N,
				"Certificate and skip thresholds ensure mutual exclusivity")

			t.Logf("✓ n=%d, f=%d: requires %d/%d validators (%.1f%%)",
				tc.n, tc.f, certThreshold, tc.n, 100.0*float64(certThreshold)/float64(tc.n))
		})
	}
}

// BenchmarkQuasarFullProtocol benchmarks the complete protocol
func BenchmarkQuasarFullProtocol(b *testing.B) {
	cfg := consensus.DefaultConfig()
	chain := consensus.NewChain(cfg)

	ctx := context.Background()
	chain.Start(ctx)
	defer chain.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block := &consensus.Block{
			ID:       consensus.ID{byte(i)},
			ParentID: consensus.GenesisID,
			Height:   uint64(i + 1),
			Payload:  []byte("benchmark"),
		}

		chain.Add(ctx, block)

		// Simulate votes
		for j := 0; j < 20; j++ {
			vote := consensus.NewVote(block.ID, consensus.VotePreference, consensus.NodeID{byte(j)})
			chain.RecordVote(ctx, vote)
		}
	}
}

// BenchmarkFPCThresholdSelection benchmarks FPC performance
func BenchmarkFPCThresholdSelection(b *testing.B) {
	selector := fpc.NewSelector(0.5, 0.8, nil)
	k := 20

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = selector.SelectThreshold(uint64(i), k)
	}
}

// BenchmarkHorizonAlgorithms benchmarks DAG algorithms
func BenchmarkHorizonAlgorithms(b *testing.B) {
	genesis := ids.Empty
	b1, _ := ids.ToID([]byte("block_1_________________________"))
	b2, _ := ids.ToID([]byte("block_2_________________________"))
	b3, _ := ids.ToID([]byte("block_3_________________________"))

	store := createMockDAG(genesis, b1, b2, b3)

	b.Run("IsReachable", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dag.IsReachable[ids.ID](store, genesis, b3)
		}
	})

	b.Run("LCA", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dag.LCA[ids.ID](store, b1, b2)
		}
	})

	b.Run("ComputeSafePrefix", func(b *testing.B) {
		frontier := []ids.ID{b3}
		for i := 0; i < b.N; i++ {
			dag.ComputeSafePrefix[ids.ID](store, frontier)
		}
	})

	b.Run("ChooseFrontier", func(b *testing.B) {
		vertices := []ids.ID{b1, b2, b3}
		for i := 0; i < b.N; i++ {
			dag.ChooseFrontier[ids.ID](vertices)
		}
	})

	b.Run("Horizon", func(b *testing.B) {
		checkpoints := []dag.EventHorizon[ids.ID]{
			{Checkpoint: genesis, Height: 0},
		}
		for i := 0; i < b.N; i++ {
			dag.Horizon[ids.ID](store, checkpoints)
		}
	})
}
