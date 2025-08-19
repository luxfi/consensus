package chain

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/prism"
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

func TestChainEngine(t *testing.T) {
	cfg := config.DefaultParams()
	cfg.K = 5
	cfg.Alpha = 0.6
	cfg.Beta = 2

	peers := []types.NodeID{"n1", "n2", "n3", "n4", "n5"}
	sel := prism.New(peers, prism.DefaultOptions())

	tx := &mockTransport{
		votes: []wave.VoteMsg[string]{
			{Item: "block1", Prefer: true, From: "n1"},
			{Item: "block1", Prefer: true, From: "n2"},
			{Item: "block1", Prefer: true, From: "n3"},
			{Item: "block1", Prefer: false, From: "n4"},
			{Item: "block1", Prefer: false, From: "n5"},
		},
	}

	engine := New[string](cfg, sel, tx)
	ctx := context.Background()

	// First tick - should build confidence
	engine.Tick(ctx, "block1")
	state, ok := engine.State("block1")
	if !ok {
		t.Fatal("expected state for block1")
	}
	if !state.Step.Prefer {
		t.Error("expected prefer=true with 3/5 votes")
	}
	if state.Step.Conf != 1 {
		t.Errorf("expected conf=1, got %d", state.Step.Conf)
	}

	// Second tick - should reach decision
	engine.Tick(ctx, "block1")
	state, ok = engine.State("block1")
	if !ok {
		t.Fatal("expected state for block1")
	}
	if !state.Decided {
		t.Error("expected decision after beta=2")
	}
	if state.Result != types.DecideAccept {
		t.Error("expected accept decision")
	}
}

func TestChainEngineReject(t *testing.T) {
	cfg := config.DefaultParams()
	cfg.K = 3
	cfg.Alpha = 0.6
	cfg.Beta = 1

	peers := []types.NodeID{"n1", "n2", "n3"}
	sel := prism.New(peers, prism.DefaultOptions())

	tx := &mockTransport{
		votes: []wave.VoteMsg[string]{
			{Item: "block2", Prefer: false, From: "n1"},
			{Item: "block2", Prefer: false, From: "n2"},
			{Item: "block2", Prefer: true, From: "n3"},
		},
	}

	engine := New[string](cfg, sel, tx)
	ctx := context.Background()

	engine.Tick(ctx, "block2")
	state, ok := engine.State("block2")
	if !ok {
		t.Fatal("expected state for block2")
	}
	if state.Step.Prefer {
		t.Error("expected prefer=false with 2/3 no votes")
	}
	if !state.Decided {
		t.Error("expected decision with beta=1")
	}
	if state.Result != types.DecideReject {
		t.Error("expected reject decision")
	}
}

func TestChainEngineMultipleBlocks(t *testing.T) {
	cfg := config.DefaultParams()
	cfg.K = 3
	cfg.Alpha = 0.6
	cfg.Beta = 2
	cfg.RoundTO = 10 * time.Millisecond

	peers := []types.NodeID{"n1", "n2", "n3"}
	sel := prism.New(peers, prism.DefaultOptions())

	// Different votes for different blocks
	votes := map[string][]wave.VoteMsg[string]{
		"block1": {
			{Item: "block1", Prefer: true, From: "n1"},
			{Item: "block1", Prefer: true, From: "n2"},
			{Item: "block1", Prefer: false, From: "n3"},
		},
		"block2": {
			{Item: "block2", Prefer: false, From: "n1"},
			{Item: "block2", Prefer: false, From: "n2"},
			{Item: "block2", Prefer: true, From: "n3"},
		},
	}

	ctx := context.Background()

	// Process block1
	tx1 := &mockTransport{votes: votes["block1"]}
	engine1 := New[string](cfg, sel, tx1)
	
	engine1.Tick(ctx, "block1")
	engine1.Tick(ctx, "block1")
	
	state1, ok := engine1.State("block1")
	if !ok || !state1.Decided || state1.Result != types.DecideAccept {
		t.Error("block1 should be accepted")
	}

	// Process block2
	tx2 := &mockTransport{votes: votes["block2"]}
	engine2 := New[string](cfg, sel, tx2)
	
	engine2.Tick(ctx, "block2")
	engine2.Tick(ctx, "block2")
	
	state2, ok := engine2.State("block2")
	if !ok || !state2.Decided || state2.Result != types.DecideReject {
		t.Error("block2 should be rejected")
	}
}

func TestChainEngineConcurrency(t *testing.T) {
	cfg := config.DefaultParams()
	cfg.K = 5
	cfg.Alpha = 0.6
	cfg.Beta = 3

	peers := []types.NodeID{"n1", "n2", "n3", "n4", "n5"}
	sel := prism.New(peers, prism.DefaultOptions())

	tx := &mockTransport{
		votes: []wave.VoteMsg[string]{
			{Item: "block", Prefer: true, From: "n1"},
			{Item: "block", Prefer: true, From: "n2"},
			{Item: "block", Prefer: true, From: "n3"},
			{Item: "block", Prefer: true, From: "n4"},
			{Item: "block", Prefer: false, From: "n5"},
		},
	}

	engine := New[string](cfg, sel, tx)
	ctx := context.Background()

	// Run ticks sequentially to reach decision
	for i := 0; i < 3; i++ {
		engine.Tick(ctx, "block")
	}

	state, ok := engine.State("block")
	if !ok {
		t.Fatal("expected state for block")
	}
	if !state.Decided {
		t.Error("expected decision after 3 ticks with beta=3")
	}
}