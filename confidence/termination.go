// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package confidence

// TerminationCondition defines when consensus should terminate
type TerminationCondition struct {
	AlphaConfidence int
	Beta            int
}

type terminationCondition = TerminationCondition // Keep internal alias for compatibility

func newSingleTerminationCondition(alphaConfidence int, beta int) []terminationCondition {
	return []terminationCondition{
		{
			AlphaConfidence: alphaConfidence,
			Beta:            beta,
		},
	}
}