package fpc

import (
	"crypto/sha256"
	"errors"
	"math"
)

var (
	// ErrEmptySeed is returned when a nil or empty seed is provided.
	ErrEmptySeed = errors.New("fpc: seed must not be empty")
)

// DeriveEpochSeed produces a per-epoch seed from an epoch number, chain ID,
// and the hash of the last finalized block from the previous epoch.
// The prevBlockHash is only known after finalization, making the seed
// unpredictable before the epoch starts.
// seed = sha256(epoch_number || chain_id || prev_block_hash)
func DeriveEpochSeed(epochNumber uint64, chainID []byte, prevBlockHash []byte) []byte {
	h := sha256.New()
	// Identity-layer canonicalization: epoch_number is bound as a
	// fixed-width 8-byte big-endian prefix. See transcript_inputs.go
	// for the helper's home and the LP-182 layer decomposition.
	h.Write(u64BEFixed(epochNumber))
	h.Write(chainID)
	h.Write(prevBlockHash)
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
	h.Write(u64BEFixed(phase))

	hash := h.Sum(nil)

	// Convert first 8 bytes of hash to uint64, normalize to [0,1]
	hashUint := u64BEFromBytes(hash[:8])
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
