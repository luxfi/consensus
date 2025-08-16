// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/luxfi/log"
)

// Validation errors
var (
	ErrKTooLow                     = errors.New("k is too low")
	ErrAlphaPreferenceTooLow       = errors.New("alpha preference is too low")
	ErrAlphaPreferenceTooHigh      = errors.New("alpha preference is too high")
	ErrAlphaConfidenceTooSmall     = errors.New("alpha confidence is too small")
	ErrBetaTooLow                  = errors.New("beta is too low")
	ErrConcurrentPollsTooLow       = errors.New("concurrent polls is too low")
	ErrConcurrentPollsTooHigh      = errors.New("concurrent polls is too high")
	ErrOptimalProcessingTooLow     = errors.New("optimal processing is too low")
	ErrMaxOutstandingItemsTooLow   = errors.New("max outstanding items is too low")
	ErrMaxItemProcessingTimeTooLow = errors.New("max item processing time is too low")
)

// ValidationMode determines how strict validation should be
type ValidationMode int

const (
	// StrictMode enforces all security and performance constraints
	StrictMode ValidationMode = iota
	// SoftMode allows some flexibility for experimental configurations
	SoftMode
)

// ValidationError contains detailed validation error information
type ValidationError struct {
	Field      string
	Value      interface{}
	Constraint string
	Severity   string // "error" or "warning"
	Suggestion string
}

func (ve ValidationError) Error() string {
	return fmt.Sprintf("%s: %s=%v violates constraint: %s", ve.Severity, ve.Field, ve.Value, ve.Constraint)
}

// ValidationResult contains all validation errors and warnings
type ValidationResult struct {
	Errors   []ValidationError
	Warnings []ValidationError
	Valid    bool
}

// Validator validates consensus configurations
type Validator struct {
	mode ValidationMode
}

// NewValidator creates a validator with strict mode by default
func NewValidator() *Validator {
	return &Validator{mode: StrictMode}
}

// WithMode sets the validation mode
func (v *Validator) WithMode(mode ValidationMode) *Validator {
	v.mode = mode
	return v
}

// Validate performs comprehensive validation of a configuration
func (v *Validator) Validate(cfg *Config) error {
	result := v.ValidateDetailed(cfg)
	if !result.Valid {
		var errStrs []string
		for _, err := range result.Errors {
			errStrs = append(errStrs, err.Error())
		}
		return fmt.Errorf("validation failed:\n%s", strings.Join(errStrs, "\n"))
	}
	return nil
}

// ValidateDetailed returns detailed validation results
func (v *Validator) ValidateDetailed(cfg *Config) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// Basic parameter validation
	v.validateBasicParameters(cfg, result)

	// Quorum validation
	v.validateQuorums(cfg, result)

	// Performance validation
	v.validatePerformance(cfg, result)

	// Security validation
	if v.mode == StrictMode {
		v.validateSecurity(cfg, result)
	}

	// Network-specific validation
	if cfg.TotalNodes > 0 {
		v.validateNetworkSpecific(cfg, result)
	}

	return result
}

