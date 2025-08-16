// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"errors"
	"time"
)

// DefaultParameters returns the default consensus parameters
var DefaultParameters = Parameters{
	K:                     20,
	AlphaPreference:       15,
	AlphaConfidence:       15,
	Beta:                  20,
	DeltaMinMS:            50,
	ConcurrentPolls:       4,
	OptimalProcessing:     10,
	MaxOutstandingItems:   256,
	MaxItemProcessingTime: 30 * time.Second,
	MinRoundInterval:      100 * time.Millisecond,
	FPC:                   DefaultFPC(), // FPC enabled by default for 50x speedup
	Quasar:                QuasarConfig{Enable: false, Precompute: 0, Threshold: 0},
}

// TestParameters returns parameters suitable for testing
var TestParameters = Parameters{
	K:                     2,
	AlphaPreference:       2,
	AlphaConfidence:       2,
	Beta:                  2,
	DeltaMinMS:            10,
	ConcurrentPolls:       1,
	OptimalProcessing:     1,
	MaxOutstandingItems:   16,
	MaxItemProcessingTime: 10 * time.Second,
	MinRoundInterval:      10 * time.Millisecond,
	FPC:                   DefaultFPC(), // FPC enabled for tests
	Quasar:                QuasarConfig{Enable: false, Precompute: 0, Threshold: 0},
}

// LocalParameters for local development (5 nodes)
var LocalParameters = Parameters{
	K:                     5,
	AlphaPreference:       3,
	AlphaConfidence:       4,
	Beta:                  3,
	DeltaMinMS:            30,
	ConcurrentPolls:       2,
	OptimalProcessing:     3,
	MaxOutstandingItems:   50,
	MaxItemProcessingTime: 369 * time.Millisecond, // 0.369s for local consensus
	MinRoundInterval:      10 * time.Millisecond,
	FPC:                   DefaultFPC(), // FPC enabled for local dev
	Quasar:                QuasarConfig{Enable: false, Precompute: 0, Threshold: 0},
}

// TestnetParameters for testnet (11 nodes)
var TestnetParameters = Parameters{
	K:                     11,
	AlphaPreference:       7,
	AlphaConfidence:       9,
	Beta:                  6,
	DeltaMinMS:            50,
	ConcurrentPolls:       4,
	OptimalProcessing:     5,
	MaxOutstandingItems:   100,
	MaxItemProcessingTime: 630 * time.Millisecond, // 0.63s for testnet consensus
	MinRoundInterval:      50 * time.Millisecond,
	FPC:                   DefaultFPC(), // FPC enabled by default
	Quasar:                QuasarConfig{Enable: false, Precompute: 0, Threshold: 0},
}

// MainnetParameters for mainnet (21 nodes)
var MainnetParameters = Parameters{
	K:                     21,
	AlphaPreference:       13,
	AlphaConfidence:       18,
	Beta:                  8,
	DeltaMinMS:            50,
	ConcurrentPolls:       8,
	OptimalProcessing:     10,
	MaxOutstandingItems:   369,
	MaxItemProcessingTime: 963 * time.Millisecond, // 0.963s for mainnet consensus
	MinRoundInterval:      100 * time.Millisecond,
	FPC:                   DefaultFPC(), // FPC enabled by default
	Quasar:                QuasarConfig{Enable: true, Precompute: 2, Threshold: 15},
}

// GetParametersByName returns parameters by preset name
func GetParametersByName(name string) (Parameters, error) {
	switch name {
	case "test":
		return TestParameters, nil
	case "local":
		return LocalParameters, nil
	case "testnet":
		return TestnetParameters, nil
	case "mainnet":
		return MainnetParameters, nil
	default:
		return Parameters{}, errors.New("unknown preset: " + name)
	}
}

// GetPresetParameters is an alias for GetParametersByName to maintain compatibility
func GetPresetParameters(preset string) (Parameters, error) {
	return GetParametersByName(preset)
}

// PresetNames returns all available preset names
func PresetNames() []string {
	return []string{"mainnet", "testnet", "local", "test"}
}

// MainnetParams returns mainnet configuration with all features enabled
func MainnetParams() Parameters {
	return Parameters{
		K:                     21,
		AlphaPreference:       15,
		AlphaConfidence:       18,
		Beta:                  6,
		DeltaMinMS:            50,
		ConcurrentPolls:       8,
		OptimalProcessing:     10,
		MaxOutstandingItems:   369,
		MaxItemProcessingTime: 963 * time.Millisecond, // 0.963s for mainnet consensus
		MinRoundInterval:      100 * time.Millisecond,
		FPC:                   DefaultFPC(),
		Quasar:                QuasarConfig{Enable: true, Precompute: 2, Threshold: 15},
	}
}
