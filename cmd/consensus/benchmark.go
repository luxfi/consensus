// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/spf13/cobra"
)

type benchmarkResult struct {
	nodes           int
	rounds          int
	duration        time.Duration
	roundsPerSecond float64
}

func runBenchmark(cmd *cobra.Command, args []string) error {
	// Get flags
	nodes, _ := cmd.Flags().GetInt("nodes")
	rounds, _ := cmd.Flags().GetInt("rounds")
	iterations, _ := cmd.Flags().GetInt("iterations")
	transport, _ := cmd.Flags().GetString("transport")
	parallel, _ := cmd.Flags().GetBool("parallel")

	// Get consensus parameters
	k, _ := cmd.Flags().GetInt("k")
	var params config.Parameters
	if k > 0 {
		params = config.Parameters{
			K:                     k,
			AlphaPreference:       (k / 2) + 1,
			AlphaConfidence:       (k / 2) + 2,
			Beta:                  k / 4,
			ConcurrentPolls:       1,
			OptimalProcessing:     10,
			MaxOutstandingItems:   256,
			MaxItemProcessingTime: 30 * time.Second,
		}
	} else {
		params = config.DefaultParameters
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
	var results []benchmarkResult

	for i := 0; i < iterations; i++ {
		start := time.Now()

		if parallel {
			runParallelBenchmark(nodes, rounds, params)
		} else {
			runSequentialBenchmark(nodes, rounds, params)
		}

		duration := time.Since(start)
		result := benchmarkResult{
			nodes:           nodes,
			rounds:          rounds,
			duration:        duration,
			roundsPerSecond: float64(rounds) / duration.Seconds(),
		}
		results = append(results, result)

		fmt.Printf("Iteration %d: %v (%.2f rounds/sec)\n", i+1, duration, result.roundsPerSecond)
	}

	// Calculate statistics
	var totalDuration time.Duration
	var totalRPS float64
	for _, r := range results {
		totalDuration += r.duration
		totalRPS += r.roundsPerSecond
	}

	avgDuration := totalDuration / time.Duration(iterations)
	avgRPS := totalRPS / float64(iterations)

	fmt.Printf("\n=== Results ===\n")
	fmt.Printf("Average duration: %v\n", avgDuration)
	fmt.Printf("Average rounds/sec: %.2f\n", avgRPS)

	return nil
}

func runSequentialBenchmark(nodes, rounds int, params config.Parameters) {
	// Simple mock consensus simulation
	var decisions int32

	for r := 0; r < rounds; r++ {
		// Simulate consensus round
		votes := make(map[ids.ID]int)

		// Each node votes
		for n := 0; n < nodes; n++ {
			choice := ids.GenerateTestID()
			votes[choice]++
		}

		// Check if consensus reached
		for _, count := range votes {
			if count >= params.AlphaConfidence {
				atomic.AddInt32(&decisions, 1)
				break
			}
		}
	}
}

func runParallelBenchmark(nodes, rounds int, params config.Parameters) {
	var wg sync.WaitGroup
	var decisions int32

	// Run nodes in parallel
	for n := 0; n < nodes; n++ {
		wg.Add(1)
		go func(nodeID int) {
			defer wg.Done()

			for r := 0; r < rounds/nodes; r++ {
				// Simulate node participating in consensus
				choice := ids.GenerateTestID()
				_ = choice
				atomic.AddInt32(&decisions, 1)
			}
		}(n)
	}

	wg.Wait()
}
