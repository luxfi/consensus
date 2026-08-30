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

// seedDomain is the label the epoch seed is taken under, so this digest cannot
// collide with any other digest the protocol takes over similar bytes.
const seedDomain = "lux.consensus.fpc.seed"

// EpochSeedPreimage returns the exact bytes DeriveEpochSeed hashes.
//
//	domain || be64(epoch) || be64(len(chainID)) || chainID
//	       || be64(len(prevBlockHash)) || prevBlockHash
//
// Every variable-length field is written at its length, so the preimage can be
// read back apart into exactly the three inputs that produced it and no others.
// Written end to end they could not be: a chain calling itself lux-mainnet||H
// derives, binding no parent at all, the seed lux-mainnet derives at parent H.
// That trades the parent away for a name the chain picks itself, and the parent
// is the input no one can know in advance — which is the whole reason it is an
// input. Lengths close that; the domain keeps the digest to itself.
//
// The corpus records these bytes, so a port is held to what it hashed and not
// only to what came out: two implementations can agree on every digest in the
// corpus and still disagree on the first case that is not in it.
func EpochSeedPreimage(epochNumber uint64, chainID []byte, prevBlockHash []byte) []byte {
	out := make([]byte, 0, len(seedDomain)+24+len(chainID)+len(prevBlockHash))
	out = append(out, seedDomain...)
	out = binary.BigEndian.AppendUint64(out, epochNumber)
	out = binary.BigEndian.AppendUint64(out, uint64(len(chainID)))
	out = append(out, chainID...)
	out = binary.BigEndian.AppendUint64(out, uint64(len(prevBlockHash)))
	out = append(out, prevBlockHash...)
	return out
}

// DeriveEpochSeed produces a per-epoch seed from an epoch number, chain ID, and
// the hash of the last finalized block from the previous epoch.
//
//	seed = sha256(EpochSeedPreimage(epoch, chainID, prevBlockHash))
//
// prevBlockHash is only known once the previous epoch has finalized, so no
// party holds the next epoch's thresholds while the current one is still open.
func DeriveEpochSeed(epochNumber uint64, chainID []byte, prevBlockHash []byte) []byte {
	sum := sha256.Sum256(EpochSeedPreimage(epochNumber, chainID, prevBlockHash))
	return sum[:]
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
