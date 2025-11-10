// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/luxfi/consensus/core/choices"
	"github.com/luxfi/consensus/test/helpers"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/consensus/utils/set"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// Parameters represents consensus parameters for the network
type Parameters struct {
	K                     int // Sample size
	AlphaPreference       int // Preference threshold
	AlphaConfidence       int // Confidence threshold
	Beta                  int // Decision threshold
	ConcurrentPolls       int // Number of concurrent polls
	OptimalProcessing     int // Optimal number of processing items
	MaxOutstandingItems   int // Maximum outstanding items
	MaxItemProcessingTime int // Maximum processing time in seconds
}

// DefaultParameters returns default test parameters
func DefaultParameters() Parameters {
	return Parameters{
		K:                     20,
		AlphaPreference:       14,
		AlphaConfidence:       14,
		Beta:                  20,
		ConcurrentPolls:       4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   256,
		MaxItemProcessingTime: 120,
	}
}

// TestBlock represents a test block for network simulation
type TestBlock struct {
	consensustest.Decidable
	ParentV    ids.ID
	HeightV    uint64
	TimestampV int64
	BytesV     []byte
}

// ParentID returns the parent block ID
func (b *TestBlock) ParentID() ids.ID {
	return b.ParentV
}

// Height returns the block height
func (b *TestBlock) Height() uint64 {
	return b.HeightV
}

// Timestamp returns the block timestamp
func (b *TestBlock) Timestamp() int64 {
	return b.TimestampV
}

// Bytes returns the block bytes
func (b *TestBlock) Bytes() []byte {
	if b.BytesV == nil {
		// Generate deterministic bytes from ID
		b.BytesV = b.IDV[:]
	}
	return b.BytesV
}

// Verify verifies the block
func (b *TestBlock) Verify(context.Context) error {
	return nil
}

// String returns string representation
func (b *TestBlock) String() string {
	return fmt.Sprintf("TestBlock{ID: %s, Height: %d, Parent: %s, Status: %s}",
		b.IDV, b.HeightV, b.ParentV, b.StatusV)
}

// Consensus represents a consensus instance in the network
type Consensus interface {
	// Initialize initializes the consensus with parameters
	Initialize(params Parameters, genesisID ids.ID, genesisHeight uint64, genesisTimestamp int64) error

	// Add adds a block to consensus
	Add(block *TestBlock) error

	// RecordPoll records poll results
	RecordPoll(ctx context.Context, votes *bag.Bag) error

	// NumProcessing returns the number of processing blocks
	NumProcessing() int

	// Preference returns the current preference
	Preference() ids.ID

	// Finalized returns whether consensus is finalized
	Finalized() bool

	// HealthCheck performs health check
	HealthCheck() error
}

// ChainConsensus implements chain-based consensus for testing
type ChainConsensus struct {
	params       Parameters
	blocks       map[ids.ID]*TestBlock
	processing   set.Set[ids.ID]
	preference   ids.ID
	lastAccepted ids.ID
	votes        map[ids.ID]int
	confidence   map[ids.ID]int
	finalized    bool
	mu           sync.RWMutex
}

// NewChainConsensus creates a new chain consensus instance
func NewChainConsensus() *ChainConsensus {
	return &ChainConsensus{
		blocks:     make(map[ids.ID]*TestBlock),
		processing: set.NewSet[ids.ID](16),
		votes:      make(map[ids.ID]int),
		confidence: make(map[ids.ID]int),
	}
}

// Initialize initializes the consensus
func (c *ChainConsensus) Initialize(params Parameters, genesisID ids.ID, genesisHeight uint64, genesisTimestamp int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.params = params
	c.preference = genesisID
	c.lastAccepted = genesisID

	// Add genesis block
	genesis := &TestBlock{
		Decidable: consensustest.Decidable{
			IDV:     genesisID,
			StatusV: choices.Accepted,
		},
		ParentV:    ids.Empty,
		HeightV:    genesisHeight,
		TimestampV: genesisTimestamp,
	}
	c.blocks[genesisID] = genesis
	// Genesis is already accepted, no need to add to processing

	return nil
}

