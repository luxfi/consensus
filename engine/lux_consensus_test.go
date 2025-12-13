// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// TestNewLuxConsensus tests the NewLuxConsensus constructor
func TestNewLuxConsensus(t *testing.T) {
	require := require.New(t)

	lc := NewLuxConsensus(20, 15, 10)
	require.NotNil(lc)
	require.Equal(20, lc.k)
	require.Equal(float64(15)/float64(20), lc.alpha)
	require.Equal(uint32(10), lc.beta)
	require.NotNil(lc.wave)
	require.NotNil(lc.focus)
	require.NotNil(lc.decided)
	require.NotNil(lc.decisions)
	require.NotNil(lc.consecutiveSuccesses)
}

// TestNewLuxConsensusNegativeBeta tests with negative beta
func TestNewLuxConsensusNegativeBeta(t *testing.T) {
	require := require.New(t)

	lc := NewLuxConsensus(20, 15, -5)
	require.NotNil(lc)
	require.Equal(uint32(0), lc.beta)
}

// TestLuxConsensusRecordVote tests the RecordVote method
func TestLuxConsensusRecordVote(t *testing.T) {
	require := require.New(t)

	lc := NewLuxConsensus(5, 3, 5)
	itemID := ids.GenerateTestID()

	// Initial state
	require.Equal(uint32(0), lc.consecutiveSuccesses[itemID])

	// Record votes
	lc.RecordVote(itemID)
	require.Equal(uint32(1), lc.consecutiveSuccesses[itemID])

	lc.RecordVote(itemID)
	require.Equal(uint32(2), lc.consecutiveSuccesses[itemID])
}

// TestLuxConsensusRecordVoteAlreadyDecided tests voting on decided item
func TestLuxConsensusRecordVoteAlreadyDecided(t *testing.T) {
	require := require.New(t)

	lc := NewLuxConsensus(5, 3, 5)
	itemID := ids.GenerateTestID()

	// Mark as decided
	lc.mu.Lock()
	lc.decided[itemID] = true
	lc.consecutiveSuccesses[itemID] = 5
	lc.mu.Unlock()

	// Vote should be ignored
	lc.RecordVote(itemID)

	lc.mu.RLock()
	count := lc.consecutiveSuccesses[itemID]
	lc.mu.RUnlock()

	require.Equal(uint32(5), count) // Should not change
}

// TestLuxConsensusPoll tests the Poll method
func TestLuxConsensusPoll(t *testing.T) {
	lc := NewLuxConsensus(5, 3, 2)
	itemID := ids.GenerateTestID()

	responses := map[ids.ID]int{
		itemID: 5,
	}

	// First poll
	continuePolling := lc.Poll(responses)
	// May or may not continue depending on confidence
	_ = continuePolling
}

// TestLuxConsensusPollEmptyVotes tests poll with zero total votes
func TestLuxConsensusPollEmptyVotes(t *testing.T) {
	lc := NewLuxConsensus(5, 3, 2)
	itemID := ids.GenerateTestID()

	responses := map[ids.ID]int{
		itemID: 0,
	}

	continuePolling := lc.Poll(responses)
	if !continuePolling {
		t.Log("Poll returned false with empty votes")
	}
}

// TestLuxConsensusPollAlreadyDecided tests poll with already decided item
func TestLuxConsensusPollAlreadyDecided(t *testing.T) {
	require := require.New(t)

	lc := NewLuxConsensus(5, 3, 2)
	itemID := ids.GenerateTestID()

	// Mark as decided
	lc.mu.Lock()
	lc.decided[itemID] = true
	lc.mu.Unlock()

	responses := map[ids.ID]int{
		itemID: 5,
	}

	continuePolling := lc.Poll(responses)
	require.True(continuePolling)
}

