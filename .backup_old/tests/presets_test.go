// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultParameters(t *testing.T) {
	require := require.New(t)

	// Verify DefaultParameters are valid
	err := DefaultParameters.Valid()
	require.NoError(err)

	// Check expected values
	require.Equal(20, DefaultParameters.K)
	require.Equal(15, DefaultParameters.AlphaPreference)
	require.Equal(15, DefaultParameters.AlphaConfidence)
	require.Equal(uint32(20), DefaultParameters.Beta)
	require.Equal(30*time.Second, DefaultParameters.MaxItemProcessingTime)
}

func TestMainnetParameters(t *testing.T) {
	require := require.New(t)

	// Verify MainnetParameters are valid
	err := MainnetParameters.Valid()
	require.NoError(err)

	// Check expected values
	require.Equal(21, MainnetParameters.K)
	require.Equal(13, MainnetParameters.AlphaPreference)
	require.Equal(18, MainnetParameters.AlphaConfidence)
	require.Equal(uint32(8), MainnetParameters.Beta)
	require.Equal(963*time.Millisecond, MainnetParameters.MaxItemProcessingTime) // 0.963s for mainnet
	require.Equal(100*time.Millisecond, MainnetParameters.MinRoundInterval)
}

func TestTestnetParameters(t *testing.T) {
	require := require.New(t)

	// Verify TestnetParameters are valid
	err := TestnetParameters.Valid()
	require.NoError(err)

	// Check expected values
	require.Equal(11, TestnetParameters.K)
	require.Equal(7, TestnetParameters.AlphaPreference)
	require.Equal(9, TestnetParameters.AlphaConfidence)
	require.Equal(uint32(6), TestnetParameters.Beta)
	require.Equal(630*time.Millisecond, TestnetParameters.MaxItemProcessingTime) // 0.63s for testnet
	require.Equal(50*time.Millisecond, TestnetParameters.MinRoundInterval)
}

func TestTestParameters(t *testing.T) {
	require := require.New(t)

	// Verify TestParameters are valid
	err := TestParameters.Valid()
	require.NoError(err)

	// Check expected values
	require.Equal(2, TestParameters.K)
	require.Equal(2, TestParameters.AlphaPreference)
	require.Equal(2, TestParameters.AlphaConfidence)
	require.Equal(uint32(2), TestParameters.Beta)
	require.Equal(10*time.Second, TestParameters.MaxItemProcessingTime)
	require.Equal(10*time.Millisecond, TestParameters.MinRoundInterval)
}

func TestLocalParameters(t *testing.T) {
	require := require.New(t)

	// Verify LocalParameters are valid
	err := LocalParameters.Valid()
	require.NoError(err)

	// Check expected values
	require.Equal(5, LocalParameters.K)
	require.Equal(3, LocalParameters.AlphaPreference)
	require.Equal(4, LocalParameters.AlphaConfidence)
	require.Equal(uint32(3), LocalParameters.Beta)
	require.Equal(369*time.Millisecond, LocalParameters.MaxItemProcessingTime) // 0.369s for local
	require.Equal(10*time.Millisecond, LocalParameters.MinRoundInterval)
}

func TestGetParametersByName(t *testing.T) {
	tests := []struct {
		name     string
		network  string
		expected Parameters
		wantErr  bool
	}{
		{
			name:     "mainnet",
			network:  "mainnet",
			expected: MainnetParameters,
			wantErr:  false,
		},
		{
			name:     "testnet",
			network:  "testnet",
			expected: TestnetParameters,
			wantErr:  false,
		},
		{
			name:     "test",
			network:  "test",
			expected: TestParameters,
			wantErr:  false,
		},
		{
			name:     "local",
			network:  "local",
			expected: LocalParameters,
			wantErr:  false,
		},
		{
			name:     "invalid",
			network:  "invalid-network",
			expected: Parameters{},
			wantErr:  true,
		},
		{
			name:     "empty string",
			network:  "",
			expected: Parameters{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			params, err := GetParametersByName(tt.network)
			if tt.wantErr {
				require.Error(err)
				require.Contains(err.Error(), "unknown preset")
			} else {
				require.NoError(err)
				require.Equal(tt.expected, params)
			}
		})
	}
}

