package prism

import (
	"context"
	"testing"

	"github.com/luxfi/consensus/types"
)

func TestDefaultSampler(t *testing.T) {
	peers := []types.NodeID{"n1", "n2", "n3", "n4", "n5"}
	opts := DefaultOptions()
	opts.MinPeers = 2
	opts.MaxPeers = 4

	s := New(peers, opts)

	// Test basic sampling
	ctx := context.Background()
	sample := s.Sample(ctx, 3, types.Topic("test"))
	if len(sample) != 3 {
		t.Errorf("expected 3 peers, got %d", len(sample))
	}

	// Verify uniqueness
	seen := make(map[types.NodeID]bool)
	for _, p := range sample {
		if seen[p] {
			t.Error("duplicate peer in sample")
		}
		seen[p] = true
	}

	// Test bounds
	sample = s.Sample(ctx, 10, types.Topic("test"))
	if len(sample) != opts.MaxPeers {
		t.Errorf("expected max %d peers, got %d", opts.MaxPeers, len(sample))
	}

	sample = s.Sample(ctx, 0, types.Topic("test"))
	if len(sample) != opts.MinPeers {
		t.Errorf("expected min %d peers, got %d", opts.MinPeers, len(sample))
	}
}

func TestHealthTracking(t *testing.T) {
	peers := []types.NodeID{"n1", "n2"}
	s := New(peers, DefaultOptions())

	// Initially all should have equal health
	ctx := context.Background()
	sample1 := s.Sample(ctx, 1, types.Topic("test"))
	if len(sample1) != 1 {
		t.Fatal("expected 1 peer")
	}

	// Report good behavior for n1
	s.Report("n1", types.ProbeGood)
	s.Report("n1", types.ProbeGood)

	// Report bad behavior for n2
	s.Report("n2", types.ProbeTimeout)
	s.Report("n2", types.ProbeBadSig)

	// Now n1 should be preferred
	counts := map[types.NodeID]int{"n1": 0, "n2": 0}
	for i := 0; i < 100; i++ {
		sample := s.Sample(ctx, 1, types.Topic("test"))
		counts[sample[0]]++
	}

	if counts["n1"] <= counts["n2"] {
		t.Error("expected n1 to be selected more often due to better health")
	}
}

func TestStakeWeighting(t *testing.T) {
	peers := []types.NodeID{"n1", "n2"}
	opts := DefaultOptions()
	opts.Stake = func(id types.NodeID) float64 {
		if id == "n1" {
			return 10.0 // n1 has 10x stake
		}
		return 1.0
	}

	s := New(peers, opts)
	ctx := context.Background()

	// n1 should be heavily preferred due to stake
	counts := map[types.NodeID]int{"n1": 0, "n2": 0}
	for i := 0; i < 100; i++ {
		sample := s.Sample(ctx, 1, types.Topic("test"))
		counts[sample[0]]++
	}

	if counts["n1"] < counts["n2"]*5 {
		t.Error("expected n1 to be selected much more often due to stake")
	}
}