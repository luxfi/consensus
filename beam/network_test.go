// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beam

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gonum.org/v1/gonum/mathext/prng"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/photon"
	"github.com/luxfi/consensus/beam/beamtest"
	"github.com/luxfi/consensus/test/consensustest"
	"github.com/luxfi/consensus/utils/sampler"
	"github.com/luxfi/consensus/utils"
)


// Network is a test helper for managing multiple consensus instances
type Network struct {
	params         photon.Parameters
	colors         []*beamtest.Block
	rngSource      sampler.Source
	nodes, running []Consensus
}

// NewNetwork creates a new test network
func NewNetwork(params photon.Parameters, numColors int, rngSource sampler.Source) *Network {
	n := &Network{
		params: params,
		colors: []*beamtest.Block{{
			Decidable: consensustest.Decidable{
				IDV:     ids.Empty.Prefix(rngSource.Uint64()),
				StatusV: choices.Unknown,
			},
			ParentV:    beamtest.GenesisID,
			HeightV:    beamtest.GenesisHeight + 1,
			TimestampV: beamtest.GenesisTimestamp,
		}},
		rngSource: rngSource,
	}

	s := sampler.NewDeterministicUniform(0) // Use a temporary seed
	for i := 1; i < numColors; i++ {
		s.Initialize(len(n.colors))
		indices, _ := s.Sample(1)
		dependency := n.colors[indices[0]]
		n.colors = append(n.colors, &beamtest.Block{
			Decidable: consensustest.Decidable{
				IDV:     ids.Empty.Prefix(rngSource.Uint64()),
				StatusV: choices.Unknown,
			},
			ParentV:    dependency.IDV,
			HeightV:    dependency.HeightV + 1,
			TimestampV: dependency.TimestampV,
		})
	}
	return n
}

func (n *Network) shuffleColors() {
	s := sampler.NewDeterministicUniform(int64(n.rngSource.Uint64() % (1 << 63))) // Convert uint64 to int64 range
	s.Initialize(len(n.colors))
	indices, _ := s.Sample(len(n.colors))
	colors := []*beamtest.Block(nil)
	for _, index := range indices {
		colors = append(colors, n.colors[int(index)])
	}
	n.colors = colors
	utils.Sort(n.colors)
}

func (n *Network) AddNode(t testing.TB, sm Consensus) error {
	ctx := consensustest.BeamContext(t, consensustest.CChainID)
	if err := sm.Initialize(ctx, n.params, beamtest.GenesisID, beamtest.GenesisHeight, beamtest.GenesisTimestamp); err != nil {
		return err
	}

	n.shuffleColors()
	for _, blk := range n.colors {
		copiedBlk := *blk
		if err := sm.Add(&copiedBlk); err != nil {
			return err
		}
	}
	n.nodes = append(n.nodes, sm)
	n.running = append(n.running, sm)
	return nil
}

func (n *Network) Finalized() bool {
	return len(n.running) == 0
}

func (n *Network) Round() error {
	if len(n.running) == 0 {
		return nil
	}

	s := sampler.NewDeterministicUniform(int64(n.rngSource.Uint64() % (1 << 63))) // Convert uint64 to int64 range
	s.Initialize(len(n.running))

	indices, _ := s.Sample(1)
	runningInd := indices[0]
	running := n.running[runningInd]

	s.Initialize(len(n.nodes))
	peerIndices, _ := s.Sample(n.params.K)
	sampledColors := bag.Bag[ids.ID]{}
	for _, index := range peerIndices {
		peer := n.nodes[index]
		sampledColors.Add(peer.Preference())
	}

	if err := running.RecordPoll(context.Background(), sampledColors); err != nil {
		return err
	}

	// If this node has been finalized, remove it from the poller
	if running.NumProcessing() == 0 {
		newSize := len(n.running) - 1
		n.running[runningInd] = n.running[newSize]
		n.running = n.running[:newSize]
	}

	return nil
}

func (n *Network) Agreement() bool {
	if len(n.nodes) == 0 {
		return true
	}
	pref := n.nodes[0].Preference()
	for _, node := range n.nodes {
		if pref != node.Preference() {
			return false
		}
	}
	return true
}

// TestNetworkBasic tests the basic functionality of the Network struct
func TestNetworkBasic(t *testing.T) {
	require := require.New(t)

	params := photon.Parameters{
		K:                     2,
		AlphaPreference:       2,
		AlphaConfidence:       2,
		Beta:                  1,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}

	source := prng.NewMT19937()
	source.Seed(42)
	n := NewNetwork(params, 5, source)

	// Verify network was created
	require.NotNil(n)
	require.Len(n.colors, 5)
	require.Empty(n.nodes)
	require.Empty(n.running)

	// Check that blocks were created with proper parent-child relationships
	for i := 1; i < len(n.colors); i++ {
		block := n.colors[i]
		require.NotEqual(ids.Empty, block.ID())
		require.Equal(choices.Unknown, block.Status())
		require.Greater(block.Height(), uint64(0))
	}
}