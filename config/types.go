// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"errors"
	"fmt"
	"time"
)

// Parameters represents consensus parameters
type Parameters struct {
	// Core parameters
	K                     int           `json:"k" yaml:"k"`
	AlphaPreference       int           `json:"alphaPreference" yaml:"alphaPreference"`
	AlphaConfidence       int           `json:"alphaConfidence" yaml:"alphaConfidence"`
	Beta                  int           `json:"beta" yaml:"beta"`
	
	// Quantum parameters
	QThreshold            int           `json:"qThreshold" yaml:"qThreshold"`
	QuasarTimeout         time.Duration `json:"quasarTimeout" yaml:"quasarTimeout"`
	
	// Advanced parameters
	ConcurrentReprisms     int           `json:"concurrentReprisms" yaml:"concurrentReprisms"`
	OptimalProcessing     int           `json:"optimalProcessing" yaml:"optimalProcessing"`
	MaxOutstandingItems   int           `json:"maxOutstandingItems" yaml:"maxOutstandingItems"`
	MaxItemProcessingTime time.Duration `json:"maxItemProcessingTime" yaml:"maxItemProcessingTime"`
	MinRoundInterval      time.Duration `json:"minRoundInterval" yaml:"minRoundInterval"`
}

// GetK returns the sample size
func (p Parameters) GetK() int {
	return p.K
}

// GetAlphaPreference returns the preference threshold
func (p Parameters) GetAlphaPreference() int {
	return p.AlphaPreference
}

// GetAlphaConfidence returns the confidence threshold
func (p Parameters) GetAlphaConfidence() int {
	return p.AlphaConfidence
}

// GetBeta returns the finalization threshold
func (p Parameters) GetBeta() int {
	return p.Beta
}

// MinPercentConnectedHealthy returns the minimum percentage of stake that must be connected
// for the network to be considered healthy
func (p Parameters) MinPercentConnectedHealthy() float64 {
	// The minimum percentage is based on the ratio of AlphaConfidence to K
	// with an additional buffer for safety
	// Formula: (AlphaConfidence/K) * 0.8 + 0.2
	const scaleFactor = 0.8
	const minBase = 0.2
	baseRatio := float64(p.AlphaConfidence) / float64(p.K)
	return baseRatio*scaleFactor + minBase
}

// Valid returns an error if the parameters are invalid
func (p Parameters) Valid() error {
	switch {
	case p.K <= 0:
		return ErrKTooLow
	case p.AlphaPreference <= p.K/2:
		return ErrAlphaPreferenceTooLow
	case p.AlphaPreference > p.K:
		return ErrAlphaPreferenceTooHigh
	case p.AlphaConfidence < p.AlphaPreference:
		return ErrAlphaConfidenceTooSmall
	case p.AlphaConfidence > p.K:
		return fmt.Errorf("alphaConfidence (%d) must be <= k (%d)", p.AlphaConfidence, p.K)
	case p.Beta <= 0:
		return ErrBetaTooLow
	case p.Beta > p.K:
		return fmt.Errorf("beta (%d) must be <= k (%d)", p.Beta, p.K)
	case p.ConcurrentReprisms <= 0:
		return ErrConcurrentReprismsTooLow
	case p.OptimalProcessing <= 0:
		return ErrOptimalProcessingTooLow
	case p.MaxOutstandingItems <= 0:
		return ErrMaxOutstandingItemsTooLow
	case p.MaxItemProcessingTime <= 0:
		return ErrMaxItemProcessingTimeTooLow
	}
	
	// Quantum parameter validation - only if they are being used (non-zero)
	if p.QThreshold != 0 || p.QuasarTimeout != 0 {
		switch {
		case p.QThreshold <= 0:
			return errors.New("qThreshold must be positive when set")
		case p.QThreshold > p.K:
			return fmt.Errorf("qThreshold (%d) must be <= k (%d)", p.QThreshold, p.K)
		case p.QuasarTimeout <= 0:
			return errors.New("quasarTimeout must be positive when set")
		}
	}
	
	return nil
}