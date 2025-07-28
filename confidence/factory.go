// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package confidence

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/poll"
)

// NewNnaryConfidence returns a new nnary confidence instance
func NewNnaryConfidence(alphaPreference, alphaConfidence, beta int, choice ids.ID) poll.Nnary {
	// We need to create a wrapper that implements poll.Nnary
	sb := newPolyConfidence(alphaPreference, newSingleTerminationCondition(alphaConfidence, beta), choice)
	return &nnaryWrapper{polyConfidence: sb}
}

// NewUnaryConfidence returns a new unary confidence instance
func NewUnaryConfidence(alphaPreference, alphaConfidence, beta int) poll.Unary {
	// We need to create a wrapper that implements poll.Unary
	sb := newUnaryConfidence(alphaPreference, newSingleTerminationCondition(alphaConfidence, beta))
	return &unaryWrapper{unaryConfidence: sb}
}

// NewBinaryConfidence returns a new binary confidence instance
func NewBinaryConfidence(alphaPreference, alphaConfidence, beta int) Confidence {
	terminationConditions := []terminationCondition{
		{
			AlphaConfidence: alphaConfidence,
			Beta:            beta,
		},
	}
	bc := newBinaryConfidence(alphaPreference, terminationConditions, 0)
	return &binaryConfidenceWrapper{binaryConfidence: bc}
}
