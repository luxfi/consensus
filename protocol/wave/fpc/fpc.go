package fpc

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
)

var (
	// ErrEmptySeed is returned when a nil or empty seed is provided.
	ErrEmptySeed = errors.New("fpc: seed must not be empty")
)

// DeriveEpochSeed produces a per-epoch seed from an epoch number and chain ID.
// seed = sha256(epoch_number || chain_id)
func DeriveEpochSeed(epochNumber uint64, chainID []byte) []byte {
	h := sha256.New()
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], epochNumber)
	h.Write(buf[:])
	h.Write(chainID)
	return h.Sum(nil)
}

// Selector provides phase-dependent threshold selection for FPC
type Selector struct {
	thetaMin float64
	thetaMax float64
	seed     []byte
}

// NewSelector creates a new FPC threshold selector.
// seed must be non-empty; use DeriveEpochSeed to produce one.
func NewSelector(thetaMin, thetaMax float64, seed []byte) (*Selector, error) {
	if len(seed) == 0 {
		return nil, ErrEmptySeed
	}
	if thetaMin <= 0 || thetaMin >= 1 {
		thetaMin = 0.5
	}
	if thetaMax <= thetaMin || thetaMax > 1 {
		thetaMax = 0.8
	}
	return &Selector{
		thetaMin: thetaMin,
		thetaMax: thetaMax,
		seed:     seed,
	}, nil
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
