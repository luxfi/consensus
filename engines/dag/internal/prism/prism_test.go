// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/types"
	"github.com/stretchr/testify/require"
)

func TestPrismBasicSampling(t *testing.T) {
	peers := []types.NodeID{"node1", "node2", "node3", "node4", "node5"}
	sampler := NewDefault(peers, Options{
		MinPeers: 2,
		MaxPeers: 10,
	})

	ctx := context.Background()

	// Sample 3 peers
	selected := sampler.Sample(ctx, 3, types.Topic("test"))
	require.Len(t, selected, 3)

	// Should be from our peer set
	for _, peer := range selected {
		require.Contains(t, peers, peer)
	}
}

func TestPrismSampleSize(t *testing.T) {
	peers := []types.NodeID{"n1", "n2", "n3", "n4", "n5", "n6", "n7", "n8", "n9", "n10"}
	sampler := NewDefault(peers, Options{
		MinPeers: 3,
		MaxPeers: 7,
	})

	ctx := context.Background()

	// Request 0 should give MinPeers
	selected := sampler.Sample(ctx, 0, types.Topic("test"))
	require.Len(t, selected, 3)

	// Request within bounds
	selected = sampler.Sample(ctx, 5, types.Topic("test"))
	require.Len(t, selected, 5)

	// Request exceeding MaxPeers should be capped
	selected = sampler.Sample(ctx, 20, types.Topic("test"))
	require.Len(t, selected, 7)
}

func TestPrismWithStake(t *testing.T) {
	peers := []types.NodeID{"high-stake", "medium-stake", "low-stake"}

	stakeFunc := func(id types.NodeID) float64 {
		switch id {
		case "high-stake":
			return 100.0
		case "medium-stake":
			return 10.0
		case "low-stake":
			return 1.0
		default:
			return 1.0
		}
	}

	sampler := NewDefault(peers, Options{
		MinPeers: 1,
		MaxPeers: 3,
		Stake:    stakeFunc,
	})

	ctx := context.Background()

	// Sample multiple times and count selections
	selections := make(map[types.NodeID]int)
	for i := 0; i < 100; i++ {
		selected := sampler.Sample(ctx, 1, types.Topic("test"))
		if len(selected) > 0 {
			selections[selected[0]]++
		}
	}

	// High stake should be selected most often
	require.Greater(t, selections["high-stake"], selections["low-stake"])
}

func TestPrismWithLatency(t *testing.T) {
	peers := []types.NodeID{"fast", "medium", "slow"}

	latencyFunc := func(id types.NodeID) time.Duration {
		switch id {
		case "fast":
			return 10 * time.Millisecond
		case "medium":
			return 100 * time.Millisecond
		case "slow":
			return 1000 * time.Millisecond
		default:
			return 100 * time.Millisecond
		}
	}

	sampler := NewDefault(peers, Options{
		MinPeers: 1,
		MaxPeers: 3,
		Latency:  latencyFunc,
	})

	ctx := context.Background()

	// Sample multiple times
	selections := make(map[types.NodeID]int)
	for i := 0; i < 100; i++ {
		selected := sampler.Sample(ctx, 1, types.Topic("test"))
		if len(selected) > 0 {
			selections[selected[0]]++
		}
	}

	// Fast peer should be selected more often
	require.Greater(t, selections["fast"], selections["slow"])
}

func TestPrismHealthReporting(t *testing.T) {
	peers := []types.NodeID{"healthy", "unhealthy"}
	sampler := NewDefault(peers, Options{
		MinPeers: 1,
		MaxPeers: 2,
	})

	// Report good behavior for healthy
	for i := 0; i < 5; i++ {
		sampler.Report("healthy", types.ProbeGood)
	}

	// Report bad behavior for unhealthy
	for i := 0; i < 5; i++ {
		sampler.Report("unhealthy", types.ProbeTimeout)
	}

	ctx := context.Background()

	// Sample multiple times
	selections := make(map[types.NodeID]int)
	for i := 0; i < 100; i++ {
		selected := sampler.Sample(ctx, 1, types.Topic("test"))
		if len(selected) > 0 {
			selections[selected[0]]++
		}
	}

	// Healthy peer should be selected more often
	require.Greater(t, selections["healthy"], selections["unhealthy"])
}

func TestPrismAllow(t *testing.T) {
	peers := []types.NodeID{"n1", "n2"}
	sampler := NewDefault(peers, Options{})

	// Should allow all topics by default
	require.True(t, sampler.Allow(types.Topic("test")))
	require.True(t, sampler.Allow(types.Topic("votes")))
	require.True(t, sampler.Allow(types.Topic("blocks")))
}

func TestPrismEmptyPeers(t *testing.T) {
	sampler := NewDefault([]types.NodeID{}, Options{})

	ctx := context.Background()
	selected := sampler.Sample(ctx, 5, types.Topic("test"))

	// Should return empty when no peers
	require.Empty(t, selected)
}

func TestPrismDeterministic(t *testing.T) {
	peers := []types.NodeID{"a", "b", "c", "d", "e"}
	sampler := NewDefault(peers, Options{
		MinPeers: 3,
		MaxPeers: 5,
	})

	ctx := context.Background()

	// Should be deterministic for same input
	selected1 := sampler.Sample(ctx, 3, types.Topic("test"))
	selected2 := sampler.Sample(ctx, 3, types.Topic("test"))

	require.Equal(t, selected1, selected2)
}

func BenchmarkPrismSample(b *testing.B) {
	// Create large peer set
	peers := make([]types.NodeID, 1000)
	for i := range peers {
		peers[i] = types.NodeID(string(rune(i)))
	}

	sampler := NewDefault(peers, Options{
		MinPeers: 10,
		MaxPeers: 100,
	})

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sampler.Sample(ctx, 20, types.Topic("bench"))
	}
}

func BenchmarkPrismWithWeighting(b *testing.B) {
	peers := make([]types.NodeID, 100)
	for i := range peers {
		peers[i] = types.NodeID(string(rune(i)))
	}

	sampler := NewDefault(peers, Options{
		MinPeers: 5,
		MaxPeers: 20,
		Stake: func(id types.NodeID) float64 {
			return float64(len(id) % 10)
		},
		Latency: func(id types.NodeID) time.Duration {
			return time.Duration(len(id)%100) * time.Millisecond
		},
	})

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sampler.Sample(ctx, 10, types.Topic("bench"))
	}
}
