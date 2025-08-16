// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"fmt"
	"time"
)

// Local returns local development parameters
func Local() Parameters {
	return Parameters{
		K:                     5,
		AlphaPreference:       3,
		AlphaConfidence:       4,
		Beta:                  3,
		MinRoundInterval:      10 * time.Millisecond,
		MaxItemProcessingTime: 5 * time.Second,
		ConcurrentPolls:       2,
		OptimalProcessing:     5,
		MaxOutstandingItems:   256,
	}
}

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
	if int(p.Beta) > p.K {
		return fmt.Errorf("beta (%d) must be <= k (%d)", p.Beta, p.K)
	}
	if p.ConcurrentPolls <= 0 {
		return fmt.Errorf("concurrentPolls = %d: fails the condition that: 0 < concurrentPolls", p.ConcurrentPolls)
	}
	if p.ConcurrentPolls > int(p.Beta) {
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
	if p.DeltaMinMS < 0 {
		return fmt.Errorf("deltaMinMS = %d: must be >= 0", p.DeltaMinMS)
	}
	return nil
}
