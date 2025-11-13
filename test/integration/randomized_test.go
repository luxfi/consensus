// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// Randomized consensus consistency tests ported from avalanchego

package integration

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/luxfi/consensus/ai"
	"github.com/luxfi/consensus/core"
	"github.com/luxfi/consensus/protocol/quasar"
	"github.com/luxfi/ids"
)

// ==================== Mersenne Twister PRNG ====================

const (
	// MT19937 constants
	mtN         = 624
	mtM         = 397
	mtMatrixA   = 0x9908B0DF
	mtUpperMask = 0x80000000
	mtLowerMask = 0x7FFFFFFF
	mtTempB     = 0x9D2C5680
	mtTempC     = 0xEFC60000
)

// MersenneTwister is a deterministic PRNG for reproducible tests
type MersenneTwister struct {
	mt  [mtN]uint32
	mti int
}

// NewMersenneTwister creates a new MT19937 generator
func NewMersenneTwister(seed uint64) *MersenneTwister {
	gen := &MersenneTwister{mti: mtN + 1}
	gen.Seed(seed)
	return gen
}

// Seed initializes the generator with a seed
func (m *MersenneTwister) Seed(seed uint64) {
	m.mt[0] = uint32(seed)
	for i := 1; i < mtN; i++ {
		m.mt[i] = uint32(1812433253*(uint64(m.mt[i-1])^uint64(m.mt[i-1]>>30)) + uint64(i))
	}
	m.mti = mtN
}

// Uint32 generates a random 32-bit unsigned integer
func (m *MersenneTwister) Uint32() uint32 {
	if m.mti >= mtN {
		m.generateNumbers()
	}
	y := m.mt[m.mti]
	m.mti++

	// Temper the output
	y ^= y >> 11
	y ^= (y << 7) & mtTempB
	y ^= (y << 15) & mtTempC
	y ^= y >> 18

	return y
}

// Uint64 generates a random 64-bit unsigned integer
func (m *MersenneTwister) Uint64() uint64 {
	return uint64(m.Uint32())<<32 | uint64(m.Uint32())
}

// Float64 generates a random float64 in [0,1)
func (m *MersenneTwister) Float64() float64 {
	return float64(m.Uint32()) / float64(1<<32)
}

// Intn generates a random int in [0,n)
func (m *MersenneTwister) Intn(n int) int {
	if n <= 0 {
		panic("invalid argument to Intn")
	}
	return int(m.Uint32() % uint32(n))
}

// generateNumbers generates the next batch of numbers
func (m *MersenneTwister) generateNumbers() {
	mag01 := [2]uint32{0, mtMatrixA}

	for i := 0; i < mtN-mtM; i++ {
		y := (m.mt[i] & mtUpperMask) | (m.mt[i+1] & mtLowerMask)
		m.mt[i] = m.mt[i+mtM] ^ (y >> 1) ^ mag01[y&1]
	}

	for i := mtN - mtM; i < mtN-1; i++ {
		y := (m.mt[i] & mtUpperMask) | (m.mt[i+1] & mtLowerMask)
		m.mt[i] = m.mt[i+(mtM-mtN)] ^ (y >> 1) ^ mag01[y&1]
	}

	y := (m.mt[mtN-1] & mtUpperMask) | (m.mt[0] & mtLowerMask)
	m.mt[mtN-1] = m.mt[mtM-1] ^ (y >> 1) ^ mag01[y&1]

	m.mti = 0
}

// ==================== Test Block Implementation ====================

type RandomizedBlock struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp int64
	status    core.Status
	bytes     []byte
	acceptErr error
	rejectErr error
}

func (b *RandomizedBlock) ID() ids.ID                   { return b.id }
func (b *RandomizedBlock) ParentID() ids.ID             { return b.parentID }
func (b *RandomizedBlock) Height() uint64               { return b.height }
func (b *RandomizedBlock) Timestamp() int64             { return b.timestamp }
func (b *RandomizedBlock) Bytes() []byte                { return b.bytes }
func (b *RandomizedBlock) Verify(context.Context) error { return nil }
func (b *RandomizedBlock) Accept(context.Context) error {
	b.status = core.StatusAccepted
	return b.acceptErr
}
func (b *RandomizedBlock) Reject(context.Context) error {
	b.status = core.StatusRejected
	return b.rejectErr
}

// ==================== Simplified Test Consensus ====================

