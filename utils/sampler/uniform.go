// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package sampler

import (
	"math/rand"
	"time"
)

// Uniform samples values without replacement in the provided range
type Uniform interface {
	Initialize(sampleRange uint64)
	// Sample returns length numbers in the range [0,sampleRange). If there
	// aren't enough numbers in the range, false is returned. If length is
	// negative the implementation may panic.
	Sample(length int) ([]uint64, bool)

	Next() (uint64, bool)
	Reset()
}

// uniformReplacer implements Uniform sampling without replacement
type uniformReplacer struct {
	rng         *rand.Rand
	sampleRange uint64
	sampled     map[uint64]uint64
	count       uint64
}

// NewUniform returns a new sampler
func NewUniform() Uniform {
	return &uniformReplacer{
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
		sampled: make(map[uint64]uint64),
	}
}

// NewDeterministicUniform returns a new sampler with deterministic randomness
func NewDeterministicUniform(seed int64) Uniform {
	return &uniformReplacer{
		rng:     rand.New(rand.NewSource(seed)),
		sampled: make(map[uint64]uint64),
	}
}

func (s *uniformReplacer) Initialize(sampleRange uint64) {
	s.sampleRange = sampleRange
	s.sampled = make(map[uint64]uint64)
	s.count = 0
}

func (s *uniformReplacer) Sample(length int) ([]uint64, bool) {
	if length < 0 {
		panic("negative sample length")
	}
	if uint64(length) > s.sampleRange-s.count {
		return nil, false
	}

	results := make([]uint64, length)
	for i := 0; i < length; i++ {
		val, ok := s.Next()
		if !ok {
			return nil, false
		}
		results[i] = val
	}
	return results, true
}

func (s *uniformReplacer) Next() (uint64, bool) {
	if s.count >= s.sampleRange {
		return 0, false
	}

	// Use Knuth's algorithm for sampling without replacement
	remaining := s.sampleRange - s.count
	index := uint64(s.rng.Int63n(int64(remaining)))
	
	// Map the index to account for already sampled values
	result := index
	if replacement, exists := s.sampled[index]; exists {
		result = replacement
	}

	// Store what index should map to if selected again
	lastIndex := s.sampleRange - s.count - 1
	if lastIndex != index {
		if replacement, exists := s.sampled[lastIndex]; exists {
			s.sampled[index] = replacement
		} else {
			s.sampled[index] = lastIndex
		}
	}

	s.count++
	return result, true
}

func (s *uniformReplacer) Reset() {
	s.sampled = make(map[uint64]uint64)
	s.count = 0
}

// Weighted defines the interface for weighted sampling
type Weighted interface {
	Initialize(weights []uint64) error
	Sample(n int) ([]int, error)
}

// WeightedWithoutReplacement implements weighted sampling without replacement
type WeightedWithoutReplacement struct {
	weights []uint64
	indices []int
	rng     Rand
}

// NewWeightedWithoutReplacement returns a new weighted sampler
func NewWeightedWithoutReplacement(rng Rand) Weighted {
	return &WeightedWithoutReplacement{
		rng: rng,
	}
}

// Initialize sets the weights
func (s *WeightedWithoutReplacement) Initialize(weights []uint64) error {
	s.weights = make([]uint64, len(weights))
	copy(s.weights, weights)
	
	s.indices = make([]int, len(weights))
	for i := range s.indices {
		s.indices[i] = i
	}
	
	return nil
}

// Sample returns n samples
func (s *WeightedWithoutReplacement) Sample(n int) ([]int, error) {
	if n > len(s.weights) {
		n = len(s.weights)
	}
	
	// Simple weighted sampling: pick randomly based on weights
	results := make([]int, 0, n)
	used := make(map[int]bool)
	
	for len(results) < n {
		// Calculate total weight of remaining items
		var totalWeight uint64
		for i, w := range s.weights {
			if !used[i] {
				totalWeight += w
			}
		}
		
		if totalWeight == 0 {
			break
		}
		
		// Pick a random weight
		r := s.rng.Uint64() % totalWeight
		
		// Find the index
		var cumWeight uint64
		for i, w := range s.weights {
			if used[i] {
				continue
			}
			cumWeight += w
			if cumWeight > r {
				results = append(results, i)
				used[i] = true
				break
			}
		}
	}
	
	return results, nil
}
