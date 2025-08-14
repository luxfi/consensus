// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"math/rand"
	"time"
	
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

// Sample represents a sample result from the validator set
type Sample struct {
	nodes []ids.NodeID
}

// List returns the list of sampled node IDs
func (s Sample) List() []ids.NodeID {
	return s.nodes
}

// Sampler is an interface for sampling consensus participants
type Sampler interface {
	// Sample returns a sample of validators
	Sample(validators bag.Bag[ids.NodeID], size int) (Sample, error)
	
	// Reset resets the sampler state
	Reset()
}

// UniformSampler samples uniformly from a set of validators
type UniformSampler struct {
	rng    *rand.Rand
}

// NewUniformSampler creates a new uniform sampler
func NewUniformSampler() Sampler {
	return &UniformSampler{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Sample returns a random sample of the specified size
func (s *UniformSampler) Sample(validators bag.Bag[ids.NodeID], size int) (Sample, error) {
	// Get all unique validators
	uniqueNodes := validators.List()
	n := len(uniqueNodes)
	
	if size > n {
		size = n
	}
	
	// Fisher-Yates shuffle for the first 'size' elements
	for i := 0; i < size; i++ {
		j := s.rng.Intn(n - i) + i
		uniqueNodes[i], uniqueNodes[j] = uniqueNodes[j], uniqueNodes[i]
	}
	
	return Sample{nodes: uniqueNodes[:size]}, nil
}

// Reset resets the sampler
func (s *UniformSampler) Reset() {
	// Reset random seed
	s.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
}

// BinarySampler manages binary consensus sampling for light protocols
type BinarySampler struct {
	preference int
	count      [2]int
}

// NewBinarySampler creates a new binary sampler with parameters
// k is the sample size, alphaPreference and alphaConfidence are thresholds
func NewBinarySampler(k, alphaPreference, alphaConfidence int) BinarySampler {
	// For binary consensus, we start with preference 0
	return BinarySampler{
		preference: 0,
	}
}

// Preference returns the current preference
func (bs *BinarySampler) Preference() int {
	return bs.preference
}

// RecordSuccessfulPoll records a successful poll result
func (bs *BinarySampler) RecordSuccessfulPoll(choice int) {
	bs.count[choice]++
	// Update preference to choice with more successful polls
	if bs.count[choice] > bs.count[1-choice] {
		bs.preference = choice
	}
}