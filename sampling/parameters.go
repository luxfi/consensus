// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package sampling

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrParametersInvalid          = errors.New("invalid parameters")
	ErrInvalidK                   = errors.New("invalid K value")
	ErrInvalidAlpha               = errors.New("invalid alpha values")
	ErrInvalidBeta                = errors.New("invalid beta value")
	ErrInvalidConcurrentRepolls   = errors.New("invalid concurrent repolls")
	ErrInvalidOptimalProcessing   = errors.New("invalid optimal processing")
	ErrInvalidMaxOutstandingItems = errors.New("invalid max outstanding items")
	
	// DefaultParameters provides default consensus parameters
	// DEPRECATED: Use config.MainnetParameters, config.TestnetParameters, or config.LocalParameters instead
	DefaultParameters = Parameters{
		K:                     21,  // Updated to match mainnet
		AlphaPreference:       13,  // Updated to match mainnet
		AlphaConfidence:       18,  // Updated to match mainnet
		Beta:                  8,   // Updated to match mainnet
		ConcurrentRepolls:     8,   // Updated to match mainnet
		OptimalProcessing:     10,  // Updated to match mainnet
		MaxOutstandingItems:   369, // Updated to match mainnet
		MaxItemProcessingTime: 963 * time.Millisecond, // 0.963s for mainnet consensus
	}
)

// Parameters defines the consensus parameters for sampling
type Parameters struct {
	// K is the initial sample size
	K int `json:"k" yaml:"k"`

	// AlphaPreference is the threshold for preference change
	AlphaPreference int `json:"alphaPreference" yaml:"alphaPreference"`

	// AlphaConfidence is the threshold for confidence increase
	AlphaConfidence int `json:"alphaConfidence" yaml:"alphaConfidence"`

	// Beta is the number of consecutive successful queries required
	Beta int `json:"beta" yaml:"beta"`

	// ConcurrentRepolls is the number of concurrent repolls allowed
	ConcurrentRepolls int `json:"concurrentRepolls" yaml:"concurrentRepolls"`

	// OptimalProcessing is the optimal number of items in processing
	OptimalProcessing int `json:"optimalProcessing" yaml:"optimalProcessing"`

	// MaxOutstandingItems is the maximum number of outstanding items
	MaxOutstandingItems int `json:"maxOutstandingItems" yaml:"maxOutstandingItems"`

	// MaxItemProcessingTime is the maximum time an item can be processing
	MaxItemProcessingTime time.Duration `json:"maxItemProcessingTime" yaml:"maxItemProcessingTime"`
}

// Verify checks if the parameters are valid
func (p Parameters) Verify() error {
	if p.K <= 0 {
		return fmt.Errorf("%w: k=%d", ErrInvalidK, p.K)
	}
	if p.AlphaPreference <= 0 || p.AlphaPreference > p.K {
		return fmt.Errorf("%w: alphaPreference=%d, k=%d", ErrInvalidAlpha, p.AlphaPreference, p.K)
	}
	if p.AlphaConfidence <= 0 || p.AlphaConfidence > p.K {
		return fmt.Errorf("%w: alphaConfidence=%d, k=%d", ErrInvalidAlpha, p.AlphaConfidence, p.K)
	}
	if p.Beta <= 0 {
		return fmt.Errorf("%w: beta=%d", ErrInvalidBeta, p.Beta)
	}
	if p.ConcurrentRepolls <= 0 {
		return fmt.Errorf("%w: concurrentRepolls=%d", ErrInvalidConcurrentRepolls, p.ConcurrentRepolls)
	}
	if p.OptimalProcessing <= 0 {
		return fmt.Errorf("%w: optimalProcessing=%d", ErrInvalidOptimalProcessing, p.OptimalProcessing)
	}
	if p.MaxOutstandingItems <= 0 {
		return fmt.Errorf("%w: maxOutstandingItems=%d", ErrInvalidMaxOutstandingItems, p.MaxOutstandingItems)
	}
	return nil
}

// DefaultParametersFunc returns default consensus parameters
// DEPRECATED: Use DefaultParameters variable or config.MainnetParameters instead
func DefaultParametersFunc() Parameters {
	return DefaultParameters
}

// MinPercentConnectedHealthy returns the minimum percentage of validators
// that must be connected for the network to be considered healthy
func (p Parameters) MinPercentConnectedHealthy() float64 {
	// For k=1 consensus, all validators (100%) must be connected
	if p.K == 1 {
		return 1.0
	}
	// For normal consensus, use a threshold based on alpha
	return float64(p.AlphaPreference) / float64(p.K)
}
