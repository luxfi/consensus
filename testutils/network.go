// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package testutils

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/luxfi/ids"
)

// Network simulates an in-memory network for testing consensus
type Network struct {
	mu sync.RWMutex
	
	// nodes tracks all nodes in the network
	nodes map[ids.NodeID]*Node
	
	// latency defines the latency between nodes (ms)
	latency map[ids.NodeID]map[ids.NodeID]int
	
	// partitions tracks network partitions
	partitions [][]ids.NodeID
	
	// dropRate is the probability of dropping a message (0.0-1.0)
	dropRate float64
	
	// rng for randomness
	rng *rand.Rand
}

// Node represents a node in the test network
type Node struct {
	ID       ids.NodeID
	Inbox    chan Message
	Outbox   chan Message
	Latency  time.Duration // default latency for this node
}

// Message represents a message in the network
type Message struct {
	From      ids.NodeID
	To        ids.NodeID
	Type      string
	Payload   interface{}
	Timestamp time.Time
	Content   []byte
}

// NewNetwork creates a new test network
func NewNetwork(seed int64) *Network {
	return &Network{
		nodes:    make(map[ids.NodeID]*Node),
		latency:  make(map[ids.NodeID]map[ids.NodeID]int),
		rng:      rand.New(rand.NewSource(seed)),
		dropRate: 0.0,
	}
}

// AddNode adds a node to the network
func (n *Network) AddNode(nodeID ids.NodeID, defaultLatency time.Duration) *Node {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	node := &Node{
		ID:      nodeID,
		Inbox:   make(chan Message, 1000),
		Outbox:  make(chan Message, 1000),
		Latency: defaultLatency,
	}
	
	n.nodes[nodeID] = node
	return node
}

// SetLatency sets the latency between two nodes
func (n *Network) SetLatency(from, to ids.NodeID, latencyMs int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	if n.latency[from] == nil {
		n.latency[from] = make(map[ids.NodeID]int)
	}
	n.latency[from][to] = latencyMs
}

// SetDropRate sets the message drop rate (0.0-1.0)
func (n *Network) SetDropRate(rate float64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.dropRate = rate
}

// Partition creates a network partition
func (n *Network) Partition(groups ...[]ids.NodeID) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.partitions = groups
}

// Heal removes all network partitions
func (n *Network) Heal() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.partitions = nil
}

// Start starts the network simulation
func (n *Network) Start(ctx context.Context) {
	// Start message routing for each node
	for _, node := range n.nodes {
		go n.routeMessages(ctx, node)
	}
}

// routeMessages handles message routing for a node
func (n *Network) routeMessages(ctx context.Context, node *Node) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-node.Outbox:
			// Check if message should be dropped
			if n.shouldDrop() {
				continue
			}
			
			// Check if nodes are partitioned
			if n.arePartitioned(msg.From, msg.To) {
				continue
			}
			
			// Get latency
			latency := n.getLatency(msg.From, msg.To)
			
			// Deliver message after latency
			go func(m Message, delay time.Duration) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
					n.mu.RLock()
					if target, ok := n.nodes[m.To]; ok {
						n.mu.RUnlock()
						select {
						case target.Inbox <- m:
						case <-ctx.Done():
						}
					} else {
						n.mu.RUnlock()
					}
				}
			}(msg, latency)
		}
	}
}

// shouldDrop returns whether a message should be dropped
func (n *Network) shouldDrop() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.rng.Float64() < n.dropRate
}

// arePartitioned checks if two nodes are in different partitions
func (n *Network) arePartitioned(from, to ids.NodeID) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	if len(n.partitions) == 0 {
		return false
	}
	
	var fromGroup, toGroup int = -1, -1
	for i, group := range n.partitions {
		for _, id := range group {
			if id == from {
				fromGroup = i
			}
			if id == to {
				toGroup = i
			}
		}
	}
	
	return fromGroup != -1 && toGroup != -1 && fromGroup != toGroup
}

// getLatency returns the latency between two nodes
func (n *Network) getLatency(from, to ids.NodeID) time.Duration {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	// Check custom latency
	if fromLatency, ok := n.latency[from]; ok {
		if toLatency, ok := fromLatency[to]; ok {
			return time.Duration(toLatency) * time.Millisecond
		}
	}
	
	// Use default latency
	if node, ok := n.nodes[from]; ok {
		return node.Latency
	}
	
	return 50 * time.Millisecond // default 50ms
}

// Stop stops the network simulation
func (n *Network) Stop() {
	// Nothing to do for in-memory network
}

// SendAsync sends a message asynchronously
func (n *Network) SendAsync(ctx context.Context, msg *Message) {
	n.mu.RLock()
	from, ok := n.nodes[msg.From]
	n.mu.RUnlock()
	
	if !ok {
		return
	}
	
	select {
	case from.Outbox <- *msg:
	case <-ctx.Done():
	}
}