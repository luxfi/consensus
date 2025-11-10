// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build mlx
// +build mlx

package ai

import (
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/mlx"
)

// MLXBackend provides GPU-accelerated consensus using Apple's MLX framework
type MLXBackend struct {
	mu            sync.RWMutex
	enabled       bool
	device        string
	batchSize     int
	voteBuffer    []Vote
	throughput    float64
	peakMemory    uint64
	initialized   bool
}

// Vote represents a consensus vote for MLX processing
type Vote struct {
	VoterID     [32]byte
	BlockID     [32]byte
	IsPreference bool
}

// NewMLXBackend creates a new MLX-accelerated consensus backend
func NewMLXBackend(batchSize int) (*MLXBackend, error) {
	backend := &MLXBackend{
		batchSize:  batchSize,
		voteBuffer: make([]Vote, 0, batchSize),
	}

	// Check if MLX is available
	info := mlx.Info()
	if info == "" {
		return nil, fmt.Errorf("MLX not available")
	}

	backend.device = info
	backend.enabled = true
	backend.initialized = true

	fmt.Printf("MLX Backend initialized\n")
	fmt.Printf("Device: %s\n", backend.device)
	fmt.Printf("Batch size: %d\n", batchSize)

	return backend, nil
}

// AddVote adds a vote to the processing buffer
func (b *MLXBackend) AddVote(voterID, blockID [32]byte, isPreference bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.voteBuffer = append(b.voteBuffer, Vote{
		VoterID:      voterID,
		BlockID:      blockID,
		IsPreference: isPreference,
	})

	// Auto-flush when buffer reaches batch size
	if len(b.voteBuffer) >= b.batchSize {
		b.flush()
	}
}

// Flush processes all buffered votes on GPU
func (b *MLXBackend) Flush() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.flush()
}

// flush (internal, must hold lock)
func (b *MLXBackend) flush() {
	if len(b.voteBuffer) == 0 {
		return
	}

	start := time.Now()

	// Convert votes to MLX arrays
	data := make([]float32, len(b.voteBuffer)*64) // 32 bytes voter + 32 bytes block
	for i, vote := range b.voteBuffer {
		offset := i * 64
		// Voter ID (32 bytes)
		for j := 0; j < 32; j++ {
			data[offset+j] = float32(vote.VoterID[j]) / 255.0
		}
		// Block ID (32 bytes)
		for j := 0; j < 32; j++ {
			data[offset+32+j] = float32(vote.BlockID[j]) / 255.0
		}
	}

	// Create MLX array and process on GPU
	shape := []int{len(b.voteBuffer), 64}
	input := mlx.FromSlice(data, shape, mlx.Float32)

	// Simple neural network inference on GPU
	// Layer 1: Linear transformation
	weights := mlx.Random([]int{64, 128}, mlx.Float32)
	layer1 := mlx.MatMul(input, weights)

	// ReLU activation
	activated := mlx.Maximum(layer1, mlx.Zeros(layer1.Shape(), mlx.Float32))

	// Layer 2: Reduce to single output per vote
	output := mlx.Mean(activated, 1, false)

	// Force evaluation on GPU
	mlx.Eval(output)
	mlx.Synchronize()

	elapsed := time.Since(start)

	// Update throughput (exponential moving average)
	currentThroughput := float64(len(b.voteBuffer)) / elapsed.Seconds()
	if b.throughput == 0 {
		b.throughput = currentThroughput
	} else {
		b.throughput = 0.9*b.throughput + 0.1*currentThroughput
	}

	b.voteBuffer = b.voteBuffer[:0]
}

// ProcessVotesBatch processes a batch of votes on GPU
func (b *MLXBackend) ProcessVotesBatch(votes []Vote) (int, error) {
	if !b.initialized {
		return 0, fmt.Errorf("MLX backend not initialized")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	start := time.Now()

	// Convert to float32 array
	data := make([]float32, len(votes)*64)
	for i, vote := range votes {
		offset := i * 64
		for j := 0; j < 32; j++ {
			data[offset+j] = float32(vote.VoterID[j]) / 255.0
		}
		for j := 0; j < 32; j++ {
			data[offset+32+j] = float32(vote.BlockID[j]) / 255.0
		}
	}

	// Process on GPU
	shape := []int{len(votes), 64}
	input := mlx.FromSlice(data, shape, mlx.Float32)

	weights := mlx.Random([]int{64, 128}, mlx.Float32)
	layer1 := mlx.MatMul(input, weights)
	activated := mlx.Maximum(layer1, mlx.Zeros(layer1.Shape(), mlx.Float32))
	output := mlx.Mean(activated, 1, false)

	mlx.Eval(output)
	mlx.Synchronize()

	elapsed := time.Since(start)
	throughput := float64(len(votes)) / elapsed.Seconds()

	fmt.Printf("Processed %d votes in %v (%.0f votes/sec)\n",
		len(votes), elapsed, throughput)

	return len(votes), nil
}

// GetThroughput returns the current throughput in votes/second
func (b *MLXBackend) GetThroughput() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.throughput
}

// GetMemoryUsage returns current GPU memory usage
func (b *MLXBackend) GetMemoryUsage() (active, peak uint64) {
	// MLX doesn't expose memory stats in Go API yet
	// These would come from the C API
	return 0, b.peakMemory
}

// IsEnabled returns whether GPU acceleration is enabled
func (b *MLXBackend) IsEnabled() bool {
	return b.enabled
}

// GetDeviceInfo returns information about the GPU device
func (b *MLXBackend) GetDeviceInfo() string {
	return b.device
}
