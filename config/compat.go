package config

import (
	"fmt"
	"time"
)

// Backwards-compatible preset Parameters used by many tools/tests.
var (
	DefaultParameters = Parameters{
		K:                     20,
		AlphaPreference:       15,
		AlphaConfidence:       15,
		Beta:                  20,
		ConcurrentPolls:       4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   256,
		MaxItemProcessingTime: 30 * time.Second,
		MinRoundInterval:      50 * time.Millisecond,
	}

	MainnetParameters = Mainnet()
	TestnetParameters = Testnet()
	LocalParameters   = Local()

	// TestParameters is a lightweight config used in unit tests.
	TestParameters = Parameters{
		K:                     2,
		AlphaPreference:       2,
		AlphaConfidence:       2,
		Beta:                  2,
		ConcurrentPolls:       1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   10,
		MaxItemProcessingTime: 10 * time.Second,
		MinRoundInterval:      10 * time.Millisecond,
	}
)

// GetParametersByName returns a preset by name.
func GetParametersByName(name string) (Parameters, error) {
	switch name {
	case "mainnet":
		return MainnetParameters, nil
	case "testnet":
		return TestnetParameters, nil
	case "local":
		return LocalParameters, nil
	case "test":
		return TestParameters, nil
	case "default":
		return DefaultParameters, nil
	default:
		return Parameters{}, fmt.Errorf("unknown preset: %s", name)
	}
}

// GetPresetParameters is an alias used by some CLI tools.
func GetPresetParameters(profile string) (Parameters, error) {
	return GetParametersByName(profile)
}
