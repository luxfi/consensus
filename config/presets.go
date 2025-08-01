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
	QThreshold:            15,
	QuasarTimeout:         50 * time.Millisecond,
	ConcurrentPolls:     4,
	OptimalProcessing:     10,
	MaxOutstandingItems:   256,
	MaxItemProcessingTime: 30 * time.Second,
	MinRoundInterval:      100 * time.Millisecond,
}

// TestParameters returns parameters suitable for testing
var TestParameters = Parameters{
	K:                     2,
	AlphaPreference:       2,
	AlphaConfidence:       2,
	Beta:                  2,
	QThreshold:            2,
	QuasarTimeout:         10 * time.Millisecond,
	ConcurrentPolls:     1,
	OptimalProcessing:     1,
	MaxOutstandingItems:   16,
	MaxItemProcessingTime: 10 * time.Second,
	MinRoundInterval:      10 * time.Millisecond,
}

// LocalParameters for local development (5 nodes)
var LocalParameters = Parameters{
	K:                     5,
	AlphaPreference:       3,
	AlphaConfidence:       4,
	Beta:                  3,
	QThreshold:            4,
	QuasarTimeout:         30 * time.Millisecond,
	ConcurrentPolls:     2,
	OptimalProcessing:     3,
	MaxOutstandingItems:   50,
	MaxItemProcessingTime: 3690 * time.Millisecond,
	MinRoundInterval:      10 * time.Millisecond,
}

// TestnetParameters for testnet (11 nodes)
var TestnetParameters = Parameters{
	K:                     11,
	AlphaPreference:       7,
	AlphaConfidence:       9,
	Beta:                  6,
	QThreshold:            8,
	QuasarTimeout:         100 * time.Millisecond,
	ConcurrentPolls:     4,
	OptimalProcessing:     5,
	MaxOutstandingItems:   100,
	MaxItemProcessingTime: 6300 * time.Millisecond,
	MinRoundInterval:      50 * time.Millisecond,
}

// MainnetParameters for mainnet (21 nodes)
var MainnetParameters = Parameters{
	K:                     21,
	AlphaPreference:       13,
	AlphaConfidence:       18,
	Beta:                  8,
	QThreshold:            15,
	QuasarTimeout:         50 * time.Millisecond,
	ConcurrentPolls:     8,
	OptimalProcessing:     10,
	MaxOutstandingItems:   369,
	MaxItemProcessingTime: 9630 * time.Millisecond,
	MinRoundInterval:      100 * time.Millisecond,
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