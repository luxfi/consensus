// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"gonum.org/v1/gonum/mathext/prng"

	"github.com/luxfi/consensus/utils/sampler"
)

// mt19937Wrapper wraps gonum's MT19937 to implement sampler.Source
type mt19937Wrapper struct {
	mt *prng.MT19937
}

// NewMT19937Source creates a new MT19937 source that implements sampler.Source
func NewMT19937Source() sampler.Source {
	return &mt19937Wrapper{
		mt: prng.NewMT19937(),
	}
}

// Seed seeds the random number generator
func (m *mt19937Wrapper) Seed(seed int64) {
	m.mt.Seed(uint64(seed))
}

// Uint64 returns a random uint64
func (m *mt19937Wrapper) Uint64() uint64 {
	return m.mt.Uint64()
}