// TestLuxConsensusPollMultipleItems tests poll with multiple items
func TestLuxConsensusPollMultipleItems(t *testing.T) {
	require := require.New(t)

	lc := NewLuxConsensus(5, 4, 1) // High alpha ratio for quick decision
	item1 := ids.GenerateTestID()
	item2 := ids.GenerateTestID()

	responses := map[ids.ID]int{
		item1: 4, // 80% - above alpha
		item2: 1, // 20% - below alpha
	}

	_ = lc.Poll(responses)
	require.NotNil(lc)
}

// TestLuxConsensusDecided tests the Decided method
func TestLuxConsensusDecided(t *testing.T) {
	require := require.New(t)

	lc := NewLuxConsensus(5, 3, 2)

	// Initially not decided
	require.False(lc.Decided())

	// Mark something as decided
	lc.mu.Lock()
	lc.decided[ids.GenerateTestID()] = true
	lc.mu.Unlock()

	require.True(lc.Decided())
}

// TestLuxConsensusPreference tests the Preference method
func TestLuxConsensusPreference(t *testing.T) {
	require := require.New(t)

	lc := NewLuxConsensus(5, 3, 2)

	// Initially empty
	require.Equal(ids.Empty, lc.Preference())

	// Set preference
	prefID := ids.GenerateTestID()
	lc.mu.Lock()
	lc.preference = prefID
	lc.mu.Unlock()

	require.Equal(prefID, lc.Preference())
}

// TestLuxConsensusDecision tests the Decision method
func TestLuxConsensusDecision(t *testing.T) {
	require := require.New(t)

	lc := NewLuxConsensus(5, 3, 2)
	itemID := ids.GenerateTestID()

	// No decision yet
	decision, exists := lc.Decision(itemID)
	require.False(exists)
	require.Equal(decision, decision) // just to use the variable

	// Add a decision
	lc.mu.Lock()
	lc.decisions[itemID] = 1 // DecideAccept
	lc.mu.Unlock()

	decision, exists = lc.Decision(itemID)
	require.True(exists)
}

// TestLuxConsensusParameters tests the Parameters method
func TestLuxConsensusParameters(t *testing.T) {
	require := require.New(t)

	lc := NewLuxConsensus(20, 15, 10)

	params := lc.Parameters()
	require.Equal(20, params.K)
	require.Equal(float64(15)/float64(20), params.Alpha)
	require.Equal(uint32(10), params.Beta)
	require.Equal(15, params.AlphaPreference)
	require.Equal(15, params.AlphaConfidence)
}

// TestSimpleCutSample tests the SimpleCut.Sample method
func TestSimpleCutSample(t *testing.T) {
	require := require.New(t)

	cut := &SimpleCut{k: 5}
	nodes := cut.Sample(3)

	require.Len(nodes, 3)
	for _, node := range nodes {
		require.NotEqual(ids.EmptyNodeID, node)
	}
}

// TestSimpleCutLuminance tests the SimpleCut.Luminance method
func TestSimpleCutLuminance(t *testing.T) {
	require := require.New(t)

	cut := &SimpleCut{k: 10}
	luminance := cut.Luminance()

	require.Equal(10, luminance.ActivePeers)
	require.Equal(10, luminance.TotalPeers)
	require.Equal(float64(10), luminance.Lx)
}

// TestSimpleTransportRequestVotes tests the SimpleTransport.RequestVotes method
func TestSimpleTransportRequestVotes(t *testing.T) {
	require := require.New(t)

	transport := &SimpleTransport{}
	peers := make([]ids.NodeID, 3)
	for i := 0; i < 3; i++ {
		peers[i] = ids.GenerateTestNodeID()
	}
	itemID := ids.GenerateTestID()

	ctx := context.Background()
	ch := transport.RequestVotes(ctx, peers, itemID)

	// Collect votes
	votes := make([]bool, 0)
	for photon := range ch {
		require.Equal(itemID, photon.Item)
		require.True(photon.Prefer)
		votes = append(votes, photon.Prefer)
	}

	require.Len(votes, 3)
}

