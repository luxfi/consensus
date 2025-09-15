package config

import "math"

// Consensus threshold constants (LP-CONSENSUS-69)
const (
	// ConsensusSuperMajority is the 69% threshold for Byzantine fault tolerance
	// This provides an additional 2% safety margin above traditional 67% BFT
	ConsensusSuperMajority = 0.69

	// ConsensusSimpleMajority is the simple majority threshold
	ConsensusSimpleMajority = 0.51

	// MaxByzantineWeight is the maximum tolerable Byzantine weight (31%)
	MaxByzantineWeight = 0.31
)

// CalculateQuorum calculates the minimum weight needed for 69% consensus
func CalculateQuorum(totalWeight uint64) uint64 {
	return uint64(math.Ceil(float64(totalWeight) * ConsensusSuperMajority))
}

// HasSuperMajority checks if the given weight meets the 69% threshold
func HasSuperMajority(weight, totalWeight uint64) bool {
	return float64(weight) >= float64(totalWeight)*ConsensusSuperMajority
}

// HasSimpleMajority checks if the given weight meets simple majority
func HasSimpleMajority(weight, totalWeight uint64) bool {
	return float64(weight) >= float64(totalWeight)*ConsensusSimpleMajority
}

// CanTolerateFailure checks if the Byzantine weight is within tolerance
func CanTolerateFailure(byzantineWeight, totalWeight uint64) bool {
	return float64(byzantineWeight) <= float64(totalWeight)*MaxByzantineWeight
}

// AlphaForK calculates the appropriate Alpha value for a given K
// to achieve 69% consensus threshold
func AlphaForK(k int) int {
	return int(math.Ceil(float64(k) * ConsensusSuperMajority))
}
