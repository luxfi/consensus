package wave

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/photon"
	"github.com/luxfi/consensus/types"
)

type mockTransport struct {
	votes []VoteMsg[string]
	delay time.Duration
}

func (m *mockTransport) RequestVotes(ctx context.Context, peers []types.NodeID, item string) (<-chan VoteMsg[string], error) {
	ch := make(chan VoteMsg[string], len(m.votes))
	
	if m.delay > 0 {
		// Simulate network delay
		go func() {
			time.Sleep(m.delay)
			for _, v := range m.votes {
				ch <- v
			}
			close(ch)
		}()
	} else {
		for _, v := range m.votes {
			ch <- v
		}
		close(ch)
	}
	return ch, nil
}

func TestWaveBasic(t *testing.T) {
	cfg := config.DefaultParams()
	cfg.K = 3
	cfg.Alpha = 0.6
	cfg.Beta = 2

	peers := []types.NodeID{"n1", "n2", "n3"}
	sel := photon.NewUniformEmitter(peers, photon.DefaultEmitterOptions())

	// Test accept case
	tx := &mockTransport{
		votes: []VoteMsg[string]{
			{Item: "item1", Prefer: true, From: "n1"},
			{Item: "item1", Prefer: true, From: "n2"},
			{Item: "item1", Prefer: false, From: "n3"},
		},
	}

	w := New[string](cfg, sel, tx)
	ctx := context.Background()

	// First round
	w.Tick(ctx, "item1")
	st, ok := w.State("item1")
	if !ok {
		t.Fatal("expected state")
	}
	if !st.Step.Prefer {
		t.Error("expected prefer=true after 2/3 yes votes")
	}
	if st.Step.Conf != 1 {
		t.Errorf("expected conf=1, got %d", st.Step.Conf)
	}

	// Second round - should increase confidence
	w.Tick(ctx, "item1")
	st, _ = w.State("item1")
	if st.Step.Conf != 2 {
		t.Errorf("expected conf=2, got %d", st.Step.Conf)
	}
	if !st.Decided {
		t.Error("expected decided after reaching beta")
	}
	if st.Result != types.DecideAccept {
		t.Error("expected accept decision")
	}
}

func TestWaveReject(t *testing.T) {
	cfg := config.DefaultParams()
	cfg.K = 3
	cfg.Alpha = 0.6
	cfg.Beta = 1

	peers := []types.NodeID{"n1", "n2", "n3"}
	sel := photon.NewUniformEmitter(peers, photon.DefaultEmitterOptions())

	tx := &mockTransport{
		votes: []VoteMsg[string]{
			{Item: "item2", Prefer: false, From: "n1"},
			{Item: "item2", Prefer: false, From: "n2"},
			{Item: "item2", Prefer: true, From: "n3"},
		},
	}

	w := New[string](cfg, sel, tx)
	ctx := context.Background()

	w.Tick(ctx, "item2")
	st, ok := w.State("item2")
	if !ok {
		t.Fatal("expected state")
	}
	if st.Step.Prefer {
		t.Error("expected prefer=false after 2/3 no votes")
	}
	if !st.Decided {
		t.Error("expected decided after reaching beta=1")
	}
	if st.Result != types.DecideReject {
		t.Error("expected reject decision")
	}
}

func TestWaveTimeout(t *testing.T) {
	cfg := config.DefaultParams()
	cfg.RoundTO = 50 * time.Millisecond

	peers := []types.NodeID{"n1", "n2", "n3"}
	sel := photon.NewUniformEmitter(peers, photon.DefaultEmitterOptions())

	// Transport with delay longer than timeout
	tx := &mockTransport{
		votes: []VoteMsg[string]{
			{Item: "item3", Prefer: true, From: "n1"},
		},
		delay: 100 * time.Millisecond, // Longer than timeout
	}

	w := New[string](cfg, sel, tx)
	ctx := context.Background()

	start := time.Now()
	w.Tick(ctx, "item3")
	elapsed := time.Since(start)

	// Should timeout before receiving vote
	if elapsed < (cfg.RoundTO - 10*time.Millisecond) {
		t.Errorf("expected to wait for timeout, elapsed=%v, timeout=%v", elapsed, cfg.RoundTO)
	}
	if elapsed > (cfg.RoundTO + 20*time.Millisecond) {
		t.Errorf("took too long, elapsed=%v, timeout=%v", elapsed, cfg.RoundTO)
	}

	st, ok := w.State("item3")
	if !ok {
		t.Fatal("expected state")
	}
	if st.Decided {
		t.Error("should not decide on timeout")
	}
}