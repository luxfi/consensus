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
	AlphaPreference       int           `json:"alpha_preference" yaml:"alpha_preference"`
	AlphaConfidence       int           `json:"alpha_confidence" yaml:"alpha_confidence"`
	Beta                  int           `json:"beta" yaml:"beta"`
	
	// Quantum parameters
	QRounds               int           `json:"q_rounds" yaml:"q_rounds"`
	QuasarTimeout         time.Duration `json:"quasar_timeout" yaml:"quasar_timeout"`
	
	// Advanced parameters
	ConcurrentPolls       int           `json:"concurrent_polls" yaml:"concurrent_polls"`
	OptimalProcessing     int           `json:"optimal_processing" yaml:"optimal_processing"`
	MaxOutstandingItems   int           `json:"max_outstanding_items" yaml:"max_outstanding_items"`
	MaxItemProcessingTime time.Duration `json:"max_item_processing_time" yaml:"max_item_processing_time"`
	MinRoundInterval      time.Duration `json:"min_round_interval" yaml:"min_round_interval"`
	
	// WaveFPC parameters (optional fast-path certification)
	EnableFPC             bool          `json:"enable_fpc" yaml:"enable_fpc"`
	FPCVoteLimit          int           `json:"fpc_vote_limit" yaml:"fpc_vote_limit"`
	FPCVotePrefix         []byte        `json:"fpc_vote_prefix" yaml:"fpc_vote_prefix"`
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
	if p.QRounds != 0 || p.QuasarTimeout != 0 {
		switch {
		case p.QRounds <= 0:
			return errors.New("q_rounds must be positive when set")
		case p.QRounds > 3:
			return fmt.Errorf("q_rounds (%d) should be <= 3 for efficiency", p.QRounds)
		case p.QuasarTimeout <= 0:
			return errors.New("quasar_timeout must be positive when set")
		}
	}
	
	return nil
}