// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beam

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gonum.org/v1/gonum/mathext/prng"

	"github.com/luxfi/consensus/photon"
	"github.com/luxfi/consensus/wave"
)

// TestConvergenceWavePhoton tests that consensus is reached when
// mixing wave and photon factories
func TestConvergenceWavePhoton(t *testing.T) {
	require := require.New(t)

	params := photon.Parameters{
		K:                     20,
		AlphaPreference:       11,
		AlphaConfidence:       11,
		Beta:                  20,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}

	for peerCount := 20; peerCount < 2000; peerCount *= 10 {
		numNodes := peerCount

		t.Run(fmt.Sprintf("%d nodes", numNodes), func(t *testing.T) {
			n := NewNetwork(params, 10, prng.NewMT19937())
			for i := 0; i < numNodes; i++ {
				var sbFactory photon.Factory
				if i%2 == 0 {
					sbFactory = wave.WaveFactory
				} else {
					sbFactory = photon.PhotonFactory
				}

				factory := TopologicalFactory{factory: sbFactory}
				sm := factory.New()
				require.NoError(n.AddNode(t, sm))
			}

			rounds := 0
			maxRounds := 100 // Prevent infinite loops in tests
			
			for !n.Finalized() && rounds < maxRounds {
				require.NoError(n.Round())
				rounds++
			}

			require.True(n.Finalized())
			require.True(n.Agreement())
		})
	}
}

// TestConvergenceMixedThreshold tests consensus with different alpha thresholds
func TestConvergenceMixedThreshold(t *testing.T) {
	require := require.New(t)

	baseParams := photon.Parameters{
		K:                     20,
		AlphaPreference:       11,
		AlphaConfidence:       11,
		Beta:                  20,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}

	numNodes := 50

	// Test with different threshold configurations
	testCases := []struct {
		name            string
		alphaPreference int
		alphaConfidence int
	}{
		{"low threshold", 8, 8},
		{"medium threshold", 11, 11},
		{"high threshold", 15, 15},
		{"mixed threshold", 10, 14},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params := baseParams
			params.AlphaPreference = tc.alphaPreference
			params.AlphaConfidence = tc.alphaConfidence

			n := NewNetwork(params, 10, prng.NewMT19937())
			
			for i := 0; i < numNodes; i++ {
				var sbFactory photon.Factory
				// Mix different factories
				if i%3 == 0 {
					sbFactory = wave.WaveFactory
				} else {
					sbFactory = photon.PhotonFactory
				}

				factory := TopologicalFactory{factory: sbFactory}
				sm := factory.New()
				require.NoError(n.AddNode(t, sm))
			}

			rounds := 0
			maxRounds := 100
			
			for !n.Finalized() && rounds < maxRounds {
				require.NoError(n.Round())
				rounds++
			}

			require.True(n.Finalized())
			require.True(n.Agreement())
		})
	}
}