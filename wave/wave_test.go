// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/internal/types"
	"github.com/luxfi/consensus/photon"
	"github.com/luxfi/consensus/prism"
	"github.com/stretchr/testify/require"
)

type ItemID string

type mockTransport struct {
	prefer bool
}

func (t mockTransport) RequestVotes(ctx context.Context, peers []types.NodeID, item ItemID) <-chan photon.Photon[ItemID] {
	out := make(chan photon.Photon[ItemID], len(peers))
	go func() {
		defer close(out)
		// Return votes immediately for testing
		for i := 0; i < len(peers); i++ {
			out <- photon.Photon[ItemID]{Item: item, Prefer: t.prefer}
		}
	}()
	return out
}

func (t mockTransport) MakeLocalPhoton(item ItemID, prefer bool) photon.Photon[ItemID] {
	return photon.Photon[ItemID]{Item: item, Prefer: prefer}
}

func TestWaveBasic(t *testing.T) {
	peers := []types.NodeID{"n1", "n2", "n3", "n4", "n5"}
	sel := prism.NewDefault(peers, prism.Options{})
	w := New[ItemID](Config{
		K:       3,
		Alpha:   0.8,
		Beta:    3,
		RoundTO: 100 * time.Millisecond,
	}, sel, mockTransport{prefer: true})

	ctx := context.Background()
	item := ItemID("block#123")

	// Run consensus rounds
	for i := 0; i < 10; i++ {
		w.Tick(ctx, item)
		st, ok := w.State(item)
		require.True(t, ok)
		
		t.Logf("Round %d: prefer=%v, conf=%d, decided=%v, stage=%v", i+1, st.Step.Prefer, st.Step.Conf, st.Decided, st.Stage)
		
		if st.Decided {
			require.Equal(t, types.DecideAccept, st.Result)
			require.True(t, st.Step.Prefer)
			require.GreaterOrEqual(t, st.Step.Conf, uint32(3))
			return
		}
	}
	
	t.Fatal("Should have decided after 5 rounds")
}

func TestWaveFPC(t *testing.T) {
	peers := []types.NodeID{"n1", "n2", "n3", "n4", "n5", "n6", "n7"}
	sel := prism.NewDefault(peers, prism.Options{})
	
	// Create wave with FPC enabled (default)
	w := New[ItemID](Config{
		K:       5,
		Alpha:   0.8,
		Beta:    5,
		Gamma:   2, // Switch to FPC after 2 inconclusive
		RoundTO: 50 * time.Millisecond,
	}, sel, mockTransport{prefer: false})

	ctx := context.Background()
	item := ItemID("tx#456")

	// Test FPC activation
	for i := 0; i < 10; i++ {
		w.Tick(ctx, item)
		st, _ := w.State(item)
		
		if st.Stage == StageFPC {
			// FPC activated
			require.GreaterOrEqual(t, i, 2)
		}
		
		if st.Decided {
			require.Equal(t, types.DecideReject, st.Result)
			return
		}
	}
}

func TestWaveTimeout(t *testing.T) {
	peers := []types.NodeID{"n1", "n2", "n3"}
	sel := prism.NewDefault(peers, prism.Options{})
	
	// Transport that never responds
	slowTransport := mockTransport{prefer: true}
	w := New[ItemID](Config{
		K:       3,
		Alpha:   0.8,
		Beta:    3,
		RoundTO: 10 * time.Millisecond, // Very short timeout
	}, sel, slowTransport)

	ctx := context.Background()
	item := ItemID("slow#789")
	
	start := time.Now()
	w.Tick(ctx, item)
	elapsed := time.Since(start)
	
	// Should timeout quickly
	require.Less(t, elapsed, 50*time.Millisecond)
	
	st, ok := w.State(item)
	require.True(t, ok)
	require.False(t, st.Decided) // Shouldn't decide on timeout alone
}