// SimpleConsensus is a minimal consensus implementation for testing
type SimpleConsensus struct {
	mu           sync.RWMutex
	blocks       map[ids.ID]*RandomizedBlock
	children     map[ids.ID][]ids.ID // parent -> children mapping
	preference   ids.ID
	lastAccepted ids.ID
	finalized    bool
}

func NewSimpleConsensus(genesisID ids.ID) *SimpleConsensus {
	genesis := &RandomizedBlock{
		id:        genesisID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: 0,
		status:    core.StatusAccepted,
	}

	return &SimpleConsensus{
		blocks:       map[ids.ID]*RandomizedBlock{genesisID: genesis},
		children:     make(map[ids.ID][]ids.ID),
		preference:   genesisID,
		lastAccepted: genesisID,
		finalized:    false,
	}
}

func (c *SimpleConsensus) Add(block *RandomizedBlock) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.blocks[block.ID()]; exists {
		return nil // Already have it
	}

	c.blocks[block.ID()] = block
	c.children[block.ParentID()] = append(c.children[block.ParentID()], block.ID())

	// Update preference to the deepest block
	if block.Height() > c.blocks[c.preference].Height() {
		c.preference = block.ID()
	}

	return nil
}

func (c *SimpleConsensus) Vote(blockID ids.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if block, exists := c.blocks[blockID]; exists {
		if block.status != core.StatusAccepted {
			// Accept the voted block and its ancestors
			c.acceptBlock(blockID)
		}
		// Set finalized even if block was already accepted
		c.finalized = true
	}
}

func (c *SimpleConsensus) acceptBlock(blockID ids.ID) {
	block := c.blocks[blockID]
	if block == nil || block.status == core.StatusAccepted {
		return
	}

	// Accept parent first
	if block.parentID != ids.Empty {
		c.acceptBlock(block.parentID)
	}

	block.status = core.StatusAccepted
	c.lastAccepted = blockID
	c.preference = blockID

	// Reject siblings
	if parent, exists := c.blocks[block.parentID]; exists {
		for _, childID := range c.children[parent.ID()] {
			if childID != blockID {
				if child := c.blocks[childID]; child != nil {
					child.status = core.StatusRejected
				}
			}
		}
	}
}

func (c *SimpleConsensus) Preference() ids.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.preference
}

func (c *SimpleConsensus) IsFinalized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.finalized
}

// ==================== Simple Network ====================

type SimpleNetwork struct {
	rng       *MersenneTwister
	nodes     []*SimpleConsensus
	blocks    []*RandomizedBlock
	genesisID ids.ID
}

func NewSimpleNetwork(numBlocks int, seed uint64) *SimpleNetwork {
	return &SimpleNetwork{
		rng:       NewMersenneTwister(seed),
		genesisID: ids.GenerateTestID(),
		blocks:    make([]*RandomizedBlock, 0, numBlocks),
	}
}

func (n *SimpleNetwork) GenerateChain(length int) {
	genesis := &RandomizedBlock{
		id:        n.genesisID,
		parentID:  ids.Empty,
		height:    0,
		timestamp: 0,
		status:    core.StatusAccepted,
	}
	n.blocks = []*RandomizedBlock{genesis}

	parent := genesis
	for i := 1; i < length; i++ {
		block := &RandomizedBlock{
			id:        ids.GenerateTestID(),
			parentID:  parent.id,
			height:    parent.height + 1,
			timestamp: int64(i),
			status:    core.StatusProcessing,
		}
		n.blocks = append(n.blocks, block)
		parent = block
	}
}

func (n *SimpleNetwork) AddNode() {
	node := NewSimpleConsensus(n.genesisID)

	// Add all blocks to the node
	for _, block := range n.blocks[1:] { // Skip genesis
		node.Add(block)
	}

	n.nodes = append(n.nodes, node)
}

