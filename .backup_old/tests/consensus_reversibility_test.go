// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package tests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/core/utils"
	"github.com/luxfi/consensus/protocol/pulse"
	"github.com/luxfi/consensus/core/fpc"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/consensus/utils/sampler"
	"github.com/luxfi/ids"
)

// TestPulseGovernance tests consensus reversibility in governance scenarios
func TestPulseGovernance(t *testing.T) {
	require := require.New(t)

	// Simulate a governance vote
	params := config.Parameters{
		K:               20,
		AlphaPreference: 11, // Simple majority
		AlphaConfidence: 15, // 3/4 super majority
		Beta:            3,  // 3 consecutive rounds
	}

	// Create governance consensus
	gov := pulse.NewPulse(params)

	// Two proposals
	proposalA := ids.GenerateTestID()
	proposalB := ids.GenerateTestID()

	require.NoError(gov.Add(proposalA))
	require.NoError(gov.Add(proposalB))

	// Initially no votes - preference is first added
	require.Equal(proposalA, gov.Preference())

	// Strong support for proposal B
	votesB := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votesB.Add(proposalB)
	}

	// Vote multiple times to overcome any preference strength
	for i := 0; i < 5; i++ {
		require.NoError(gov.RecordVotes(votesB))
	}
	require.Equal(proposalB, gov.Preference())

	// Continue to finalize
	for i := 0; i < int(params.Beta); i++ {
		require.NoError(gov.RecordVotes(votesB))
	}

	require.True(gov.Finalized())
	require.Equal(proposalB, gov.Preference())
}

// TestConsensusReversibilityWindow tests the window for preference changes
func TestConsensusReversibilityWindow(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters

	w := wave.NewWave(params)

	// Add three choices
	choiceA := ids.GenerateTestID()
	choiceB := ids.GenerateTestID()
	choiceC := ids.GenerateTestID()

	require.NoError(w.Add(choiceA))
	require.NoError(w.Add(choiceB))
	require.NoError(w.Add(choiceC))

	// Track preference changes
	preferences := []ids.ID{}

	// Round 1: Vote for A
	votesA := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		votesA.Add(choiceA)
	}
	require.NoError(w.RecordVotes(votesA))
	preferences = append(preferences, w.Preference())

	// Round 2: Vote for B
	votesB := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		votesB.Add(choiceB)
	}
	require.NoError(w.RecordVotes(votesB))
	preferences = append(preferences, w.Preference())

	// Round 3: Vote for C
	votesC := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		votesC.Add(choiceC)
	}
	require.NoError(w.RecordVotes(votesC))
	preferences = append(preferences, w.Preference())

	// Verify preferences changed
	require.Equal(choiceA, preferences[0])
	require.Equal(choiceB, preferences[1])
	require.Equal(choiceC, preferences[2])

	// Now try to finalize on C
	strongVotesC := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		strongVotesC.Add(choiceC)
	}

	for i := 0; i < int(params.Beta); i++ {
		require.NoError(w.RecordVotes(strongVotesC))
	}

	require.True(w.Finalized())
	require.Equal(choiceC, w.Preference())
}

// TestPartialVoteReversibility tests reversibility with partial votes
func TestPartialVoteReversibility(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               20,
		AlphaPreference: 11,
		AlphaConfidence: 15,
		Beta:            5,
	}

	p := pulse.NewPulse(params)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Blue))

	// Start with slight preference for Red
	round1 := bag.Bag[ids.ID]{}
	for i := 0; i < 11; i++ {
		round1.Add(Red)
	}
	for i := 0; i < 9; i++ {
		round1.Add(Blue)
	}

	require.NoError(p.RecordVotes(round1))
	require.Equal(Red, p.Preference())

	// Blue gains slight majority
	round2 := bag.Bag[ids.ID]{}
	for i := 0; i < 9; i++ {
		round2.Add(Red)
	}
	for i := 0; i < 11; i++ {
		round2.Add(Blue)
	}

	require.NoError(p.RecordVotes(round2))
	require.Equal(Blue, p.Preference())

	// Blue strengthens to confidence threshold
	round3 := bag.Bag[ids.ID]{}
	for i := 0; i < 5; i++ {
		round3.Add(Red)
	}
	for i := 0; i < 15; i++ {
		round3.Add(Blue)
	}

	// Finalize on Blue
	for i := 0; i < int(params.Beta); i++ {
		require.NoError(p.RecordVotes(round3))
	}

	require.True(p.Finalized())
	require.Equal(Blue, p.Preference())
}

