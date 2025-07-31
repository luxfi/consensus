// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package sampler

import (
	"errors"
	"math"
)

var (
	ErrOutOfRange      = errors.New("out of range")
	ErrInsufficientWeight = errors.New("insufficient weight")
)

// uniformSource wraps a Source to provide uniform sampling over a range
type uniformSource struct {
	max    uint64
	source Source
}

// NewUniformSource creates a new uniform source over [0, max)
func NewUniformSource(max uint64, source Source) *uniformSource {
	return &uniformSource{
		max:    max,
		source: source,
	}
}

// Uint64 returns a uniformly distributed value in [0, max)
func (u *uniformSource) Uint64() uint64 {
	return u.source.Uint64() % u.max
}

// weightedWithoutReplacement implements WeightedWithoutReplacement
type weightedWithoutReplacement struct {
	weights []uint64
	totalWeight uint64
	source  Source
}

// NewWeightedWithoutReplacement creates a new weighted sampler without replacement
func NewWeightedWithoutReplacement(source ...Source) WeightedWithoutReplacement {
	var s Source
	if len(source) > 0 {
		s = source[0]
	} else {
		s = NewSource(0)
	}
	return &weightedWithoutReplacement{
		source: s,
	}
}

// Initialize sets the weights
func (w *weightedWithoutReplacement) Initialize(weights []uint64) error {
	w.weights = make([]uint64, len(weights))
	copy(w.weights, weights)
	
	w.totalWeight = 0
	for _, weight := range weights {
		if weight > math.MaxUint64 - w.totalWeight {
			return ErrOutOfRange
		}
		w.totalWeight += weight
	}
	
	return nil
}

// Sample returns a sample of indices
// This samples weight without replacement, but can return duplicate indices
func (w *weightedWithoutReplacement) Sample(size int) ([]int, bool) {
	if size == 0 {
		return []int{}, true
	}
	if w.totalWeight == 0 || uint64(size) > w.totalWeight {
		return nil, false
	}
	
	indices := make([]int, size)
	usedWeights := make(map[uint64]bool)
	
	for i := 0; i < size; i++ {
		var weight uint64
		// Keep sampling until we get an unused weight
		for {
			weight = w.source.Uint64() % w.totalWeight
			if !usedWeights[weight] {
				usedWeights[weight] = true
				break
			}
		}
		
		// Find which index this weight corresponds to
		cumWeight := uint64(0)
		for j := 0; j < len(w.weights); j++ {
			cumWeight += w.weights[j]
			if weight < cumWeight {
				indices[i] = j
				break
			}
		}
	}
	
	return indices, true
}