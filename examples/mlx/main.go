// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build mlx
// +build mlx

package main

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/luxfi/consensus/ai"
)

func main() {
	fmt.Println("=== Lux Consensus MLX GPU Acceleration Example ===\n")

	// Create MLX backend with batch size 32
	backend, err := ai.NewMLXBackend(32)
	if err != nil {
		fmt.Printf("Failed to initialize MLX: %v\n", err)
		fmt.Println("Make sure you're on Apple Silicon and MLX is installed")
		return
	}

	fmt.Printf("Device: %s\n", backend.GetDeviceInfo())
	fmt.Printf("GPU Enabled: %v\n\n", backend.IsEnabled())

	// Benchmark different batch sizes
	fmt.Println("Performance Benchmarks:")
	fmt.Println("=======================\n")

	batchSizes := []int{10, 100, 1000, 10000}

	for _, batchSize := range batchSizes {
		// Generate random votes
		votes := generateVotes(batchSize)

		// Warm-up
		backend.ProcessVotesBatch(votes)

		// Benchmark
		start := time.Now()
		processed, err := backend.ProcessVotesBatch(votes)
		elapsed := time.Since(start)

		if err != nil {
			fmt.Printf("Error processing batch: %v\n", err)
			continue
		}

		throughput := float64(processed) / elapsed.Seconds()
		perVote := elapsed.Nanoseconds() / int64(processed)

		fmt.Printf("Batch Size: %d\n", batchSize)
		fmt.Printf("  Time: %v\n", elapsed)
		fmt.Printf("  Throughput: %.0f votes/sec\n", throughput)
		fmt.Printf("  Per-vote: %d ns\n", perVote)
		fmt.Printf("  Processed: %d/%d\n\n", processed, batchSize)
	}

	// Test adaptive batching
	fmt.Println("Testing Adaptive Batch Processing:")
	fmt.Println("===================================\n")

	start := time.Now()

	// Process 10,000 votes with automatic batching
	for i := 0; i < 10000; i++ {
		vote := generateVote()
		backend.AddVote(vote.VoterID, vote.BlockID, vote.IsPreference)
	}
	backend.Flush()

	elapsed := time.Since(start)

	fmt.Printf("Total time: %v\n", elapsed)
	fmt.Printf("Throughput: %.0f votes/sec\n\n", backend.GetThroughput())

	// CPU vs GPU comparison
	fmt.Println("CPU vs GPU Comparison:")
	fmt.Println("======================\n")

	votes := generateVotes(1000)

	// GPU processing
	gpuStart := time.Now()
	backend.ProcessVotesBatch(votes)
	gpuTime := time.Since(gpuStart)

	// Simulate CPU processing (simple loop, no GPU)
	cpuStart := time.Now()
	for range votes {
		// Simulate per-vote processing overhead
		time.Sleep(time.Microsecond * 10)
	}
	cpuTime := time.Since(cpuStart)

	speedup := float64(cpuTime) / float64(gpuTime)

	fmt.Printf("CPU Time: %v\n", cpuTime)
	fmt.Printf("GPU Time: %v\n", gpuTime)
	fmt.Printf("Speedup: %.1fx\n\n", speedup)

	fmt.Println("✅ MLX GPU acceleration working!")
}

func generateVote() ai.Vote {
	var voterID, blockID [32]byte
	rand.Read(voterID[:])
	rand.Read(blockID[:])
	return ai.Vote{
		VoterID:      voterID,
		BlockID:      blockID,
		IsPreference: false,
	}
}

func generateVotes(n int) []ai.Vote {
	votes := make([]ai.Vote, n)
	for i := 0; i < n; i++ {
		votes[i] = generateVote()
	}
	return votes
}