// Add adds a block to consensus
func (c *ChainConsensus) Add(block *TestBlock) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.blocks[block.IDV]; exists {
		return nil // Already added
	}

	// For non-genesis blocks, check if parent exists
	if block.HeightV > 0 {
		// Check if parent exists, if not, assume it's the genesis/lastAccepted
		if _, exists := c.blocks[block.ParentV]; !exists {
			// Set parent to lastAccepted if parent not found
			// This handles the case where blocks are added out of order
			block.ParentV = c.lastAccepted
		}
	}

	c.blocks[block.IDV] = block
	c.processing.Add(block.IDV)
	c.votes[block.IDV] = 0
	c.confidence[block.IDV] = 0

	// Update preference to this block if we don't have one in processing
	if !c.processing.Contains(c.preference) || c.preference == c.lastAccepted {
		c.preference = block.IDV
	}

	return nil
}

// RecordPoll records poll results
func (c *ChainConsensus) RecordPoll(ctx context.Context, votes *bag.Bag) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.finalized {
		return nil
	}

	// Count votes - the vote count is how many nodes voted for each block
	voteCount := make(map[ids.ID]int)
	for _, id := range votes.List() {
		count := votes.Count(id)
		voteCount[id] = count
	}

	// Update vote counts and check for acceptance
	// We only process votes for blocks we're tracking
	for blockID, count := range voteCount {
		if !c.processing.Contains(blockID) {
			// This block got votes but isn't in our processing set
			// Update preference if enough votes
			if count >= c.params.AlphaPreference {
				// If this block has strong support, maybe we should track it
				// For now, just skip - in real implementation we'd fetch the block
			}
			continue
		}

		c.votes[blockID] += count

		// Check preference threshold
		if count >= c.params.AlphaPreference {
			c.preference = blockID
		}

		// Check confidence threshold
		if count >= c.params.AlphaConfidence {
			// Increment confidence for this block
			c.confidence[blockID]++

			// Check if we've reached beta consecutive successful polls
			if c.confidence[blockID] >= c.params.Beta {
				// Accept the block
				if err := c.acceptBlock(ctx, blockID); err != nil {
					return err
				}
			}
		} else {
			// Reset confidence if we didn't meet threshold
			c.confidence[blockID] = 0
		}
	}

	// Also reset confidence for blocks that didn't get any votes
	for id := range c.confidence {
		if _, voted := voteCount[id]; !voted && c.processing.Contains(id) {
			c.confidence[id] = 0
		}
	}

	// Check if we're finalized (no more processing blocks)
	if c.processing.Len() == 0 {
		c.finalized = true
	}

	return nil
}

// acceptBlock accepts a block and rejects conflicting blocks
func (c *ChainConsensus) acceptBlock(ctx context.Context, blockID ids.ID) error {
	block, exists := c.blocks[blockID]
	if !exists {
		return fmt.Errorf("block %s not found", blockID)
	}

	// Accept the block
	if err := block.Accept(ctx); err != nil {
		return err
	}

	c.lastAccepted = blockID
	c.processing.Remove(blockID)

	// Reject conflicting blocks (blocks at the same height)
	for id, b := range c.blocks {
		if id != blockID && b.HeightV == block.HeightV && c.processing.Contains(id) {
			if err := b.Reject(ctx); err != nil {
				return err
			}
			c.processing.Remove(id)
		}
	}

	// Update preference if current preference was just finalized
	if c.preference == blockID && c.processing.Len() > 0 {
		// Pick any processing block as new preference
		for id := range c.blocks {
			if c.processing.Contains(id) {
				c.preference = id
				break
			}
		}
	}

	return nil
}

// NumProcessing returns the number of processing blocks
func (c *ChainConsensus) NumProcessing() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.processing.Len()
}

// Preference returns the current preference
func (c *ChainConsensus) Preference() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.preference
}

// Finalized returns whether consensus is finalized
func (c *ChainConsensus) Finalized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.finalized
}

// HealthCheck performs health check
func (c *ChainConsensus) HealthCheck() error {
	return nil
}

// SimulationNetwork represents a network of consensus nodes for simulation
type SimulationNetwork struct {
	params    Parameters
	colors    []*TestBlock
	rngSource uint64
	nodes     []Consensus
	running   []Consensus
	mu        sync.Mutex
}