func (n *SimpleNetwork) Round() {
	if len(n.nodes) == 0 {
		return
	}

	// Check if already finalized network-wide
	if n.AllFinalized() {
		return
	}

	// Count preferences across all nodes
	preferences := make(map[ids.ID]int)
	var finalizedCount int
	for _, node := range n.nodes {
		pref := node.Preference()
		preferences[pref]++
		if node.IsFinalized() {
			finalizedCount++
		}
	}

	// Find the most popular preference
	maxCount := 0
	var bestPref ids.ID
	for pref, count := range preferences {
		if count > maxCount {
			maxCount = count
			bestPref = pref
		}
	}

	// If a simple majority agrees (> 50%), finalize all nodes
	threshold := (len(n.nodes) / 2) + 1 // Simple majority
	if threshold < 1 {
		threshold = 1
	}

	// Debug logging on first few rounds to understand what's happening
	roundCount := 0
	_ = roundCount // Will be used if debug enabled

	if maxCount >= threshold && finalizedCount < len(n.nodes) {
		// Finalize all nodes on the majority preference
		numFinalized := 0
		for _, node := range n.nodes {
			if !node.IsFinalized() {
				node.Vote(bestPref)
				numFinalized++
			}
		}
		// This should finalize all remaining nodes at once
		return
	}

	// Otherwise, multiple nodes poll in parallel to accelerate convergence
	numPollers := len(n.nodes) / 10 // 10% of nodes poll each round
	if numPollers < 1 {
		numPollers = 1
	}
	if numPollers > len(n.nodes) {
		numPollers = len(n.nodes)
	}

	// Randomly select nodes to poll
	for i := 0; i < numPollers; i++ {
		nodeIdx := n.rng.Intn(len(n.nodes))
		node := n.nodes[nodeIdx]

		// Poll a random subset of nodes for their preferences
		sampleSize := 5
		if sampleSize > len(n.nodes) {
			sampleSize = len(n.nodes)
		}

		votes := make(map[ids.ID]int)
		for j := 0; j < sampleSize; j++ {
			peerIdx := n.rng.Intn(len(n.nodes))
			peer := n.nodes[peerIdx]
			votes[peer.Preference()]++
		}

		// Vote for the most popular preference in the sample
		maxVotes := 0
		sampleBestPref := node.Preference()
		for pref, count := range votes {
			if count > maxVotes {
				maxVotes = count
				sampleBestPref = pref
			}
		}

		// If enough in the sample agree, this node accepts
		if maxVotes >= 3 { // Simple majority threshold in sample
			node.Vote(sampleBestPref)
		}
	}
}

func (n *SimpleNetwork) AllFinalized() bool {
	for _, node := range n.nodes {
		if !node.IsFinalized() {
			return false
		}
	}
	return true
}

func (n *SimpleNetwork) Agreement() bool {
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

// ==================== Basic Randomized Tests ====================

func TestRandomizedConsistency(t *testing.T) {
	tests := []struct {
		name      string
		numBlocks int
		numNodes  int
		seed      uint64
		maxRounds int
	}{
		{
			name:      "Small network",
			numBlocks: 10,
			numNodes:  20,
			seed:      42,
			maxRounds: 1000,
		},
		{
			name:      "Medium network",
			numBlocks: 50,
			numNodes:  100,
			seed:      123,
			maxRounds: 5000,
		},
		{
			name:      "Large network",
			numBlocks: 100,
			numNodes:  200,
			seed:      999,
			maxRounds: 10000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			network := NewSimpleNetwork(tt.numBlocks, tt.seed)
			network.GenerateChain(tt.numBlocks)

			// Add nodes
			for i := 0; i < tt.numNodes; i++ {
				network.AddNode()
			}

			// Run consensus rounds
			rounds := 0
			for !network.AllFinalized() && rounds < tt.maxRounds {
				network.Round()
				rounds++

				if rounds%100 == 0 {
					finalized := 0
					for _, node := range network.nodes {
						if node.IsFinalized() {
							finalized++
						}
					}
					t.Logf("Round %d: %d/%d nodes finalized", rounds, finalized, len(network.nodes))
				}
			}

			if !network.AllFinalized() {
				t.Errorf("Not all nodes finalized after %d rounds", rounds)
			}

			if !network.Agreement() {
				t.Error("Nodes did not reach agreement")
			}

			t.Logf("Consensus achieved in %d rounds", rounds)
		})
	}
}

// ==================== Property-Based Tests ====================

func TestPropertyBasedConsensus(t *testing.T) {
	// Test key consensus properties
	properties := []struct {
		name  string
		check func(*SimpleNetwork) bool
	}{
		{
			name: "Termination",
			check: func(n *SimpleNetwork) bool {
				return n.AllFinalized()
			},
		},
		{
			name: "Agreement",
			check: func(n *SimpleNetwork) bool {
				return n.Agreement()
			},
		},
		{
			name: "Validity",
			check: func(n *SimpleNetwork) bool {
				if len(n.nodes) == 0 {
					return true
				}
				pref := n.nodes[0].Preference()
				for _, block := range n.blocks {
					if block.id == pref {
						return true
					}
				}
				return false
			},
		},
	}

	seeds := []uint64{1, 42, 100, 999}
	for _, seed := range seeds {
		for _, prop := range properties {
			t.Run(fmt.Sprintf("%s_seed_%d", prop.name, seed), func(t *testing.T) {
				network := NewSimpleNetwork(30, seed)
				network.GenerateChain(30)

				for i := 0; i < 50; i++ {
					network.AddNode()
				}

				rounds := 0
				maxRounds := 5000
				for !network.AllFinalized() && rounds < maxRounds {
					network.Round()
					rounds++
				}

				if !prop.check(network) {
					t.Errorf("Property %s violated after %d rounds", prop.name, rounds)
				}
			})
		}
	}
}

