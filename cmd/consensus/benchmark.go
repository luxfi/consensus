// Copyright (C) 2024-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/factories"
	"github.com/luxfi/consensus/poll"
	"github.com/luxfi/ids"
)

type benchmarkResult struct {
	nodes           int
	rounds          int
	duration        time.Duration
	roundsPerSecond float64
	finalizedNodes  int
}

func runBenchmark(cmd *cobra.Command, args []string) error {
	// Get benchmark parameters
	nodes, _ := cmd.Flags().GetInt("nodes")
	rounds, _ := cmd.Flags().GetInt("rounds")
	transport, _ := cmd.Flags().GetString("transport")
	parallel, _ := cmd.Flags().GetBool("parallel")
	iterations, _ := cmd.Flags().GetInt("iterations")
	
	// Get consensus parameters
	params := config.DefaultParameters
	k, _ := cmd.Flags().GetInt("k")
	if k > 0 {
		params.K = k
	}

	fmt.Printf("=== Consensus Benchmark ===\n")
	fmt.Printf("Nodes: %d\n", nodes)
	fmt.Printf("Rounds: %d\n", rounds)
	fmt.Printf("Transport: %s\n", transport)
	fmt.Printf("Parallel: %v\n", parallel)
	fmt.Printf("Iterations: %d\n", iterations)
	fmt.Printf("K: %d\n", params.K)
	fmt.Printf("CPU cores: %d\n", runtime.NumCPU())
	
	// Run benchmarks
	results := make([]benchmarkResult, iterations)
	
	for i := 0; i < iterations; i++ {
		fmt.Printf("\nIteration %d/%d...\n", i+1, iterations)
		
		var result benchmarkResult
		if transport == "zmq" {
			result = runZMQBenchmark(nodes, rounds, params, parallel)
		} else {
			result = runLocalBenchmark(nodes, rounds, params, parallel)
		}
		
		results[i] = result
		fmt.Printf("  Duration: %v\n", result.duration)
		fmt.Printf("  Rounds/sec: %.2f\n", result.roundsPerSecond)
		fmt.Printf("  Finalized: %d/%d\n", result.finalizedNodes, nodes)
	}
	
	// Calculate statistics
	var totalDuration time.Duration
	var totalRPS float64
	var minDuration = results[0].duration
	var maxDuration = results[0].duration
	
	for _, r := range results {
		totalDuration += r.duration
		totalRPS += r.roundsPerSecond
		if r.duration < minDuration {
			minDuration = r.duration
		}
		if r.duration > maxDuration {
			maxDuration = r.duration
		}
	}
	
	avgDuration := totalDuration / time.Duration(iterations)
	avgRPS := totalRPS / float64(iterations)
	
	// Print summary
	fmt.Printf("\n=== Benchmark Summary ===\n")
	fmt.Printf("Average duration: %v\n", avgDuration)
	fmt.Printf("Min duration: %v\n", minDuration)
	fmt.Printf("Max duration: %v\n", maxDuration)
	fmt.Printf("Average rounds/sec: %.2f\n", avgRPS)
	fmt.Printf("Total messages: %d\n", nodes*rounds*params.K)
	fmt.Printf("Messages/sec: %.2f\n", float64(nodes*rounds*params.K)/avgDuration.Seconds())

	return nil
}

func runLocalBenchmark(nodes, rounds int, params config.Parameters, parallel bool) benchmarkResult {
	start := time.Now()
	
	// Convert parameters
	pollParams := poll.ConvertConfigParams(params)
	factory := factories.ConfidenceFactory
	
	// Create nodes
	consensusNodes := make([]poll.Unary, nodes)
	for i := 0; i < nodes; i++ {
		consensusNodes[i] = factory.NewUnary(pollParams)
	}
	
	// Choice IDs
	choices := make([]ids.ID, nodes)
	choice0 := ids.GenerateTestID()
	choice1 := ids.GenerateTestID()
	
	// Initialize with random preferences
	for i := 0; i < nodes; i++ {
		if i%2 == 0 {
			choices[i] = choice0
		} else {
			choices[i] = choice1
		}
	}
	
	// Run consensus rounds
	finalizedCount := int32(0)
	
	for round := 0; round < rounds; round++ {
		if parallel {
			// Parallel execution
			var wg sync.WaitGroup
			wg.Add(nodes)
			
			for i := 0; i < nodes; i++ {
				go func(nodeIdx int) {
					defer wg.Done()
					
					node := consensusNodes[nodeIdx]
					if node.Finalized() {
						return
					}
					
					// Create votes by sampling K nodes
					votes := make([]ids.ID, params.K)
					for j := 0; j < params.K; j++ {
						// Simple sampling: take sequential nodes
						peerIdx := (nodeIdx + j) % nodes
						votes[j] = consensusNodes[peerIdx].Preference()
					}
					
					node.RecordPoll(votes)
					
					if node.Finalized() {
						atomic.AddInt32(&finalizedCount, 1)
					}
				}(i)
			}
			
			wg.Wait()
		} else {
			// Sequential execution
			for i := 0; i < nodes; i++ {
				node := consensusNodes[i]
				if node.Finalized() {
					continue
				}
				
				// Create votes by sampling K nodes
				votes := make([]ids.ID, params.K)
				for j := 0; j < params.K; j++ {
					peerIdx := (i + j) % nodes
					votes[j] = consensusNodes[peerIdx].Preference()
				}
				
				node.RecordPoll(votes)
				
				if node.Finalized() {
					finalizedCount++
				}
			}
		}
		
		// Check if all finalized
		if int(atomic.LoadInt32(&finalizedCount)) == nodes {
			rounds = round + 1
			break
		}
	}
	
	duration := time.Since(start)
	
	return benchmarkResult{
		nodes:           nodes,
		rounds:          rounds,
		duration:        duration,
		roundsPerSecond: float64(rounds) / duration.Seconds(),
		finalizedNodes:  int(finalizedCount),
	}
}

func runZMQBenchmark(nodes, rounds int, params config.Parameters, parallel bool) benchmarkResult {
	// TODO: Implement ZMQ-based benchmark
	// This would use ZeroMQ for inter-node communication
	// allowing benchmarking across multiple machines
	
	fmt.Println("ZMQ benchmark not yet implemented")
	return benchmarkResult{
		nodes:    nodes,
		rounds:   rounds,
		duration: time.Second,
	}
}

func init() {
	// Add flags to benchmark command
	benchCmd := benchmarkCmd()
	benchCmd.Flags().Int("iterations", 3, "Number of benchmark iterations")
	benchCmd.Flags().Int("k", 0, "Sample size (K parameter)")
}