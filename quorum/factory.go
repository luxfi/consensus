// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"fmt"
)

// Type represents different threshold strategies
type Type string

const (
	StaticType          Type = "static"
	WeightedStaticType  Type = "weighted_static"
	DynamicType         Type = "dynamic"
	AdaptiveDynamicType Type = "adaptive_dynamic"
)

// Config holds threshold configuration
type Config struct {
	Type Type
	
	// Static configuration
	Threshold       int
	WeightThreshold uint64
	
	// Dynamic configuration
	PreferenceThreshold int
	ConfidenceThreshold int
	
	// Adaptive parameters
	EnableAdaptation bool
}

// Factory creates threshold instances based on configuration
type Factory struct{}

// NewFactory creates a new threshold factory
func NewFactory() *Factory {
	return &Factory{}
}

// NewThreshold creates a threshold instance from configuration
func (f *Factory) NewThreshold(cfg Config) (Threshold, error) {
	switch cfg.Type {
	case StaticType:
		if cfg.Threshold <= 0 {
			return nil, fmt.Errorf("static threshold must be positive")
		}
		return NewStatic(cfg.Threshold), nil
		
	case WeightedStaticType:
		if cfg.WeightThreshold == 0 {
			return nil, fmt.Errorf("weighted threshold must be positive")
		}
		return NewWeightedStatic(cfg.WeightThreshold), nil
		
	case DynamicType:
		if cfg.PreferenceThreshold <= 0 || cfg.ConfidenceThreshold <= 0 {
			return nil, fmt.Errorf("dynamic thresholds must be positive")
		}
		return NewDynamic(cfg.PreferenceThreshold, cfg.ConfidenceThreshold), nil
		
	case AdaptiveDynamicType:
		if cfg.PreferenceThreshold <= 0 || cfg.ConfidenceThreshold <= 0 {
			return nil, fmt.Errorf("adaptive thresholds must be positive")
		}
		return NewAdaptiveDynamic(cfg.PreferenceThreshold, cfg.ConfidenceThreshold), nil
		
	default:
		return nil, fmt.Errorf("unknown threshold type: %s", cfg.Type)
	}
}

// NewDynamicThreshold creates a dynamic threshold instance
func (f *Factory) NewDynamicThreshold(cfg Config) (DynamicThreshold, error) {
	switch cfg.Type {
	case DynamicType:
		return NewDynamic(cfg.PreferenceThreshold, cfg.ConfidenceThreshold), nil
		
	case AdaptiveDynamicType:
		return NewAdaptiveDynamic(cfg.PreferenceThreshold, cfg.ConfidenceThreshold), nil
		
	default:
		return nil, fmt.Errorf("type %s does not support dynamic thresholds", cfg.Type)
	}
}

// NewWeightedThreshold creates a weighted threshold instance
func (f *Factory) NewWeightedThreshold(cfg Config) (WeightedThreshold, error) {
	switch cfg.Type {
	case WeightedStaticType:
		return NewWeightedStatic(cfg.WeightThreshold), nil
		
	default:
		return nil, fmt.Errorf("type %s does not support weighted thresholds", cfg.Type)
	}
}