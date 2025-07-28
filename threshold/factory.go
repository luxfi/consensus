// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package threshold

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/poll"
)

// NewNnaryThreshold returns a new nnary threshold instance
func NewNnaryThreshold(alphaPreference, alphaConfidence, beta int, choice ids.ID) poll.Nnary {
	sf := NewNetwork(alphaPreference, newSingleTerminationCondition(alphaConfidence, beta), choice)
	return &nnaryWrapper{multiThreshold: &sf}
}

// NewUnaryThreshold returns a new unary threshold instance
func NewUnaryThreshold(alphaPreference, alphaConfidence, beta int) poll.Unary {
	sf := NewFlat(alphaPreference, newSingleTerminationCondition(alphaConfidence, beta))
	return &unaryWrapper{unaryThreshold: &sf}
}
