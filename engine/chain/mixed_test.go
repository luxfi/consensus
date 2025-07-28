// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	consensuslog "github.com/luxfi/consensus/log"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/factories"
	"github.com/luxfi/consensus/poll"
)

func TestConvergenceSampling(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
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
			n := NewNetwork(params, 10, NewMT19937Source())
			for i := 0; i < numNodes; i++ {
				var sbFactory poll.Factory
				if i%2 == 0 {
					sbFactory = poll.DefaultFactory
				} else {
					params := factories.Parameters{
						K:               params.K,
						AlphaPreference: params.AlphaPreference,
						AlphaConfidence: params.AlphaConfidence,
						Beta:            params.Beta,
					}
					sbFactory = factories.NewConfidenceFactory(consensuslog.NewNoOpLogger(), prometheus.NewRegistry(), params)
				}

				factory := TopologicalFactory{factory: sbFactory}
				sm := factory.New()
				require.NoError(n.AddNode(t, sm))
			}

			for !n.Finalized() {
				require.NoError(n.Round())
			}

			require.True(n.Agreement())
		})
	}
}