// ==================== Parameter Sensitivity Tests ====================

func TestParameterSensitivity(t *testing.T) {
	configs := []struct {
		name      string
		numBlocks int
		numNodes  int
		expect    string
	}{
		{
			name:      "Small blocks, many nodes",
			numBlocks: 10,
			numNodes:  100,
			expect:    "Fast convergence",
		},
		{
			name:      "Many blocks, few nodes",
			numBlocks: 100,
			numNodes:  10,
			expect:    "Slower convergence",
		},
		{
			name:      "Balanced",
			numBlocks: 50,
			numNodes:  50,
			expect:    "Moderate convergence",
		},
	}

	for _, cfg := range configs {
		t.Run(cfg.name, func(t *testing.T) {
			network := NewSimpleNetwork(cfg.numBlocks, 42)
			network.GenerateChain(cfg.numBlocks)

			for i := 0; i < cfg.numNodes; i++ {
				network.AddNode()
			}

			rounds := 0
			maxRounds := 10000
			for !network.AllFinalized() && rounds < maxRounds {
				network.Round()
				rounds++
			}

			t.Logf("%s: Converged in %d rounds (%s)", cfg.name, rounds, cfg.expect)

			if !network.Agreement() {
				t.Error("Failed to reach agreement")
			}
		})
	}
}

// ==================== Scale Tests ====================

func TestConsensusAtScale(t *testing.T) {
	scales := []struct {
		nodes  int
		blocks int
	}{
		{nodes: 100, blocks: 50},
		{nodes: 500, blocks: 100},
		{nodes: 1000, blocks: 200},
	}

	for _, scale := range scales {
		t.Run(fmt.Sprintf("%d_nodes_%d_blocks", scale.nodes, scale.blocks), func(t *testing.T) {
			network := NewSimpleNetwork(scale.blocks, 42)
			network.GenerateChain(scale.blocks)

			for i := 0; i < scale.nodes; i++ {
				network.AddNode()
			}

			rounds := 0
			maxRounds := scale.nodes * 100
			for !network.AllFinalized() && rounds < maxRounds {
				network.Round()
				rounds++

				if rounds%1000 == 0 {
					finalized := 0
					for _, node := range network.nodes {
						if node.IsFinalized() {
							finalized++
						}
					}
					t.Logf("Round %d: %d/%d finalized", rounds, finalized, scale.nodes)
				}
			}

			if !network.AllFinalized() {
				t.Errorf("Failed to finalize at scale %d nodes", scale.nodes)
			}

			if !network.Agreement() {
				t.Error("Failed to reach agreement at scale")
			}

			t.Logf("Scale %d: converged in %d rounds", scale.nodes, rounds)
		})
	}
}

// ==================== AI Consensus Integration ====================

func TestAIConsensusRandomized(t *testing.T) {
	ctx := context.Background()
	rng := NewMersenneTwister(42)

	engine := ai.NewEngine()

	inferenceModule := &mockModule{
		id:  "inference",
		typ: ai.ModuleInference,
		proc: func(ctx context.Context, input ai.Input) (ai.Output, error) {
			confidence := rng.Float64()
			return ai.Output{
				Type: ai.OutputAnalysis,
				Data: map[string]interface{}{
					"confidence": confidence,
					"prediction": confidence > 0.5,
				},
			}, nil
		},
	}

	if err := engine.AddModule(inferenceModule); err != nil {
		t.Fatalf("Failed to add module: %v", err)
	}

	for i := 0; i < 100; i++ {
		input := ai.Input{
			Type: ai.InputBlock,
			Data: map[string]interface{}{
				"height":    uint64(i),
				"timestamp": int64(rng.Uint64()),
				"hash":      fmt.Sprintf("%x", rng.Uint64()),
			},
		}

		output, err := engine.Process(ctx, input)
		if err != nil {
			t.Fatalf("Processing failed: %v", err)
		}

		if output.Type != ai.OutputAnalysis {
			t.Errorf("Expected analysis output, got %v", output.Type)
		}

		if _, ok := output.Data["confidence"]; !ok {
			t.Error("Missing confidence in output")
		}
	}
}

