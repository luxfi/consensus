package fpc

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
)

// Selector provides phase-dependent threshold selection for FPC
type Selector struct {
	thetaMin float64
	thetaMax float64
	seed     []byte
}

// NewSelector creates a new FPC threshold selector
func NewSelector(thetaMin, thetaMax float64, seed []byte) *Selector {
	if thetaMin <= 0 || thetaMin >= 1 {
		thetaMin = 0.5
	}
	if thetaMax <= thetaMin || thetaMax > 1 {
		thetaMax = 0.8
	}
	if seed == nil {
		seed = []byte("lux-fpc-default-seed")
	}
	return &Selector{
		thetaMin: thetaMin,
		thetaMax: thetaMax,
		seed:     seed,
	}
}

// SelectThreshold picks θ ∈ [θ_min, θ_max] using PRF for phase
// Returns α = ⌈θ·k⌉ for both preference and confidence
func (s *Selector) SelectThreshold(phase uint64, k int) int {
	theta := s.computeTheta(phase)
	return int(math.Ceil(theta * float64(k)))
}

// computeTheta uses PRF to deterministically select θ for a given phase
func (s *Selector) computeTheta(phase uint64) float64 {
	// Create PRF input: seed || phase
	h := sha256.New()
	h.Write(s.seed)

	phaseBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(phaseBytes, phase)
	h.Write(phaseBytes)

	hash := h.Sum(nil)

	// Convert first 8 bytes of hash to uint64, normalize to [0,1]
	hashUint := binary.BigEndian.Uint64(hash[:8])
	normalized := float64(hashUint) / float64(^uint64(0))

	// Scale to [thetaMin, thetaMax]
	theta := s.thetaMin + normalized*(s.thetaMax-s.thetaMin)

	return theta
}

// Theta returns the raw theta value for a phase (for testing/debugging)
func (s *Selector) Theta(phase uint64) float64 {
	return s.computeTheta(phase)
}

// Range returns the configured theta range
func (s *Selector) Range() (min, max float64) {
	return s.thetaMin, s.thetaMax
}

// DefaultSelector returns a selector with default parameters
func DefaultSelector() *Selector {
	return NewSelector(0.5, 0.8, nil)
}

// SelectThreshold is a convenience function using default selector
func SelectThreshold(phase uint64, k int, thetaMin, thetaMax float64) int {
	s := NewSelector(thetaMin, thetaMax, nil)
	return s.SelectThreshold(phase, k)
}
