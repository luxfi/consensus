// Copyright (C) 2025, Lux Industries, Inc. All rights reserved.
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
func (w *weightedWithoutReplacement) Sample(size int) ([]int, bool) {
	if size > len(w.weights) || w.totalWeight == 0 {
		return nil, false
	}
	
	indices := make([]int, size)
	selected := make(map[int]bool)
	remainingWeight := w.totalWeight
	
	for i := 0; i < size; i++ {
		if remainingWeight == 0 {
			return nil, false
		}
		
		r := w.source.Uint64() % remainingWeight
		cumWeight := uint64(0)
		
		for j := 0; j < len(w.weights); j++ {
			if selected[j] {
				continue
			}
			
			cumWeight += w.weights[j]
			if r < cumWeight {
				indices[i] = j
				selected[j] = true
				remainingWeight -= w.weights[j]
				break
			}
		}
	}
	
	return indices, true
}