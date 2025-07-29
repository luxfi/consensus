// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

import (
	"math/rand"
	"sync"

	"github.com/luxfi/ids"
)

// TestNetwork simulates a consensus network for testing.
// It provides an in-memory network without any transport layer.
type TestNetwork struct {
	mu        sync.RWMutex
	params    Parameters
	nodes     []Consensus
	running   []Consensus
	choices   []ids.ID
	rng       *rand.Rand
	factory   Factory
}

// NewTestNetwork creates a new test network.
func NewTestNetwork(params Parameters, factory Factory, numChoices int, seed int64) *TestNetwork {
	n := &TestNetwork{
		params:  params,
		factory: factory,
		rng:     rand.New(rand.NewSource(seed)),
		choices: make([]ids.ID, numChoices),
	}
	
	// Generate choice IDs
	for i := 0; i < numChoices; i++ {
		n.choices[i] = ids.GenerateTestID()
	}
	
	return n
}

// AddNode adds a node with a random initial preference.
func (n *TestNetwork) AddNode() Consensus {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	// Select random initial choice
	choice := n.choices[n.rng.Intn(len(n.choices))]
	node := n.factory.NewConsensus(n.params, choice)
	
	// Add all choices to the node
	for _, c := range n.choices {
		if c != choice {
			node.Add(c)
		}
	}
	
	n.nodes = append(n.nodes, node)
	if !node.Finalized() {
		n.running = append(n.running, node)
	}
	
	return node
}

// AddNodeWithChoice adds a node with a specific initial preference.
func (n *TestNetwork) AddNodeWithChoice(choiceIndex int) Consensus {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	if choiceIndex >= len(n.choices) {
		choiceIndex = 0
	}
	
	choice := n.choices[choiceIndex]
	node := n.factory.NewConsensus(n.params, choice)
	
	// Add all choices to the node
	for _, c := range n.choices {
		if c != choice {
			node.Add(c)
		}
	}
	
	n.nodes = append(n.nodes, node)
	if !node.Finalized() {
		n.running = append(n.running, node)
	}
	
	return node
}

// Round executes one round of consensus.
func (n *TestNetwork) Round() {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	if len(n.running) == 0 {
		return
	}
	
	// Select a random running node
	nodeIdx := n.rng.Intn(len(n.running))
	node := n.running[nodeIdx]
	
	// Sample K nodes
	k := n.params.K()
	if k > len(n.nodes) {
		k = len(n.nodes)
	}
	
	// Create vote bag
	bag := Bag[ids.ID]{}
	
	// Sample nodes with replacement
	for i := 0; i < k; i++ {
		sampledIdx := n.rng.Intn(len(n.nodes))
		sampled := n.nodes[sampledIdx]
		bag.Add(sampled.Preference())
	}
	
	// Record the poll
	if !node.RecordPoll(bag) {
		node.RecordUnsuccessfulPoll()
	}
	
	// Remove finalized nodes from running list
	if node.Finalized() {
		n.running[nodeIdx] = n.running[len(n.running)-1]
		n.running = n.running[:len(n.running)-1]
	}
}

// Finalized returns true if all nodes have finalized.
func (n *TestNetwork) Finalized() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	return len(n.running) == 0
}

// Agreement returns true if all nodes prefer the same choice.
func (n *TestNetwork) Agreement() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	if len(n.nodes) == 0 {
		return true
	}
	
	pref := n.nodes[0].Preference()
	for _, node := range n.nodes[1:] {
		if node.Preference() != pref {
			return false
		}
	}
	
	return true
}

// Disagreement returns true if finalized nodes disagree.
func (n *TestNetwork) Disagreement() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	var finalizedPref ids.ID
	foundFinalized := false
	
	for _, node := range n.nodes {
		if node.Finalized() {
			if !foundFinalized {
				finalizedPref = node.Preference()
				foundFinalized = true
			} else if node.Preference() != finalizedPref {
				return true
			}
		}
	}
	
	return false
}

// Choices returns the available choices in the network.
func (n *TestNetwork) Choices() []ids.ID {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	choices := make([]ids.ID, len(n.choices))
	copy(choices, n.choices)
	return choices
}

// Nodes returns all nodes in the network.
func (n *TestNetwork) Nodes() []Consensus {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	nodes := make([]Consensus, len(n.nodes))
	copy(nodes, n.nodes)
	return nodes
}

// Running returns all running (non-finalized) nodes.
func (n *TestNetwork) Running() []Consensus {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	running := make([]Consensus, len(n.running))
	copy(running, n.running)
	return running
}