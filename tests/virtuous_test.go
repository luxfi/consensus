// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package tests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/core/utils"
	"github.com/luxfi/consensus/protocol/photon"
	"github.com/luxfi/consensus/protocol/pulse"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/consensus/utils/sampler"
	"github.com/luxfi/ids"
)

// TestVirtuousBehavior tests virtuous consensus behavior
func TestVirtuousBehavior(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	// Test virtuous behavior in each protocol
	t.Run("Photon", func(t *testing.T) {
		p := photon.NewPhoton(params)
		require.NoError(p.Add(Red))
		
		// Virtuous: consistent voting
		votes := bag.Bag[ids.ID]{}
		for i := 0; i < params.AlphaConfidence; i++ {
			votes.Add(Red)
		}
		
		rounds := 0
		for !p.Finalized() && rounds < int(params.Beta)*2 {
			require.NoError(p.RecordVotes(votes))
			rounds++
		}
		
		require.True(p.Finalized())
		require.Equal(int(params.Beta), rounds) // Should finalize in exactly Beta rounds
	})
	
	t.Run("Pulse", func(t *testing.T) {
		p := pulse.NewPulse(params)
		require.NoError(p.Add(Red))
		require.NoError(p.Add(Blue))
		
		// Virtuous: all nodes vote Blue
		votes := bag.Bag[ids.ID]{}
		for i := 0; i < params.AlphaConfidence; i++ {
			votes.Add(Blue)
		}
		
		rounds := 0
		for !p.Finalized() && rounds < int(params.Beta)*2 {
			require.NoError(p.RecordVotes(votes))
			rounds++
		}
		
		require.True(p.Finalized())
		require.Equal(Blue, p.Preference())
		require.Equal(int(params.Beta), rounds)
	})
	
	t.Run("Wave", func(t *testing.T) {
		w := wave.NewWave(params)
		choices := make([]ids.ID, 5)
		for i := 0; i < 5; i++ {
			choices[i] = ids.GenerateTestID()
			require.NoError(w.Add(choices[i]))
		}
		
		// Virtuous: consensus on second choice
		target := choices[1]
		votes := bag.Bag[ids.ID]{}
		for i := 0; i < params.AlphaConfidence; i++ {
			votes.Add(target)
		}
		
		rounds := 0
		for !w.Finalized() && rounds < int(params.Beta)*2 {
			require.NoError(w.RecordVotes(votes))
			rounds++
		}
		
		require.True(w.Finalized())
		require.Equal(target, w.Preference())
		require.Equal(int(params.Beta), rounds)
	})
}

// TestVirtuousNetwork tests virtuous behavior in network
func TestVirtuousNetwork(t *testing.T) {
	require := require.New(t)

	// Use parameters that allow convergence with multiple choices
	params := config.Parameters{
		K:               5,  // Enough nodes to avoid ties
		AlphaPreference: 3,  // Majority needed
		AlphaConfidence: 4,  // Strong majority for confidence  
		Beta:            3,  // Reasonable finalization threshold
	}
	
	// Create context
	ctx := &interfaces.Runtime{
		NetworkID: 1,
		ChainID:   ids.GenerateTestID(),
		NodeID:    ids.GenerateTestNodeID(),
	}
	factory := utils.NewFactory(ctx)
	
	// Create network with 2 choices to improve convergence chances
	network := NewTestNetwork(factory, params, 2, sampler.NewSource(42))
	
	// Add virtuous nodes - need to add all choices for pulse to work properly
	for i := 0; i < params.K; i++ {
		network.AddNode(func(f *utils.Factory, p config.Parameters, choice ids.ID) interfaces.Consensus {
			node := pulse.NewPulse(p)
			// First add the initial choice
			_ = node.Add(choice)
			// Then add the other choices so nodes can switch preferences
			for _, c := range network.colors {
				if c != choice {
					_ = node.Add(c)
				}
			}
			return node
		})
	}
	
	// Run virtuous consensus
	rounds := 0
	for !network.Finalized() && rounds < 100 {
		network.Round()
		rounds++
		// Log progress periodically
		if rounds % 10 == 0 || rounds <= 5 {
			// Count preferences
			prefCounts := make(map[ids.ID]int)
			for _, node := range network.nodes {
				prefCounts[node.Preference()]++
			}
			t.Logf("Round %d: %d nodes still running, preferences: %v", rounds, len(network.running), prefCounts)
		}
	}
	
	t.Logf("Network state after %d rounds: Finalized=%v, Agreement=%v", 
		rounds, network.Finalized(), network.Agreement())
	
	require.True(network.Finalized())
	require.True(network.Agreement())
	// With random initial preferences and 3 choices, convergence may take longer
	require.LessOrEqual(rounds, 100) // Allow up to 100 rounds for convergence
	
	t.Logf("Virtuous network finalized in %d rounds", rounds)
}

