package consensus

import (
	"context"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/wave"
	"github.com/luxfi/consensus/types"
)

type mockTransport struct {
	votes []wave.VoteMsg[string]
}

func (m *mockTransport) RequestVotes(ctx context.Context, peers []types.NodeID, item string) (<-chan wave.VoteMsg[string], error) {
	ch := make(chan wave.VoteMsg[string], len(m.votes))
	for _, v := range m.votes {
		ch <- v
	}
	close(ch)
	return ch, nil
}

func TestNewChainEngine(t *testing.T) {
	peers := []types.NodeID{"n1", "n2", "n3"}
	transport := &mockTransport{
		votes: []wave.VoteMsg[string]{
			{Item: "item1", Prefer: true, From: "n1"},
			{Item: "item1", Prefer: true, From: "n2"},
			{Item: "item1", Prefer: false, From: "n3"},
		},
	}

	cfg := Config(3)
	engine := NewChainEngine[string](cfg, peers, transport)

	ctx := context.Background()
	engine.Tick(ctx, "item1")

	state, ok := engine.State("item1")
	if !ok {
		t.Fatal("expected state for item1")
	}
	if !state.Step.Prefer {
		t.Error("expected prefer=true with 2/3 votes")
	}
}

func TestConfig(t *testing.T) {
	tests := []struct {
		nodes int
		alpha float64
		beta  uint32
	}{
		{5, 0.6, 3},
		{11, 0.7, 6},
		{21, 0.8, 15},
		{100, 0.8, 30},
	}

	for _, test := range tests {
		cfg := Config(test.nodes)
		if cfg.K != test.nodes {
			t.Errorf("nodes %d: expected K=%d, got %d", test.nodes, test.nodes, cfg.K)
		}
		if cfg.Alpha != test.alpha {
			t.Errorf("nodes %d: expected Alpha=%f, got %f", test.nodes, test.alpha, cfg.Alpha)
		}
		if cfg.Beta != test.beta {
			t.Errorf("nodes %d: expected Beta=%d, got %d", test.nodes, test.beta, cfg.Beta)
		}
	}
}

func TestNewFinalizer(t *testing.T) {
	f := NewFinalizer[string]()

	// Test initial state
	finalized, depth := f.Finalized("block1")
	if finalized || depth != 0 {
		t.Error("new block should not be finalized")
	}

	// Add decision
	f.OnDecide("block1", DecideAccept)
	finalized, depth = f.Finalized("block1")
	if !finalized || depth == 0 {
		t.Error("decided block should be finalized")
	}

	// Test reject
	f.OnDecide("block2", DecideReject)
	finalized, _ = f.Finalized("block2")
	if finalized {
		t.Error("rejected block should not be finalized")
	}
}

func TestNewSampler(t *testing.T) {
	peers := []NodeID{"n1", "n2", "n3", "n4", "n5"}
	sampler := NewSampler[string](peers, DefaultSamplerOptions())

	ctx := context.Background()
	sample := sampler.Sample(ctx, 3, types.Topic("test"))

	if len(sample) != 3 {
		t.Errorf("expected 3 peers, got %d", len(sample))
	}

	// Check uniqueness
	seen := make(map[NodeID]bool)
	for _, p := range sample {
		if seen[p] {
			t.Error("duplicate peer in sample")
		}
		seen[p] = true
	}
}

func TestConstants(t *testing.T) {
	// Test that constants are properly exported
	if DecideAccept != types.DecideAccept {
		t.Error("DecideAccept mismatch")
	}
	if DecideReject != types.DecideReject {
		t.Error("DecideReject mismatch")
	}
	if DecideUndecided != types.DecideUndecided {
		t.Error("DecideUndecided mismatch")
	}
}

func TestDefaultParams(t *testing.T) {
	cfg := config.DefaultParams()
	if cfg.K == 0 {
		t.Error("default K should not be 0")
	}
	if cfg.Alpha == 0 {
		t.Error("default Alpha should not be 0")
	}
	if cfg.Beta == 0 {
		t.Error("default Beta should not be 0")
	}
}