// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/ids"
	"log/slog"
)

var logger = slog.Default().With("module", "sim")

// Node represents a simulated network node
type Node struct {
	ID        ids.NodeID
	Byzantine bool
	Choice    int // 0 or 1 for binary consensus
}

// SimulationResult contains the results of a consensus simulation
type SimulationResult struct {
	Rounds          int
	Finalized       bool
	FinalChoice     int
	AgreementRatio  float64
	NetworkQueries  int
}

func main() {
	// Command line flags
	network := flag.String("network", "mainnet", "Network type: mainnet, testnet, or local")
	numNodes := flag.Int("nodes", 100, "Total number of nodes to simulate")
	byzantineNodes := flag.Int("byzantine", 0, "Number of Byzantine nodes")
	rounds := flag.Int("rounds", 100, "Maximum rounds to simulate")
	simulations := flag.Int("sims", 100, "Number of simulations to run")
	initialSplit := flag.Float64("split", 0.5, "Initial preference split (0.0-1.0)")
	samplerType := flag.String("sampler", "uniform", "Sampler type: uniform")
	verbose := flag.Bool("verbose", false, "Show detailed round-by-round progress")
	seed := flag.Int64("seed", 0, "Random seed (0 for time-based)")
	flag.Parse()

	// Initialize random seed
	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	rand.Seed(*seed)
	logger.Info("Starting consensus simulator", "seed", *seed)

	// Load configuration
	var cfg *config.Config
	switch *network {
	case "mainnet":
		cfg = &config.MainnetConfig
	case "testnet":
		cfg = &config.TestnetConfig
	case "local":
		cfg = &config.LocalConfig
	default:
		logger.Error("Invalid network type", "network", *network)
		os.Exit(1)
	}

	// Override node count
	if *numNodes > 0 {
		cfg.TotalNodes = *numNodes
	}

	// Create nodes
	nodes := createNodes(*numNodes, *byzantineNodes, *initialSplit)

	fmt.Printf("\n=== Consensus Metastability Simulator ===\n")
	fmt.Printf("\nConfiguration:\n")
	fmt.Printf("  Network:              %s\n", *network)
	fmt.Printf("  Total Nodes:          %d\n", *numNodes)
	fmt.Printf("  Byzantine Nodes:      %d (%.1f%%)\n", *byzantineNodes, float64(*byzantineNodes)/float64(*numNodes)*100)
	fmt.Printf("  Initial Split:        %.1f%% / %.1f%%\n", *initialSplit*100, (1-*initialSplit)*100)
	fmt.Printf("  Sampler Type:         %s\n", *samplerType)
	fmt.Printf("  K (sample size):      %d\n", cfg.K)
	fmt.Printf("  Alpha Preference:     %d\n", cfg.AlphaPreference)
	fmt.Printf("  Alpha Confidence:     %d\n", cfg.AlphaConfidence)
	fmt.Printf("  Beta:                 %d\n", cfg.Beta)
	fmt.Printf("  Simulations:          %d\n", *simulations)
	fmt.Printf("  Max Rounds:           %d\n", *rounds)

	// Run simulations
	results := make([]SimulationResult, *simulations)
	finalizedCount := 0
	totalRounds := 0
	choice0Wins := 0
	choice1Wins := 0
	
	startTime := time.Now()
	
	for i := 0; i < *simulations; i++ {
		if !*verbose && i%10 == 0 {
			fmt.Printf("\rRunning simulation %d/%d...", i+1, *simulations)
		}
		
		result := runSimulation(nodes, cfg, *samplerType, *rounds, *verbose)
		results[i] = result
		
		if result.Finalized {
			finalizedCount++
			totalRounds += result.Rounds
			if result.FinalChoice == 0 {
				choice0Wins++
			} else {
				choice1Wins++
			}
		}
	}
	
	duration := time.Since(startTime)
	
	// Analyze results
	fmt.Printf("\n\n=== Simulation Results ===\n")
	fmt.Printf("\nFinalization Rate:    %.1f%% (%d/%d)\n", 
		float64(finalizedCount)/float64(*simulations)*100, finalizedCount, *simulations)
	
	if finalizedCount > 0 {
		avgRounds := float64(totalRounds) / float64(finalizedCount)
		fmt.Printf("Average Rounds:       %.1f\n", avgRounds)
		fmt.Printf("Choice 0 Wins:        %.1f%% (%d/%d)\n", 
			float64(choice0Wins)/float64(finalizedCount)*100, choice0Wins, finalizedCount)
		fmt.Printf("Choice 1 Wins:        %.1f%% (%d/%d)\n", 
			float64(choice1Wins)/float64(finalizedCount)*100, choice1Wins, finalizedCount)
		
		// Calculate expected finality time
		if cfg.NetworkLatency > 0 {
			expectedFinality := time.Duration(avgRounds) * cfg.NetworkLatency
			fmt.Printf("Expected Finality:    %s (with %s network latency)\n", 
				expectedFinality, cfg.NetworkLatency)
		}
	}
	
	// Network efficiency
	totalQueries := 0
	for _, result := range results {
		totalQueries += result.NetworkQueries
	}
	avgQueries := float64(totalQueries) / float64(*simulations)
	theoreticalQueries := float64(cfg.K * cfg.Beta * *simulations)
	efficiency := (avgQueries / theoreticalQueries) * 100
	
	fmt.Printf("\n=== Network Efficiency ===\n")
	fmt.Printf("Total Network Queries:    %d\n", totalQueries)
	fmt.Printf("Average Queries/Sim:      %.1f\n", avgQueries)
	fmt.Printf("Theoretical Maximum:      %.0f\n", theoreticalQueries)
	fmt.Printf("Query Efficiency:         %.1f%%\n", efficiency)
	
	fmt.Printf("\nSimulation Time:          %s\n", duration)
	fmt.Printf("Simulations/Second:       %.1f\n", float64(*simulations)/duration.Seconds())
	
	// Metastability analysis
	if *byzantineNodes == 0 && finalizedCount < *simulations {
		fmt.Printf("\n⚠️  Metastability Detected!\n")
		fmt.Printf("   %d simulations failed to finalize within %d rounds\n", 
			*simulations-finalizedCount, *rounds)
		fmt.Printf("   This suggests the parameters may need adjustment for the given network conditions.\n")
	}
}