func TestPresetConsistency(t *testing.T) {
	require := require.New(t)

	presets := []struct {
		name   string
		params Parameters
	}{
		{"mainnet", MainnetParameters},
		{"testnet", TestnetParameters},
		{"test", TestParameters},
		{"local", LocalParameters},
	}

	for _, preset := range presets {
		t.Run(preset.name, func(t *testing.T) {
			// Verify AlphaPreference > K/2
			require.Greater(preset.params.AlphaPreference, preset.params.K/2)

			// Verify AlphaConfidence >= AlphaPreference
			require.GreaterOrEqual(preset.params.AlphaConfidence, preset.params.AlphaPreference)

			// Verify AlphaConfidence <= K
			require.LessOrEqual(preset.params.AlphaConfidence, preset.params.K)

			// Verify Beta > 0 and <= K
			require.Greater(preset.params.Beta, uint32(0))
			require.LessOrEqual(int(preset.params.Beta), preset.params.K)

			// Verify ConcurrentPolls > 0
			require.Greater(preset.params.ConcurrentPolls, 0)

			// Verify OptimalProcessing > 0
			require.Greater(preset.params.OptimalProcessing, 0)

			// Verify MaxOutstandingItems >= OptimalProcessing
			require.GreaterOrEqual(preset.params.MaxOutstandingItems, preset.params.OptimalProcessing)

			// Verify MaxItemProcessingTime > 0
			require.Greater(preset.params.MaxItemProcessingTime, time.Duration(0))

			// Verify MinRoundInterval is within expected range
			require.GreaterOrEqual(preset.params.MinRoundInterval, 1*time.Millisecond)
			require.LessOrEqual(preset.params.MinRoundInterval, 500*time.Millisecond)
		})
	}
}

func TestPresetNetworkCharacteristics(t *testing.T) {
	require := require.New(t)

	// Test that mainnet has the highest security parameters
	require.Greater(MainnetParameters.AlphaConfidence, TestnetParameters.AlphaConfidence)
	require.Greater(MainnetParameters.K, LocalParameters.K)

	// Test that local has the fastest consensus
	require.Less(LocalParameters.MaxItemProcessingTime, MainnetParameters.MaxItemProcessingTime)
	require.Less(LocalParameters.K, MainnetParameters.K)

	// Test that test parameters have a low MinRoundInterval for fast testing
	require.Equal(10*time.Millisecond, TestParameters.MinRoundInterval)
	require.Equal(10*time.Millisecond, LocalParameters.MinRoundInterval)
}

func TestPresetScaling(t *testing.T) {
	require := require.New(t)

	// Verify that processing time scales with network size
	require.Greater(MainnetParameters.MaxItemProcessingTime, TestnetParameters.MaxItemProcessingTime)
	require.Greater(TestnetParameters.MaxItemProcessingTime, LocalParameters.MaxItemProcessingTime)

	// Verify that K scales with network requirements
	require.Equal(21, MainnetParameters.K)
	require.Equal(11, TestnetParameters.K)
	require.Equal(5, LocalParameters.K)
}

func BenchmarkGetParametersByName(b *testing.B) {
	networks := []string{"mainnet", "testnet", "test", "local"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GetParametersByName(networks[i%len(networks)])
	}
}

func TestMinPercentConnectedHealthyForPresets(t *testing.T) {
	tests := []struct {
		name     string
		params   Parameters
		minRatio float64
		maxRatio float64
	}{
		{
			name:     "mainnet",
			params:   MainnetParameters,
			minRatio: 0.85,
			maxRatio: 0.90,
		},
		{
			name:     "testnet",
			params:   TestnetParameters,
			minRatio: 0.85,
			maxRatio: 0.90,
		},
		{
			name:     "local",
			params:   LocalParameters,
			minRatio: 0.80,
			maxRatio: 0.85,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			ratio := tt.params.MinPercentConnectedHealthy()
			require.GreaterOrEqual(ratio, tt.minRatio)
			require.LessOrEqual(ratio, tt.maxRatio)
		})
	}
}