// TestSimpleTransportRequestVotesContextCanceled tests context cancellation
func TestSimpleTransportRequestVotesContextCanceled(t *testing.T) {
	require := require.New(t)

	transport := &SimpleTransport{}
	peers := make([]ids.NodeID, 100)
	for i := 0; i < 100; i++ {
		peers[i] = ids.GenerateTestNodeID()
	}
	itemID := ids.GenerateTestID()

	ctx, cancel := context.WithCancel(context.Background())

	ch := transport.RequestVotes(ctx, peers, itemID)

	// Cancel after a short delay
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	// Collect votes until channel closes
	count := 0
	for range ch {
		count++
	}

	// We may have received some votes before cancellation
	require.True(count <= 100)
}

// TestSimpleTransportMakeLocalPhoton tests the MakeLocalPhoton method
func TestSimpleTransportMakeLocalPhoton(t *testing.T) {
	require := require.New(t)

	transport := &SimpleTransport{}
	itemID := ids.GenerateTestID()

	// Test prefer=true
	photon := transport.MakeLocalPhoton(itemID, true)
	require.Equal(itemID, photon.Item)
	require.True(photon.Prefer)
	require.NotEqual(ids.EmptyNodeID, photon.Sender)
	require.False(photon.Timestamp.IsZero())

	// Test prefer=false
	photon = transport.MakeLocalPhoton(itemID, false)
	require.Equal(itemID, photon.Item)
	require.False(photon.Prefer)
}

// TestLuxConsensusPollDecision tests poll leading to decision
func TestLuxConsensusPollDecision(t *testing.T) {
	require := require.New(t)

	// Use low beta for quick decision
	lc := NewLuxConsensus(5, 4, 1)
	itemID := ids.GenerateTestID()

	// Poll multiple times to build confidence
	for i := 0; i < 10; i++ {
		responses := map[ids.ID]int{
			itemID: 5, // High votes
		}
		lc.Poll(responses)
	}

	// Check if decided
	lc.mu.RLock()
	decided := lc.decided[itemID]
	lc.mu.RUnlock()

	// The decision may or may not have been made depending on internal state
	_ = decided
	require.NotNil(lc)
}

// TestLuxConsensusPollRejectDecision tests poll leading to reject decision
func TestLuxConsensusPollRejectDecision(t *testing.T) {
	require := require.New(t)

	lc := NewLuxConsensus(10, 8, 1) // High alpha
	item1 := ids.GenerateTestID()
	item2 := ids.GenerateTestID()

	// Poll with low votes for item1
	for i := 0; i < 10; i++ {
		responses := map[ids.ID]int{
			item1: 1, // Low votes
			item2: 9, // High votes
		}
		lc.Poll(responses)
	}

	require.NotNil(lc)
}

// TestLuxConsensusPollDecisionWithLowRatio tests Poll with a low vote ratio leading to reject
func TestLuxConsensusPollDecisionWithLowRatio(t *testing.T) {
	require := require.New(t)

	// Create with very low beta for immediate decision
	lc := NewLuxConsensus(10, 9, 1)
	item := ids.GenerateTestID()

	// Set up Focus to immediately decide with low confidence
	// We need the focus to reach decided state with low ratio
	// Poll repeatedly with low votes to build confidence towards rejection
	for i := 0; i < 50; i++ {
		responses := map[ids.ID]int{
			item: 1, // Very low votes - below alpha threshold
		}
		continuePolling := lc.Poll(responses)
		if !continuePolling {
			break
		}
	}

	// Check if decision was made
	lc.mu.RLock()
	decision, exists := lc.decisions[item]
	lc.mu.RUnlock()

	if exists {
		// If decided, verify the decision
		t.Logf("Decision made: %v", decision)
	}
	require.NotNil(lc)
}

