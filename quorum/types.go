// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

// terminationCondition defines the alpha confidence and beta thresholds
// required for a threshold instance to finalize
type terminationCondition struct {
	alphaConfidence int
	beta            int
}

// TerminationCondition is the public alias for termination conditions
type TerminationCondition = terminationCondition

func newSingleTerminationCondition(alphaConfidence int, beta int) []terminationCondition {
	return []terminationCondition{
		{
			alphaConfidence: alphaConfidence,
			beta:            beta,
		},
	}
}
