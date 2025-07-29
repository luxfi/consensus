// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import "github.com/luxfi/ids"

var WaveFactory Factory = waveFactory{}

type waveFactory struct{}

func (waveFactory) NewPolyadic(params Parameters, choice ids.ID) Polyadic {
	return NewPolyadicWave(params.AlphaPreference, []TerminationCondition{
		{AlphaConfidence: params.AlphaConfidence, Beta: params.Beta},
	}, choice)
}

func (waveFactory) NewMonadic(params Parameters) Monadic {
	return NewMonadicWave(params.AlphaPreference, []TerminationCondition{
		{AlphaConfidence: params.AlphaConfidence, Beta: params.Beta},
	})
}

// NewFactory creates a new wave factory for the galaxy runtime
func NewFactory() Factory {
	return WaveFactory
}

// Wave represents the propagation stage interface for galaxy runtime
type Wave interface {
	// Propagate propagates a decision
	Propagate() error
	
	// GetPropagationCount returns the current propagation count
	GetPropagationCount() int
}