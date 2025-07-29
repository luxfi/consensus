// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidK is returned when K is invalid.
	ErrInvalidK = errors.New("k must be positive")
	
	// ErrInvalidAlpha is returned when alpha values are invalid.
	ErrInvalidAlpha = errors.New("alpha values must be positive and alpha preference <= alpha confidence")
	
	// ErrInvalidBeta is returned when beta is invalid.
	ErrInvalidBeta = errors.New("beta must be positive")
	
	// ErrInvalidBetaVirtuous is returned when beta virtuous values are invalid.
	ErrInvalidBetaVirtuous = errors.New("beta virtuous values must be positive and <= beta rogue")
	
	// ErrInvalidConcurrentRepolls is returned when concurrent repolls is invalid.
	ErrInvalidConcurrentRepolls = errors.New("concurrent repolls must be positive")
)

// params implements the Parameters interface.
type params struct {
	k                    int
	alphaPreference      int
	alphaConfidence      int
	beta                 int
	betaVirtuous         int
	betaRogue            int
	concurrentRepolls    int
	optimalProcessing    int
	maxOutstandingItems  int
	maxItemProcessingTime int64
}

// DefaultParameters returns the default consensus parameters.
var DefaultParameters = &params{
	k:                    20,
	alphaPreference:      15,
	alphaConfidence:      15,
	beta:                 20,
	betaVirtuous:         15,
	betaRogue:            20,
	concurrentRepolls:    4,
	optimalProcessing:    10,
	maxOutstandingItems:  256,
	maxItemProcessingTime: 30 * 1000, // 30 seconds in milliseconds
}

// TestParameters returns parameters suitable for testing.
var TestParameters = &params{
	k:                    2,
	alphaPreference:      2,
	alphaConfidence:      2,
	beta:                 1,
	betaVirtuous:         1,
	betaRogue:            2,
	concurrentRepolls:    1,
	optimalProcessing:    1,
	maxOutstandingItems:  16,
	maxItemProcessingTime: 10 * 1000, // 10 seconds in milliseconds
}

// NewParameters creates new consensus parameters.
func NewParameters(k, alphaPreference, alphaConfidence, beta int) Parameters {
	return &params{
		k:                    k,
		alphaPreference:      alphaPreference,
		alphaConfidence:      alphaConfidence,
		beta:                 beta,
		betaVirtuous:         beta,
		betaRogue:            beta,
		concurrentRepolls:    4,
		optimalProcessing:    10,
		maxOutstandingItems:  256,
		maxItemProcessingTime: 30 * 1000,
	}
}

// K returns the sample size.
func (p *params) K() int {
	return p.k
}

// AlphaPreference returns the preference threshold.
func (p *params) AlphaPreference() int {
	return p.alphaPreference
}

// AlphaConfidence returns the confidence threshold.
func (p *params) AlphaConfidence() int {
	return p.alphaConfidence
}

// Beta returns the finalization threshold.
func (p *params) Beta() int {
	return p.beta
}

// Valid returns an error if the parameters are invalid.
func (p *params) Valid() error {
	switch {
	case p.k <= 0:
		return ErrInvalidK
	case p.alphaPreference <= 0 || p.alphaConfidence <= 0:
		return ErrInvalidAlpha
	case p.alphaPreference > p.alphaConfidence:
		return ErrInvalidAlpha
	case p.beta <= 0:
		return ErrInvalidBeta
	case p.betaVirtuous <= 0 || p.betaRogue <= 0:
		return ErrInvalidBetaVirtuous
	case p.betaVirtuous > p.betaRogue:
		return ErrInvalidBetaVirtuous
	case p.concurrentRepolls <= 0:
		return ErrInvalidConcurrentRepolls
	default:
		return nil
	}
}

// String returns a string representation of the parameters.
func (p *params) String() string {
	return fmt.Sprintf(
		"Parameters{K=%d, AlphaPreference=%d, AlphaConfidence=%d, Beta=%d}",
		p.k, p.alphaPreference, p.alphaConfidence, p.beta,
	)
}