// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package poll

import (
	"github.com/luxfi/ids"
)


// PolySampler samples between many choices (poly/nnary)
type PolySampler struct {
	preference ids.ID
}

// NewPolySampler creates a new poly sampler
func NewPolySampler(choice ids.ID) PolySampler {
	return PolySampler{preference: choice}
}

// Preference returns the current preference
func (p PolySampler) Preference() ids.ID {
	return p.preference
}

// RecordSuccessfulPoll records a successful poll for the given choice
func (p *PolySampler) RecordSuccessfulPoll(choice ids.ID) {
	p.preference = choice
}

// MultiSampler is an alias for PolySampler
type MultiSampler = PolySampler

// NewMultiSampler creates a new multi sampler (alias for NewPolySampler)
func NewMultiSampler(choice ids.ID) MultiSampler {
	return NewPolySampler(choice)
}

// NewUnary creates a new unary poll (stub)
func NewUnary(alphaPreference int) Poll {
	// TODO: Implement proper unary poll
	return &poll{
		votes: make(map[ids.ID]int),
	}
}

// NewBinary creates a new binary poll (stub)
func NewBinary(alphaPreference int) Poll {
	// TODO: Implement proper binary poll
	return &poll{
		votes: make(map[ids.ID]int),
	}
}

// NewMany creates a new many-choice poll (stub)
func NewMany(alphaPreference int) Poll {
	// TODO: Implement proper many-choice poll
	return &poll{
		votes: make(map[ids.ID]int),
	}
}


// NewSamplerFromConfig creates a sampler from config
func NewSamplerFromConfig(cfg interface{}) Sampler {
	return NewUniformSampler()
}