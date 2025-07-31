// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/spf13/cobra"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

type nodeState struct {
	id         ids.ID
	preference ids.ID
	confidence int
	byzantine  bool
}

func runSimulator(cmd *cobra.Command, args []string) error {
	// Get simulation parameters
	nodes, _ := cmd.Flags().GetInt("nodes")
	rounds, _ := cmd.Flags().GetInt("rounds")
	byzantinePercent, _ := cmd.Flags().GetFloat64("byzantine")
	latencyMs, _ := cmd.Flags().GetInt("latency")
	dropRate, _ := cmd.Flags().GetFloat64("drop-rate")
	
	// Get consensus parameters
	preset, _ := cmd.Flags().GetString("preset")
	params, err := config.GetParametersByName(preset)
	if err != nil {
		return fmt.Errorf("invalid preset: %w", err)
	}
	
	byzantineNodes := int(float64(nodes) * byzantinePercent)
	honestNodes := nodes - byzantineNodes
	
	fmt.Printf("=== Consensus Simulation ===\n")
	fmt.Printf("Total nodes: %d (honest: %d, byzantine: %d)\n", nodes, honestNodes, byzantineNodes)
	fmt.Printf("Rounds: %d\n", rounds)
	fmt.Printf("Network latency: %dms\n", latencyMs)
	fmt.Printf("Packet drop rate: %.1f%%\n", dropRate*100)
	fmt.Printf("Consensus parameters: %s\n", preset)
	fmt.Printf("\n")
	
	// Create nodes
	nodeStates := make([]nodeState, nodes)
	for i := 0; i < nodes; i++ {
		nodeStates[i] = nodeState{
			id:         ids.GenerateTestID(),
			preference: ids.Empty,
			confidence: 0,
			byzantine:  i < byzantineNodes,
		}
	}
	
	// Run simulation
	startTime := time.Now()
	finalizedRound := -1
	
	// Initial preference
	choiceA := ids.GenerateTestID()
	choiceB := ids.GenerateTestID()
	
	for round := 0; round < rounds; round++ {
		// Each node samples k others
		votesA := 0
		votesB := 0
		
		for i, node := range nodeStates {
			if node.byzantine {
				// Byzantine nodes vote randomly
				if rand.Float64() < 0.5 {
					votesA++
				} else {
					votesB++
				}
			} else {
				// Honest nodes follow protocol
				sample := sampleNodes(nodeStates, params.K, i)
				countA, countB := countVotes(sample, choiceA, choiceB)
				
				if countA >= params.AlphaPreference {
					nodeStates[i].preference = choiceA
					votesA++
				} else if countB >= params.AlphaPreference {
					nodeStates[i].preference = choiceB
					votesB++
				}
				
				// Update confidence
				if nodeStates[i].preference != ids.Empty {
					if (nodeStates[i].preference == choiceA && countA >= params.AlphaConfidence) ||
					   (nodeStates[i].preference == choiceB && countB >= params.AlphaConfidence) {
						nodeStates[i].confidence++
					} else {
						nodeStates[i].confidence = 0
					}
				}
			}
		}
		
		// Check finalization
		finalizedA := 0
		finalizedB := 0
		for _, node := range nodeStates {
			if node.confidence >= params.Beta {
				if node.preference == choiceA {
					finalizedA++
				} else if node.preference == choiceB {
					finalizedB++
				}
			}
		}
		
		fmt.Printf("Round %3d: A=%3d B=%3d (finalized: A=%d B=%d)\n", 
			round+1, votesA, votesB, finalizedA, finalizedB)
		
		// Check if consensus reached
		if finalizedA > nodes/2 || finalizedB > nodes/2 {
			finalizedRound = round + 1
			break
		}
		
		// Simulate network delay
		time.Sleep(time.Duration(latencyMs) * time.Millisecond)
	}
	
	duration := time.Since(startTime)
	
	fmt.Printf("\n=== Results ===\n")
	if finalizedRound > 0 {
		fmt.Printf("Consensus reached in round %d\n", finalizedRound)
		fmt.Printf("Time to finality: %v\n", duration)
	} else {
		fmt.Printf("No consensus after %d rounds\n", rounds)
	}
	
	return nil
}


func sampleNodes(nodes []nodeState, k int, excludeIdx int) []nodeState {
	sample := make([]nodeState, 0, k)
	indices := make([]int, 0, len(nodes)-1)
	
	for i := range nodes {
		if i != excludeIdx {
			indices = append(indices, i)
		}
	}
	
	rand.Shuffle(len(indices), func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})
	
	for i := 0; i < k && i < len(indices); i++ {
		sample = append(sample, nodes[indices[i]])
	}
	
	return sample
}

func countVotes(sample []nodeState, choiceA, choiceB ids.ID) (int, int) {
	countA := 0
	countB := 0
	
	for _, node := range sample {
		if node.preference == choiceA {
			countA++
		} else if node.preference == choiceB {
			countB++
		}
	}
	
	return countA, countB
}
