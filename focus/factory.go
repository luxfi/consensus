// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package focus

import "github.com/luxfi/ids"

var FocusFactory Factory = focusFactory{}

type focusFactory struct{}

func (focusFactory) NewDyadic(params Parameters, choice int) Dyadic {
	terminationConditions := []terminationCondition{
		{alphaConfidence: params.AlphaConfidence, beta: params.Beta},
	}
	df := newDyadicFocus(params.AlphaPreference, terminationConditions, choice)
	return &df
}

func (focusFactory) NewMonadic(params Parameters) Monadic {
	terminationConditions := []terminationCondition{
		{alphaConfidence: params.AlphaConfidence, beta: params.Beta},
	}
	mf := newMonadicFocus(params.AlphaPreference, terminationConditions)
	return &mf
}

func (focusFactory) NewPolyadic(params Parameters, choice ids.ID) Polyadic {
	terminationConditions := []terminationCondition{
		{alphaConfidence: params.AlphaConfidence, beta: params.Beta},
	}
	pf := newPolyadicFocus(params.AlphaPreference, terminationConditions, choice)
	return &pf
}

// NewFactory creates a new focus factory for the galaxy runtime
func NewFactory() Factory {
	return FocusFactory
}

// Focus represents the confidence aggregation stage for galaxy runtime
type Focus interface {
	// Aggregate aggregates confidence
	Aggregate() error
	
	// GetConfidenceLevel returns the current confidence level
	GetConfidenceLevel() int
}