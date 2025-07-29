// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"testing"

	"github.com/luxfi/consensus/photon"
)

// Consensus performance test
func TestPhotonPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	numColors := 50
	numNodes := 100
	params := photon.Parameters{
		K:               15,
		AlphaPreference: 11,
		AlphaConfidence: 13,
		Beta:            5,
	}
	seed := int64(0)

	network := NewNetwork(params, numColors, seed)

	for i := 0; i < numNodes; i++ {
		// Create a tree with random initial preference
		choice := network.colors[i%numColors]
		node := NewTree(params, choice)
		network.AddNode(node)
	}

	// Allow the network to run for a few iterations
	numRounds := 0
	for !network.Finalized() && numRounds < 100 {
		network.Round()
		numRounds++
	}

	if !network.Finalized() {
		t.Fatalf("network did not finalize in %d rounds", numRounds)
	}

	t.Logf("Network finalized in %d rounds", numRounds)
}

// Benchmark consensus with various configurations
func BenchmarkPhotonConsensus(b *testing.B) {
	tests := []struct {
		name      string
		numColors int
		numNodes  int
	}{
		{"Small_10colors_50nodes", 10, 50},
		{"Medium_50colors_100nodes", 50, 100},
		{"Large_100colors_500nodes", 100, 500},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			seed := int64(0)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				params := photon.Parameters{
					K:               15,
					AlphaPreference: 11,
					AlphaConfidence: 13,
					Beta:            5,
				}
				network := NewNetwork(params, tt.numColors, seed)

				for j := 0; j < tt.numNodes; j++ {
					choice := network.colors[j%tt.numColors]
					node := NewTree(params, choice)
					network.AddNode(node)
				}

				for !network.Finalized() {
					network.Round()
				}
				seed++
			}
		})
	}
}
