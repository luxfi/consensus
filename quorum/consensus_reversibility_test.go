// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/luxfi/consensus/photon"
)

// TestPhotonGovernance tests that the photon consensus can handle
// governance scenarios where initial majority can be reversed
func TestPhotonGovernance(t *testing.T) {
	require := require.New(t)
	
	numColors := 2
	numNodes := 100
	numByzantine := 10
	params := photon.Parameters{
		K:               20,
		AlphaPreference: 15,
		AlphaConfidence: 17,
		Beta:            10,
	}
	seed := int64(0)
	
	network := NewNetwork(params, numColors, seed)
	
	// Add honest nodes - initially 60% prefer color 0
	for i := 0; i < 60; i++ {
		node := NewTree(params, network.colors[0])
		network.AddNodeSpecificColor(node, 0, []int{1})
	}
	
	// Add 30 honest nodes preferring color 1
	for i := 0; i < 30; i++ {
		node := NewTree(params, network.colors[1])
		network.AddNodeSpecificColor(node, 1, []int{0})
	}
	
	// Add byzantine nodes preferring color 1
	for i := 0; i < numByzantine; i++ {
		byzantine := NewByzantine()
		network.AddNodeSpecificColor(byzantine, 1, []int{0})
	}
	
	// Run consensus rounds
	rounds := 0
	maxRounds := 100
	
	for !network.Finalized() && rounds < maxRounds {
		network.Round()
		rounds++
	}
	
	// Despite initial majority for color 0, with byzantine influence
	// the network should still converge
	require.True(network.Finalized(), "Network should finalize")
	require.False(network.Disagreement(), "No disagreement should exist")
	
	t.Logf("Consensus reached in %d rounds", rounds)
}

// TestPhotonNetworkSplit tests consensus behavior during network splits
func TestPhotonNetworkSplit(t *testing.T) {
	require := require.New(t)
	
	numColors := 2
	numNodes := 50
	params := photon.Parameters{
		K:               10,
		AlphaPreference: 7,
		AlphaConfidence: 8,
		Beta:            5,
	}
	seed := int64(42)
	
	network := NewNetwork(params, numColors, seed)
	
	// Create two groups that initially can't communicate
	// Group 1: 25 nodes preferring color 0
	for i := 0; i < numNodes/2; i++ {
		node := NewTree(params, network.colors[0])
		// Only see color 0 initially
		network.AddNodeSpecificColor(node, 0, []int{})
	}
	
	// Group 2: 25 nodes preferring color 1
	for i := numNodes/2; i < numNodes; i++ {
		node := NewTree(params, network.colors[1])
		// Only see color 1 initially
		network.AddNodeSpecificColor(node, 1, []int{})
	}
	
	// Simulate network partition healing after some rounds
	// by adding the missing color to all nodes
	partitionRounds := 10
	for i := 0; i < partitionRounds; i++ {
		network.Round()
	}
	
	// Now heal the partition - add missing colors
	for _, node := range network.nodes {
		for _, color := range network.colors {
			node.Add(color)
		}
	}
	
	// Continue consensus after partition heals
	rounds := partitionRounds
	maxRounds := 200
	
	for !network.Finalized() && rounds < maxRounds {
		network.Round()
		rounds++
	}
	
	// Eventually one color should win
	require.True(network.Finalized(), "Network should finalize after partition heals")
	require.False(network.Disagreement(), "All nodes should agree on same value")
	
	t.Logf("Consensus reached in %d rounds (partition healed at round %d)", rounds, partitionRounds)
}

