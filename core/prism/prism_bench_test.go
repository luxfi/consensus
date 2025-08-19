package prism

import (
	"context"
	"testing"

	"github.com/luxfi/consensus/types"
)

func BenchmarkSampler(b *testing.B) {
	peers := make([]types.NodeID, 100)
	for i := 0; i < 100; i++ {
		peers[i] = types.NodeID(string(rune(i)))
	}

	s := New(peers, DefaultOptions())
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Sample(ctx, 21, types.Topic("test"))
	}
}

func BenchmarkSamplerWithHealth(b *testing.B) {
	peers := make([]types.NodeID, 100)
	for i := 0; i < 100; i++ {
		peers[i] = types.NodeID(string(rune(i)))
	}

	s := New(peers, DefaultOptions())
	ctx := context.Background()

	// Add some health reports
	for i := 0; i < 50; i++ {
		s.Report(peers[i], types.ProbeGood)
	}
	for i := 50; i < 100; i++ {
		s.Report(peers[i], types.ProbeTimeout)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Sample(ctx, 21, types.Topic("test"))
	}
}

func BenchmarkSamplerLargeK(b *testing.B) {
	peers := make([]types.NodeID, 1000)
	for i := 0; i < 1000; i++ {
		peers[i] = types.NodeID(string(rune(i)))
	}

	s := New(peers, DefaultOptions())
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Sample(ctx, 100, types.Topic("test"))
	}
}