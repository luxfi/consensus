// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build !cgo || !accel

package ai

import (
	"fmt"
	"sync"
)

// Backend provides a stub consensus backend when luxfi/accel is not available.
// This stub allows the package to compile without CGO dependencies.
type Backend struct {
	mu          sync.RWMutex
	batchSize   int
	throughput  float64
	initialized bool
}

// NewBackend creates a stub consensus backend.
// Returns an error indicating acceleration is not available.
func NewBackend(batchSize int) (*Backend, error) {
	return &Backend{
		batchSize:   batchSize,
		initialized: false,
	}, nil
}

// ProcessVotesBatch is a stub that returns an error when accel is not available.
func (b *Backend) ProcessVotesBatch(votes []Vote) (int, error) {
	if len(votes) == 0 {
		return 0, nil
	}
	// Stub implementation: just count votes without GPU acceleration
	return len(votes), nil
}

// ComputeQuorum is a stub that computes quorum without GPU acceleration.
func (b *Backend) ComputeQuorum(votes []Vote, validators []ValidatorInfo, threshold float64) (*QuorumResult, error) {
	if len(validators) == 0 {
		return nil, fmt.Errorf("no validators provided")
	}

	// Create a map of validator weights
	weights := make(map[[32]byte]uint64)
	var totalWeight uint64
	for _, v := range validators {
		weights[v.ValidatorID] = v.Weight
		totalWeight += v.Weight
	}

	// Count voted weight
	var votedWeight uint64
	votedValidators := make(map[[32]byte]bool)
	for _, vote := range votes {
		if vote.IsPreference && !votedValidators[vote.VoterID] {
			if weight, ok := weights[vote.VoterID]; ok {
				votedWeight += weight
				votedValidators[vote.VoterID] = true
			}
		}
	}

	quorumWeight := uint64(float64(totalWeight) * threshold)
	hasQuorum := votedWeight >= quorumWeight

	return &QuorumResult{
		HasQuorum:    hasQuorum,
		TotalWeight:  totalWeight,
		VotedWeight:  votedWeight,
		QuorumWeight: quorumWeight,
	}, nil
}

// GetThroughput returns zero when acceleration is not available.
func (b *Backend) GetThroughput() float64 {
	return 0
}

// IsEnabled returns false when acceleration is not available.
func (b *Backend) IsEnabled() bool {
	return false
}

// GetDeviceInfo returns empty string when acceleration is not available.
func (b *Backend) GetDeviceInfo() string {
	return ""
}

// NewMLXBackend creates a stub backend when MLX is not available.
func NewMLXBackend(batchSize int) (*Backend, error) {
	return NewBackend(batchSize)
}

// NewAccelBackend creates a stub backend when acceleration is not available.
func NewAccelBackend(batchSize int) (*Backend, error) {
	return NewBackend(batchSize)
}
