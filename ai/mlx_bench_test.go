// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build mlx
// +build mlx

package ai

import (
	"fmt"
	"testing"
)

// BenchmarkMLXBatchProcessing benchmarks MLX GPU batch vote processing
func BenchmarkMLXBatchProcessing(b *testing.B) {
	backend, err := NewMLXBackend(1000)
	if err != nil {
		b.Skip("MLX not available")
	}

	// Create test votes
	votes := make([]Vote, 1000)
	for i := range votes {
		votes[i] = Vote{
			VoterID:      [32]byte{byte(i), byte(i >> 8)},
			BlockID:      [32]byte{byte(i >> 16), byte(i >> 24)},
			IsPreference: i%2 == 0,
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	var totalProcessed int
	for i := 0; i < b.N; i++ {
		processed, _ := backend.ProcessVotesBatch(votes)
		totalProcessed += processed
	}

	b.StopTimer()
	throughput := backend.GetThroughput()
	b.ReportMetric(throughput, "votes/sec")
	b.ReportMetric(float64(totalProcessed)/b.Elapsed().Seconds(), "avg-votes/sec")
}

// BenchmarkMLXBatchProcessing100 benchmarks smaller batches
func BenchmarkMLXBatchProcessing100(b *testing.B) {
	backend, err := NewMLXBackend(100)
	if err != nil {
		b.Skip("MLX not available")
	}

	votes := make([]Vote, 100)
	for i := range votes {
		votes[i] = Vote{
			VoterID:      [32]byte{byte(i), byte(i + 1)},
			BlockID:      [32]byte{byte(i * 2), byte(i*2 + 1)},
			IsPreference: i%2 == 0,
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	var totalProcessed int
	for i := 0; i < b.N; i++ {
		processed, _ := backend.ProcessVotesBatch(votes)
		totalProcessed += processed
	}

	b.StopTimer()
	throughput := backend.GetThroughput()
	b.ReportMetric(throughput, "votes/sec")
	b.ReportMetric(float64(totalProcessed)/b.Elapsed().Seconds(), "avg-votes/sec")
}

// BenchmarkMLXBatchProcessing10000 benchmarks large batches
func BenchmarkMLXBatchProcessing10000(b *testing.B) {
	backend, err := NewMLXBackend(10000)
	if err != nil {
		b.Skip("MLX not available")
	}

	votes := make([]Vote, 10000)
	for i := range votes {
		votes[i] = Vote{
			VoterID:      [32]byte{byte(i), byte(i >> 8), byte(i >> 16), byte(i >> 24)},
			BlockID:      [32]byte{byte(i * 2), byte((i * 2) >> 8), byte((i * 2) >> 16), byte((i * 2) >> 24)},
			IsPreference: i%2 == 0,
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	var totalProcessed int
	for i := 0; i < b.N; i++ {
		processed, _ := backend.ProcessVotesBatch(votes)
		totalProcessed += processed
	}

	b.StopTimer()
	throughput := backend.GetThroughput()
	b.ReportMetric(throughput, "votes/sec")
	b.ReportMetric(float64(totalProcessed)/b.Elapsed().Seconds(), "avg-votes/sec")
}

// BenchmarkMLXVaryingBatchSizes benchmarks different batch sizes
func BenchmarkMLXVaryingBatchSizes(b *testing.B) {
	batchSizes := []int{10, 50, 100, 500, 1000, 5000, 10000}

	for _, size := range batchSizes {
		b.Run(fmt.Sprintf("batch_%d", size), func(b *testing.B) {
			backend, err := NewMLXBackend(size)
			if err != nil {
				b.Skip("MLX not available")
			}

			votes := make([]Vote, size)
			for i := range votes {
				votes[i] = Vote{
					VoterID:      [32]byte{byte(i), byte(i >> 8)},
					BlockID:      [32]byte{byte(i * 2), byte((i * 2) >> 8)},
					IsPreference: i%2 == 0,
				}
			}

			b.ResetTimer()
			b.ReportAllocs()

			var totalProcessed int
			for i := 0; i < b.N; i++ {
				processed, _ := backend.ProcessVotesBatch(votes)
				totalProcessed += processed
			}

			b.StopTimer()
			throughput := backend.GetThroughput()
			b.ReportMetric(throughput, "votes/sec")
			b.ReportMetric(float64(size), "batch-size")
			b.ReportMetric(float64(totalProcessed)/b.Elapsed().Seconds(), "avg-votes/sec")
		})
	}
}

// BenchmarkMLXMemoryUsage benchmarks memory allocation patterns
func BenchmarkMLXMemoryUsage(b *testing.B) {
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			backend, err := NewMLXBackend(size)
			if err != nil {
				b.Skip("MLX not available")
			}

			votes := make([]Vote, size)
			for i := range votes {
				votes[i] = Vote{
					VoterID:      [32]byte{byte(i), byte(i >> 8)},
					BlockID:      [32]byte{byte(i * 2), byte((i * 2) >> 8)},
					IsPreference: i%2 == 0,
				}
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, _ = backend.ProcessVotesBatch(votes)
			}

			b.StopTimer()
			// Report memory metrics
			b.ReportMetric(float64(size*64*4), "input-bytes")   // 64 floats per vote * 4 bytes per float
			b.ReportMetric(float64(size*128*4), "hidden-bytes") // hidden layer size
			b.ReportMetric(float64(size*1*4), "output-bytes")   // output layer size
		})
	}
}
