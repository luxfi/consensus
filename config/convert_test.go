// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigToParameters(t *testing.T) {
	require := require.New(t)

	config := Config{
		K:                     21,
		AlphaPreference:       13,
		AlphaConfidence:       18,
		Beta:                  8,
		ConcurrentPolls:       4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   100,
		MaxItemProcessingTime: 5 * time.Second,
		MinRoundInterval:      100 * time.Millisecond,
	}

	params := config.ToParameters()

	require.Equal(config.K, params.K)
	require.Equal(config.AlphaPreference, params.AlphaPreference)
	require.Equal(config.AlphaConfidence, params.AlphaConfidence)
	require.Equal(uint32(config.Beta), params.Beta)
	require.Equal(config.ConcurrentPolls, params.ConcurrentPolls)
	require.Equal(config.OptimalProcessing, params.OptimalProcessing)
	require.Equal(config.MaxOutstandingItems, params.MaxOutstandingItems)
	require.Equal(config.MaxItemProcessingTime, params.MaxItemProcessingTime)
	require.Equal(config.MinRoundInterval, params.MinRoundInterval)
}

func TestConfigToParametersWithAdvancedFields(t *testing.T) {
	require := require.New(t)

	config := Config{
		K:                     30,
		AlphaPreference:       20,
		AlphaConfidence:       25,
		Beta:                  10,
		ConcurrentPolls:       5,
		OptimalProcessing:     15,
		MaxOutstandingItems:   200,
		MaxItemProcessingTime: 10 * time.Second,
		MinRoundInterval:      200 * time.Millisecond,
		// Advanced fields that should be ignored
		MixedQueryNumPushVdr: 10,
		NetworkLatency:       50 * time.Millisecond,
		TotalNodes:           100,
		ExpectedFailureRate:  0.1,
	}

	params := config.ToParameters()

	// Core parameters should be converted
	require.Equal(config.K, params.K)
	require.Equal(config.AlphaPreference, params.AlphaPreference)
	require.Equal(config.AlphaConfidence, params.AlphaConfidence)
	require.Equal(uint32(config.Beta), params.Beta)
	require.Equal(config.ConcurrentPolls, params.ConcurrentPolls)
	require.Equal(config.OptimalProcessing, params.OptimalProcessing)
	require.Equal(config.MaxOutstandingItems, params.MaxOutstandingItems)
	require.Equal(config.MaxItemProcessingTime, params.MaxItemProcessingTime)
	require.Equal(config.MinRoundInterval, params.MinRoundInterval)

	// FPC and Quasar parameters should be default
	require.False(params.FPC.Enable)
	require.False(params.Quasar.Enable)
}

func TestConfigToParametersMinimal(t *testing.T) {
	require := require.New(t)

	// Test with minimal config
	config := Config{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 4,
		Beta:            2,
	}

	params := config.ToParameters()

	require.Equal(5, params.K)
	require.Equal(3, params.AlphaPreference)
	require.Equal(4, params.AlphaConfidence)
	require.Equal(uint32(2), params.Beta)

	// Default values should be zero
	require.Equal(0, params.ConcurrentPolls)
	require.Equal(0, params.OptimalProcessing)
	require.Equal(0, params.MaxOutstandingItems)
	require.Equal(time.Duration(0), params.MaxItemProcessingTime)
	require.Equal(time.Duration(0), params.MinRoundInterval)
}

func TestConfigToParametersWithDeltaMinMS(t *testing.T) {
	require := require.New(t)

	config := Config{
		K:                     21,
		AlphaPreference:       13,
		AlphaConfidence:       18,
		Beta:                  8,
		ConcurrentPolls:       4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   100,
		MaxItemProcessingTime: 5 * time.Second,
		MinRoundInterval:      100 * time.Millisecond,
	}

	params := config.ToParameters()

	// All parameters should be converted
	require.Equal(config.K, params.K)
	require.Equal(config.AlphaPreference, params.AlphaPreference)
	require.Equal(config.AlphaConfidence, params.AlphaConfidence)
	require.Equal(uint32(config.Beta), params.Beta)
	require.Equal(config.ConcurrentPolls, params.ConcurrentPolls)
	require.Equal(config.OptimalProcessing, params.OptimalProcessing)
	require.Equal(config.MaxOutstandingItems, params.MaxOutstandingItems)
	require.Equal(config.MaxItemProcessingTime, params.MaxItemProcessingTime)
	require.Equal(config.MinRoundInterval, params.MinRoundInterval)

	// FPC and Quasar parameters should be default (disabled)
	require.False(params.FPC.Enable)
	require.False(params.Quasar.Enable)
}

func BenchmarkConfigToParameters(b *testing.B) {
	config := Config{
		K:                     21,
		AlphaPreference:       13,
		AlphaConfidence:       18,
		Beta:                  8,
		ConcurrentPolls:       4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   100,
		MaxItemProcessingTime: 5 * time.Second,
		MinRoundInterval:      100 * time.Millisecond,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.ToParameters()
	}
}
