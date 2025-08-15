// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"fmt"
	"time"
)

// Parameters contains consensus configuration
type Parameters struct {
	// Sampling/thresholds
	K               int // sample size
	AlphaPreference int // α_p preference threshold
	AlphaConfidence int // α_c confidence threshold
	Beta            int // β (consecutive successes)

	// Timing
	MinRoundInterval      time.Duration // Δ_min
	MaxItemProcessingTime time.Duration // Max time for item processing

	// Feature flags
	EnableFPC bool

	// Advanced parameters
	ConcurrentPolls     int
	OptimalProcessing   int
	MaxOutstandingItems int

	// Quantum parameters (optional)
	QThreshold    int           // Ringtail quorum threshold
	QuasarTimeout time.Duration // Timeout for Quasar phase
}

// Mainnet returns mainnet parameters
func Mainnet() Parameters {
	return Parameters{
		K:                     21,
		AlphaPreference:       15,
		AlphaConfidence:       18,
		Beta:                  8,
		MinRoundInterval:      50 * time.Millisecond,
		MaxItemProcessingTime: 10 * time.Second,
		EnableFPC:             true,
		ConcurrentPolls:       4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   1024,
	}
}

// Testnet returns testnet parameters
func Testnet() Parameters {
	return Parameters{
		K:                     11,
		AlphaPreference:       7,
		AlphaConfidence:       9,
		Beta:                  6,
		MinRoundInterval:      50 * time.Millisecond,
		MaxItemProcessingTime: 10 * time.Second,
		EnableFPC:             true,
		ConcurrentPolls:       4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   1024,
	}
}

// Local returns local development parameters
func Local() Parameters {
	return Parameters{
		K:                     5,
		AlphaPreference:       3,
		AlphaConfidence:       4,
		Beta:                  3,
		MinRoundInterval:      10 * time.Millisecond,
		MaxItemProcessingTime: 5 * time.Second,
		EnableFPC:             true,
		ConcurrentPolls:       2,
		OptimalProcessing:     5,
		MaxOutstandingItems:   256,
	}
}

// Getters for backward compatibility
func (p Parameters) GetK() int               { return p.K }
func (p Parameters) GetAlphaPreference() int { return p.AlphaPreference }
func (p Parameters) GetAlphaConfidence() int { return p.AlphaConfidence }
func (p Parameters) GetBeta() int            { return p.Beta }

// MinPercentConnectedHealthy computes the minimal percent of peers that must be
// connected for the node to be considered healthy.
func (p Parameters) MinPercentConnectedHealthy() float64 {
	const buffer = 0.2
	if p.K == 0 {
		return buffer
	}
	alphaRatio := float64(p.AlphaConfidence) / float64(p.K)
	return alphaRatio*(1-buffer) + buffer
}

// Valid verifies the parameter set is internally consistent and returns an
// error describing the first violation, if any.
func (p Parameters) Valid() error {
	if p.K <= 0 {
		return fmt.Errorf("k = %d: fails the condition that: 0 < k", p.K)
	}
	if p.AlphaPreference <= p.K/2 {
		return fmt.Errorf("k = %d, alphaPreference = %d: fails the condition that: k/2 < alphaPreference", p.K, p.AlphaPreference)
	}
	if p.AlphaPreference > p.K {
		return fmt.Errorf("k = %d, alphaPreference = %d: fails the condition that: alphaPreference <= k", p.K, p.AlphaPreference)
	}
	if p.AlphaConfidence < p.AlphaPreference {
		return fmt.Errorf("alphaPreference = %d, alphaConfidence = %d: fails the condition that: alphaPreference <= alphaConfidence", p.AlphaPreference, p.AlphaConfidence)
	}
	if p.AlphaConfidence > p.K {
		return fmt.Errorf("k = %d, alphaConfidence = %d: fails the condition that: alphaConfidence <= k", p.K, p.AlphaConfidence)
	}
	if p.Beta <= 0 {
		return fmt.Errorf("beta = %d: fails the condition that: 0 < beta", p.Beta)
	}
	if p.Beta > p.K {
		return fmt.Errorf("beta (%d) must be <= k (%d)", p.Beta, p.K)
	}
	if p.ConcurrentPolls <= 0 {
		return fmt.Errorf("concurrentPolls = %d: fails the condition that: 0 < concurrentPolls", p.ConcurrentPolls)
	}
	if p.ConcurrentPolls > p.Beta {
		return fmt.Errorf("concurrentPolls = %d, Beta = %d: fails the condition that: concurrentPolls <= Beta", p.ConcurrentPolls, p.Beta)
	}
	if p.OptimalProcessing <= 0 {
		return fmt.Errorf("optimalProcessing = %d: fails the condition that: 0 < optimalProcessing", p.OptimalProcessing)
	}
	if p.MaxOutstandingItems <= 0 {
		return fmt.Errorf("maxOutstandingItems = %d: fails the condition that: 0 < maxOutstandingItems", p.MaxOutstandingItems)
	}
	if p.MaxItemProcessingTime <= 0 {
		return fmt.Errorf("maxItemProcessingTime = %d: fails the condition that: 0 < maxItemProcessingTime", p.MaxItemProcessingTime)
	}
	if p.QThreshold > 0 && p.QuasarTimeout <= 0 {
		return fmt.Errorf("quasarTimeout must be positive when set")
	}
	if p.QuasarTimeout > 0 && p.QThreshold <= 0 {
		return fmt.Errorf("qThreshold must be positive when set")
	}
	if p.QThreshold > p.K {
		return fmt.Errorf("qThreshold (%d) must be <= k (%d)", p.QThreshold, p.K)
	}
	return nil
}
