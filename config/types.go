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
	ConcurrentPolls       int           `json:"concurrentPolls" yaml:"concurrentPolls"`
	OptimalProcessing     int           `json:"optimalProcessing" yaml:"optimalProcessing"`
	MaxOutstandingItems   int           `json:"maxOutstandingItems" yaml:"maxOutstandingItems"`
	MaxItemProcessingTime time.Duration `json:"maxItemProcessingTime" yaml:"maxItemProcessingTime"`
	MinRoundInterval      time.Duration `json:"minRoundInterval" yaml:"minRoundInterval"`
	
	// WaveFPC parameters (optional fast-path certification)
	EnableFPC             bool          `json:"enableFPC" yaml:"enableFPC"`
	FPCVoteLimit          int           `json:"fpcVoteLimit" yaml:"fpcVoteLimit"`
	FPCVotePrefix         []byte        `json:"fpcVotePrefix" yaml:"fpcVotePrefix"`
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
		return fmt.Errorf("k = %d: fails the condition that: 0 < k", p.K)
	case p.AlphaPreference <= p.K/2:
		return fmt.Errorf("k = %d, alphaPreference = %d: fails the condition that: k/2 < alphaPreference", p.K, p.AlphaPreference)
	case p.AlphaPreference > p.K:
		return fmt.Errorf("k = %d, alphaPreference = %d: fails the condition that: alphaPreference <= k", p.K, p.AlphaPreference)
	case p.AlphaConfidence < p.AlphaPreference:
		return fmt.Errorf("alphaPreference = %d, alphaConfidence = %d: fails the condition that: alphaPreference <= alphaConfidence", p.AlphaPreference, p.AlphaConfidence)
	case p.AlphaConfidence > p.K:
		return fmt.Errorf("k = %d, alphaConfidence = %d: fails the condition that: alphaConfidence <= k", p.K, p.AlphaConfidence)
	case p.Beta <= 0:
		return fmt.Errorf("beta = %d: fails the condition that: 0 < beta", p.Beta)
	case p.Beta > p.K:
		return fmt.Errorf("beta (%d) must be <= k (%d)", p.Beta, p.K)
	case p.ConcurrentPolls <= 0:
		return fmt.Errorf("concurrentPolls = %d: fails the condition that: 0 < concurrentPolls", p.ConcurrentPolls)
	case p.ConcurrentPolls > p.Beta:
		return fmt.Errorf("concurrentPolls = %d, beta = %d: fails the condition that: concurrentPolls <= beta", p.ConcurrentPolls, p.Beta)
	case p.OptimalProcessing <= 0:
		return fmt.Errorf("optimalProcessing = %d: fails the condition that: 0 < optimalProcessing", p.OptimalProcessing)
	case p.MaxOutstandingItems <= 0:
		return fmt.Errorf("maxOutstandingItems = %d: fails the condition that: 0 < maxOutstandingItems", p.MaxOutstandingItems)
	case p.MaxItemProcessingTime <= 0:
		return fmt.Errorf("maxItemProcessingTime = %d: fails the condition that: 0 < maxItemProcessingTime", p.MaxItemProcessingTime)
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