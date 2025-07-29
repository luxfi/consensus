// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

// GetPresetParameters is an alias for GetParametersByName to maintain compatibility
func GetPresetParameters(preset string) (Parameters, error) {
	return GetParametersByName(preset)
}

// PresetNames returns all available preset names.
func PresetNames() []string {
	return []string{"mainnet", "testnet", "local", "hightps"}
}