func (v *Validator) validateBasicParameters(cfg *Config, result *ValidationResult) {
	// K validation
	if cfg.K < 1 {
		v.addError(result, "K", cfg.K, "must be at least 1", "Set K >= 1")
	} else if cfg.K < 5 && v.mode == StrictMode {
		v.addWarning(result, "K", cfg.K, "very small K reduces security", "Consider K >= 5 for production")
	}

	// Beta validation
	if cfg.Beta < 1 {
		v.addError(result, "Beta", cfg.Beta, "must be at least 1", "Set Beta >= 1")
	} else if cfg.Beta < 4 && v.mode == StrictMode {
		v.addWarning(result, "Beta", cfg.Beta, "low Beta reduces security", "Consider Beta >= 4")
	} else if cfg.Beta > 100 {
		v.addWarning(result, "Beta", cfg.Beta, "very high Beta increases latency", "Consider Beta <= 50")
	}

	// ConcurrentPolls validation
	if cfg.ConcurrentPolls < 1 {
		v.addError(result, "ConcurrentPolls", cfg.ConcurrentPolls,
			"must be at least 1", "Set ConcurrentPolls >= 1")
	}
	if cfg.ConcurrentPolls > cfg.Beta {
		v.addError(result, "ConcurrentPolls", cfg.ConcurrentPolls,
			fmt.Sprintf("cannot exceed Beta (%d)", cfg.Beta),
			fmt.Sprintf("Set ConcurrentPolls <= %d", cfg.Beta))
	}

	// Processing parameters
	if cfg.OptimalProcessing < 1 {
		v.addError(result, "OptimalProcessing", cfg.OptimalProcessing,
			"must be at least 1", "Set OptimalProcessing >= 1")
	}
	if cfg.MaxOutstandingItems < cfg.OptimalProcessing {
		v.addError(result, "MaxOutstandingItems", cfg.MaxOutstandingItems,
			fmt.Sprintf("must be >= OptimalProcessing (%d)", cfg.OptimalProcessing),
			fmt.Sprintf("Set MaxOutstandingItems >= %d", cfg.OptimalProcessing))
	}

	// Time validation
	if cfg.MaxItemProcessingTime < 100*time.Millisecond {
		v.addError(result, "MaxItemProcessingTime", cfg.MaxItemProcessingTime,
			"must be at least 100ms", "Set MaxItemProcessingTime >= 100ms")
	}

	// MinRoundInterval validation
	if cfg.MinRoundInterval < 1*time.Millisecond || cfg.MinRoundInterval > 500*time.Millisecond {
		v.addError(result, "MinRoundInterval", cfg.MinRoundInterval,
			"must be in range [1ms, 500ms]",
			fmt.Sprintf("Set MinRoundInterval between 1ms and 500ms"))
	} else {
		// Soft warnings for edge cases
		if cfg.MinRoundInterval < 10*time.Millisecond {
			log.Warn("⚠️ Low MinRoundInterval detected: CPU/network may be overloaded",
				"interval", cfg.MinRoundInterval)
			v.addWarning(result, "MinRoundInterval", cfg.MinRoundInterval,
				"very low interval (<10ms) may overload CPU/network",
				"Consider MinRoundInterval >= 10ms unless on high-performance network")
		}
		if cfg.MinRoundInterval > 200*time.Millisecond {
			log.Warn("⚠️ High MinRoundInterval detected: latency may spike",
				"interval", cfg.MinRoundInterval)
			v.addWarning(result, "MinRoundInterval", cfg.MinRoundInterval,
				"high interval (>200ms) increases latency",
				"Consider MinRoundInterval <= 200ms for better performance")
		}
	}
}

func (v *Validator) validateQuorums(cfg *Config, result *ValidationResult) {
	// Basic quorum constraints
	minAlpha := cfg.K/2 + 1

	if cfg.AlphaPreference < minAlpha {
		v.addError(result, "AlphaPreference", cfg.AlphaPreference,
			fmt.Sprintf("must be > K/2 (min: %d)", minAlpha),
			fmt.Sprintf("Set AlphaPreference >= %d", minAlpha))
	}

	if cfg.AlphaPreference > cfg.K {
		v.addError(result, "AlphaPreference", cfg.AlphaPreference,
			fmt.Sprintf("cannot exceed K (%d)", cfg.K),
			fmt.Sprintf("Set AlphaPreference <= %d", cfg.K))
	}

	if cfg.AlphaConfidence < cfg.AlphaPreference {
		v.addError(result, "AlphaConfidence", cfg.AlphaConfidence,
			fmt.Sprintf("must be >= AlphaPreference (%d)", cfg.AlphaPreference),
			fmt.Sprintf("Set AlphaConfidence >= %d", cfg.AlphaPreference))
	}

	if cfg.AlphaConfidence > cfg.K {
		v.addError(result, "AlphaConfidence", cfg.AlphaConfidence,
			fmt.Sprintf("cannot exceed K (%d)", cfg.K),
			fmt.Sprintf("Set AlphaConfidence <= %d", cfg.K))
	}

	// Security recommendations
	if v.mode == StrictMode {
		recommendedAlphaConf := (cfg.K * 3 / 4) + 1
		if cfg.AlphaConfidence < recommendedAlphaConf {
			v.addWarning(result, "AlphaConfidence", cfg.AlphaConfidence,
				fmt.Sprintf("below recommended 3K/4+1 (%d)", recommendedAlphaConf),
				fmt.Sprintf("Consider AlphaConfidence >= %d for better security", recommendedAlphaConf))
		}
	}
}