// TestNetworkPartitionReversibility tests consensus with network partitions
func TestNetworkPartitionReversibility(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters

	// Create context
	ctx := &interfaces.Runtime{
		NetworkID: 1,
		ChainID:   ids.GenerateTestID(),
		NodeID:    ids.GenerateTestNodeID(),
	}
	factory := utils.NewFactory(ctx)

	// Create two partitions
	partition1 := NewTestNetwork(factory, params, 2, sampler.NewSource(42))
	partition2 := NewTestNetwork(factory, params, 2, sampler.NewSource(43))

	// Add nodes to each partition
	for i := 0; i < 10; i++ {
		partition1.AddNode(func(f *utils.Factory, p config.Parameters, choice ids.ID) interfaces.Consensus {
			node := pulse.NewPulse(p)
			_ = node.Add(choice)
			return node
		})

		partition2.AddNode(func(f *utils.Factory, p config.Parameters, choice ids.ID) interfaces.Consensus {
			node := pulse.NewPulse(p)
			_ = node.Add(choice)
			return node
		})
	}

	// Run each partition independently
	rounds := 0
	for rounds < 50 {
		if !partition1.Finalized() {
			partition1.Round()
		}
		if !partition2.Finalized() {
			partition2.Round()
		}

		if partition1.Finalized() && partition2.Finalized() {
			break
		}
		rounds++
	}

	// Both partitions should finalize
	require.True(partition1.Finalized())
	require.True(partition2.Finalized())

	// They may have different preferences
	pref1 := partition1.nodes[0].Preference()
	pref2 := partition2.nodes[0].Preference()

	t.Logf("Partition 1 preference: %s", pref1)
	t.Logf("Partition 2 preference: %s", pref2)

	// Each partition should have internal agreement
	require.True(partition1.Agreement())
	require.True(partition2.Agreement())
}

// TestConsensusStability tests consensus stability under changing conditions
func TestConsensusStability(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters

	// Create multiple instances
	instances := make([]*wave.Wave, 20)
	for i := range instances {
		instances[i] = wave.NewWave(params)
		_ = instances[i].Add(Red)
		_ = instances[i].Add(Blue)
		_ = instances[i].Add(Green)
	}

	// Simulate changing vote patterns
	patterns := []struct {
		name  string
		votes func() bag.Bag[ids.ID]
	}{
		{
			name: "Strong Red",
			votes: func() bag.Bag[ids.ID] {
				v := bag.Bag[ids.ID]{}
				for i := 0; i < params.AlphaConfidence; i++ {
					v.Add(Red)
				}
				return v
			},
		},
		{
			name: "Weak Blue",
			votes: func() bag.Bag[ids.ID] {
				v := bag.Bag[ids.ID]{}
				for i := 0; i < params.AlphaPreference; i++ {
					v.Add(Blue)
				}
				return v
			},
		},
		{
			name: "Strong Green",
			votes: func() bag.Bag[ids.ID] {
				v := bag.Bag[ids.ID]{}
				for i := 0; i < params.AlphaConfidence; i++ {
					v.Add(Green)
				}
				return v
			},
		},
	}

	// Apply vote patterns
	for _, pattern := range patterns {
		votes := pattern.votes()
		for _, instance := range instances {
			if !instance.Finalized() {
				_ = instance.RecordVotes(votes)
			}
		}
	}

	// Continue with strong Green until all finalize
	greenVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		greenVotes.Add(Green)
	}

	for round := 0; round < int(params.Beta)*2; round++ {
		allFinalized := true
		for _, instance := range instances {
			if !instance.Finalized() {
				_ = instance.RecordVotes(greenVotes)
				allFinalized = false
			}
		}
		if allFinalized {
			break
		}
	}

	// All should finalize on Green
	for _, instance := range instances {
		require.True(instance.Finalized())
		require.Equal(Green, instance.Preference())
	}
}
