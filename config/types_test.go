// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParametersGetters(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
	}

	require.Equal(21, params.GetK())
	require.Equal(13, params.GetAlphaPreference())
	require.Equal(18, params.GetAlphaConfidence())
	require.Equal(8, params.GetBeta())
}

func TestMinPercentConnectedHealthy(t *testing.T) {
	tests := []struct {
		name     string
		params   Parameters
		expected float64
	}{
		{
			name: "mainnet parameters",
			params: Parameters{
				K:               21,
				AlphaPreference: 13,
				AlphaConfidence: 18,
			},
			expected: (18.0/21.0)*0.8 + 0.2, // ~0.886
		},
		{
			name: "testnet parameters",
			params: Parameters{
				K:               11,
				AlphaPreference: 7,
				AlphaConfidence: 9,
			},
			expected: (9.0/11.0)*0.8 + 0.2, // ~0.854
		},
		{
			name: "small network",
			params: Parameters{
				K:               5,
				AlphaPreference: 3,
				AlphaConfidence: 4,
			},
			expected: (4.0/5.0)*0.8 + 0.2, // 0.84
		},
		{
			name: "high confidence",
			params: Parameters{
				K:               100,
				AlphaPreference: 60,
				AlphaConfidence: 90,
			},
			expected: (90.0/100.0)*0.8 + 0.2, // 0.92
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			result := tt.params.MinPercentConnectedHealthy()
			require.InDelta(tt.expected, result, 0.001)
		})
	}
}

