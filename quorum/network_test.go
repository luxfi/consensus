// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quorum

import (
	"fmt"
	"math/rand"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus"
	"github.com/luxfi/consensus/photon"
	"github.com/luxfi/consensus/utils"
	"github.com/luxfi/consensus/utils/sampler"
)

// Network simulates a consensus network for testing
type Network struct {
	params    photon.Parameters
	colors    []ids.ID
	nodes     []ConsensusNode
	running   []ConsensusNode
	rngSource *rand.Rand
}

// ConsensusNode interface for network testing
type ConsensusNode interface {
	Add(choice ids.ID)
	Preference() ids.ID
	RecordPoll(votes *utils.Bag) bool
	RecordUnsuccessfulPoll()
	Finalized() bool
	String() string
}

// NewNetwork creates a new test network
func NewNetwork(params photon.Parameters, numColors int, seed int64) *Network {
	n := &Network{
		params:    params,
		colors:    make([]ids.ID, numColors),
		rngSource: rand.New(rand.NewSource(seed)),
	}
	
	// Generate color IDs
	for i := 0; i < numColors; i++ {
		n.colors[i] = ids.GenerateTestID()
	}
	
	return n
}

// AddNode adds a new consensus node with random initial preference
func (n *Network) AddNode(node ConsensusNode) {
	// Initialize with a random color preference
	s := sampler.NewUniform()
	s.Initialize(len(n.colors))
	
	indices := make([]uint64, len(n.colors))
	for i := range indices {
		indices[i] = uint64(n.rngSource.Intn(len(n.colors)))
	}
	
	// Add the first color as initial preference
	node.Add(n.colors[indices[0]])
	
	// Add remaining colors as options
	for _, idx := range indices[1:] {
		node.Add(n.colors[idx])
	}
	
	n.nodes = append(n.nodes, node)
	if !node.Finalized() {
		n.running = append(n.running, node)
	}
}

// AddNodeSpecificColor adds a node with specific initial preference
func (n *Network) AddNodeSpecificColor(node ConsensusNode, initialPreference int, options []int) {
	// Add initial preference
	node.Add(n.colors[initialPreference])
	
	// Add other options
	for _, opt := range options {
		if opt < len(n.colors) {
			node.Add(n.colors[opt])
		}
	}
	
	n.nodes = append(n.nodes, node)
	if !node.Finalized() {
		n.running = append(n.running, node)
	}
}

// Finalized returns true if all nodes have finalized
func (n *Network) Finalized() bool {
	return len(n.running) == 0
}

// Round simulates one round of consensus polling
func (n *Network) Round() {
	if len(n.running) == 0 {
		return
	}
	
	// Select a random running node
	runningIdx := n.rngSource.Intn(len(n.running))
	running := n.running[runningIdx]
	
	// Sample K nodes for polling
	numSamples := n.params.K
	if numSamples > len(n.nodes) {
		numSamples = len(n.nodes)
	}
	
	// Create vote bag
	votes := utils.NewBag()
	
	// Sample nodes and collect votes
	sampled := make(map[int]bool)
	for i := 0; i < numSamples; i++ {
		// Sample without replacement
		var nodeIdx int
		for {
			nodeIdx = n.rngSource.Intn(len(n.nodes))
			if !sampled[nodeIdx] {
				sampled[nodeIdx] = true
				break
			}
			// If we've sampled all nodes, break
			if len(sampled) >= len(n.nodes) {
				break
			}
		}
		
		node := n.nodes[nodeIdx]
		pref := node.Preference()
		votes.Add(pref)
	}
	
	// Record the poll result
	successful := running.RecordPoll(votes)
	if !successful {
		running.RecordUnsuccessfulPoll()
	}
	
	// Remove finalized nodes from running list
	if running.Finalized() {
		n.running[runningIdx] = n.running[len(n.running)-1]
		n.running = n.running[:len(n.running)-1]
	}
}

// Disagreement returns true if nodes have finalized on different values
func (n *Network) Disagreement() bool {
	var finalizedPref ids.ID
	foundFinalized := false
	
	for _, node := range n.nodes {
		if node.Finalized() {
			if !foundFinalized {
				finalizedPref = node.Preference()
				foundFinalized = true
			} else if finalizedPref != node.Preference() {
				return true
			}
		}
	}
	
	return false
}

// Agreement returns true if all nodes prefer the same value
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

// Byzantine represents a byzantine node for testing
type Byzantine struct {
	preference ids.ID
	choices    map[ids.ID]bool
}

// NewByzantine creates a new byzantine node
func NewByzantine() *Byzantine {
	return &Byzantine{
		choices: make(map[ids.ID]bool),
	}
}

func (b *Byzantine) Add(choice ids.ID) {
	b.choices[choice] = true
	if b.preference == ids.Empty {
		b.preference = choice
	}
}

func (b *Byzantine) Preference() ids.ID {
	return b.preference
}

func (b *Byzantine) RecordPoll(votes *utils.Bag) bool {
	// Byzantine node doesn't update preference based on polls
	return false
}

func (b *Byzantine) RecordUnsuccessfulPoll() {
	// No-op for byzantine nodes
}

func (b *Byzantine) Finalized() bool {
	// Byzantine nodes never finalize
	return false
}

func (b *Byzantine) String() string {
	return fmt.Sprintf("Byzantine{pref=%s}", b.preference)
}