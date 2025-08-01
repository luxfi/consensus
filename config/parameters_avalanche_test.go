// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestParametersVerifyAvalancheCompatibility tests parameter validation with
// all the same test cases as Avalanche to ensure compatibility
func TestParametersVerifyAvalancheCompatibility(t *testing.T) {
	tests := []struct {
		name          string
		params        Parameters
		wantError     bool
		errorContains string
	}{
		{
			name: "valid minimal",
			params: Parameters{
				K:                     1,
				AlphaPreference:       1,
				AlphaConfidence:       1,
				Beta:                  1,
				ConcurrentPolls:    1,
				OptimalProcessing:     1,
				MaxOutstandingItems:   1,
				MaxItemProcessingTime: 1,
			},
			wantError: false,
		},
		{
			name: "invalid K=0",
			params: Parameters{
				K:                     0,
				AlphaPreference:       1,
				AlphaConfidence:       1,
				Beta:                  1,
				ConcurrentPolls:    1,
				OptimalProcessing:     1,
				MaxOutstandingItems:   1,
				MaxItemProcessingTime: 1,
			},
			wantError:     true,
			errorContains: "k",
		},
		{
			name: "invalid AlphaPreference <= K/2",
			params: Parameters{
				K:                     2,
				AlphaPreference:       1, // 1 <= 2/2
				AlphaConfidence:       1,
				Beta:                  1,
				ConcurrentPolls:    1,
				OptimalProcessing:     1,
				MaxOutstandingItems:   1,
				MaxItemProcessingTime: 1,
			},
			wantError:     true,
			errorContains: "alphaPreference",
		},
		{
			name: "invalid AlphaPreference=0",
			params: Parameters{
				K:                     1,
				AlphaPreference:       0,
				AlphaConfidence:       1,
				Beta:                  1,
				ConcurrentPolls:    1,
				OptimalProcessing:     1,
				MaxOutstandingItems:   1,
				MaxItemProcessingTime: 1,
			},
			wantError:     true,
			errorContains: "alphaPreference",
		},
		{
			name: "invalid AlphaConfidence < AlphaPreference",
			params: Parameters{
				K:                     3,
				AlphaPreference:       3,
				AlphaConfidence:       2,
				Beta:                  1,
				ConcurrentPolls:    1,
				OptimalProcessing:     1,
				MaxOutstandingItems:   1,
				MaxItemProcessingTime: 1,
			},
			wantError:     true,
			errorContains: "alphaConfidence",
		},
		{
			name: "invalid beta=0",
			params: Parameters{
				K:                     1,
				AlphaPreference:       1,
				AlphaConfidence:       1,
				Beta:                  0,
				ConcurrentPolls:    1,
				OptimalProcessing:     1,
				MaxOutstandingItems:   1,
				MaxItemProcessingTime: 1,
			},
			wantError:     true,
			errorContains: "beta",
		},
		{
			name: "valid high alphaPreference",
			params: Parameters{
				K:                     30,
				AlphaPreference:       28,
				AlphaConfidence:       30,
				Beta:                  2,
				ConcurrentPolls:    1,
				OptimalProcessing:     1,
				MaxOutstandingItems:   1,
				MaxItemProcessingTime: 1,
			},
			wantError: false,
		},
		{
			name: "valid alphaConfidence=K",
			params: Parameters{
				K:                     3,
				AlphaPreference:       2,
				AlphaConfidence:       3,
				Beta:                  2,
				ConcurrentPolls:    1,
				OptimalProcessing:     1,
				MaxOutstandingItems:   1,
				MaxItemProcessingTime: 1,
			},
			wantError: false,
		},
		{
			name: "too few ConcurrentPolls",
			params: Parameters{
				K:                     1,
				AlphaPreference:       1,
				AlphaConfidence:       1,
				Beta:                  1,
				ConcurrentPolls:    0,
				OptimalProcessing:     1,
				MaxOutstandingItems:   1,
				MaxItemProcessingTime: 1,
			},
			wantError:     true,
			errorContains: "concurrentPolls",
		},
		{
			name: "too many ConcurrentPolls",
			params: Parameters{
				K:                     1,
				AlphaPreference:       1,
				AlphaConfidence:       1,
				Beta:                  1,
				ConcurrentPolls:    2, // > Beta
				OptimalProcessing:     1,
				MaxOutstandingItems:   1,
				MaxItemProcessingTime: 1,
			},
			wantError:     true,
			errorContains: "concurrentPolls",
		},
		{
			name: "invalid OptimalProcessing",
			params: Parameters{
				K:                     1,
				AlphaPreference:       1,
				AlphaConfidence:       1,
				Beta:                  1,
				ConcurrentPolls:    1,
				OptimalProcessing:     0,
				MaxOutstandingItems:   1,
				MaxItemProcessingTime: 1,
			},
			wantError:     true,
			errorContains: "optimalProcessing",
		},
		{
			name: "invalid MaxOutstandingItems",
			params: Parameters{
				K:                     1,
				AlphaPreference:       1,
				AlphaConfidence:       1,
				Beta:                  1,
				ConcurrentPolls:    1,
				OptimalProcessing:     1,
				MaxOutstandingItems:   0,
				MaxItemProcessingTime: 1,
			},
			wantError:     true,
			errorContains: "maxOutstandingItems",
		},
		{
			name: "invalid MaxItemProcessingTime",
			params: Parameters{
				K:                     1,
				AlphaPreference:       1,
				AlphaConfidence:       1,
				Beta:                  1,
				ConcurrentPolls:    1,
				OptimalProcessing:     1,
				MaxOutstandingItems:   1,
				MaxItemProcessingTime: 0,
			},
			wantError:     true,
			errorContains: "maxItemProcessingTime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.Valid()
			if tt.wantError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestMinPercentConnectedHealthyAvalancheCompatibility tests the MinPercentConnectedHealthy
// calculation matches Avalanche's implementation
func TestMinPercentConnectedHealthyAvalancheCompatibility(t *testing.T) {
	tests := []struct {
		name                        string
		params                      Parameters
		expectedMinPercentConnected float64
	}{
		{
			name:                        "default parameters",
			params:                      DefaultParameters,
			expectedMinPercentConnected: 0.8, // (15/20)*0.8 + 0.2 = 0.6 + 0.2 = 0.8
		},
		{
			name: "custom K=5",
			params: Parameters{
				K:               5,
				AlphaConfidence: 4,
			},
			expectedMinPercentConnected: 0.84, // (4/5)*0.8 + 0.2 = 0.64 + 0.2 = 0.84
		},
		{
			name: "large K",
			params: Parameters{
				K:               1001,
				AlphaConfidence: 501,
			},
			expectedMinPercentConnected: 0.6, // (501/1001)*0.8 + 0.2 ≈ 0.4 + 0.2 = 0.6
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minStake := tt.params.MinPercentConnectedHealthy()
			require.InEpsilon(t, tt.expectedMinPercentConnected, minStake, 0.001)
		})
	}
}

// TestAllPresetsValid ensures all our presets pass validation
func TestAllPresetsValid(t *testing.T) {
	presets := map[string]Parameters{
		"default": DefaultParameters,
		"test":    TestParameters,
		"local":   LocalParameters,
		"testnet": TestnetParameters,
		"mainnet": MainnetParameters,
	}

	for name, params := range presets {
		t.Run(name, func(t *testing.T) {
			err := params.Valid()
			require.NoError(t, err, "preset %s should be valid", name)

			// Also verify MinPercentConnectedHealthy returns a reasonable value
			minPercent := params.MinPercentConnectedHealthy()
			require.Greater(t, minPercent, 0.0)
			require.LessOrEqual(t, minPercent, 1.0)
		})
	}
}

// TestParameterEdgeCases tests edge cases in parameter validation
func TestParameterEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		params Parameters
		valid  bool
	}{
		{
			name: "K=1 with all 1s",
			params: Parameters{
				K:                     1,
				AlphaPreference:       1,
				AlphaConfidence:       1,
				Beta:                  1,
				ConcurrentPolls:    1,
				OptimalProcessing:     1,
				MaxOutstandingItems:   1,
				MaxItemProcessingTime: 1,
			},
			valid: true,
		},
		{
			name: "very large K",
			params: Parameters{
				K:                     10000,
				AlphaPreference:       5001,
				AlphaConfidence:       7500,
				Beta:                  100,
				ConcurrentPolls:    10,
				OptimalProcessing:     50,
				MaxOutstandingItems:   1000,
				MaxItemProcessingTime: time.Minute,
			},
			valid: true,
		},
		{
			name: "alphaPreference > K",
			params: Parameters{
				K:                     10,
				AlphaPreference:       11,
				AlphaConfidence:       11,
				Beta:                  5,
				ConcurrentPolls:    2,
				OptimalProcessing:     5,
				MaxOutstandingItems:   10,
				MaxItemProcessingTime: time.Second,
			},
			valid: false,
		},
		{
			name: "beta > K",
			params: Parameters{
				K:                     10,
				AlphaPreference:       7,
				AlphaConfidence:       8,
				Beta:                  11,
				ConcurrentPolls:    2,
				OptimalProcessing:     5,
				MaxOutstandingItems:   10,
				MaxItemProcessingTime: time.Second,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.Valid()
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestQuantumParametersValidation tests our quantum-specific parameters
func TestQuantumParametersValidation(t *testing.T) {
	tests := []struct {
		name      string
		params    Parameters
		wantError bool
	}{
		{
			name: "valid quantum parameters",
			params: Parameters{
				K:                     20,
				AlphaPreference:       15,
				AlphaConfidence:       15,
				Beta:                  20,
				QThreshold:            15,
				QuasarTimeout:         50 * time.Millisecond,
				ConcurrentPolls:    4,
				OptimalProcessing:     10,
				MaxOutstandingItems:   256,
				MaxItemProcessingTime: 30 * time.Second,
			},
			wantError: false,
		},
		{
			name: "zero quantum parameters (should be valid - optional)",
			params: Parameters{
				K:                     20,
				AlphaPreference:       15,
				AlphaConfidence:       15,
				Beta:                  20,
				QThreshold:            0,
				QuasarTimeout:         0,
				ConcurrentPolls:    4,
				OptimalProcessing:     10,
				MaxOutstandingItems:   256,
				MaxItemProcessingTime: 30 * time.Second,
			},
			wantError: false,
		},
		{
			name: "invalid QThreshold > K",
			params: Parameters{
				K:                     20,
				AlphaPreference:       15,
				AlphaConfidence:       15,
				Beta:                  20,
				QThreshold:            21,
				QuasarTimeout:         50 * time.Millisecond,
				ConcurrentPolls:    4,
				OptimalProcessing:     10,
				MaxOutstandingItems:   256,
				MaxItemProcessingTime: 30 * time.Second,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.Valid()
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}