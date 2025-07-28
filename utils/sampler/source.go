// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package sampler

import "math/rand"

// Source represents a source of randomness
type Source interface {
	Seed(int64)
	Uint64() uint64
}

// Rand is an alias for Source for compatibility
type Rand = Source

// source wraps a rand.Source to implement our Source interface
type source struct {
	*rand.Rand
}

// NewSource returns a new Source with the given seed
func NewSource(seed int64) Source {
	return &source{
		Rand: rand.New(rand.NewSource(seed)),
	}
}