package wave

import (
	"context"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/prism"
	"github.com/luxfi/consensus/types"
)

func BenchmarkWaveConsensus(b *testing.B) {
	cfg := config.DefaultParams()
	cfg.K = 21
	cfg.Alpha = 0.8
	cfg.Beta = 15

	peers := make([]types.NodeID, 21)
	for i := 0; i < 21; i++ {
		peers[i] = types.NodeID(string(rune('a' + i)))
	}

	sel := prism.New(peers, prism.DefaultOptions())
	
	// Create votes (80% accept)
	votes := make([]VoteMsg[string], 21)
	for i := 0; i < 17; i++ {
		votes[i] = VoteMsg[string]{Item: "item", Prefer: true, From: peers[i]}
	}
	for i := 17; i < 21; i++ {
		votes[i] = VoteMsg[string]{Item: "item", Prefer: false, From: peers[i]}
	}

	tx := &mockTransport{votes: votes}
	w := New[string](cfg, sel, tx)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		item := "item" + string(rune(i))
		w.Tick(ctx, item)
	}
}

func BenchmarkWaveLargeNetwork(b *testing.B) {
	cfg := config.DefaultParams()
	cfg.K = 100
	cfg.Alpha = 0.8
	cfg.Beta = 30

	peers := make([]types.NodeID, 100)
	for i := 0; i < 100; i++ {
		peers[i] = types.NodeID(string(rune(i)))
	}

	sel := prism.New(peers, prism.DefaultOptions())
	
	votes := make([]VoteMsg[string], 100)
	for i := 0; i < 80; i++ {
		votes[i] = VoteMsg[string]{Item: "item", Prefer: true, From: peers[i]}
	}
	for i := 80; i < 100; i++ {
		votes[i] = VoteMsg[string]{Item: "item", Prefer: false, From: peers[i]}
	}

	tx := &mockTransport{votes: votes}
	w := New[string](cfg, sel, tx)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		item := "item" + string(rune(i))
		w.Tick(ctx, item)
	}
}