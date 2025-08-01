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
	require.Equal(t, 21, MainnetParameters.K)
	require.Equal(t, 13, MainnetParameters.AlphaPreference)
	require.Equal(t, 18, MainnetParameters.AlphaConfidence)
	require.Equal(t, 8, MainnetParameters.Beta)
	require.Equal(t, 15, MainnetParameters.QThreshold)
	require.Equal(t, 50*time.Millisecond, MainnetParameters.QuasarTimeout)
	
	// Test testnet quantum parameters
	require.Equal(t, 11, TestnetParameters.K)
	require.Equal(t, 7, TestnetParameters.AlphaPreference)
	require.Equal(t, 9, TestnetParameters.AlphaConfidence)
	require.Equal(t, 6, TestnetParameters.Beta)
	require.Equal(t, 8, TestnetParameters.QThreshold)
	require.Equal(t, 100*time.Millisecond, TestnetParameters.QuasarTimeout)
}

func TestQuantumThresholdValidation(t *testing.T) {
	params := Parameters{
		K:                     21,
		AlphaPreference:       13,
		AlphaConfidence:       18,
		Beta:                  8,
		ConcurrentPolls:     8,
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
