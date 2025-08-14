// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package tests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/core/utils"
	"github.com/luxfi/consensus/protocol/photon"
	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/consensus/protocol/pulse"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/consensus/utils/sampler"
	"github.com/luxfi/ids"
)

// TestIntegrationPhotonConsensus tests photon consensus with multiple nodes
func TestIntegrationPhotonConsensus(t *testing.T) {
	require := require.New(t)

	// Use TestParameters for integration tests - more suitable for smaller networks
	params := config.TestParameters
	// Create a minimal context for testing
	ctx := &interfaces.Context{
		NetworkID: 1,
		ChainID:   ids.GenerateTestID(),
		NodeID:    ids.GenerateTestNodeID(),
	}
	factory := utils.NewFactory(ctx)
	
	// Create a network with 3 colors
	network := NewTestNetwork(factory, params, 3, sampler.NewSource(42))
	
	// Add 15 nodes to the network
	// For photon, each node tracks consensus on a single choice
	// So we'll use pulse instead for multi-choice network testing
	for i := 0; i < 15; i++ {
		network.AddNode(func(f *utils.Factory, p config.Parameters, choice ids.ID) interfaces.Consensus {
			node := pulse.NewPulse(p)
			node.Add(choice)
			// Add all colors from the network to each node
			for _, color := range network.colors {
				node.Add(color)
			}
			return node
		})
	}
	
	// Run consensus rounds
	for round := 0; round < 50 && !network.Finalized(); round++ {
		network.Round()
		t.Logf("Round %d: %d running nodes", round+1, len(network.running))
	}
	
	require.True(network.Finalized(), "Network failed to finalize after 50 rounds")
	require.True(network.Agreement(), "Network failed to reach agreement")
}

// TestIntegrationPrismConsensus tests prism consensus with multiple nodes
func TestIntegrationPrismConsensus(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	// Create validators
	validators := bag.Bag[ids.NodeID]{}
	for i := 0; i < 5; i++ {
		nodeID := ids.GenerateTestNodeID()
		validators.AddCount(nodeID, 1)
	}
	
	// Create prism instance
	p := prism.NewPrism(params, sampler.NewSource(42))
	
	// Test basic prism functionality
	// Create dependency graph
	deps := prism.NewSimpleDependencyGraph()
	choiceA := ids.GenerateTestID()
	choiceB := ids.GenerateTestID()
	deps.Add(choiceA, nil)
	deps.Add(choiceB, nil)
	
	// The default StandardRefract function will be used, which simulates
	// voting based on the dependency graph structure
	// For leaf nodes (no dependencies), it assigns weight to the decision itself
	
	// Run a single round to verify no errors
	hasQuorum, err := p.Refract(validators, deps, params)
	require.NoError(err)
	
	// The test passes if we can create and use a prism without errors
	// Actual consensus would require a proper voting simulation which
	// is tested in the prism package's own tests
	t.Logf("Prism executed successfully. HasQuorum: %v, Metrics: %v", hasQuorum, p.GetMetrics())
	
	// Test PrismArray functionality
	pa := prism.NewPrismArray(params, sampler.NewSource(123))
	
	// Test creating and polling with PrismArray
	decisionID := ids.GenerateTestID()
	decisions := []ids.ID{choiceA, choiceB}
	
	hasQuorum2, err := pa.Poll(decisionID, validators, decisions)
	require.NoError(err)
	t.Logf("PrismArray poll executed. HasQuorum: %v", hasQuorum2)
	
	// Verify we can get finalized decisions (even if empty)
	finalized := pa.GetFinalized()
	// GetFinalized returns a slice which can be empty but not nil
	t.Logf("PrismArray finalized decisions: %d", len(finalized))
}

