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
		runtimeParams = TestParameters
		initialized = true
	}
	
	return runtimeParams
}

// SetRuntime updates the runtime consensus parameters
func SetRuntime(params Parameters) {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	
	runtimeParams = params
	initialized = true
}

// UpdateRuntimeParameter updates a single runtime parameter
// This is temporarily disabled as it requires mutable parameters
func UpdateRuntimeParameter(key string, value interface{}) error {
	return fmt.Errorf("runtime parameter updates are temporarily disabled during migration")
	
	// TODO: Implement when we have mutable parameter structs
	// The original implementation tried to modify interface values
}

// LoadRuntimeFromFile loads runtime parameters from a JSON file
func LoadRuntimeFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read runtime config file: %w", err)
	}
	
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse runtime config: %w", err)
	}
	
	params := config.ToParameters()
	SetRuntime(params)
	
	return nil
}

// SaveRuntimeToFile saves the current runtime parameters to a JSON file
func SaveRuntimeToFile(path string) error {
	runtimeMu.RLock()
	params := runtimeParams
	runtimeMu.RUnlock()
	
	// Convert parameters to a map for JSON encoding
	data := map[string]interface{}{
		"k":                     params.GetK(),
		"alphaPreference":       params.GetAlphaPreference(),
		"alphaConfidence":       params.GetAlphaConfidence(),
		"beta":                  params.GetBeta(),
		"concurrentReprisms":     params.ConcurrentReprisms,
		"optimalProcessing":     params.OptimalProcessing,
		"maxOutstandingItems":   params.MaxOutstandingItems,
		"maxItemProcessingTime": params.MaxItemProcessingTime,
		"minRoundInterval":      params.MinRoundInterval,
	}
	
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal runtime config: %w", err)
	}
	
	if err := os.WriteFile(path, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write runtime config file: %w", err)
	}
	
	return nil
}

// GetRuntimeOverrides returns the current parameter overrides
func GetRuntimeOverrides() map[string]interface{} {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	
	// Return a copy to prevent external modification
	copy := make(map[string]interface{})
	for k, v := range runtimeOverrides {
		copy[k] = v
	}
	return copy
}

// Helper functions
func toInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case string:
		var i int
		_, err := fmt.Sscanf(val, "%d", &i)
		return i, err == nil
	}
	return 0, false
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
	}
	return 0, false
}