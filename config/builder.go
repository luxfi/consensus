// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"fmt"
	"time"
)

// NetworkType represents different network configurations
type NetworkType string

const (
	MainnetNetwork NetworkType = "mainnet"
	TestnetNetwork NetworkType = "testnet"
	LocalNetwork   NetworkType = "local"
)

// Config holds all consensus parameters
type Config struct {
	// Core parameters
	K                     int           `json:"k"`
	AlphaPreference       int           `json:"alphaPreference"`
	AlphaConfidence       int           `json:"alphaConfidence"`
	Beta                  int           `json:"beta"`
	ConcurrentRepolls     int           `json:"concurrentRepolls"`
	OptimalProcessing     int           `json:"optimalProcessing"`
	MaxOutstandingItems   int           `json:"maxOutstandingItems"`
	MaxItemProcessingTime time.Duration `json:"maxItemProcessingTime"`
	MinRoundInterval      time.Duration `json:"minRoundInterval"`

	// Advanced parameters
	MixedQueryNumPushVdr int           `json:"mixedQueryNumPushVdr,omitempty"`
	NetworkLatency       time.Duration `json:"networkLatency,omitempty"`
	
	// Network characteristics (for reference)
	TotalNodes           int     `json:"totalNodes,omitempty"`
	ExpectedFailureRate  float64 `json:"expectedFailureRate,omitempty"`
	
	// Quantum consensus parameters
	QThreshold           int           `json:"qThreshold,omitempty"`
	QuasarTimeout        time.Duration `json:"quasarTimeout,omitempty"`
}

// Builder provides a fluent interface for constructing consensus configurations
type Builder struct {
	config *Config
	err    error
}

// NewBuilder creates a new configuration builder
func NewBuilder() *Builder {
	return &Builder{
		config: &Config{
			// Sensible defaults
			K:                     11,
			AlphaPreference:       7,
			AlphaConfidence:       9,
			Beta:                  10,
			ConcurrentRepolls:     10,
			OptimalProcessing:     10,
			MaxOutstandingItems:   256,
			MaxItemProcessingTime: 10 * time.Second,
			MinRoundInterval:      100 * time.Millisecond,
			MixedQueryNumPushVdr:  10,
		},
	}
}

// FromPreset loads a preset configuration
func (b *Builder) FromPreset(preset NetworkType) *Builder {
	if b.err != nil {
		return b
	}

	switch preset {
	case MainnetNetwork:
		b.config = &MainnetConfig
	case TestnetNetwork:
		b.config = &TestnetConfig
	case LocalNetwork:
		b.config = &LocalConfig
	default:
		b.err = fmt.Errorf("unknown preset: %s", preset)
	}
	
	// Clone to avoid modifying presets
	if b.config != nil {
		clone := *b.config
		b.config = &clone
	}
	
	return b
}

// WithSampleSize sets the sample size K
func (b *Builder) WithSampleSize(k int) *Builder {
	if b.err != nil {
		return b
	}
	
	if k < 1 {
		b.err = fmt.Errorf("K must be at least 1, got %d", k)
		return b
	}
	
	b.config.K = k
	
	// Auto-adjust quorums if needed
	if b.config.AlphaPreference > k {
		b.config.AlphaPreference = (k * 2 / 3) + 1
	}
	if b.config.AlphaConfidence > k {
		b.config.AlphaConfidence = (k * 3 / 4) + 1
	}
	
	return b
}

// WithQuorums sets the preference and confidence quorums
func (b *Builder) WithQuorums(alphaPref, alphaConf int) *Builder {
	if b.err != nil {
		return b
	}
	
	// Validate quorum constraints
	minAlpha := b.config.K/2 + 1
	if alphaPref < minAlpha {
		b.err = fmt.Errorf("AlphaPreference must be > K/2, got %d (min: %d)", alphaPref, minAlpha)
		return b
	}
	if alphaConf < alphaPref {
		b.err = fmt.Errorf("AlphaConfidence must be >= AlphaPreference, got %d < %d", alphaConf, alphaPref)
		return b
	}
	if alphaConf > b.config.K {
		b.err = fmt.Errorf("AlphaConfidence must be <= K, got %d > %d", alphaConf, b.config.K)
		return b
	}
	
	b.config.AlphaPreference = alphaPref
	b.config.AlphaConfidence = alphaConf
	
	return b
}

// WithBeta sets the consecutive rounds threshold
func (b *Builder) WithBeta(beta int) *Builder {
	if b.err != nil {
		return b
	}
	
	if beta < 1 {
		b.err = fmt.Errorf("Beta must be at least 1, got %d", beta)
		return b
	}
	
	b.config.Beta = beta
	
	// Auto-set concurrent repolls if not explicitly set
	if b.config.ConcurrentRepolls > beta {
		b.config.ConcurrentRepolls = beta
	}
	
	return b
}

// WithConcurrentRepolls sets the pipelining factor
func (b *Builder) WithConcurrentRepolls(concurrent int) *Builder {
	if b.err != nil {
		return b
	}
	
	if concurrent < 1 {
		b.err = fmt.Errorf("ConcurrentRepolls must be at least 1, got %d", concurrent)
		return b
	}
	if concurrent > b.config.Beta {
		b.err = fmt.Errorf("ConcurrentRepolls cannot exceed Beta, got %d > %d", concurrent, b.config.Beta)
		return b
	}
	
	b.config.ConcurrentRepolls = concurrent
	return b
}