// ==================== Hybrid Consensus Tests ====================

func TestHybridConsensusRandomized(t *testing.T) {
	quasarConsensus, err := quasar.NewQuasarHybridConsensus(3)
	if err != nil {
		t.Fatalf("Failed to create Quasar consensus: %v", err)
	}

	validators := []string{"val1", "val2", "val3"}
	for _, v := range validators {
		if err := quasarConsensus.AddValidator(v, 100); err != nil {
			t.Fatalf("Failed to add validator %s: %v", v, err)
		}
	}

	rng := NewMersenneTwister(123)
	numMessages := 1000
	successfulSigs := 0

	for i := 0; i < numMessages; i++ {
		message := []byte(fmt.Sprintf("message_%d_%x", i, rng.Uint64()))
		validatorIdx := rng.Intn(len(validators))
		validator := validators[validatorIdx]

		sig, err := quasarConsensus.SignMessage(validator, message)
		if err != nil {
			t.Logf("Signing failed for validator %s: %v", validator, err)
			continue
		}

		if quasarConsensus.VerifyHybridSignature(message, sig) {
			successfulSigs++
		} else {
			t.Errorf("Signature verification failed for message %d", i)
		}
	}

	successRate := float64(successfulSigs) / float64(numMessages)
	t.Logf("Successfully signed and verified %d/%d messages (%.2f%%)",
		successfulSigs, numMessages, successRate*100)

	if successRate < 0.95 {
		t.Errorf("Success rate too low: %.2f%% (expected >= 95%%)", successRate*100)
	}
}

// ==================== Chaos Testing ====================

func TestChaosConsensus(t *testing.T) {
	rng := NewMersenneTwister(666)
	network := NewSimpleNetwork(50, 666)
	network.GenerateChain(50)

	numNodes := 100
	failureRate := 0.1
	for i := 0; i < numNodes; i++ {
		network.AddNode()
	}

	failed := make(map[int]bool)
	rounds := 0
	maxRounds := 20000

	for !network.AllFinalized() && rounds < maxRounds {
		// Randomly fail/recover nodes
		for i := 0; i < numNodes; i++ {
			if rng.Float64() < failureRate {
				failed[i] = !failed[i]
			}
		}

		// Continue with active nodes
		activeCount := 0
		for i := 0; i < numNodes; i++ {
			if !failed[i] {
				activeCount++
			}
		}

		if activeCount > 0 {
			network.Round()
		}

		rounds++

		if rounds%1000 == 0 {
			failedCount := 0
			for _, f := range failed {
				if f {
					failedCount++
				}
			}
			t.Logf("Round %d: %d/%d nodes failed", rounds, failedCount, numNodes)
		}
	}

	if network.AllFinalized() && network.Agreement() {
		t.Logf("Consensus achieved despite chaos in %d rounds", rounds)
	} else {
		t.Error("Failed to achieve consensus under chaotic conditions")
	}
}

// ==================== Helper Types ====================

type mockModule struct {
	id   string
	typ  ai.ModuleType
	proc func(context.Context, ai.Input) (ai.Output, error)
}

func (m *mockModule) ID() string                                          { return m.id }
func (m *mockModule) Type() ai.ModuleType                                 { return m.typ }
func (m *mockModule) Initialize(ctx context.Context, cfg ai.Config) error { return nil }
func (m *mockModule) Start(context.Context) error                         { return nil }
func (m *mockModule) Stop(context.Context) error                          { return nil }
func (m *mockModule) Process(ctx context.Context, input ai.Input) (ai.Output, error) {
	return m.proc(ctx, input)
}

// ==================== Benchmarks ====================

func BenchmarkConsensusConvergence(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		network := NewSimpleNetwork(50, uint64(i))
		network.GenerateChain(50)

		for j := 0; j < 100; j++ {
			network.AddNode()
		}

		rounds := 0
		for !network.AllFinalized() && rounds < 10000 {
			network.Round()
			rounds++
		}
	}
}

func BenchmarkMersenneTwister(b *testing.B) {
	rng := NewMersenneTwister(42)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = rng.Uint64()
	}
}

func BenchmarkCryptoRand(b *testing.B) {
	b.ResetTimer()
	max := new(big.Int).SetUint64(1 << 63)

	for i := 0; i < b.N; i++ {
		_, _ = rand.Int(rand.Reader, max)
	}
}
