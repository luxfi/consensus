package config

import "math"

// ConsensusSuperMajority is the 69% agreement threshold (LP-CONSENSUS-69),
// two points above the traditional 67% BFT bound.
//
// This is the float form, for sizing a sample. Deciding whether a given set of
// votes carries a quorum is exact integer arithmetic over stake — see
// quorum_threshold.go. Do not reintroduce a float predicate here: rounding
// twice, two different ways, is how a chain disagrees with itself.
const ConsensusSuperMajority = 0.69

// AlphaForK returns the votes needed out of a K-node sample to reach
// ConsensusSuperMajority, rounding up so the threshold is never understated.
func AlphaForK(k int) int {
	return int(math.Ceil(float64(k) * ConsensusSuperMajority))
}
