// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/confidence"
	"github.com/luxfi/consensus/poll"
)

// Factory creates consensus instances
type Factory struct {
	params config.Parameters
}

// NewFactory creates a new consensus factory
func NewFactory(params config.Parameters) *Factory {
	return &Factory{
		params: params,
	}
}


// NewPoll creates a new poll instance
func (f *Factory) NewPoll(numChoices int) poll.Poll {
	if numChoices == 1 {
		return poll.NewUnary(f.params.AlphaPreference)
	} else if numChoices == 2 {
		return poll.NewBinary(f.params.AlphaPreference)
	}
	return poll.NewMany(f.params.AlphaPreference)
}

// NewConfidence creates a new confidence instance
func (f *Factory) NewConfidence() confidence.Confidence {
	// Create a basic confidence instance
	// Note: confidence.NewUnaryConfidence returns poll.Unary, not confidence.Confidence
	// This is a type mismatch that needs proper implementation
	return &basicConfidence{
		alphaPreference: f.params.AlphaPreference,
		alphaConfidence: f.params.AlphaConfidence,
		beta:            f.params.Beta,
	}
}

// Parameters returns the consensus parameters
func (f *Factory) Parameters() config.Parameters {
	return f.params
}

// basicConfidence is a simple implementation of confidence.Confidence
type basicConfidence struct {
	alphaPreference int
	alphaConfidence int
	beta            int
	count           int
	consecutive     int
	finalized       bool
}

// RecordPoll records a successful poll result
func (c *basicConfidence) RecordPoll(count int) {
	if c.finalized {
		return
	}
	
	if count >= c.alphaPreference {
		c.consecutive++
		if c.consecutive >= c.beta {
			c.finalized = true
		}
	} else {
		c.consecutive = 0
	}
}

// RecordUnsuccessfulPoll records an unsuccessful poll
func (c *basicConfidence) RecordUnsuccessfulPoll() {
	if c.finalized {
		return
	}
	c.consecutive = 0
}

// Finalized returns whether confidence threshold has been reached
func (c *basicConfidence) Finalized() bool {
	return c.finalized
}