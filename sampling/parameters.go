// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package sampling

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidK                   = errors.New("invalid K value")
	ErrInvalidAlpha               = errors.New("invalid alpha values")
	ErrInvalidBeta                = errors.New("invalid beta value")
	ErrInvalidConcurrentRepolls   = errors.New("invalid concurrent repolls")
	ErrInvalidOptimalProcessing   = errors.New("invalid optimal processing")
	ErrInvalidMaxOutstandingItems = errors.New("invalid max outstanding items")
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

// DefaultParameters returns default consensus parameters
func DefaultParameters() Parameters {
	return Parameters{
		K:                     20,
		AlphaPreference:       15,
		AlphaConfidence:       15,
		Beta:                  20,
		ConcurrentRepolls:     4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   1024,
		MaxItemProcessingTime: 2 * time.Minute,
	}
}