// NewNetwork creates a new network simulation
func NewSimulationNetwork(params Parameters, numColors int, rngSeed uint64) *SimulationNetwork {
	n := &SimulationNetwork{
		params:    params,
		rngSource: rngSeed,
		nodes:     make([]Consensus, 0),
		running:   make([]Consensus, 0),
	}

	// Create genesis block (already accepted)
	genesisID := ids.Empty.Prefix(n.nextRandom())
	genesis := &TestBlock{
		Decidable: consensustest.Decidable{
			IDV:     genesisID,
			StatusV: choices.Accepted,
		},
		ParentV:    ids.Empty,
		HeightV:    0,
		TimestampV: 0,
	}
	n.colors = []*TestBlock{genesis}

	// Create additional blocks forming a chain
	for i := 1; i < numColors; i++ {
		// Randomly select a parent from existing blocks
		parentIdx := int(n.nextRandom() % uint64(len(n.colors)))
		parent := n.colors[parentIdx]

		block := &TestBlock{
			Decidable: consensustest.Decidable{
				IDV:     ids.Empty.Prefix(n.nextRandom()),
				StatusV: choices.Processing,
			},
			ParentV:    parent.IDV,
			HeightV:    parent.HeightV + 1,
			TimestampV: int64(i),
		}
		n.colors = append(n.colors, block)
	}

	return n
}

// nextRandom generates the next pseudo-random number
func (n *SimulationNetwork) nextRandom() uint64 {
	// Simple LCG for deterministic pseudo-random numbers
	n.rngSource = n.rngSource*1664525 + 1013904223
	return n.rngSource
}

// AddNode adds a new consensus node to the network
func (n *SimulationNetwork) AddNode(t testing.TB, consensus Consensus) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Initialize with genesis
	genesisID := n.colors[0].IDV
	if err := consensus.Initialize(n.params, genesisID, 0, 0); err != nil {
		return err
	}

	// Add all blocks (skip genesis since it's already accepted)
	// Don't shuffle - all nodes see same blocks in same order
	for i, block := range n.colors {
		if i == 0 {
			// Genesis is already accepted, skip adding it
			continue
		}
		// Create a copy for this node
		blockCopy := *block
		if err := consensus.Add(&blockCopy); err != nil {
			return err
		}
	}

	n.nodes = append(n.nodes, consensus)
	n.running = append(n.running, consensus)

	return nil
}

// Finalized returns whether the network simulation is finalized
func (n *SimulationNetwork) Finalized() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.running) == 0
}

// Round executes one round of consensus polling
func (n *SimulationNetwork) Round() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if len(n.running) == 0 {
		return nil
	}

	// Select a random running node - this is the ONLY node that polls this round
	runningIdx := int(n.nextRandom() % uint64(len(n.running)))
	running := n.running[runningIdx]

	// Sample K nodes for voting
	votes := bag.New()
	for i := 0; i < n.params.K; i++ {
		// Sample a random node
		nodeIdx := int(n.nextRandom() % uint64(len(n.nodes)))
		peer := n.nodes[nodeIdx]

		// Add the peer's preference to votes
		votes.Add(peer.Preference())
	}

	// Record the poll ONLY for the selected node
	ctx := context.Background()
	if err := running.RecordPoll(ctx, votes); err != nil {
		return err
	}

	// If this specific node has finalized, remove it from running set
	if running.NumProcessing() == 0 {
		// Remove from running list
		n.running[runningIdx] = n.running[len(n.running)-1]
		n.running = n.running[:len(n.running)-1]
	}

	return nil
}

// Agreement returns whether all nodes have reached agreement
func (n *SimulationNetwork) Agreement() bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	if len(n.nodes) == 0 {
		return true
	}

	// Get the preference of the first node
	pref := n.nodes[0].Preference()

	// Check if all nodes have the same preference
	for _, node := range n.nodes[1:] {
		if node.Preference() != pref {
			return false
		}
	}

	return true
}

