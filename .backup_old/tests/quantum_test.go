// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQuantumParameters(t *testing.T) {
	// Test mainnet parameters
	require.Equal(t, 21, MainnetParameters.K)
	require.Equal(t, 13, MainnetParameters.AlphaPreference)
	require.Equal(t, 18, MainnetParameters.AlphaConfidence)
	require.Equal(t, uint32(8), MainnetParameters.Beta)
	require.Equal(t, 50, MainnetParameters.DeltaMinMS)
	require.True(t, MainnetParameters.FPC.Enable)
	require.True(t, MainnetParameters.Quasar.Enable)

	// Test testnet parameters
	require.Equal(t, 11, TestnetParameters.K)
	require.Equal(t, 7, TestnetParameters.AlphaPreference)
	require.Equal(t, 9, TestnetParameters.AlphaConfidence)
	require.Equal(t, uint32(6), TestnetParameters.Beta)
	require.Equal(t, 50, TestnetParameters.DeltaMinMS)
	require.True(t, TestnetParameters.FPC.Enable)
}

func TestQuantumThresholdValidation(t *testing.T) {
	params := Parameters{
		K:                     21,
		AlphaPreference:       13,
		AlphaConfidence:       18,
		Beta:                  8,
		DeltaMinMS:            50,
		ConcurrentPolls:       8,
		OptimalProcessing:     10,
		MaxOutstandingItems:   256,
		MaxItemProcessingTime: 9630 * time.Millisecond,
		MinRoundInterval:      200 * time.Millisecond,
		FPC:                   DefaultFPC(),
		Quasar:                QuasarConfig{Enable: true, Precompute: 2, Threshold: 15},
	}

	// Should be valid
	require.NoError(t, params.Validate())

	// Test Quasar threshold bounds
	params.Quasar.Threshold = 0
	require.NoError(t, params.Validate()) // 0 means use default

	// Test invalid Beta
	params.Beta = 0
	require.Error(t, params.Validate())
}
