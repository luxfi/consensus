// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus_test

import (
	"fmt"
	"testing"
	"time"

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

// BenchmarkConsensus benchmarks consensus protocols
func BenchmarkConsensus(b *testing.B) {
	params := config.DefaultParameters
	
	b.Run("Photon", func(b *testing.B) {
		benchmarkProtocol(b, func() interfaces.Consensus {
			return photon.NewPhoton(params)
		}, params)
	})
	
	b.Run("Pulse", func(b *testing.B) {
		benchmarkProtocol(b, func() interfaces.Consensus {
			return pulse.NewPulse(params)
		}, params)
	})
	
	b.Run("Wave", func(b *testing.B) {
		benchmarkProtocol(b, func() interfaces.Consensus {
			return wave.NewWave(params)
		}, params)
	})
}

func benchmarkProtocol(b *testing.B, factory func() interfaces.Consensus, params config.Parameters) {
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		consensus := factory()
		
		// Add choices
		choices := []ids.ID{
			ids.GenerateTestID(),
			ids.GenerateTestID(),
			ids.GenerateTestID(),
		}
		
		for _, choice := range choices {
			consensus.Add(choice)
		}
		
		// Vote until finalized
		votes := bag.Bag[ids.ID]{}
		for j := 0; j < params.AlphaConfidence; j++ {
			votes.Add(choices[0])
		}
		
		for !consensus.Finalized() {
			consensus.RecordVotes(votes)
		}
	}
}

// TestDualAlphaOptimization tests the dual alpha parameter optimization
func TestDualAlphaOptimization(t *testing.T) {
	require := require.New(t)

	// Test with different alpha configurations
	testCases := []struct {
		name            string
		alphaPreference int
		alphaConfidence int
		expectedRounds  int
	}{
		{
			name:            "Equal alphas",
			alphaPreference: 15,
			alphaConfidence: 15,
			expectedRounds:  20,
		},
		{
			name:            "Lower preference alpha",
			alphaPreference: 10,
			alphaConfidence: 15,
			expectedRounds:  20,
		},
		{
			name:            "Higher preference alpha", 
			alphaPreference: 18,
			alphaConfidence: 15,
			expectedRounds:  20,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params := config.Parameters{
				K:               21,
				AlphaPreference: tc.alphaPreference,
				AlphaConfidence: tc.alphaConfidence,
				Beta:            20,
			}
			
			// Create network
			ctx := &interfaces.Context{
				NetworkID: 1,
				ChainID:   ids.GenerateTestID(),
				NodeID:    ids.GenerateTestNodeID(),
			}
			factory := utils.NewFactory(ctx)
			network := NewTestNetwork(factory, params, 3, sampler.NewSource(42))
			
			// Add nodes
			for i := 0; i < 21; i++ {
				network.AddNode(func(f *utils.Factory, p config.Parameters, choice ids.ID) interfaces.Consensus {
					node := pulse.NewPulse(p)
					node.Add(choice)
					return node
				})
			}
			
			// Run consensus
			rounds := 0
			for !network.Finalized() && rounds < 100 {
				network.Round()
				rounds++
			}
			
			require.True(network.Finalized())
			require.True(network.Agreement())
			require.LessOrEqual(rounds, tc.expectedRounds)
		})
	}
}

// TestTreeConvergenceOptimization tests convergence optimization in tree structure
func TestTreeConvergenceOptimization(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	
	// Measure convergence time for different network sizes
	sizes := []int{10, 20, 50, 100}
	
	for _, size := range sizes {
		t.Run(fmt.Sprintf("Size%d", size), func(t *testing.T) {
			start := time.Now()
			
			// Create nodes
			nodes := make([]*wave.Wave, size)
			for i := range nodes {
				nodes[i] = wave.NewWave(params)
				// Add multiple choices
				for j := 0; j < 5; j++ {
					nodes[i].Add(ids.GenerateTestID())
				}
			}
			
			// Simulate consensus
			target := ids.GenerateTestID()
			votes := bag.Bag[ids.ID]{}
			for i := 0; i < params.AlphaConfidence; i++ {
				votes.Add(target)
			}
			
			rounds := 0
			allFinalized := false
			for !allFinalized && rounds < 100 {
				allFinalized = true
				for _, node := range nodes {
					if !node.Finalized() {
						node.RecordVotes(votes)
						allFinalized = false
					}
				}
				rounds++
			}
			
			duration := time.Since(start)
			t.Logf("Size %d: %d rounds, %v duration", size, rounds, duration)
			
			require.True(allFinalized)
			require.Less(rounds, 50) // Should converge reasonably fast
		})
	}
}

// BenchmarkLargeNetwork benchmarks large network consensus
func BenchmarkLargeNetwork(b *testing.B) {
	params := config.DefaultParameters
	
	sizes := []int{100, 500, 1000}
	
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				// Create nodes
				nodes := make([]*pulse.Pulse, size)
				for j := range nodes {
					nodes[j] = pulse.NewPulse(params)
					nodes[j].Add(Red)
					nodes[j].Add(Blue)
				}
				
				// Vote until all finalized
				votes := bag.Bag[ids.ID]{}
				for j := 0; j < params.AlphaConfidence; j++ {
					votes.Add(Blue)
				}
				
				finalized := false
				for !finalized {
					finalized = true
					for _, node := range nodes {
						if !node.Finalized() {
							node.RecordVotes(votes)
							finalized = false
						}
					}
				}
			}
		})
	}
}

// TestParallelConsensus tests parallel consensus execution
func TestParallelConsensus(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	numNodes := 100
	
	// Create nodes
	nodes := make([]*pulse.Pulse, numNodes)
	for i := range nodes {
		nodes[i] = pulse.NewPulse(params)
		nodes[i].Add(Red)
		nodes[i].Add(Blue)
	}
	
	// Run consensus in parallel
	done := make(chan bool, numNodes)
	
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votes.Add(Blue)
	}
	
	start := time.Now()
	
	for i := range nodes {
		go func(node *pulse.Pulse) {
			for !node.Finalized() {
				node.RecordVotes(votes)
			}
			done <- true
		}(nodes[i])
	}
	
	// Wait for all to finish
	for i := 0; i < numNodes; i++ {
		<-done
	}
	
	duration := time.Since(start)
	t.Logf("Parallel consensus for %d nodes: %v", numNodes, duration)
	
	// Verify all agreed
	for _, node := range nodes {
		require.True(node.Finalized())
		require.Equal(Blue, node.Preference())
	}
}