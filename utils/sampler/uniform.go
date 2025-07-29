// Copyright (C) 2019-2024, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package sampler

import (
	"math/rand"
)

// uniform implements Uniform
type uniform struct {
	count int
	rng   *rand.Rand
}

// NewUniform creates a new uniform sampler
func NewUniform() Uniform {
	return &uniform{
		rng: rand.New(rand.NewSource(rand.Int63())),
	}
}

// NewDeterministicUniform creates a new deterministic uniform sampler
func NewDeterministicUniform(seed int64) Uniform {
	return &uniform{
		rng: rand.New(rand.NewSource(seed)),
	}
}

// Initialize sets the count
func (u *uniform) Initialize(count int) error {
	u.count = count
	return nil
}

// Sample returns a sample of indices
func (u *uniform) Sample(size int) ([]int, bool) {
	if size > u.count {
		return nil, false
	}
	
	indices := make([]int, size)
	selected := make(map[int]bool)
	
	for i := 0; i < size; i++ {
		for {
			idx := u.rng.Intn(u.count)
			if !selected[idx] {
				indices[i] = idx
				selected[idx] = true
				break
			}
		}
	}
	
	return indices, true
}