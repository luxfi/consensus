// Copyright (C) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package sampler

// Sampler is an interface for sampling elements
type Sampler interface {
	Sample(size int) ([]int, bool)
}

// Weighted is a sampler for sampling without replacement
type Weighted interface {
	Sampler
	Initialize(weights []uint64) error
}

// WeightedWithoutReplacement is the interface for weighted sampling without replacement
type WeightedWithoutReplacement interface {
	Weighted
}

// Uniform is the interface for uniform sampling
type Uniform interface {
	Sampler
	Initialize(count int) error
}

// Source is a source of randomness
type Source interface {
	Uint64() uint64
}