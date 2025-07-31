// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package tests

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/core/utils"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/consensus/utils/sampler"
)

type newConsensusFunc func(factory *utils.Factory, params config.Parameters, choice ids.ID) interfaces.Consensus

type TestNetwork struct {
	params         config.Parameters
	colors         []ids.ID
	rngSource      sampler.Source
	nodes, running []interfaces.Consensus
	factory        *utils.Factory
}

// Create a new network with [numColors] different possible colors to finalize.
func NewTestNetwork(factory *utils.Factory, params config.Parameters, numColors int, rngSource sampler.Source) *TestNetwork {
	n := &TestNetwork{
		params:    params,
		rngSource: rngSource,
		factory:   factory,
	}
	for i := 0; i < numColors; i++ {
		n.colors = append(n.colors, ids.Empty.Prefix(uint64(i)))
	}
	return n
}

func (n *TestNetwork) AddNode(newConsensusFunc newConsensusFunc) interfaces.Consensus {
	seed := n.rngSource.Uint64()
	s := sampler.NewDeterministicUniform(int64(seed))
	s.Initialize(len(n.colors))
	indices, _ := s.Sample(len(n.colors))

	consensus := newConsensusFunc(n.factory, n.params, n.colors[int(indices[0])])
	for _, index := range indices[1:] {
		consensus.Add(n.colors[int(index)])
	}

	n.nodes = append(n.nodes, consensus)
	if !consensus.Finalized() {
		n.running = append(n.running, consensus)
	}
	return consensus
}

func (n *TestNetwork) Finalized() bool {
	return len(n.running) == 0
}

func (n *TestNetwork) Round() {
	if len(n.running) == 0 {
		return
	}

	votes := bag.Bag[ids.ID]{}
	for _, node := range n.nodes {
		votes.Add(node.Preference())
	}

	// Update the preference of each node
	newlyFinalized := []interfaces.Consensus{}
	for _, node := range n.running {
		if node.RecordVotes(votes) == nil {
			if node.Finalized() {
				newlyFinalized = append(newlyFinalized, node)
			}
		}
	}

	// Remove newly finalized nodes from the running list
	for _, node := range newlyFinalized {
		for i := 0; i < len(n.running); i++ {
			if n.running[i] == node {
				n.running = append(n.running[:i], n.running[i+1:]...)
				break
			}
		}
	}
}

func (n *TestNetwork) Agreement() bool {
	if len(n.nodes) == 0 {
		return true
	}
	
	pref := n.nodes[0].Preference()
	for _, node := range n.nodes[1:] {
		if pref != node.Preference() {
			return false
		}
	}
	return true
}