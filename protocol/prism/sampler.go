// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"github.com/luxfi/ids"
)


// PolySampler samples between many choices (polyadic)
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

// RecordSuccessfulPrism records a successful prism for the given choice
func (p *PolySampler) RecordSuccessfulPoll(choice ids.ID) {
	p.preference = choice
}

// MultiSampler is an alias for PolySampler
type MultiSampler = PolySampler

// NewMultiSampler creates a new multi sampler (alias for NewPolySampler)
func NewMultiSampler(choice ids.ID) MultiSampler {
	return NewPolySampler(choice)
}

// TODO: These functions need to be moved or reimplemented
// as they're trying to create poll objects that don't exist


// NewSamplerFromConfig creates a sampler from config
func NewSamplerFromConfig(cfg interface{}) Sampler {
	return NewUniformSampler()
}