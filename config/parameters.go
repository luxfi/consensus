// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import "time"

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