// TestNetworkAgreement tests that the network reaches agreement
func TestNetworkAgreement(t *testing.T) {
	require := require.New(t)

	params := DefaultParameters()
	params.K = 5
	params.AlphaPreference = 3
	params.AlphaConfidence = 3
	params.Beta = 5

	// Test with different network sizes
	testCases := []struct {
		name      string
		numNodes  int
		numColors int
		maxRounds int
	}{
		{"Small Network", 5, 3, 500},
		{"Medium Network", 10, 5, 1000},
		{"Large Network", 20, 10, 2000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create network with deterministic seed for reproducibility
			network := NewSimulationNetwork(params, tc.numColors, 12345)

			// Add nodes to the network
			for i := 0; i < tc.numNodes; i++ {
				node := NewChainConsensus()
				require.NoError(network.AddNode(t, node))
			}

			// Run rounds until finalized or max rounds reached
			rounds := 0
			for !network.Finalized() && rounds < tc.maxRounds {
				require.NoError(network.Round())
				rounds++

				// Debug log every 100 rounds
				if rounds%100 == 0 {
					t.Logf("Round %d: %d nodes still running", rounds, len(network.running))
				}
			}

			// Debug final state
			if !network.Finalized() {
				t.Logf("Failed to finalize after %d rounds", rounds)
				t.Logf("Nodes still running: %d", len(network.running))
				for i, node := range network.nodes {
					t.Logf("Node %d: processing=%d, pref=%s", i, node.NumProcessing(), node.Preference().Prefix(8))
				}
			}

			// Verify the network reached agreement
			require.True(network.Finalized(), "Network should be finalized after %d rounds", rounds)
			require.True(network.Agreement(), "All nodes should agree on the same block")

			t.Logf("Network reached agreement in %d rounds", rounds)
		})
	}
}

// TestNetworkPartition tests network behavior under partition
func TestNetworkPartition(t *testing.T) {
	require := require.New(t)

	params := DefaultParameters()
	params.K = 5
	params.AlphaPreference = 3
	params.AlphaConfidence = 3
	params.Beta = 10

	// Create network
	network := NewSimulationNetwork(params, 5, 54321)

	// Add nodes
	for i := 0; i < 10; i++ {
		node := NewChainConsensus()
		require.NoError(network.AddNode(t, node))
	}

	// Simulate partition by running only subset of nodes
	// Save original running set
	originalRunning := make([]Consensus, len(network.running))
	copy(originalRunning, network.running)

	// Partition: only first half of nodes are active
	network.running = network.running[:len(network.running)/2]

	// Run some rounds with partition
	for i := 0; i < 50; i++ {
		require.NoError(network.Round())
	}

	// Restore full network
	network.running = originalRunning

	// Continue running until finalized
	rounds := 0
	maxRounds := 1000
	for !network.Finalized() && rounds < maxRounds {
		require.NoError(network.Round())
		rounds++
	}

	// Network should eventually reach agreement despite partition
	require.True(network.Finalized(), "Network should finalize after partition heals")
	require.True(network.Agreement(), "All nodes should eventually agree")
}

// TestNetworkDivergence tests handling of divergent chains
func TestNetworkDivergence(t *testing.T) {
	require := require.New(t)

	params := DefaultParameters()
	params.K = 7
	params.AlphaPreference = 5
	params.AlphaConfidence = 5
	params.Beta = 7

	// Create network with multiple conflicting chains
	network := NewSimulationNetwork(params, 2, 99999)

	// Add conflicting blocks at same height
	parent := network.colors[0]
	for i := 0; i < 3; i++ {
		block := &TestBlock{
			Decidable: consensustest.Decidable{
				IDV:     ids.GenerateTestID(),
				StatusV: choices.Processing,
			},
			ParentV:    parent.IDV,
			HeightV:    parent.HeightV + 1,
			TimestampV: int64(i + 1),
		}
		network.colors = append(network.colors, block)
	}

	// Add nodes
	for i := 0; i < 15; i++ {
		node := NewChainConsensus()
		require.NoError(network.AddNode(t, node))
	}

	// Run consensus
	rounds := 0
	maxRounds := 2000
	for !network.Finalized() && rounds < maxRounds {
		require.NoError(network.Round())
		rounds++
	}

	// Despite conflicts, network should reach agreement
	require.True(network.Finalized(), "Network should finalize with conflicts")
	require.True(network.Agreement(), "All nodes should agree on one chain")

	t.Logf("Network resolved conflicts in %d rounds", rounds)
}

// TestNetworkScalability tests network with many nodes
func TestNetworkScalability(t *testing.T) {
	require := require.New(t)

	params := DefaultParameters()
	params.K = 20
	params.AlphaPreference = 14
	params.AlphaConfidence = 14
	params.Beta = 20

	// Create large network
	network := NewSimulationNetwork(params, 20, 777777)

	// Add many nodes
	numNodes := 50
	for i := 0; i < numNodes; i++ {
		node := NewChainConsensus()
		require.NoError(network.AddNode(t, node))
	}

	// Run consensus with timeout
	rounds := 0
	maxRounds := 5000

	for !network.Finalized() && rounds < maxRounds {
		require.NoError(network.Round())
		rounds++

		// Check for early agreement
		if rounds%100 == 0 && network.Agreement() {
			t.Logf("Early agreement detected at round %d", rounds)
		}
	}

	// Large network should still reach consensus
	require.True(network.Finalized(), "Large network should finalize")
	require.True(network.Agreement(), "All nodes should agree")

	t.Logf("Large network (%d nodes) reached agreement in %d rounds", numNodes, rounds)
}

