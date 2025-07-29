// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package consensus provides a modular, composable consensus framework
// implementing DAG (Graph) consensus variants including Wave, Focus,
// and Beam++, as well as BFT consensus through the integrated BFT package.
package core

import (
	"time"
	
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/poll"
	"github.com/luxfi/consensus/quorum"
)

// Re-export core types for convenience
type (
	// Configuration types
	Config          = config.Config
	ConfigBuilder   = config.Builder
	NetworkType     = config.NetworkType
	ValidationMode  = config.ValidationMode
	ValidationError = config.ValidationError
	ValidationResult = config.ValidationResult
	
	// Polling types
	Sampler         = poll.Sampler
	SamplerType     = poll.SamplerType
	
	// Quorum types
	Threshold       = quorum.Threshold
	ThresholdResult = quorum.Result
	WeightedThreshold = quorum.WeightedThreshold
	DynamicThreshold  = quorum.DynamicThreshold
)

// Re-export constants
const (
	// Network types
	MainnetNetwork = config.MainnetNetwork
	TestnetNetwork = config.TestnetNetwork
	LocalNetwork   = config.LocalNetwork
	
	// Validation modes
	StrictMode = config.StrictMode
	SoftMode   = config.SoftMode
	
	// Sampler types
	UniformSamplerType = poll.UniformSampler
	StakeSamplerType   = poll.StakeSampler
	
	// Threshold types
	StaticThreshold         = quorum.StaticType
	WeightedStaticThreshold = quorum.WeightedStaticType
	DynamicThresholdType    = quorum.DynamicType
	AdaptiveDynamicThreshold = quorum.AdaptiveDynamicType
)

// Re-export functions
var (
	// Config functions
	NewConfigBuilder        = config.NewBuilder
	NewConfigValidator      = config.NewValidator
	ValidateForProduction   = config.ValidateForProduction
	
	// Poll functions
	NewUnaryPoll = poll.NewUnary
	NewBinaryPoll  = poll.NewBinary
	NewManyPoll  = poll.NewMany
	NewSamplerFactory       = poll.NewFactory
	NewSamplerFromConfig    = poll.NewSamplerFromConfig
	
	// Quorum functions
	NewStaticThreshold      = quorum.NewStatic
	NewWeightedStatic       = quorum.NewWeightedStatic
	NewDynamicThreshold     = quorum.NewDynamic
	NewAdaptiveDynamic      = quorum.NewAdaptiveDynamic
	NewThresholdFactory     = quorum.NewFactory
)

// Engine represents a consensus engine instance
type Engine interface {
	// Start begins consensus operations
	Start() error
	
	// Stop halts consensus operations
	Stop() error
	
	// IsRunning returns true if the engine is active
	IsRunning() bool
	
	// Metrics returns current consensus metrics
	Metrics() Metrics
}

// Metrics contains consensus performance metrics
type Metrics struct {
	// Consensus rounds completed
	RoundsCompleted uint64
	
	// Average finality time
	AverageFinality time.Duration
	
	// Current confidence level
	Confidence float64
	
	// Network participation rate
	ParticipationRate float64
	
	// Throughput in transactions per second
	ThroughputTPS float64
}

// ConsensusFactory creates consensus engines based on configuration
type ConsensusFactory struct {
	config   *Config
	sampler  Sampler
	threshold Threshold
}

// NewConsensusFactory creates a new consensus factory
func NewConsensusFactory(cfg *Config) (*ConsensusFactory, error) {
	// Validate configuration
	validator := NewConfigValidator()
	if err := validator.Validate(cfg); err != nil {
		return nil, err
	}
	
	return &ConsensusFactory{
		config: cfg,
	}, nil
}

// WithSampler sets the sampler to use
func (f *ConsensusFactory) WithSampler(sampler Sampler) *ConsensusFactory {
	f.sampler = sampler
	return f
}

// WithThreshold sets the threshold to use
func (f *ConsensusFactory) WithThreshold(threshold Threshold) *ConsensusFactory {
	f.threshold = threshold
	return f
}

// CreateEngine creates a new consensus engine
func (f *ConsensusFactory) CreateEngine(engineType string) (Engine, error) {
	// This is a placeholder - actual engine implementations will be added
	// when we refactor the engine package
	return nil, nil
}

// Preset configurations
var (
	MainnetConfig = config.MainnetConfig
	TestnetConfig = config.TestnetConfig
	LocalConfig   = config.LocalConfig
)