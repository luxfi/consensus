// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build mlx
// +build mlx

package ai

import (
	"testing"
)

// TestMLXBackendInitialization tests MLX backend initialization
func TestMLXBackendInitialization(t *testing.T) {
	backend, err := NewMLXBackend(100)
	if err != nil {
		t.Fatalf("Failed to initialize MLX backend: %v", err)
	}

	if !backend.IsEnabled() {
		t.Error("MLX backend should be enabled")
	}

	if backend.GetDeviceInfo() == "" {
		t.Error("Device info should not be empty")
	}

	if backend.batchSize != 100 {
		t.Errorf("Expected batch size 100, got %d", backend.batchSize)
	}
}

// TestMLXBackendProcessVotes tests vote processing
func TestMLXBackendProcessVotes(t *testing.T) {
	backend, err := NewMLXBackend(10)
	if err != nil {
		t.Fatalf("Failed to initialize MLX backend: %v", err)
	}

	// Create test votes
	votes := make([]Vote, 10)
	for i := range votes {
		votes[i] = Vote{
			VoterID:      [32]byte{byte(i), byte(i + 1)},
			BlockID:      [32]byte{byte(i * 2), byte(i * 2 + 1)},
			IsPreference: i%2 == 0,
		}
	}

	// Process votes
	processed, err := backend.ProcessVotesBatch(votes)
	if err != nil {
		t.Fatalf("Failed to process votes: %v", err)
	}

	if processed != len(votes) {
		t.Errorf("Expected to process %d votes, got %d", len(votes), processed)
	}

	// Check throughput was updated
	throughput := backend.GetThroughput()
	if throughput <= 0 {
		t.Error("Throughput should be positive after processing")
	}
	t.Logf("Processing throughput: %.2f votes/sec", throughput)
}

// TestMLXBackendEmptyBatch tests processing empty batch
func TestMLXBackendEmptyBatch(t *testing.T) {
	backend, err := NewMLXBackend(10)
	if err != nil {
		t.Fatalf("Failed to initialize MLX backend: %v", err)
	}

	// Process empty batch
	processed, err := backend.ProcessVotesBatch([]Vote{})
	if err != nil {
		t.Fatalf("Failed to process empty batch: %v", err)
	}

	if processed != 0 {
		t.Errorf("Expected to process 0 votes, got %d", processed)
	}
}

// TestMLXBackendLargeBatch tests processing large batches
func TestMLXBackendLargeBatch(t *testing.T) {
	backend, err := NewMLXBackend(10000)
	if err != nil {
		t.Fatalf("Failed to initialize MLX backend: %v", err)
	}

	// Create large batch of votes
	votes := make([]Vote, 10000)
	for i := range votes {
		votes[i] = Vote{
			VoterID:      [32]byte{byte(i), byte(i >> 8), byte(i >> 16)},
			BlockID:      [32]byte{byte(i * 2), byte((i * 2) >> 8), byte((i * 2) >> 16)},
			IsPreference: i%2 == 0,
		}
	}

	// Process votes
	processed, err := backend.ProcessVotesBatch(votes)
	if err != nil {
		t.Fatalf("Failed to process large batch: %v", err)
	}

	if processed != len(votes) {
		t.Errorf("Expected to process %d votes, got %d", len(votes), processed)
	}

	throughput := backend.GetThroughput()
	t.Logf("Large batch throughput: %.2f votes/sec", throughput)
}

// TestMLXBackendConcurrency tests concurrent vote processing
func TestMLXBackendConcurrency(t *testing.T) {
	backend, err := NewMLXBackend(100)
	if err != nil {
		t.Fatalf("Failed to initialize MLX backend: %v", err)
	}

	// Create votes for concurrent processing
	votes := make([]Vote, 100)
	for i := range votes {
		votes[i] = Vote{
			VoterID:      [32]byte{byte(i)},
			BlockID:      [32]byte{byte(i * 2)},
			IsPreference: i%2 == 0,
		}
	}

	// Run concurrent processing
	done := make(chan bool, 3)
	for i := 0; i < 3; i++ {
		go func() {
			defer func() { done <- true }()
			_, err := backend.ProcessVotesBatch(votes)
			if err != nil {
				t.Errorf("Concurrent processing failed: %v", err)
			}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Check throughput after concurrent processing
	throughput := backend.GetThroughput()
	if throughput <= 0 {
		t.Error("Throughput should be positive after concurrent processing")
	}
	t.Logf("Concurrent processing throughput: %.2f votes/sec", throughput)
}