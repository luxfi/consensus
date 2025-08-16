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
	// Core sampling parameters
	K               int    `json:"k" yaml:"k"`                               // sample size per poll
	AlphaPreference int    `json:"alpha_preference" yaml:"alpha_preference"` // threshold for preference
	AlphaConfidence int    `json:"alpha_confidence" yaml:"alpha_confidence"` // threshold for confidence
	Beta            uint32 `json:"beta" yaml:"beta"`                         // consecutive successes for finalize
	DeltaMinMS      int    `json:"delta_min_ms" yaml:"delta_min_ms"`         // nominal poll round interval (ms)

	// Advanced parameters
	ConcurrentPolls       int           `json:"concurrent_polls" yaml:"concurrent_polls"`
	OptimalProcessing     int           `json:"optimal_processing" yaml:"optimal_processing"`
	MaxOutstandingItems   int           `json:"max_outstanding_items" yaml:"max_outstanding_items"`
	MaxItemProcessingTime time.Duration `json:"max_item_processing_time" yaml:"max_item_processing_time"`
	MinRoundInterval      time.Duration `json:"min_round_interval" yaml:"min_round_interval"`

	// Fast-path voting configuration
	FPC FPCConfig `json:"fpc" yaml:"fpc"`

	// PQ overlay configuration
	Quasar QuasarConfig `json:"quasar" yaml:"quasar"`
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

// GetBeta returns the beta parameter
func (p Parameters) GetBeta() int {
	return int(p.Beta)
}

// GetConcurrentPolls returns the concurrent polls parameter
func (p Parameters) GetConcurrentPolls() int {
	return p.ConcurrentPolls
}

// GetOptimalProcessing returns the optimal processing parameter
func (p Parameters) GetOptimalProcessing() int {
	return p.OptimalProcessing
}

// GetMaxOutstandingItems returns the max outstanding items parameter
func (p Parameters) GetMaxOutstandingItems() int {
	return p.MaxOutstandingItems
}

// GetMaxItemProcessingTime returns the max item processing time
func (p Parameters) GetMaxItemProcessingTime() time.Duration {
	return p.MaxItemProcessingTime
}

// GetMinRoundInterval returns the min round interval
func (p Parameters) GetMinRoundInterval() time.Duration {
	return p.MinRoundInterval
}

// GetEnableFPC returns whether FPC is enabled
func (p Parameters) GetEnableFPC() bool {
	return p.FPC.Enable
}

// Validate returns an error if the parameters are invalid
func (p Parameters) Validate() error {
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
	if p.DeltaMinMS < 0 {
		return fmt.Errorf("deltaMinMS = %d: must be >= 0", p.DeltaMinMS)
	}
	if p.OptimalProcessing <= 0 {
		return fmt.Errorf("optimalProcessing = %d: fails the condition that: 0 < optimalProcessing", p.OptimalProcessing)
	}
	if p.MaxOutstandingItems <= 0 {
		return fmt.Errorf("maxOutstandingItems = %d: fails the condition that: 0 < maxOutstandingItems", p.MaxOutstandingItems)
	}
	if p.MaxItemProcessingTime <= 0 {
		return fmt.Errorf("maxItemProcessingTime = %v: fails the condition that: 0 < maxItemProcessingTime", p.MaxItemProcessingTime)
	}
	return nil
}

// ErrParametersInvalid is returned when parameters are invalid
var ErrParametersInvalid = errors.New("invalid parameters")