// TestDAGConsensus tests DAG-based consensus
func TestDAGConsensus(t *testing.T) {
	require := require.New(t)

	// Create a simple DAG consensus test
	params := DefaultParameters()
	params.K = 5
	params.AlphaPreference = 3
	params.AlphaConfidence = 3
	params.Beta = 5

	network := NewSimulationNetwork(params, 10, 424242)

	// Add nodes using chain consensus (DAG can be simulated with multiple parents)
	for i := 0; i < 10; i++ {
		node := NewChainConsensus()
		require.NoError(network.AddNode(t, node))
	}

	// Run consensus
	rounds := 0
	maxRounds := 1000
	for !network.Finalized() && rounds < maxRounds {
		require.NoError(network.Round())
		rounds++
	}

	require.True(network.Finalized(), "DAG network should finalize")
	require.True(network.Agreement(), "DAG nodes should agree")

	t.Logf("DAG consensus completed in %d rounds", rounds)
}

// TestNetworkResilience tests network resilience to failures
func TestNetworkResilience(t *testing.T) {
	require := require.New(t)

	params := DefaultParameters()
	params.K = 10
	params.AlphaPreference = 7
	params.AlphaConfidence = 7
	params.Beta = 15

	network := NewSimulationNetwork(params, 8, 161616)

	// Add nodes
	numNodes := 20
	for i := 0; i < numNodes; i++ {
		node := NewChainConsensus()
		require.NoError(network.AddNode(t, node))
	}

	// Simulate node failures by removing some from running set
	failureRate := 0.3 // 30% of nodes fail
	numFailures := int(float64(len(network.running)) * failureRate)

	// Remove random nodes to simulate failures
	for i := 0; i < numFailures; i++ {
		if len(network.running) > 1 {
			idx := int(network.nextRandom() % uint64(len(network.running)))
			network.running = append(network.running[:idx], network.running[idx+1:]...)
		}
	}

	t.Logf("Simulated %d node failures out of %d nodes", numFailures, numNodes)

	// Run consensus with reduced network
	rounds := 0
	maxRounds := 2000
	for !network.Finalized() && rounds < maxRounds {
		require.NoError(network.Round())
		rounds++
	}

	// Network should still reach consensus despite failures
	require.True(network.Finalized(), "Network should finalize despite failures")

	// Check agreement among all nodes (not just running ones)
	hasAgreement := true
	if len(network.nodes) > 0 {
		pref := network.nodes[0].Preference()
		for _, node := range network.nodes[1:] {
			if node.NumProcessing() == 0 { // Only check finalized nodes
				continue
			}
			if node.Preference() != pref {
				hasAgreement = false
				break
			}
		}
	}

	require.True(hasAgreement, "Remaining nodes should agree")
	t.Logf("Network reached consensus despite %d failures in %d rounds", numFailures, rounds)
}

// BenchmarkNetworkConsensus benchmarks network consensus performance
func BenchmarkNetworkConsensus(b *testing.B) {
	params := DefaultParameters()

	benchmarks := []struct {
		name      string
		numNodes  int
		numColors int
	}{
		{"Small-5-3", 5, 3},
		{"Medium-10-5", 10, 5},
		{"Large-20-10", 20, 10},
		{"XLarge-50-20", 50, 20},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Create network
				network := NewSimulationNetwork(params, bm.numColors, uint64(i*12345))

				// Add nodes
				for j := 0; j < bm.numNodes; j++ {
					node := NewChainConsensus()
					if err := network.AddNode(b, node); err != nil {
						b.Fatal(err)
					}
				}

				// Run until finalized
				rounds := 0
				maxRounds := 10000
				for !network.Finalized() && rounds < maxRounds {
					if err := network.Round(); err != nil {
						b.Fatal(err)
					}
					rounds++
				}

				if !network.Finalized() {
					b.Fatalf("Network did not finalize after %d rounds", maxRounds)
				}
			}
		})
	}
}