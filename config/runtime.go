// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	// runtimeParams holds the current runtime consensus parameters
	runtimeParams Parameters
	
	// runtimeMu protects runtimeParams during updates
	runtimeMu sync.RWMutex
	
	// runtimeOverrides stores parameter overrides
	runtimeOverrides map[string]interface{}
	
	// initialized tracks if runtime params have been set
	initialized bool
)

// InitializeRuntime sets the runtime parameters based on network name
func InitializeRuntime(network string) error {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	
	params, err := GetParametersByName(network)
	if err != nil {
		return err
	}
	
	runtimeParams = params
	runtimeOverrides = make(map[string]interface{})
	initialized = true
	return nil
}

// GetRuntime returns the current runtime consensus parameters
func GetRuntime() Parameters {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	
	if !initialized {
		// Default to testnet if not initialized
		return TestnetParameters
	}
	
	return runtimeParams
}

// OverrideRuntime updates specific runtime parameters
// This allows consensus tools to modify parameters at runtime
func OverrideRuntime(updates map[string]interface{}) error {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	
	if !initialized {
		runtimeParams = TestnetParameters
		runtimeOverrides = make(map[string]interface{})
		initialized = true
	}
	
	// Create a copy to modify
	params := runtimeParams
	
	// Apply updates
	for key, value := range updates {
		runtimeOverrides[key] = value
		
		switch key {
		case "K", "k":
			if v, ok := toInt(value); ok {
				params.K = v
			}
		case "AlphaPreference", "alphaPreference":
			if v, ok := toInt(value); ok {
				params.AlphaPreference = v
			}
		case "AlphaConfidence", "alphaConfidence":
			if v, ok := toInt(value); ok {
				params.AlphaConfidence = v
			}
		case "Beta", "beta":
			if v, ok := toInt(value); ok {
				params.Beta = v
			}
		case "ConcurrentRepolls", "concurrentRepolls":
			if v, ok := toInt(value); ok {
				params.ConcurrentRepolls = v
			}
		case "OptimalProcessing", "optimalProcessing":
			if v, ok := toInt(value); ok {
				params.OptimalProcessing = v
			}
		case "MaxOutstandingItems", "maxOutstandingItems":
			if v, ok := toInt(value); ok {
				params.MaxOutstandingItems = v
			}
		case "MaxItemProcessingTime", "maxItemProcessingTime":
			if d, ok := toDuration(value); ok {
				params.MaxItemProcessingTime = d
			}
		case "MinRoundInterval", "minRoundInterval":
			if d, ok := toDuration(value); ok {
				params.MinRoundInterval = d
			}
		default:
			return fmt.Errorf("unknown parameter: %s", key)
		}
	}
	
	// Validate the new parameters
	if err := params.Valid(); err != nil {
		return fmt.Errorf("invalid parameters: %w", err)
	}
	
	// Update runtime params
	runtimeParams = params
	return nil
}

// LoadRuntimeFromFile loads consensus parameters from a JSON file
func LoadRuntimeFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	
	var params Parameters
	if err := json.Unmarshal(data, &params); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}
	
	if err := params.Valid(); err != nil {
		return fmt.Errorf("invalid parameters in config file: %w", err)
	}
	
	runtimeMu.Lock()
	runtimeParams = params
	runtimeOverrides = make(map[string]interface{})
	initialized = true
	runtimeMu.Unlock()
	
	return nil
}

// SaveRuntimeToFile saves the current runtime parameters to a JSON file
func SaveRuntimeToFile(path string) error {
	runtimeMu.RLock()
	params := runtimeParams
	overrides := make(map[string]interface{})
	for k, v := range runtimeOverrides {
		overrides[k] = v
	}
	runtimeMu.RUnlock()
	
	// Create output structure with overrides info
	output := struct {
		Parameters Parameters             `json:"parameters"`
		Overrides  map[string]interface{} `json:"overrides,omitempty"`
		Generated  time.Time              `json:"generated"`
	}{
		Parameters: params,
		Overrides:  overrides,
		Generated:  time.Now(),
	}
	
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal parameters: %w", err)
	}
	
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	
	return nil
}

// ResetRuntime resets runtime parameters to defaults for the given network
func ResetRuntime(network string) error {
	params, err := GetParametersByName(network)
	if err != nil {
		return err
	}
	
	runtimeMu.Lock()
	runtimeParams = params
	runtimeOverrides = make(map[string]interface{})
	initialized = true
	runtimeMu.Unlock()
	
	return nil
}

// GetRuntimeOverrides returns the current parameter overrides
func GetRuntimeOverrides() map[string]interface{} {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	
	overrides := make(map[string]interface{})
	for k, v := range runtimeOverrides {
		overrides[k] = v
	}
	return overrides
}

// Helper functions to convert interface{} to specific types
func toInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	default:
		return 0, false
	}
}

func toDuration(v interface{}) (time.Duration, bool) {
	switch val := v.(type) {
	case time.Duration:
		return val, true
	case string:
		d, err := time.ParseDuration(val)
		return d, err == nil
	case int64:
		return time.Duration(val), true
	case float64:
		return time.Duration(val), true
	default:
		return 0, false
	}
}