// TestVirtuousMinorityHonest tests virtuous behavior with minority honest nodes
func TestVirtuousMinorityHonest(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	
	// Create wave instances
	numNodes := params.K
	honestNodes := params.AlphaConfidence // Just enough honest
	byzantineNodes := numNodes - honestNodes
	
	nodes := make([]*wave.Wave, numNodes)
	for i := range nodes {
		nodes[i] = wave.NewWave(params)
		require.NoError(nodes[i].Add(Red))
		require.NoError(nodes[i].Add(Blue))
		require.NoError(nodes[i].Add(Green))
	}
	
	// Virtuous target
	virtuousChoice := Blue
	
	// Simulate rounds
	rounds := 0
	maxRounds := int(params.Beta) * 10
	
	for rounds < int(maxRounds) {
		votes := bag.Bag[ids.ID]{}
		
		// Honest nodes vote virtuously
		for i := 0; i < honestNodes; i++ {
			votes.Add(virtuousChoice)
		}
		
		// Byzantine nodes vote randomly
		for i := 0; i < byzantineNodes; i++ {
			choices := []ids.ID{Red, Blue, Green}
			votes.Add(choices[i%3])
		}
		
		// Apply votes to all nodes
		allFinalized := true
		for _, node := range nodes {
			if !node.Finalized() {
				require.NoError(node.RecordVotes(votes))
				allFinalized = false
			}
		}
		
		if allFinalized {
			break
		}
		rounds++
	}
	
	// Honest nodes should converge on virtuous choice
	honestFinalized := 0
	honestAgreed := 0
	for i := 0; i < honestNodes; i++ {
		if nodes[i].Finalized() {
			honestFinalized++
			if nodes[i].Preference() == virtuousChoice {
				honestAgreed++
			}
		}
	}
	
	t.Logf("Honest nodes finalized: %d/%d, agreed on virtuous: %d", 
		honestFinalized, honestNodes, honestAgreed)
	
	// Most honest nodes should finalize on virtuous choice
	require.Greater(honestAgreed, honestNodes/2)
}

// TestVirtuousConvergenceSpeed tests speed of virtuous convergence
func TestVirtuousConvergenceSpeed(t *testing.T) {
	params := config.TestParameters
	
	// Test convergence speed for different network sizes
	sizes := []int{5, 10, 20, 50}
	
	for _, size := range sizes {
		t.Run(fmt.Sprintf("Size%d", size), func(t *testing.T) {
			nodes := make([]*pulse.Pulse, size)
			for i := range nodes {
				nodes[i] = pulse.NewPulse(params)
				_ = nodes[i].Add(Red)
				_ = nodes[i].Add(Blue)
			}
			
			// All vote Blue (virtuous)
			votes := bag.Bag[ids.ID]{}
			for i := 0; i < params.AlphaConfidence; i++ {
				votes.Add(Blue)
			}
			
			rounds := 0
			allFinalized := false
			for rounds < int(params.Beta)*2 && !allFinalized {
				rounds++
				allFinalized = true
				for _, node := range nodes {
					if !node.Finalized() {
						_ = node.RecordVotes(votes)
						allFinalized = false
					}
				}
			}
			
			// Virtuous should finalize in Beta+1 rounds when starting with wrong preference
			// (1 round to switch preference, then Beta rounds to build confidence)
			require.Equal(t, int(params.Beta)+1, rounds)
			
			// All should agree
			for _, node := range nodes {
				require.True(t, node.Finalized())
				require.Equal(t, Blue, node.Preference())
			}
		})
	}
}

// TestVirtuousRecovery tests recovery to virtuous behavior
func TestVirtuousRecovery(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	w := wave.NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))
	require.NoError(w.Add(Green))
	
	// Start with non-virtuous (flipping) behavior
	for i := 0; i < 5; i++ {
		votes := bag.Bag[ids.ID]{}
		choice := []ids.ID{Red, Blue, Green}[i%3]
		for j := 0; j < params.AlphaPreference; j++ {
			votes.Add(choice)
		}
		require.NoError(w.RecordVotes(votes))
	}
	
	// Should not be finalized due to flipping
	require.False(w.Finalized())
	
	// Now become virtuous - consistent Blue votes
	virtuousVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		virtuousVotes.Add(Blue)
	}
	
	// Should recover and finalize
	rounds := 0
	for !w.Finalized() && rounds < int(params.Beta)*2 {
		require.NoError(w.RecordVotes(virtuousVotes))
		rounds++
	}
	
	require.True(w.Finalized())
	require.Equal(Blue, w.Preference())
	// After flipping, confidence may already be partially built
	// so it might take less than Beta rounds to finalize
	require.LessOrEqual(rounds, int(params.Beta))
}

// TestVirtuousStability tests stability under virtuous conditions
func TestVirtuousStability(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	
	// Create multiple protocol instances
	protocols := []struct {
		name     string
		instance interface {
			Add(ids.ID) error
			RecordVotes(bag.Bag[ids.ID]) error
			Finalized() bool
			Preference() ids.ID
		}
	}{
		{"Photon", photon.NewPhoton(params)},
		{"Pulse", pulse.NewPulse(params)},
		{"Wave", wave.NewWave(params)},
	}
	
	// Add same choices to all
	for _, p := range protocols {
		require.NoError(p.instance.Add(Red))
		if p.name != "Photon" { // Photon only supports one choice
			require.NoError(p.instance.Add(Blue))
		}
	}
	
	// Virtuous votes
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votes.Add(Red)
	}
	
	// All should finalize in Beta rounds
	for _, p := range protocols {
		t.Run(p.name, func(t *testing.T) {
			rounds := 0
			for !p.instance.Finalized() && rounds < int(params.Beta)*2 {
				require.NoError(p.instance.RecordVotes(votes))
				rounds++
			}
			
			require.True(p.instance.Finalized())
			require.Equal(Red, p.instance.Preference())
			require.Equal(int(params.Beta), rounds)
		})
	}
}

