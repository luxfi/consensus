// Copyright (C) 2024-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/spf13/cobra"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/factories"
	"github.com/luxfi/consensus/poll"
	"github.com/luxfi/ids"
)

func runSimulator(cmd *cobra.Command, args []string) error {
	// Get simulation parameters
	nodes, _ := cmd.Flags().GetInt("nodes")
	rounds, _ := cmd.Flags().GetInt("rounds")
	byzantinePercent, _ := cmd.Flags().GetFloat64("byzantine")
	latencyMs, _ := cmd.Flags().GetInt("latency")
	factory, _ := cmd.Flags().GetString("factory")
	
	// Get consensus parameters
	params := config.DefaultParameters
	k, _ := cmd.Flags().GetInt("k")
	if k > 0 {
		params.K = k
	}

	fmt.Printf("=== Consensus Simulation ===\n")
	fmt.Printf("Nodes: %d\n", nodes)
	fmt.Printf("Rounds: %d\n", rounds)
	fmt.Printf("Byzantine nodes: %.1f%%\n", byzantinePercent*100)
	fmt.Printf("Network latency: %dms\n", latencyMs)
	fmt.Printf("Factory: %s\n", factory)
	fmt.Printf("K: %d\n", params.K)
	
	// Initialize random seed
	rand.Seed(time.Now().UnixNano())
	
	// Create nodes
	byzantineCount := int(float64(nodes) * byzantinePercent)
	fmt.Printf("\nInitializing %d honest nodes and %d byzantine nodes...\n", 
		nodes-byzantineCount, byzantineCount)

	// Select factory
	var pollFactory poll.Factory
	switch factory {
	case "confidence":
		pollFactory = factories.ConfidenceFactory
	case "threshold":
		pollFactory = factories.FlatFactory
	default:
		pollFactory = poll.DefaultFactory
	}

	// Convert config parameters to poll parameters
	pollParams := poll.ConvertConfigParams(params)
	
	// Create consensus instances
	consensusNodes := make([]poll.Unary, nodes)
	choices := make([]ids.ID, nodes)
	
	// Initialize with random preferences
	choice0 := ids.GenerateTestID()
	choice1 := ids.GenerateTestID()
	
	for i := 0; i < nodes; i++ {
		consensusNodes[i] = pollFactory.NewUnary(pollParams)
		if rand.Float64() < 0.5 {
			choices[i] = choice0
		} else {
			choices[i] = choice1
		}
	}

	// Mark byzantine nodes
	byzantineNodes := make(map[int]bool)
	for i := 0; i < byzantineCount; i++ {
		idx := rand.Intn(nodes)
		byzantineNodes[idx] = true
	}

	// Run simulation
	fmt.Printf("\nRunning simulation...\n")
	startTime := time.Now()
	finalizedCount := 0
	
	for round := 0; round < rounds; round++ {
		// Check if all honest nodes have finalized
		allFinalized := true
		for i, node := range consensusNodes {
			if !byzantineNodes[i] && !node.Finalized() {
				allFinalized = false
				break
			}
		}
		
		if allFinalized {
			fmt.Printf("\nAll honest nodes finalized after %d rounds\n", round)
			break
		}
		
		// Simulate network delay
		if latencyMs > 0 {
			time.Sleep(time.Duration(latencyMs) * time.Millisecond)
		}
		
		// Each node samples K peers
		for i, node := range consensusNodes {
			if node.Finalized() {
				continue
			}
			
			// Sample K random peers
			votes := make([]ids.ID, 0, params.K)
			sampled := make(map[int]bool)
			
			for len(votes) < params.K {
				peer := rand.Intn(nodes)
				if sampled[peer] {
					continue
				}
				sampled[peer] = true
				
				// Byzantine nodes may lie
				if byzantineNodes[peer] {
					// Byzantine strategy: always vote for minority
					if rand.Float64() < 0.8 { // 80% malicious behavior
						votes = append(votes, choice1) // Assume choice1 is minority
					} else {
						votes = append(votes, choices[peer])
					}
				} else {
					votes = append(votes, consensusNodes[peer].Preference())
				}
			}
			
			// Record poll results
			node.RecordPoll(votes)
			
			// Update choice for this node
			choices[i] = node.Preference()
			
			// Count finalized nodes
			if node.Finalized() && !byzantineNodes[i] {
				finalizedCount++
			}
		}
		
		// Progress update every 10 rounds
		if round%10 == 0 && round > 0 {
			finalized := 0
			for i, node := range consensusNodes {
				if !byzantineNodes[i] && node.Finalized() {
					finalized++
				}
			}
			fmt.Printf("Round %d: %d/%d honest nodes finalized\n", 
				round, finalized, nodes-byzantineCount)
		}
	}

	elapsed := time.Since(startTime)
	
	// Results
	fmt.Printf("\n=== Simulation Results ===\n")
	fmt.Printf("Total time: %v\n", elapsed)
	
	// Count final preferences
	preference0 := 0
	preference1 := 0
	for i, node := range consensusNodes {
		if byzantineNodes[i] {
			continue
		}
		if node.Preference() == choice0 {
			preference0++
		} else {
			preference1++
		}
	}
	
	fmt.Printf("Final consensus:\n")
	fmt.Printf("  Choice 0: %d nodes\n", preference0)
	fmt.Printf("  Choice 1: %d nodes\n", preference1)
	
	// Check if consensus was achieved
	if preference0 == nodes-byzantineCount || preference1 == nodes-byzantineCount {
		fmt.Printf("✓ Consensus achieved!\n")
	} else {
		fmt.Printf("✗ Consensus not achieved\n")
	}

	return nil
}

func init() {
	// Add flags to sim command
	simCmd := simCmd()
	simCmd.Flags().Int("nodes", 100, "Number of nodes in the network")
	simCmd.Flags().Int("rounds", 50, "Maximum number of rounds to simulate")
	simCmd.Flags().Float64("byzantine", 0.2, "Percentage of byzantine nodes (0.0-0.33)")
	simCmd.Flags().Int("latency", 10, "Network latency in milliseconds")
	simCmd.Flags().String("factory", "confidence", "Consensus factory: confidence, threshold, default")
	simCmd.Flags().Int("k", 0, "Sample size (K parameter)")
}