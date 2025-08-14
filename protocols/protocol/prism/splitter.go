// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"errors"
	"fmt"

	"github.com/luxfi/consensus/utils/sampler"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

var (
	ErrInsufficientWeight = errors.New("insufficient weight for sampling")
	ErrInvalidK           = errors.New("invalid sample size k")
)

// SimpleSplitter takes the full beam of validators and splits off the sample you need
type SimpleSplitter struct {
	sampler sampler.WeightedWithoutReplacement
}

// Ensure SimpleSplitter implements Splitter
var _ Splitter = (*SimpleSplitter)(nil)

// NewSplitter creates a new splitter with the given random source
func NewSplitter(source sampler.Source) *SimpleSplitter {
	return &SimpleSplitter{
		sampler: sampler.NewWeightedWithoutReplacement(source),
	}
}

// Sample splits off k validators from the full beam, weighted by stake
// This is the prism's input face - where light enters and gets split
func (s *SimpleSplitter) Sample(validators bag.Bag[ids.NodeID], k int) ([]ids.NodeID, error) {
	if k <= 0 {
		return nil, ErrInvalidK
	}

	// Get all validators from the bag
	vdrList := validators.List()
	if len(vdrList) == 0 {
		return nil, ErrInsufficientWeight
	}

	// If k is greater than validator count, return all
	if k >= len(vdrList) {
		return vdrList, nil
	}

	// Build weighted list
	vdrs := make([]ids.NodeID, 0, len(vdrList))
	weights := make([]uint64, 0, len(vdrList))
	
	for _, vdr := range vdrList {
		weight := uint64(validators.Count(vdr))
		if weight > 0 {
			vdrs = append(vdrs, vdr)
			weights = append(weights, weight)
		}
	}

	if len(vdrs) < k {
		return nil, fmt.Errorf("%w: need %d validators, only have %d with positive weight", 
			ErrInsufficientWeight, k, len(vdrs))
	}

	// Initialize sampler with weights
	if err := s.sampler.Initialize(weights); err != nil {
		return nil, fmt.Errorf("failed to initialize sampler: %w", err)
	}
	
	// Sample k validators weighted by stake
	indices, ok := s.sampler.Sample(k)
	if !ok {
		return nil, fmt.Errorf("failed to sample validators")
	}

	// Convert indices to node IDs
	sample := make([]ids.NodeID, k)
	for i, idx := range indices {
		sample[i] = vdrs[idx]
	}

	return sample, nil
}

// // SampleWithProbability samples validators where each is included with probability p
// // This creates a variable-sized sample, useful for gossip protocols
// func (s *SimpleSplitter) SampleWithProbability(validators ValidatorSet, p float64) ([]ids.NodeID, error) {
// 	if p <= 0 || p > 1 {
// 		return nil, fmt.Errorf("probability must be in (0, 1], got %f", p)
// 	}
// 
// 	vdrMap := validators.GetValidators()
// 	if len(vdrMap) == 0 {
// 		return nil, ErrInsufficientWeight
// 	}
// 
// 	// For probability sampling, we use the geometric distribution
// 	// to determine how many validators to sample
// 	expectedSize := int(math.Ceil(float64(len(vdrMap)) * p))
// 	if expectedSize == 0 {
// 		expectedSize = 1
// 	}
// 
// 	return s.Sample(validators, expectedSize)
// }

// // SampleWeighted samples k validators with explicit weight adjustments
// // Useful for biasing samples toward specific validators
// func (s *SimpleSplitter) SampleWeighted(
// 	validators ValidatorSet,
// 	k int,
// 	weightMultipliers map[ids.NodeID]float64,
// ) ([]ids.NodeID, error) {
// 	if k <= 0 {
// 		return nil, ErrInvalidK
// 	}
// 
// 	vdrMap := validators.GetValidators()
// 	if len(vdrMap) == 0 {
// 		return nil, ErrInsufficientWeight
// 	}
// 
// 	// Build weighted list with multipliers
// 	vdrs := make([]ids.NodeID, 0, len(vdrMap))
// 	weights := make([]uint64, 0, len(vdrMap))
// 	
// 	for vdr := range vdrMap {
// 		baseWeight := validators.GetWeight(vdr)
// 		if baseWeight == 0 {
// 			continue
// 		}
// 
// 		// Apply multiplier if provided
// 		multiplier := 1.0
// 		if m, ok := weightMultipliers[vdr]; ok && m > 0 {
// 			multiplier = m
// 		}
// 
// 		adjustedWeight := uint64(float64(baseWeight) * multiplier)
// 		if adjustedWeight > 0 {
// 			vdrs = append(vdrs, vdr)
// 			weights = append(weights, adjustedWeight)
// 		}
// 	}
// 
// 	if len(vdrs) < k {
// 		return nil, fmt.Errorf("%w: need %d validators, only have %d with positive weight", 
// 			ErrInsufficientWeight, k, len(vdrs))
// 	}
// 
// 	// Initialize sampler with adjusted weights
// 	if err := s.sampler.Initialize(weights); err != nil {
// 		return nil, fmt.Errorf("failed to initialize sampler: %w", err)
// 	}
// 	
// 	// Sample with adjusted weights
// 	indices, ok := s.sampler.Sample(k)
// 	if !ok {
// 		return nil, fmt.Errorf("failed to sample validators")
// 	}
// 
// 	sample := make([]ids.NodeID, k)
// 	for i, idx := range indices {
// 		sample[i] = vdrs[idx]
// 	}
// 
// 	return sample, nil
// }