// WithMinRoundInterval sets the minimum interval between consensus rounds
func (b *Builder) WithMinRoundInterval(interval time.Duration) *Builder {
	if b.err != nil {
		return b
	}
	
	b.config.MinRoundInterval = interval
	return b
}

// WithTargetFinality calculates parameters for target finality time
func (b *Builder) WithTargetFinality(target time.Duration, networkLatencyMs int) *Builder {
	if b.err != nil {
		return b
	}
	
	// Calculate required Beta
	roundTime := time.Duration(networkLatencyMs) * time.Millisecond
	requiredBeta := int(target / roundTime)
	
	if requiredBeta < 4 {
		requiredBeta = 4 // Minimum for security
	}
	
	b.config.Beta = requiredBeta
	b.config.ConcurrentRepolls = requiredBeta // Full pipelining
	b.config.NetworkLatency = roundTime
	
	return b
}

// ForNodeCount optimizes parameters for a specific network size
func (b *Builder) ForNodeCount(totalNodes int) *Builder {
	if b.err != nil {
		return b
	}
	
	b.config.TotalNodes = totalNodes
	
	// Adjust K based on network size
	switch {
	case totalNodes <= 30:
		b.config.K = totalNodes
	case totalNodes <= 100:
		b.config.K = 21
	case totalNodes <= 500:
		b.config.K = 35
	default:
		b.config.K = 50
	}
	
	// Recalculate quorums
	b.config.AlphaPreference = (b.config.K * 2 / 3) + 1
	b.config.AlphaConfidence = (b.config.K * 3 / 4) + 1
	
	// Adjust Beta for larger networks
	if totalNodes > 100 && b.config.Beta < 15 {
		b.config.Beta = 15
		b.config.ConcurrentRepolls = 15
	}
	
	return b
}

// OptimizeForLatency optimizes for low latency
func (b *Builder) OptimizeForLatency() *Builder {
	if b.err != nil {
		return b
	}
	
	// Reduce Beta for faster finality
	if b.config.Beta > 5 {
		b.config.Beta = 5
	}
	b.config.ConcurrentRepolls = b.config.Beta
	
	// Increase processing capacity
	b.config.OptimalProcessing = 32
	b.config.MaxOutstandingItems = 1024
	
	return b
}

// OptimizeForSecurity optimizes for maximum security
func (b *Builder) OptimizeForSecurity() *Builder {
	if b.err != nil {
		return b
	}
	
	// Increase Beta for better security
	if b.config.Beta < 20 {
		b.config.Beta = 20
	}
	
	// Use conservative quorums
	b.config.AlphaConfidence = (b.config.K * 4 / 5) + 1
	if b.config.AlphaConfidence > b.config.K {
		b.config.AlphaConfidence = b.config.K
	}
	
	return b
}

// OptimizeForThroughput optimizes for high throughput
func (b *Builder) OptimizeForThroughput() *Builder {
	if b.err != nil {
		return b
	}
	
	b.config.OptimalProcessing = 64
	b.config.MaxOutstandingItems = 4096
	b.config.ConcurrentRepolls = b.config.Beta // Max pipelining
	
	return b
}

// Build returns the final configuration
func (b *Builder) Build() (*Config, error) {
	if b.err != nil {
		return nil, b.err
	}
	
	// Final validation
	validator := NewValidator()
	if err := validator.Validate(b.config); err != nil {
		return nil, err
	}
	
	return b.config, nil
}

// Preset configurations
var (
	MainnetConfig = Config{
		K:                     21,
		AlphaPreference:       13,
		AlphaConfidence:       18,
		Beta:                  8,
		ConcurrentRepolls:     8,
		OptimalProcessing:     10,
		MaxOutstandingItems:   256,
		MaxItemProcessingTime: 9630 * time.Millisecond,
		MinRoundInterval:      200 * time.Millisecond,
		MixedQueryNumPushVdr:  10,
		TotalNodes:            21,
		ExpectedFailureRate:   0.20,
		QThreshold:            15,
		QuasarTimeout:         50 * time.Millisecond,
	}
	
	TestnetConfig = Config{
		K:                     11,
		AlphaPreference:       7,
		AlphaConfidence:       9,
		Beta:                  6,
		ConcurrentRepolls:     6,
		OptimalProcessing:     10,
		MaxOutstandingItems:   256,
		MaxItemProcessingTime: 6300 * time.Millisecond,
		MinRoundInterval:      100 * time.Millisecond,
		MixedQueryNumPushVdr:  10,
		TotalNodes:            11,
		ExpectedFailureRate:   0.20,
		QThreshold:            8,
		QuasarTimeout:         100 * time.Millisecond,
	}
	
	LocalConfig = Config{
		K:                     5,
		AlphaPreference:       4,
		AlphaConfidence:       4,
		Beta:                  3,
		ConcurrentRepolls:     3,
		OptimalProcessing:     10,
		MaxOutstandingItems:   256,
		MaxItemProcessingTime: 3690 * time.Millisecond,
		MinRoundInterval:      10 * time.Millisecond,
		MixedQueryNumPushVdr:  10,
		TotalNodes:            5,
		ExpectedFailureRate:   0.10,
		QThreshold:            3,
		QuasarTimeout:         20 * time.Millisecond,
	}
)