func TestParametersValid(t *testing.T) {
	tests := []struct {
		name    string
		params  Parameters
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid mainnet parameters",
			params: Parameters{
				K:                     21,
				AlphaPreference:       13,
				AlphaConfidence:       18,
				Beta:                  8,
				ConcurrentPolls:       4,
				OptimalProcessing:     10,
				MaxOutstandingItems:   100,
				MaxItemProcessingTime: 5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "K too low",
			params: Parameters{
				K: 0,
			},
			wantErr: true,
			errMsg:  "k = 0: fails the condition that: 0 < k",
		},
		{
			name: "AlphaPreference too low",
			params: Parameters{
				K:               10,
				AlphaPreference: 5, // Should be > K/2
			},
			wantErr: true,
			errMsg:  "k = 10, alphaPreference = 5: fails the condition that: k/2 < alphaPreference",
		},
		{
			name: "AlphaPreference too high",
			params: Parameters{
				K:               10,
				AlphaPreference: 11,
			},
			wantErr: true,
			errMsg:  "k = 10, alphaPreference = 11: fails the condition that: alphaPreference <= k",
		},
		{
			name: "AlphaConfidence too small",
			params: Parameters{
				K:               10,
				AlphaPreference: 7,
				AlphaConfidence: 6, // Should be >= AlphaPreference
			},
			wantErr: true,
			errMsg:  "alphaPreference = 7, alphaConfidence = 6: fails the condition that: alphaPreference <= alphaConfidence",
		},
		{
			name: "AlphaConfidence too high",
			params: Parameters{
				K:               10,
				AlphaPreference: 7,
				AlphaConfidence: 11,
			},
			wantErr: true,
			errMsg:  "k = 10, alphaConfidence = 11: fails the condition that: alphaConfidence <= k",
		},
		{
			name: "Beta too low",
			params: Parameters{
				K:               10,
				AlphaPreference: 7,
				AlphaConfidence: 8,
				Beta:            0,
			},
			wantErr: true,
			errMsg:  "beta = 0: fails the condition that: 0 < beta",
		},
		{
			name: "Beta too high",
			params: Parameters{
				K:               10,
				AlphaPreference: 7,
				AlphaConfidence: 8,
				Beta:            11,
			},
			wantErr: true,
			errMsg:  "beta (11) must be <= k (10)",
		},
		{
			name: "ConcurrentPolls too low",
			params: Parameters{
				K:               10,
				AlphaPreference: 7,
				AlphaConfidence: 8,
				Beta:            5,
				ConcurrentPolls: 0,
			},
			wantErr: true,
			errMsg:  "concurrentPolls = 0: fails the condition that: 0 < concurrentPolls",
		},
		{
			name: "OptimalProcessing too low",
			params: Parameters{
				K:                 10,
				AlphaPreference:   7,
				AlphaConfidence:   8,
				Beta:              5,
				ConcurrentPolls:   2,
				OptimalProcessing: 0,
			},
			wantErr: true,
			errMsg:  "optimalProcessing = 0: fails the condition that: 0 < optimalProcessing",
		},
		{
			name: "MaxOutstandingItems too low",
			params: Parameters{
				K:                   10,
				AlphaPreference:     7,
				AlphaConfidence:     8,
				Beta:                5,
				ConcurrentPolls:     2,
				OptimalProcessing:   5,
				MaxOutstandingItems: 0,
			},
			wantErr: true,
			errMsg:  "maxOutstandingItems = 0: fails the condition that: 0 < maxOutstandingItems",
		},
		{
			name: "MaxItemProcessingTime too low",
			params: Parameters{
				K:                     10,
				AlphaPreference:       7,
				AlphaConfidence:       8,
				Beta:                  5,
				ConcurrentPolls:       2,
				OptimalProcessing:     5,
				MaxOutstandingItems:   10,
				MaxItemProcessingTime: 0,
			},
			wantErr: true,
			errMsg:  "maxItemProcessingTime = 0: fails the condition that: 0 < maxItemProcessingTime",
		},
		{
			name: "valid with quantum parameters",
			params: Parameters{
				K:                     21,
				AlphaPreference:       13,
				AlphaConfidence:       18,
				Beta:                  8,
				ConcurrentPolls:       4,
				OptimalProcessing:     10,
				MaxOutstandingItems:   100,
				MaxItemProcessingTime: 5 * time.Second,
				DeltaMinMS:            50,
				MinRoundInterval:      100 * time.Millisecond,
			},
			wantErr: false,
		},
		{
			name: "invalid quantum threshold",
			params: Parameters{
				K:                     21,
				AlphaPreference:       13,
				AlphaConfidence:       18,
				Beta:                  8,
				ConcurrentPolls:       4,
				OptimalProcessing:     10,
				MaxOutstandingItems:   100,
				MaxItemProcessingTime: 5 * time.Second,
				DeltaMinMS:            0, // Should be positive
				MinRoundInterval:      100 * time.Millisecond,
			},
			wantErr: true,
			errMsg:  "qThreshold must be positive when set",
		},
		{
			name: "beta too high",
			params: Parameters{
				K:                     21,
				AlphaPreference:       13,
				AlphaConfidence:       18,
				Beta:                  8,
				ConcurrentPolls:       4,
				OptimalProcessing:     10,
				MaxOutstandingItems:   100,
				MaxItemProcessingTime: 5 * time.Second,
				DeltaMinMS:            50,
				MinRoundInterval:      100 * time.Millisecond,
			},
			wantErr: true,
			errMsg:  "qThreshold (22) must be <= k (21)",
		},
		{
			name: "invalid quantum timeout",
			params: Parameters{
				K:                     21,
				AlphaPreference:       13,
				AlphaConfidence:       18,
				Beta:                  8,
				ConcurrentPolls:       4,
				OptimalProcessing:     10,
				MaxOutstandingItems:   100,
				MaxItemProcessingTime: 5 * time.Second,
				DeltaMinMS:            50,
				MinRoundInterval:      0, // Should be positive
			},
			wantErr: true,
			errMsg:  "quasarTimeout must be positive when set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			err := tt.params.Validate()
			if tt.wantErr {
				require.Error(err)
				if tt.errMsg != "" {
					require.Contains(err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(err)
			}
		})
	}
}

func TestParametersValidEdgeCases(t *testing.T) {
	require := require.New(t)

	// Test AlphaPreference boundary
	params := Parameters{
		K:                     10,
		AlphaPreference:       6, // Exactly K/2 + 1
		AlphaConfidence:       8,
		Beta:                  5,
		ConcurrentPolls:       2,
		OptimalProcessing:     5,
		MaxOutstandingItems:   10,
		MaxItemProcessingTime: 1 * time.Second,
	}
	require.NoError(params.Validate())

	// Test AlphaConfidence = AlphaPreference (should be valid)
	params.AlphaConfidence = params.AlphaPreference
	require.NoError(params.Validate())

	// Test Beta = K (should be valid)
	params.Beta = uint32(params.K)
	require.NoError(params.Validate())

	// Test minimal valid configuration
	minParams := Parameters{
		K:                     1,
		AlphaPreference:       1,
		AlphaConfidence:       1,
		Beta:                  1,
		ConcurrentPolls:       1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1 * time.Nanosecond,
	}
	require.NoError(minParams.Validate())
}

func TestParametersValidQuantumOnlyOne(t *testing.T) {
	require := require.New(t)

	// Valid base parameters
	baseParams := Parameters{
		K:                     21,
		AlphaPreference:       13,
		AlphaConfidence:       18,
		Beta:                  8,
		ConcurrentPolls:       4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   100,
		MaxItemProcessingTime: 5 * time.Second,
	}

	// Test with FPC disabled
	params := baseParams
	params.FPC.Enable = false
	err := params.Validate()
	require.NoError(err)

	// Test with FPC enabled
	params = baseParams
	params.FPC = DefaultFPC()
	err = params.Validate()
	require.NoError(err)

	// Test with Quasar enabled
	params = baseParams
	params.Quasar = QuasarConfig{Enable: true, Precompute: 2, Threshold: 15}
	require.NoError(params.Validate())

	// Test with both FPC and Quasar enabled
	params = baseParams
	params.FPC = DefaultFPC()
	params.Quasar = QuasarConfig{Enable: true, Precompute: 2, Threshold: 15}
	require.NoError(params.Validate())
}

func BenchmarkParametersValid(b *testing.B) {
	params := Parameters{
		K:                     21,
		AlphaPreference:       13,
		AlphaConfidence:       18,
		Beta:                  8,
		ConcurrentPolls:       4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   100,
		MaxItemProcessingTime: 5 * time.Second,
		DeltaMinMS:            50,
		MinRoundInterval:      100 * time.Millisecond,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = params.Validate()
	}
}

func BenchmarkMinPercentConnectedHealthy(b *testing.B) {
	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = params.MinPercentConnectedHealthy()
	}
}
