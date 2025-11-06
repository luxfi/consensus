// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/consensus/types"
	"github.com/stretchr/testify/require"
)

// mockTransport implements Transport for testing
type mockTransport[T comparable] struct {
	votes     map[T][]Photon[T]
	responses chan Photon[T]
}

func newMockTransport[T comparable]() *mockTransport[T] {
	return &mockTransport[T]{
		votes:     make(map[T][]Photon[T]),
		responses: make(chan Photon[T], 100),
	}
}

func (m *mockTransport[T]) RequestVotes(ctx context.Context, peers []types.NodeID, item T) <-chan Photon[T] {
	ch := make(chan Photon[T], len(m.votes[item]))
	go func() {
		defer close(ch)
		for _, vote := range m.votes[item] {
			select {
			case ch <- vote:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

func (m *mockTransport[T]) MakeLocalPhoton(item T, prefer bool) Photon[T] {
	nodeID := [20]byte{1}
	return Photon[T]{
		Item:      item,
		Prefer:    prefer,
		Sender:    nodeID,
		Timestamp: time.Now(),
	}
}

func (m *mockTransport[T]) AddVote(item T, prefer bool) {
	if m.votes[item] == nil {
		m.votes[item] = make([]Photon[T], 0)
	}
	nodeID := [20]byte{byte(len(m.votes[item]) + 1)}
	m.votes[item] = append(m.votes[item], Photon[T]{
		Item:      item,
		Prefer:    prefer,
		Sender:    nodeID,
		Timestamp: time.Now(),
	})
}

// mockCut implements prism.Cut for testing
type mockCut[T comparable] struct {
	peers []types.NodeID
}

func newMockCut[T comparable](n int) *mockCut[T] {
	peers := make([]types.NodeID, n)
	for i := 0; i < n; i++ {
		nodeID := [20]byte{byte(i + 1)}
		peers[i] = nodeID
	}
	return &mockCut[T]{peers: peers}
}

func (m *mockCut[T]) Sample(k int) []types.NodeID {
	if k > len(m.peers) {
		k = len(m.peers)
	}
	return m.peers[:k]
}

func (m *mockCut[T]) Luminance() prism.Luminance {
	return prism.Luminance{
		ActivePeers: len(m.peers),
		TotalPeers:  len(m.peers),
		Lx:          float64(len(m.peers)),
	}
}

// TestWaveBasicVoting tests basic wave consensus voting
func TestWaveBasicVoting(t *testing.T) {
	require := require.New(t)

	cfg := Config{
		K:       5,
		Alpha:   0.8,
		Beta:    3,
		RoundTO: 100 * time.Millisecond,
	}

	cut := newMockCut[string](10)
	tx := newMockTransport[string]()
	wave := New[string](cfg, cut, tx)

	// Add unanimous votes for "tx1"
	for i := 0; i < 5; i++ {
		tx.AddVote("tx1", true)
	}

	ctx := context.Background()
	wave.Tick(ctx, "tx1")

	// Check state after one round
	state, exists := wave.State("tx1")
	require.True(exists)
	require.False(state.Decided) // Need Beta rounds
	require.Equal(uint32(1), state.Count)
}

// TestWaveConfidenceBuilding tests confidence building over multiple rounds
func TestWaveConfidenceBuilding(t *testing.T) {
	require := require.New(t)

	cfg := Config{
		K:       5,
		Alpha:   0.8,
		Beta:    3,
		RoundTO: 100 * time.Millisecond,
	}

	cut := newMockCut[string](10)
	tx := newMockTransport[string]()
	wave := New[string](cfg, cut, tx)

	// Add unanimous votes
	for i := 0; i < 5; i++ {
		tx.AddVote("tx1", true)
	}

	ctx := context.Background()

	// Run Beta rounds
	for i := uint32(0); i < cfg.Beta; i++ {
		wave.Tick(ctx, "tx1")
	}

	// Should be decided after Beta rounds
	state, exists := wave.State("tx1")
	require.True(exists)
	require.True(state.Decided)
	require.Equal(types.DecideAccept, state.Result)
}

// TestWavePreferenceSwitch tests preference switching
func TestWavePreferenceSwitch(t *testing.T) {
	require := require.New(t)

	cfg := Config{
		K:       5,
		Alpha:   0.8,
		Beta:    3,
		RoundTO: 100 * time.Millisecond,
	}

	cut := newMockCut[string](10)
	tx := newMockTransport[string]()
	wave := New[string](cfg, cut, tx)

	ctx := context.Background()

	// First round: majority yes
	for i := 0; i < 5; i++ {
		tx.AddVote("tx1", true)
	}
	wave.Tick(ctx, "tx1")

	state, _ := wave.State("tx1")
	require.Equal(uint32(1), state.Count)
	require.True(wave.Preference("tx1"))

	// Clear votes and add majority no
	tx.votes["tx1"] = nil
	for i := 0; i < 5; i++ {
		tx.AddVote("tx1", false)
	}
	wave.Tick(ctx, "tx1")

	// Count should reset on preference switch
	state, _ = wave.State("tx1")
	require.Equal(uint32(1), state.Count)
	require.False(wave.Preference("tx1"))
}

// TestWaveSplitVote tests handling of split votes
func TestWaveSplitVote(t *testing.T) {
	require := require.New(t)

	cfg := Config{
		K:       10,
		Alpha:   0.8, // Need 8 votes
		Beta:    3,
		RoundTO: 100 * time.Millisecond,
	}

	cut := newMockCut[string](15)
	tx := newMockTransport[string]()
	wave := New[string](cfg, cut, tx)

	// Split vote: 5 yes, 5 no (neither reaches threshold)
	for i := 0; i < 5; i++ {
		tx.AddVote("tx1", true)
		tx.AddVote("tx1", false)
	}

	ctx := context.Background()
	wave.Tick(ctx, "tx1")

	// Count should reset on split vote
	state, _ := wave.State("tx1")
	require.Equal(uint32(0), state.Count)
}

// TestWaveMultipleItems tests handling multiple items concurrently
func TestWaveMultipleItems(t *testing.T) {
	require := require.New(t)

	cfg := Config{
		K:       5,
		Alpha:   0.8,
		Beta:    3,
		RoundTO: 100 * time.Millisecond,
	}

	cut := newMockCut[string](10)
	tx := newMockTransport[string]()
	wave := New[string](cfg, cut, tx)

	// Add votes for multiple items
	for i := 0; i < 5; i++ {
		tx.AddVote("tx1", true)
		tx.AddVote("tx2", false)
		tx.AddVote("tx3", true)
	}

	ctx := context.Background()

	// Process all items
	wave.Tick(ctx, "tx1")
	wave.Tick(ctx, "tx2")
	wave.Tick(ctx, "tx3")

	// Check all states
	state1, _ := wave.State("tx1")
	state2, _ := wave.State("tx2")
	state3, _ := wave.State("tx3")

	require.Equal(uint32(1), state1.Count)
	require.True(wave.Preference("tx1"))

	require.Equal(uint32(1), state2.Count)
	require.False(wave.Preference("tx2"))

	require.Equal(uint32(1), state3.Count)
	require.True(wave.Preference("tx3"))
}

// TestWaveTimeout tests handling of vote timeout
func TestWaveTimeout(t *testing.T) {
	require := require.New(t)

	cfg := Config{
		K:       5,
		Alpha:   0.8,
		Beta:    3,
		RoundTO: 50 * time.Millisecond,
	}

	cut := newMockCut[string](10)
	tx := newMockTransport[string]()
	wave := New[string](cfg, cut, tx)

	// Don't add any votes (will timeout)
	ctx := context.Background()
	wave.Tick(ctx, "tx1")

	// Should have state but no progress
	_, exists := wave.State("tx1")
	require.True(exists)
}

// TestWaveDecisionPersistence tests that decisions persist
func TestWaveDecisionPersistence(t *testing.T) {
	require := require.New(t)

	cfg := Config{
		K:       5,
		Alpha:   0.8,
		Beta:    3,
		RoundTO: 100 * time.Millisecond,
	}

	cut := newMockCut[string](10)
	tx := newMockTransport[string]()
	wave := New[string](cfg, cut, tx)

	// Add unanimous votes
	for i := 0; i < 5; i++ {
		tx.AddVote("tx1", true)
	}

	ctx := context.Background()

	// Reach decision
	for i := uint32(0); i < cfg.Beta; i++ {
		wave.Tick(ctx, "tx1")
	}

	state, _ := wave.State("tx1")
	require.True(state.Decided)

	// Clear votes and add opposite votes
	tx.votes["tx1"] = nil
	for i := 0; i < 5; i++ {
		tx.AddVote("tx1", false)
	}

	// Additional tick should not change decision
	wave.Tick(ctx, "tx1")

	state, _ = wave.State("tx1")
	require.True(state.Decided)
	require.Equal(types.DecideAccept, state.Result) // Original decision persists
}

// TestWaveHighThroughput tests wave under high transaction load
func TestWaveHighThroughput(t *testing.T) {
	require := require.New(t)

	cfg := Config{
		K:       5,
		Alpha:   0.8,
		Beta:    3,
		RoundTO: 100 * time.Millisecond,
	}

	cut := newMockCut[string](100)
	tx := newMockTransport[string]()
	wave := New[string](cfg, cut, tx)

	ctx := context.Background()

	// Process many items
	for i := 0; i < 100; i++ {
		item := string(rune('a' + i))
		for j := 0; j < 5; j++ {
			tx.AddVote(item, true)
		}
		wave.Tick(ctx, item)
	}

	// Verify all processed
	for i := 0; i < 100; i++ {
		item := string(rune('a' + i))
		_, exists := wave.State(item)
		require.True(exists)
	}
}