func (v *Validator) validatePerformance(cfg *Config, result *ValidationResult) {
	// Estimate finality time
	if cfg.NetworkLatency > 0 {
		expectedFinality := time.Duration(cfg.Beta) * cfg.NetworkLatency
		if expectedFinality > 10*time.Second && v.mode == StrictMode {
			v.addWarning(result, "Beta", cfg.Beta,
				fmt.Sprintf("results in %.1fs finality with %dms network latency",
					expectedFinality.Seconds(), cfg.NetworkLatency.Milliseconds()),
				"Consider reducing Beta or improving network latency")
		}
	}

	// Throughput warnings
	if cfg.MaxOutstandingItems > 10000 {
		v.addWarning(result, "MaxOutstandingItems", cfg.MaxOutstandingItems,
			"very high value may cause memory issues",
			"Consider MaxOutstandingItems <= 10000")
	}
}

func (v *Validator) validateSecurity(cfg *Config, result *ValidationResult) {
	// Calculate fault tolerance
	byzantineTolerance := cfg.K - cfg.AlphaConfidence
	tolerancePercent := float64(byzantineTolerance) / float64(cfg.K) * 100

	if tolerancePercent < 20 {
		v.addWarning(result, "AlphaConfidence", cfg.AlphaConfidence,
			fmt.Sprintf("low Byzantine tolerance (%.1f%%)", tolerancePercent),
			"Consider lower AlphaConfidence for better fault tolerance")
	}

	// Beta security check
	if cfg.Beta < 10 && cfg.K > 20 {
		v.addWarning(result, "Beta", cfg.Beta,
			"low Beta for large network may reduce security",
			"Consider Beta >= 10 for networks with K > 20")
	}
}

func (v *Validator) validateNetworkSpecific(cfg *Config, result *ValidationResult) {
	// Check if K is appropriate for network size
	if cfg.TotalNodes > 0 {
		if cfg.K > cfg.TotalNodes {
			v.addError(result, "K", cfg.K,
				fmt.Sprintf("cannot exceed total nodes (%d)", cfg.TotalNodes),
				fmt.Sprintf("Set K <= %d", cfg.TotalNodes))
		}

		// Sampling ratio recommendations
		samplingRatio := float64(cfg.K) / float64(cfg.TotalNodes)
		if samplingRatio < 0.1 && cfg.TotalNodes < 100 {
			v.addWarning(result, "K", cfg.K,
				fmt.Sprintf("low sampling ratio (%.1f%%) for small network", samplingRatio*100),
				"Consider increasing K for better security")
		}
	}
}

func (v *Validator) addError(result *ValidationResult, field string, value interface{},
	constraint string, suggestion string,
) {
	result.Errors = append(result.Errors, ValidationError{
		Field:      field,
		Value:      value,
		Constraint: constraint,
		Severity:   "error",
		Suggestion: suggestion,
	})
	result.Valid = false
}

func (v *Validator) addWarning(result *ValidationResult, field string, value interface{},
	constraint string, suggestion string,
) {
	result.Warnings = append(result.Warnings, ValidationError{
		Field:      field,
		Value:      value,
		Constraint: constraint,
		Severity:   "warning",
		Suggestion: suggestion,
	})
}

// ValidateForProduction performs strict validation for production use
func ValidateForProduction(cfg *Config, totalNodes int) error {
	validator := NewValidator().WithMode(StrictMode)
	cfg.TotalNodes = totalNodes

	result := validator.ValidateDetailed(cfg)

	// Additional production checks
	if cfg.K < 5 {
		return fmt.Errorf("K must be at least 5 for production (got %d)", cfg.K)
	}

	if cfg.Beta < 4 {
		return fmt.Errorf("Beta must be at least 4 for production (got %d)", cfg.Beta)
	}

	if totalNodes > 0 && float64(cfg.K)/float64(totalNodes) < 0.2 {
		return fmt.Errorf("sampling ratio too low for production: %.1f%% (minimum 20%%)",
			float64(cfg.K)/float64(totalNodes)*100)
	}

	if !result.Valid {
		var errStrs []string
		for _, err := range result.Errors {
			errStrs = append(errStrs, err.Error())
		}
		return fmt.Errorf("validation failed:\n%s", strings.Join(errStrs, "\n"))
	}

	return nil
}
