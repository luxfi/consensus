// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package ai

import (
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/accel/ops/consensus"
)

// Vote represents a consensus vote for batch processing.
type Vote struct {
	VoterID      [32]byte
	BlockID      [32]byte
	IsPreference bool
}

// ValidatorInfo contains validator information for quorum calculations.
type ValidatorInfo struct {
	ValidatorID [32]byte
	Weight      uint64
}

// Backend provides GPU-accelerated consensus using luxfi/accel.
type Backend struct {
	mu          sync.RWMutex
	batchSize   int
	throughput  float64
	initialized bool
}

// NewBackend creates a GPU-accelerated consensus backend.
func NewBackend(batchSize int) (*Backend, error) {
	return &Backend{
		batchSize:   batchSize,
		initialized: true,
	}, nil
}

// ProcessVotesBatch processes a batch of votes using GPU acceleration.
func (b *Backend) ProcessVotesBatch(votes []Vote) (int, error) {
	if !b.initialized {
		return 0, fmt.Errorf("backend not initialized")
	}
	if len(votes) == 0 {
		return 0, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	start := time.Now()

	voteData := make([]consensus.VoteData, len(votes))
	for i, v := range votes {
		voteData[i] = consensus.VoteData{
			VoterID:      v.VoterID,
			BlockID:      v.BlockID,
			IsPreference: v.IsPreference,
		}
	}

	processed, err := consensus.ProcessVotesBatch(voteData)
	if err != nil {
		return 0, err
	}

	elapsed := time.Since(start)
	throughput := float64(processed) / elapsed.Seconds()

	if b.throughput == 0 {
		b.throughput = throughput
	} else {
		b.throughput = 0.9*b.throughput + 0.1*throughput
	}

	return processed, nil
}

// ComputeQuorum checks if a quorum is reached for a set of votes.
func (b *Backend) ComputeQuorum(votes []Vote, validators []ValidatorInfo, threshold float64) (*consensus.QuorumResult, error) {
	if !b.initialized {
		return nil, fmt.Errorf("backend not initialized")
	}

	voteData := make([]consensus.VoteData, len(votes))
	for i, v := range votes {
		voteData[i] = consensus.VoteData{
			VoterID:      v.VoterID,
			BlockID:      v.BlockID,
			IsPreference: v.IsPreference,
		}
	}

	validatorWeights := make([]consensus.ValidatorWeight, len(validators))
	for i, v := range validators {
		validatorWeights[i] = consensus.ValidatorWeight{
			ValidatorID: v.ValidatorID,
			Weight:      v.Weight,
		}
	}

	return consensus.ComputeQuorum(voteData, validatorWeights, threshold)
}

// GetThroughput returns the current throughput in votes/second.
func (b *Backend) GetThroughput() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.throughput
}

// IsEnabled returns true if the backend is initialized and ready.
func (b *Backend) IsEnabled() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.initialized
}

// GetDeviceInfo returns information about the acceleration device.
func (b *Backend) GetDeviceInfo() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if !b.initialized {
		return ""
	}
	return "luxfi/accel consensus accelerator"
}

// NewMLXBackend creates a backend with MLX-compatible settings.
// This is an alias for NewBackend maintained for backward compatibility.
func NewMLXBackend(batchSize int) (*Backend, error) {
	return NewBackend(batchSize)
}

// NewAccelBackend creates an accelerated consensus backend.
// This is an alias for NewBackend maintained for backward compatibility.
func NewAccelBackend(batchSize int) (*Backend, error) {
	return NewBackend(batchSize)
}
