package photon

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/types"
)

func TestUniformEmitter(t *testing.T) {
	peers := []types.NodeID{"node1", "node2", "node3", "node4", "node5"}
	opts := EmitterOptions{
		MinPeers: 2,
		MaxPeers: 4,
	}
	emitter := NewUniformEmitter(peers, opts)

	t.Run("basic emission", func(t *testing.T) {
		ctx := context.Background()
		emitted, err := emitter.Emit(ctx, 3, 12345)
		if err != nil {
			t.Fatalf("emission failed: %v", err)
		}
		if len(emitted) != 3 {
			t.Errorf("expected 3 nodes, got %d", len(emitted))
		}

		// Verify all emitted nodes are valid
		for _, node := range emitted {
			found := false
			for _, peer := range peers {
				if node == peer {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("invalid node emitted: %s", node)
			}
		}
	})

	t.Run("luminance tracking", func(t *testing.T) {
		// Test that nodes with more successful votes get higher brightness
		ctx := context.Background()

		// Boost node1's brightness
		for i := 0; i < 10; i++ {
			emitter.Report(peers[0], true) // Increase lux for node1
		}

		// Dim node5's brightness
		for i := 0; i < 10; i++ {
			emitter.Report(peers[4], false) // Decrease lux for node5
		}

		// Count emissions over multiple rounds
		emissions := make(map[types.NodeID]int)
		for i := 0; i < 100; i++ {
			emitted, _ := emitter.Emit(ctx, 1, uint64(i))
			if len(emitted) > 0 {
				emissions[emitted[0]]++
			}
		}

		// node1 should be emitted more often than node5
		if emissions[peers[0]] <= emissions[peers[4]] {
			t.Errorf("brighter node1 (%d emissions) should be selected more than dimmer node5 (%d emissions)",
				emissions[peers[0]], emissions[peers[4]])
		}
	})

	t.Run("respect limits", func(t *testing.T) {
		ctx := context.Background()

		// Request more than MaxPeers
		emitted, _ := emitter.Emit(ctx, 10, 999)
		if len(emitted) > opts.MaxPeers {
			t.Errorf("emitted %d nodes, exceeds MaxPeers %d", len(emitted), opts.MaxPeers)
		}

		// Request 0 should use MinPeers
		emitted, _ = emitter.Emit(ctx, 0, 888)
		if len(emitted) != opts.MinPeers {
			t.Errorf("expected MinPeers %d, got %d", opts.MinPeers, len(emitted))
		}
	})

	t.Run("with stake weighting", func(t *testing.T) {
		// Create emitter with stake weighting
		stakeOpts := EmitterOptions{
			MinPeers: 2,
			MaxPeers: 4,
			Stake: func(id types.NodeID) float64 {
				// node1 has 10x more stake
				if id == peers[0] {
					return 10.0
				}
				return 1.0
			},
		}
		stakeEmitter := NewUniformEmitter(peers, stakeOpts)

		ctx := context.Background()
		emissions := make(map[types.NodeID]int)
		for i := 0; i < 100; i++ {
			emitted, _ := stakeEmitter.Emit(ctx, 1, uint64(i))
			if len(emitted) > 0 {
				emissions[emitted[0]]++
			}
		}

		// node1 with 10x stake should be selected much more often
		if emissions[peers[0]] < emissions[peers[1]]*3 {
			t.Errorf("high-stake node1 (%d) should be selected much more than node2 (%d)",
				emissions[peers[0]], emissions[peers[1]])
		}
	})

	t.Run("with latency penalty", func(t *testing.T) {
		// Create emitter with latency penalties
		latencyOpts := EmitterOptions{
			MinPeers: 2,
			MaxPeers: 4,
			Latency: func(id types.NodeID) time.Duration {
				// node5 has high latency
				if id == peers[4] {
					return 1000 * time.Millisecond
				}
				return 10 * time.Millisecond
			},
		}
		latencyEmitter := NewUniformEmitter(peers, latencyOpts)

		ctx := context.Background()
		emissions := make(map[types.NodeID]int)
		for i := 0; i < 100; i++ {
			emitted, _ := latencyEmitter.Emit(ctx, 1, uint64(i))
			if len(emitted) > 0 {
				emissions[emitted[0]]++
			}
		}

		// node5 with high latency should be selected less often
		if emissions[peers[4]] >= emissions[peers[0]] {
			t.Errorf("high-latency node5 (%d) should be selected less than node1 (%d)",
				emissions[peers[4]], emissions[peers[0]])
		}
	})
}

func TestSamplerCompatibility(t *testing.T) {
	peers := []types.NodeID{"node1", "node2", "node3"}
	adapter := NewSampler(peers, DefaultEmitterOptions())

	t.Run("Sample method", func(t *testing.T) {
		ctx := context.Background()
		sampled := adapter.Sample(ctx, 2, types.Topic("test"))
		if len(sampled) != 2 {
			t.Errorf("expected 2 nodes, got %d", len(sampled))
		}
	})

	t.Run("Report method", func(t *testing.T) {
		// Should not panic
		adapter.Report(peers[0], types.ProbeGood)
		adapter.Report(peers[1], types.ProbeTimeout)
	})

	t.Run("Allow method", func(t *testing.T) {
		if !adapter.Allow(types.Topic("any")) {
			t.Error("Allow should always return true for compatibility")
		}
	})
}

func BenchmarkEmission(b *testing.B) {
	peers := make([]types.NodeID, 100)
	for i := 0; i < 100; i++ {
		peers[i] = types.NodeID(string(rune('A' + i%26)))
	}

	emitter := NewUniformEmitter(peers, DefaultEmitterOptions())
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = emitter.Emit(ctx, 20, uint64(i))
	}
}

func BenchmarkLuminanceUpdate(b *testing.B) {
	peers := make([]types.NodeID, 100)
	for i := 0; i < 100; i++ {
		peers[i] = types.NodeID(string(rune('A' + i%26)))
	}

	emitter := NewUniformEmitter(peers, DefaultEmitterOptions())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		emitter.Report(peers[i%100], i%2 == 0)
	}
}
