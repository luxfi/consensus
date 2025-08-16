// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import "math/rand"

// Sampler provides sampling operations for consensus
type Sampler interface {
	// Sample returns k random elements
	Sample(k int) []interface{}

	// Add adds an element to the sampling pool
	Add(element interface{})

	// Remove removes an element from the sampling pool
	Remove(element interface{})

	// Size returns the current pool size
	Size() int
}

// UniformSampler implements uniform random sampling
type UniformSampler struct {
	pool []interface{}
	rng  *rand.Rand
}

// NewUniformSampler creates a new uniform sampler
func NewUniformSampler() *UniformSampler {
	return &UniformSampler{
		pool: make([]interface{}, 0),
		rng:  rand.New(rand.NewSource(42)),
	}
}

// Sample returns k random elements
func (s *UniformSampler) Sample(k int) []interface{} {
	if k > len(s.pool) {
		k = len(s.pool)
	}

	// Fisher-Yates shuffle for sampling
	result := make([]interface{}, k)
	perm := s.rng.Perm(len(s.pool))
	for i := 0; i < k; i++ {
		result[i] = s.pool[perm[i]]
	}
	return result
}

// Add adds an element to the pool
func (s *UniformSampler) Add(element interface{}) {
	s.pool = append(s.pool, element)
}

// Remove removes an element from the pool
func (s *UniformSampler) Remove(element interface{}) {
	for i, e := range s.pool {
		if e == element {
			s.pool = append(s.pool[:i], s.pool[i+1:]...)
			return
		}
	}
}

// Size returns the pool size
func (s *UniformSampler) Size() int {
	return len(s.pool)
}
