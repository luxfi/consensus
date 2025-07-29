// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQuantumParameters(t *testing.T) {
	// Test mainnet quantum parameters match LP-99
	require.Equal(t, 21, MainnetConfig.K)
	require.Equal(t, 13, MainnetConfig.AlphaPreference)
	require.Equal(t, 18, MainnetConfig.AlphaConfidence)
	require.Equal(t, 8, MainnetConfig.Beta)
	require.Equal(t, 15, MainnetConfig.QThreshold)
	require.Equal(t, 50*time.Millisecond, MainnetConfig.QuasarTimeout)
	
	// Test testnet quantum parameters
	require.Equal(t, 11, TestnetConfig.K)
	require.Equal(t, 7, TestnetConfig.AlphaPreference)
	require.Equal(t, 9, TestnetConfig.AlphaConfidence)
	require.Equal(t, 6, TestnetConfig.Beta)
	require.Equal(t, 8, TestnetConfig.QThreshold)
	require.Equal(t, 100*time.Millisecond, TestnetConfig.QuasarTimeout)
}

func TestQuantumThresholdValidation(t *testing.T) {
	params := Parameters{
		K:                     21,
		AlphaPreference:       13,
		AlphaConfidence:       18,
		Beta:                  8,
		ConcurrentRepolls:     8,
		OptimalProcessing:     10,
		MaxOutstandingItems:   256,
		MaxItemProcessingTime: 9630 * time.Millisecond,
		MinRoundInterval:      200 * time.Millisecond,
		QThreshold:            15,
		QuasarTimeout:         50 * time.Millisecond,
	}
	
	// Should be valid
	require.NoError(t, params.Valid())
	
	// Test QThreshold bounds - too low
	params.QThreshold = 0
	require.Error(t, params.Valid())
	
	// Test QThreshold bounds - too high
	params.QThreshold = 22
	require.Error(t, params.Valid())
}
