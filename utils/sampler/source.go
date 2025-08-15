// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package sampler

import (
	"math/rand"
	"sync"
)

// source implements Source
type source struct {
	rng *rand.Rand
	mu  sync.Mutex
}

// NewSource creates a new source of randomness
func NewSource(seed int64) Source {
	return &source{
		rng: rand.New(rand.NewSource(seed)),
	}
}

// Uint64 returns a random uint64
func (s *source) Uint64() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return uint64(s.rng.Int63())<<1 | uint64(s.rng.Int63n(2))
}