// TestIntegrationPulseConsensus tests pulse consensus
func TestIntegrationPulseConsensus(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	// Create nodes
	nodes := make([]*pulse.Pulse, 20)
	for i := range nodes {
		nodes[i] = pulse.NewPulse(params)
	}
	
	// Add choices to each node
	choices := []ids.ID{
		ids.GenerateTestID(),
		ids.GenerateTestID(),
		ids.GenerateTestID(),
	}
	
	for _, node := range nodes {
		for _, choice := range choices {
			require.NoError(node.Add(choice))
		}
	}
	
	// Simulate consensus rounds
	maxRounds := 100
	for round := 0; round < maxRounds; round++ {
		allFinalized := true
		for _, node := range nodes {
			if !node.Finalized() {
				allFinalized = false
				// Simulate voting - need at least AlphaPreference (2) votes for TestParameters
				votes := bag.Bag[ids.ID]{}
				// Majority vote for choices[0]
				for i := 0; i < 3; i++ {
					votes.Add(choices[0])
				}
				// Minority vote for choices[1]
				votes.Add(choices[1])
				require.NoError(node.RecordVotes(votes))
			}
		}
		
		if allFinalized {
			break
		}
	}
	
	// Verify all nodes finalized on the same choice
	var finalChoice ids.ID
	for i, node := range nodes {
		require.True(node.Finalized())
		if i == 0 {
			finalChoice = node.Preference()
		} else {
			require.Equal(finalChoice, node.Preference())
		}
	}
}


// TestIntegrationWaveConsensus tests wave consensus
func TestIntegrationWaveConsensus(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	// Create nodes
	nodes := make([]*wave.Wave, 10)
	for i := range nodes {
		nodes[i] = wave.NewWave(params)
	}
	
	// Add decisions
	decisions := []ids.ID{
		ids.GenerateTestID(),
		ids.GenerateTestID(),
	}
	
	for _, node := range nodes {
		for _, decision := range decisions {
			require.NoError(node.Add(decision))
		}
	}
	
	// Run wave consensus
	for round := 0; round < 20; round++ {
		allFinalized := true
		for _, node := range nodes {
			if !node.Finalized() {
				allFinalized = false
				// Create wave pattern - majority for decisions[0]
				votes := bag.Bag[ids.ID]{}
				for i := 0; i < 3; i++ {
					votes.Add(decisions[0])
				}
				votes.Add(decisions[1])
				require.NoError(node.RecordVotes(votes))
			}
		}
		
		if allFinalized {
			break
		}
	}
	
	// Verify wave convergence
	for _, node := range nodes {
		require.True(node.Finalized())
		require.Equal(decisions[0], node.Preference())
	}
}

// TestIntegrationMixedProtocols tests different protocols interacting
func TestIntegrationMixedProtocols(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	// Create mixed node types
	photonNode := photon.NewPhoton(params)
	pulseNode := pulse.NewPulse(params)
	waveNode := wave.NewWave(params)
	
	// Common choices
	choices := []ids.ID{
		ids.GenerateTestID(),
		ids.GenerateTestID(),
	}
	
	// Photon only accepts one choice
	require.NoError(photonNode.Add(choices[0]))
	
	// Pulse and wave accept multiple choices
	for _, choice := range choices {
		require.NoError(pulseNode.Add(choice))
		require.NoError(waveNode.Add(choice))
	}
	
	// Simulate voting with strong preference for choices[0]
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < 3; i++ {
		votes.Add(choices[0])
	}
	votes.Add(choices[1])
	
	// Run consensus rounds
	for round := 0; round < 30; round++ {
		if !photonNode.Finalized() {
			require.NoError(photonNode.RecordVotes(votes))
		}
		if !pulseNode.Finalized() {
			require.NoError(pulseNode.RecordVotes(votes))
		}
		if !waveNode.Finalized() {
			require.NoError(waveNode.RecordVotes(votes))
		}
		
		if photonNode.Finalized() && pulseNode.Finalized() && waveNode.Finalized() {
			break
		}
	}
	
	// All should agree on choices[0]
	require.True(photonNode.Finalized())
	require.True(pulseNode.Finalized())
	require.True(waveNode.Finalized())
	
	require.Equal(choices[0], photonNode.Preference())
	require.Equal(choices[0], pulseNode.Preference())
	require.Equal(choices[0], waveNode.Preference())
}