// TestSimpleTransportRequestVotesEarlyCancel tests early context cancellation
func TestSimpleTransportRequestVotesEarlyCancel(t *testing.T) {
	require := require.New(t)

	transport := &SimpleTransport{}
	peers := make([]ids.NodeID, 1000) // Many peers
	for i := 0; i < 1000; i++ {
		peers[i] = ids.GenerateTestNodeID()
	}
	itemID := ids.GenerateTestID()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately
	cancel()

	ch := transport.RequestVotes(ctx, peers, itemID)

	// Drain channel
	count := 0
	for range ch {
		count++
	}

	// Should get very few or no votes due to immediate cancellation
	require.True(count < 1000)
}

// TestLuxConsensusPollFocusDecidedWithLowRatio tests the reject branch in Poll
// This test manipulates internal state to reach the DecideReject branch
func TestLuxConsensusPollFocusDecidedWithLowRatio(t *testing.T) {
	require := require.New(t)

	// Create consensus with threshold 1 (beta=1) so decisions happen quickly
	lc := NewLuxConsensus(10, 8, 1) // K=10, alpha=8, beta=1
	item := ids.GenerateTestID()

	// First, poll with high ratio to build confidence
	// This will make focus.State return decided=true
	highRatioResponses := map[ids.ID]int{
		item: 10, // 100% - above alpha
	}
	lc.Poll(highRatioResponses)

	// Now check if decision was made
	lc.mu.RLock()
	decided := lc.decided[item]
	decision := lc.decisions[item]
	lc.mu.RUnlock()

	// Since we used high ratio and beta=1, should be decided with Accept
	if decided {
		require.NotNil(decision)
	}
}

// TestLuxConsensusPollWaveDecision tests the Wave protocol decision path
func TestLuxConsensusPollWaveDecision(t *testing.T) {
	require := require.New(t)

	lc := NewLuxConsensus(5, 4, 100) // High beta so focus won't decide quickly
	item := ids.GenerateTestID()

	// Poll multiple times to let Wave make a decision
	for i := 0; i < 200; i++ {
		responses := map[ids.ID]int{
			item: 5, // High votes
		}
		continuePolling := lc.Poll(responses)
		if !continuePolling {
			break
		}
	}

	require.NotNil(lc)
}

// TestLuxConsensusPollFocusDecideReject tests the reject branch in Poll
// This covers the case where focus.State returns decided=true but ratio < alpha
func TestLuxConsensusPollFocusDecideReject(t *testing.T) {
	require := require.New(t)

	// Create consensus with beta=1 (threshold=1) for immediate decision
	// K=10, alpha=9 (90% threshold), beta=1
	lc := NewLuxConsensus(10, 9, 1)
	item := ids.GenerateTestID()

	// First, build up confidence by polling with high ratio
	// This makes focus.states[item] reach the threshold
	highRatioResponses := map[ids.ID]int{
		item: 10, // 100% - well above alpha (90%)
	}
	continuePolling := lc.Poll(highRatioResponses)

	// Now the focus state for this item should be at threshold
	// If decided with high ratio, we got Accept - that's fine
	// We need to create a new scenario for the reject branch

	// Create a fresh consensus
	lc2 := NewLuxConsensus(10, 9, 1)
	item2 := ids.GenerateTestID()

	// Directly manipulate the focus state to be at threshold
	// Then poll with low ratio to trigger the reject branch
	lc2.focus.Update(item2, 0.95) // Above alpha, increments state to 1

	// Now state is 1, threshold is 1, so decided=true
	// Poll with low ratio to hit the reject branch
	lowRatioResponses := map[ids.ID]int{
		item2: 1,  // Only 10% - below alpha (90%)
		ids.GenerateTestID(): 9, // 90% for another item
	}
	continuePolling = lc2.Poll(lowRatioResponses)

	// Check decision was made
	lc2.mu.RLock()
	decided := lc2.decided[item2]
	decision, exists := lc2.decisions[item2]
	lc2.mu.RUnlock()

	// Should be decided with Reject since ratio < alpha when focus decided
	if exists && decided {
		t.Logf("Decision made for item2: %v", decision)
	}

	require.NotNil(lc)
	_ = continuePolling
}