func createNodes(total, byzantine int, initialSplit float64) []Node {
	nodes := make([]Node, total)
	
	// Assign Byzantine nodes
	byzantineIndices := make(map[int]bool)
	for i := 0; i < byzantine; i++ {
		for {
			idx := rand.Intn(total)
			if !byzantineIndices[idx] {
				byzantineIndices[idx] = true
				break
			}
		}
	}
	
	// Create nodes with initial preferences
	choice0Count := int(float64(total) * initialSplit)
	for i := 0; i < total; i++ {
		nodeID := ids.BuildTestNodeID([]byte(fmt.Sprintf("node-%d", i)))
		nodes[i] = Node{
			ID:        nodeID,
			Byzantine: byzantineIndices[i],
		}
		
		if i < choice0Count {
			nodes[i].Choice = 0
		} else {
			nodes[i].Choice = 1
		}
	}
	
	// Shuffle to randomize distribution
	rand.Shuffle(len(nodes), func(i, j int) {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	})
	
	return nodes
}

func runSimulation(nodes []Node, cfg *config.Config, samplerType string, maxRounds int, verbose bool) SimulationResult {
	// Create sampler (uniform random sampling)
	var sampler prism.Sampler
	if samplerType == "uniform" {
		// Use standard sampler with random source for uniform sampling
		sampler = prism.NewSampler(nil)
	} else {
		logger.Error("Unknown sampler type", "type", samplerType)
		os.Exit(1)
	}
	
	// Create validator list
	validators := make([]ids.NodeID, 0, len(nodes))
	for _, node := range nodes {
		validators = append(validators, node.ID)
	}
	
	// Initialize consensus state
	preference := 0 // Start with choice 0
	betaConsecutive := 0
	networkQueries := 0
	
	if verbose {
		fmt.Printf("\n--- Starting Simulation ---\n")
	}
	
	for round := 1; round <= maxRounds; round++ {
		// Sample K nodes
		sample, err := sampler.Sample(validators, cfg.K)
		if err != nil {
			logger.Error("Failed to sample nodes", "error", err)
			continue
		}
		networkQueries += cfg.K
		
		// Count votes in sample
		votes := make([]int, 2)
		for _, nodeID := range sample {
			// Find node by ID
			var node *Node
			for i := range nodes {
				if nodes[i].ID == nodeID {
					node = &nodes[i]
					break
				}
			}
			
			if node.Byzantine {
				// Byzantine nodes always vote for minority to disrupt
				votes[1-preference]++
			} else {
				votes[node.Choice]++
			}
		}
		
		// Determine if we met alpha preference threshold
		metAlphaPref := false
		newPreference := preference
		
		if votes[0] >= cfg.AlphaPreference {
			metAlphaPref = true
			newPreference = 0
		} else if votes[1] >= cfg.AlphaPreference {
			metAlphaPref = true
			newPreference = 1
		}
		
		// Update honest nodes based on prism result
		if metAlphaPref && newPreference != preference {
			// Preference changed - update all honest nodes
			for i := range nodes {
				if !nodes[i].Byzantine {
					nodes[i].Choice = newPreference
				}
			}
			preference = newPreference
			betaConsecutive = 0
		}
		
		// Check alpha confidence threshold
		if votes[preference] >= cfg.AlphaConfidence {
			betaConsecutive++
		} else {
			betaConsecutive = 0
		}
		
		if verbose {
			fmt.Printf("Round %3d: Sample votes [%d, %d], Preference=%d, Beta=%d/%d\n",
				round, votes[0], votes[1], preference, betaConsecutive, cfg.Beta)
		}
		
		// Check finalization
		if betaConsecutive >= cfg.Beta {
			// Calculate final agreement
			finalCount := 0
			for _, node := range nodes {
				if !node.Byzantine && node.Choice == preference {
					finalCount++
				}
			}
			agreement := float64(finalCount) / float64(len(nodes)-countByzantine(nodes))
			
			if verbose {
				fmt.Printf("\n✅ Finalized on choice %d after %d rounds (%.1f%% agreement)\n",
					preference, round, agreement*100)
			}
			
			return SimulationResult{
				Rounds:         round,
				Finalized:      true,
				FinalChoice:    preference,
				AgreementRatio: agreement,
				NetworkQueries: networkQueries,
			}
		}
	}
	
	// Failed to finalize
	if verbose {
		fmt.Printf("\n❌ Failed to finalize after %d rounds\n", maxRounds)
	}
	
	return SimulationResult{
		Rounds:         maxRounds,
		Finalized:      false,
		NetworkQueries: networkQueries,
	}
}

func countByzantine(nodes []Node) int {
	count := 0
	for _, node := range nodes {
		if node.Byzantine {
			count++
		}
	}
	return count
}