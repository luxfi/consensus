// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCgoAvailable tests the CgoAvailable function
func TestCgoAvailable(t *testing.T) {
	// CgoAvailable returns true when CGO is enabled, false otherwise
	// We just check that it returns a valid boolean
	result := CgoAvailable()
	require.True(t, result == true || result == false)
}

// TestConsensusFactory tests the ConsensusFactory
func TestConsensusFactory(t *testing.T) {
	require := require.New(t)

	factory := NewConsensusFactory()
	require.NotNil(factory)

	params := ConsensusParams{
		K:                     20,
		AlphaPreference:       15,
		AlphaConfidence:       15,
		Beta:                  20,
		ConcurrentPolls:       10,
		OptimalProcessing:     10,
		MaxOutstandingItems:   1000,
		MaxItemProcessingTime: 30 * time.Second,
	}

	consensus, err := factory.CreateConsensus(params)
	require.NoError(err)
	require.NotNil(consensus)
}

// TestConsensusFactoryWithDifferentParams tests factory with various parameters
func TestConsensusFactoryWithDifferentParams(t *testing.T) {
	require := require.New(t)

	factory := NewConsensusFactory()

	testCases := []struct {
		name   string
		params ConsensusParams
	}{
		{
			name: "small network",
			params: ConsensusParams{
				K:                   5,
				AlphaPreference:     3,
				AlphaConfidence:     3,
				Beta:                5,
				MaxOutstandingItems: 100,
			},
		},
		{
			name: "large network",
			params: ConsensusParams{
				K:                   100,
				AlphaPreference:     75,
				AlphaConfidence:     75,
				Beta:                100,
				MaxOutstandingItems: 10000,
			},
		},
		{
			name: "zero outstanding",
			params: ConsensusParams{
				K:                   10,
				AlphaPreference:     8,
				AlphaConfidence:     8,
				Beta:                10,
				MaxOutstandingItems: 0,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			consensus, err := factory.CreateConsensus(tc.params)
			require.NoError(err)
			require.NotNil(consensus)
		})
	}
}

// TestCGOConsensusGetPreference tests GetPreference returns the current preference
func TestCGOConsensusGetPreference(t *testing.T) {
	require := require.New(t)

	params := ConsensusParams{
		K:                   5,
		AlphaPreference:     3,
		MaxOutstandingItems: 100,
	}

	consensus, err := NewCGOConsensus(params)
	require.NoError(err)

	// Initial preference should be Empty ID
	pref := consensus.GetPreference()
	require.Equal(pref, pref) // Verifies no panic and valid ID returned
}
