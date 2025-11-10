// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build mlx
// +build mlx

package ai

import (
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
			VoterID:      [32]byte{byte(i)},
			BlockID:      [32]byte{byte(i >> 8)},
			IsPreference: i%2 == 0,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = backend.ProcessVotesBatch(votes)
	}
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
			VoterID:      [32]byte{byte(i)},
			BlockID:      [32]byte{byte(i >> 8)},
			IsPreference: i%2 == 0,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = backend.ProcessVotesBatch(votes)
	}
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
			VoterID:      [32]byte{byte(i)},
			BlockID:      [32]byte{byte(i >> 8)},
			IsPreference: i%2 == 0,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = backend.ProcessVotesBatch(votes)
	}
}
