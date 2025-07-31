// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

// terminationCondition defines the alpha confidence and beta thresholds
// required for a threshold instance to finalize
type terminationCondition struct {
	AlphaConfidence int
	Beta            int
}

// TerminationCondition is the public alias for termination conditions
type TerminationCondition = terminationCondition

func newSingleTerminationCondition(alphaConfidence int, beta int) []terminationCondition {
	return []terminationCondition{
		{
			AlphaConfidence: alphaConfidence,
			Beta:            beta,
		},
	}
}
