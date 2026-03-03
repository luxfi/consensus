// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"crypto/rand"
	"encoding/binary"
	"errors"

	"github.com/luxfi/consensus/core/types"
)

var (
	ErrNoValidators     = errors.New("stake weighted cut: no validators with positive weight")
	ErrInsufficientPeers = errors.New("stake weighted cut: fewer validators than requested sample size")
)

// Validator pairs a NodeID with its stake weight.
type Validator struct {
	ID     types.NodeID
	Weight uint64
}

// StakeWeightedCut implements Cut[T] with stake-proportional sampling.
// Validators are selected with probability proportional to their stake weight
// using crypto/rand for cryptographic randomness.
type StakeWeightedCut struct {
	validators  []Validator
	totalWeight uint64
}

// NewStakeWeightedCut creates a new stake-weighted cut.
// Zero-weight validators are silently excluded.
func NewStakeWeightedCut(validators []Validator) (*StakeWeightedCut, error) {
	// Filter out zero-weight validators and compute total weight.
	filtered := make([]Validator, 0, len(validators))
	var total uint64
	for _, v := range validators {
		if v.Weight == 0 {
			continue
		}
		filtered = append(filtered, v)
		total += v.Weight
	}
	if len(filtered) == 0 {
		return nil, ErrNoValidators
	}
	return &StakeWeightedCut{
		validators:  filtered,
		totalWeight: total,
	}, nil
}

// Sample returns k validators selected with probability proportional to stake.
// If there are fewer validators than k, all validators are returned.
// Uses weighted sampling without replacement.
func (c *StakeWeightedCut) Sample(k int) []types.NodeID {
	n := len(c.validators)
	if k <= 0 {
		return nil
	}
	if k >= n {
		result := make([]types.NodeID, n)
		for i, v := range c.validators {
			result[i] = v.ID
		}
		return result
	}

	// Weighted sampling without replacement.
	// Copy validators so we can remove selected ones.
	pool := make([]Validator, n)
	copy(pool, c.validators)
	remaining := c.totalWeight

	result := make([]types.NodeID, 0, k)
	for i := 0; i < k; i++ {
		// Pick a random weight target in [0, remaining).
		target := cryptoRandUint64(remaining)

		// Walk cumulative weights to find the selected validator.
		var cumulative uint64
		for j := range pool {
			cumulative += pool[j].Weight
			if target < cumulative {
				result = append(result, pool[j].ID)
				remaining -= pool[j].Weight
				// Remove selected validator by swapping with last.
				pool[j] = pool[len(pool)-1]
				pool = pool[:len(pool)-1]
				break
			}
		}
	}
	return result
}

// Luminance implements Cut interface.
func (c *StakeWeightedCut) Luminance() Luminance {
	n := len(c.validators)
	lx := float64(n)
	if n >= 100 {
		lx = 500.0
	} else if n >= 20 {
		lx = 300.0
	}
	return Luminance{
		ActivePeers: n,
		TotalPeers:  n,
		Lx:          lx,
	}
}

// cryptoRandUint64 returns a cryptographically secure random uint64 in [0, max).
func cryptoRandUint64(max uint64) uint64 {
	if max <= 1 {
		return 0
	}
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return binary.LittleEndian.Uint64(buf[:]) % max
}
