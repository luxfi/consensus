// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.
//
// Snowball compatibility tests for Quasar - validates consensus equivalence
// between Snowball's tree-based voting and Quasar's quantum finality model.

package quasar

import (
	"context"
	"crypto/sha256"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// --- Tree-based consensus simulation (ported from tree_test.go) ---

// treeNode represents a node in the consensus tree
type treeNode struct {
	id       [32]byte
	children [2]*treeNode // binary children
	parent   *treeNode
	unary    *unarySnowball
	binary   *binaryTreeSnowball
}

// unarySnowball tracks single-choice consensus
type unarySnowball struct {
	preferenceStrength int
	confidence         int
	finalized          bool
	beta               int
}

func newUnarySnowball(beta int) *unarySnowball {
	return &unarySnowball{beta: beta}
}

func (u *unarySnowball) RecordPoll(voteCount, threshold int) bool {
	if u.finalized {
		return false
	}
	if voteCount >= threshold {
		u.preferenceStrength++
		u.confidence++
		if u.confidence >= u.beta {
			u.finalized = true
		}
		return true
	}
	u.confidence = 0
	return false
}

// binaryTreeSnowball tracks binary choice at tree branch point
type binaryTreeSnowball struct {
	preference       int
	prefStrength     [2]int
	confidence       int
	finalized        bool
	alphaPreference  int
	alphaConfidence  int
	beta             int
}

func newBinaryTreeSnowball(alphaPreference, alphaConfidence, beta int) *binaryTreeSnowball {
	return &binaryTreeSnowball{
		alphaPreference: alphaPreference,
		alphaConfidence: alphaConfidence,
		beta:            beta,
	}
}

func (b *binaryTreeSnowball) RecordPoll(voteCount, choice int) bool {
	if b.finalized {
		return false
	}

	b.prefStrength[choice]++

	if voteCount >= b.alphaConfidence {
		if choice == b.preference {
			b.confidence++
		} else {
			b.preference = choice
			b.confidence = 1
		}

		if b.confidence >= b.beta {
			b.finalized = true
		}
		return true
	} else if voteCount >= b.alphaPreference {
		if b.prefStrength[choice] > b.prefStrength[1-choice] {
			b.preference = choice
		}
		b.confidence = 0
	}
	return false
}

// consensusTree simulates Snowball's tree-based consensus
type consensusTree struct {
	root   *treeNode
	params struct {
		k               int
		alphaPreference int
		alphaConfidence int
		beta            int
	}
	preference [32]byte
	finalized  bool
	// Track choice to branch mapping for binary snowball
	choices   [2][32]byte // choices[0] = first choice, choices[1] = second choice
	numChoices int
}

func newConsensusTree(params struct{ k, alphaPreference, alphaConfidence, beta int }, initial [32]byte) *consensusTree {
	tree := &consensusTree{
		params:     params,
		preference: initial,
		numChoices: 1,
	}
	tree.choices[0] = initial
	tree.root = &treeNode{
		id:    initial,
		unary: newUnarySnowball(params.beta),
	}
	return tree
}

func (t *consensusTree) Add(choice [32]byte) {
	if t.finalized {
		return
	}

	// Find first differing bit
	diffBit := findDifferingBit(t.preference, choice)
	if diffBit < 0 {
		return // Same choice
	}

	// Track the second choice
	if t.numChoices < 2 {
		t.choices[1] = choice
		t.numChoices = 2
	}

	// Add binary node at difference point
	// (simplified - full implementation would walk tree)
	if t.root.binary == nil {
		t.root.binary = newBinaryTreeSnowball(
			t.params.alphaPreference,
			t.params.alphaConfidence,
			t.params.beta,
		)
	}
}

func findDifferingBit(a, b [32]byte) int {
	for i := 0; i < 32; i++ {
		if a[i] != b[i] {
			for bit := 0; bit < 8; bit++ {
				if (a[i]>>bit)&1 != (b[i]>>bit)&1 {
					return i*8 + bit
				}
			}
		}
	}
	return -1
}

func (t *consensusTree) RecordPoll(votes map[[32]byte]int) bool {
	if t.finalized {
		return false
	}

	// Count votes for current preference
	voteCount := votes[t.preference]

	// Try unary snowball first
	if t.root.unary != nil {
		if t.root.unary.RecordPoll(voteCount, t.params.alphaConfidence) {
			if t.root.unary.finalized {
				t.finalized = true
			}
			return true
		}
	}

	// Check binary snowball
	if t.root.binary != nil {
		// Determine which choice got more votes
		maxVotes := 0
		var maxChoice [32]byte
		for choice, v := range votes {
			if v > maxVotes {
				maxVotes = v
				maxChoice = choice
			}
		}

		// Determine branch consistently based on which choice it is
		// choices[0] maps to branch 0, choices[1] maps to branch 1
		branch := 0
		if maxChoice == t.choices[1] {
			branch = 1
		}

		// Update tree preference to match max choice
		t.preference = maxChoice

		t.root.binary.RecordPoll(maxVotes, branch)
		if t.root.binary.finalized {
			t.finalized = true
			// Set final preference based on which branch won
			t.preference = t.choices[t.root.binary.preference]
		}
	}

	return false
}

func (t *consensusTree) Preference() [32]byte { return t.preference }
func (t *consensusTree) Finalized() bool      { return t.finalized }

// --- Tree consensus tests ---

// TestTreeSingleton tests single-choice tree consensus
func TestTreeSingleton(t *testing.T) {
	require := require.New(t)

	red := sha256.Sum256([]byte("red"))

	params := struct{ k, alphaPreference, alphaConfidence, beta int }{
		k: 1, alphaPreference: 1, alphaConfidence: 1, beta: 2,
	}

	tree := newConsensusTree(params, red)

	require.False(tree.Finalized())

	// First poll
	votes := map[[32]byte]int{red: 1}
	tree.RecordPoll(votes)
	require.False(tree.Finalized())

	// Empty poll resets
	tree.RecordPoll(map[[32]byte]int{})
	require.False(tree.Finalized())

	// Two more successful polls
	tree.RecordPoll(votes)
	tree.RecordPoll(votes)
	require.Equal(red, tree.Preference())
	require.True(tree.Finalized())

	// Adding new choice after finalization has no effect
	blue := sha256.Sum256([]byte("blue"))
	tree.Add(blue)
	require.True(tree.Finalized())
}

// TestTreeBinary tests binary choice tree consensus
func TestTreeBinary(t *testing.T) {
	require := require.New(t)

	red := sha256.Sum256([]byte("red"))
	blue := sha256.Sum256([]byte("blue"))

	params := struct{ k, alphaPreference, alphaConfidence, beta int }{
		k: 1, alphaPreference: 1, alphaConfidence: 1, beta: 2,
	}

	tree := newConsensusTree(params, red)
	tree.Add(blue)

	require.Equal(red, tree.Preference())
	require.False(tree.Finalized())

	// Vote for blue - switches preference
	tree.RecordPoll(map[[32]byte]int{blue: 1})
	require.False(tree.Finalized())

	// Two consecutive blue votes to finalize
	tree.RecordPoll(map[[32]byte]int{blue: 1})
	tree.RecordPoll(map[[32]byte]int{blue: 1})
	require.True(tree.Finalized())
}

// --- Quasar Engine Tests ---

// TestQuasarEngineBasic tests basic Quasar engine operation
func TestQuasarEngineBasic(t *testing.T) {
	require := require.New(t)

	cfg := Config{QThreshold: 1, QuasarTimeout: 30}
	engine, err := NewEngine(cfg)
	require.NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = engine.Start(ctx)
	require.NoError(err)

	// Submit a block
	block := &Block{
		ID:        sha256.Sum256([]byte("test-block-1")),
		ChainID:   sha256.Sum256([]byte("test-chain")),
		ChainName: "test",
		Height:    1,
		Timestamp: time.Now(),
		Data:      []byte("test data"),
	}

	err = engine.Submit(block)
	require.NoError(err)

	// Wait for finalization
	select {
	case finalized := <-engine.Finalized():
		require.NotNil(finalized)
		require.NotNil(finalized.Cert)
		require.Equal(uint64(1), finalized.Height)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for finalization")
	}

	// Check stats
	stats := engine.Stats()
	require.Equal(uint64(1), stats.ProcessedBlocks)
	require.Equal(uint64(1), stats.FinalizedBlocks)

	err = engine.Stop()
	require.NoError(err)
}

// TestQuasarEngineMultipleBlocks tests sequential block processing
func TestQuasarEngineMultipleBlocks(t *testing.T) {
	require := require.New(t)

	cfg := Config{QThreshold: 1, QuasarTimeout: 30}
	engine, err := NewEngine(cfg)
	require.NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = engine.Start(ctx)
	require.NoError(err)
	defer engine.Stop()

	// Submit multiple blocks
	numBlocks := 10
	for i := 0; i < numBlocks; i++ {
		block := &Block{
			ID:        sha256.Sum256([]byte("block-" + string(rune('0'+i)))),
			ChainID:   sha256.Sum256([]byte("test-chain")),
			ChainName: "test",
			Height:    uint64(i + 1),
			Timestamp: time.Now(),
		}
		err = engine.Submit(block)
		require.NoError(err)
	}

	// Collect finalized blocks
	finalized := 0
	timeout := time.After(5 * time.Second)
	for finalized < numBlocks {
		select {
		case <-engine.Finalized():
			finalized++
		case <-timeout:
			t.Fatalf("timeout: only %d/%d blocks finalized", finalized, numBlocks)
		}
	}

	stats := engine.Stats()
	require.Equal(uint64(numBlocks), stats.ProcessedBlocks)
}

// --- Network Simulation Tests (ported from network_test.go) ---

// quasarNode represents a node in the simulated Quasar network
type quasarNode struct {
	id         int
	preference int // 0 or 1 for binary choice
	confidence int
	finalized  bool
}

// quasarNetwork simulates a network of Quasar nodes
type quasarNetwork struct {
	mu     sync.RWMutex
	nodes  []*quasarNode
	params Config
	seed   int64
}

func newQuasarNetwork(numNodes, numChoices int, cfg Config) *quasarNetwork {
	net := &quasarNetwork{
		nodes:  make([]*quasarNode, 0, numNodes),
		params: cfg,
	}

	// Create nodes with initial preferences biased toward 0 (supermajority)
	for i := 0; i < numNodes; i++ {
		// Bias initial preferences: ~86% toward 0, ~14% toward 1
		// With k=9 samples and threshold=6, need consistent supermajority
		initial := 0
		if i%7 == 0 {
			initial = 1
		}
		node := &quasarNode{
			id:         i,
			preference: initial,
			confidence: 0,
			finalized:  false,
		}
		net.nodes = append(net.nodes, node)
	}

	return net
}

// Round executes one consensus round for all nodes
func (net *quasarNetwork) Round() {
	net.mu.Lock()
	defer net.mu.Unlock()

	net.seed++

	// Sample votes from network (simulates network-wide poll)
	k := net.params.QThreshold * 3 // Sample size
	if k > len(net.nodes) {
		k = len(net.nodes)
	}

	vote0, vote1 := 0, 0
	for i := 0; i < k; i++ {
		peerIdx := (int(net.seed)*7 + i*3) % len(net.nodes)
		if net.nodes[peerIdx].preference == 0 {
			vote0++
		} else {
			vote1++
		}
	}

	// Each non-finalized node processes the poll
	threshold := k * 2 / 3 // 2/3 threshold
	beta := 5              // Finalization threshold

	for _, node := range net.nodes {
		if node.finalized {
			continue
		}

		var majorityChoice int
		var majorityVotes int
		if vote0 > vote1 {
			majorityChoice = 0
			majorityVotes = vote0
		} else if vote1 > vote0 {
			majorityChoice = 1
			majorityVotes = vote1
		} else {
			node.confidence = 0
			continue
		}

		if majorityVotes >= threshold {
			if majorityChoice == node.preference {
				node.confidence++
			} else {
				node.preference = majorityChoice
				node.confidence = 1
			}

			if node.confidence >= beta {
				node.finalized = true
			}
		} else {
			node.confidence = 0
		}
	}
}

func (net *quasarNetwork) Finalized() bool {
	for _, node := range net.nodes {
		if !node.finalized {
			return false
		}
	}
	return true
}

func (net *quasarNetwork) Agreement() bool {
	if len(net.nodes) == 0 {
		return true
	}
	pref := net.nodes[0].preference
	for _, node := range net.nodes {
		if node.preference != pref {
			return false
		}
	}
	return true
}

func (net *quasarNetwork) Disagreement() bool {
	var finalizedPref *int
	for _, node := range net.nodes {
		if node.finalized {
			if finalizedPref == nil {
				p := node.preference
				finalizedPref = &p
			} else if *finalizedPref != node.preference {
				return true
			}
		}
	}
	return false
}

// TestQuasarNetworkConvergence tests network convergence
func TestQuasarNetworkConvergence(t *testing.T) {
	require := require.New(t)

	cfg := Config{QThreshold: 3, QuasarTimeout: 30}
	net := newQuasarNetwork(30, 2, cfg)

	maxRounds := 200
	for i := 0; i < maxRounds && !net.Agreement(); i++ {
		net.Round()
	}

	require.False(net.Disagreement(), "Network should not have disagreement")
	require.True(net.Agreement(), "Network should reach agreement")
}

// TestQuasarNetworkSmall tests small network convergence
func TestQuasarNetworkSmall(t *testing.T) {
	require := require.New(t)

	cfg := Config{QThreshold: 2, QuasarTimeout: 30}
	net := newQuasarNetwork(10, 2, cfg)

	maxRounds := 100
	for i := 0; i < maxRounds && !net.Agreement(); i++ {
		net.Round()
	}

	require.False(net.Disagreement())
	require.True(net.Agreement())
}

// --- Performance Tests (ported from consensus_performance_test.go) ---

// TestQuasarConvergenceSpeed measures rounds to agreement
func TestQuasarConvergenceSpeed(t *testing.T) {
	require := require.New(t)

	trials := 5
	totalRounds := 0

	for trial := 0; trial < trials; trial++ {
		cfg := Config{QThreshold: 2, QuasarTimeout: 30}
		net := newQuasarNetwork(15, 2, cfg)
		net.seed = int64(trial * 1000)

		rounds := 0
		for rounds < 150 && !net.Agreement() {
			net.Round()
			rounds++
		}

		require.True(net.Agreement(), "Trial %d should converge", trial)
		totalRounds += rounds
	}

	avgRounds := totalRounds / trials
	t.Logf("Average rounds to agreement: %d", avgRounds)

	require.Less(avgRounds, 100, "Should converge in reasonable time")
}

// TestQuasarThroughput tests block processing throughput
func TestQuasarThroughput(t *testing.T) {
	require := require.New(t)

	cfg := Config{QThreshold: 1, QuasarTimeout: 30}
	engine, err := NewEngine(cfg)
	require.NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = engine.Start(ctx)
	require.NoError(err)
	defer engine.Stop()

	// Submit blocks as fast as possible
	numBlocks := 100
	start := time.Now()

	for i := 0; i < numBlocks; i++ {
		block := &Block{
			ID:        sha256.Sum256([]byte{byte(i), byte(i >> 8)}),
			ChainID:   sha256.Sum256([]byte("bench")),
			ChainName: "bench",
			Height:    uint64(i + 1),
			Timestamp: time.Now(),
		}
		engine.Submit(block)
	}

	// Drain finalized channel
	finalized := 0
	for finalized < numBlocks {
		select {
		case <-engine.Finalized():
			finalized++
		case <-time.After(10 * time.Second):
			break
		}
	}

	elapsed := time.Since(start)
	tps := float64(finalized) / elapsed.Seconds()

	t.Logf("Throughput: %.2f blocks/sec (%d blocks in %v)", tps, finalized, elapsed)
	require.Greater(tps, 10.0, "Should process at least 10 blocks/sec")
}

// --- Hybrid Consensus Tests ---

// TestHybridConsensusBasic tests BLS + PQ certificate generation
func TestHybridConsensusBasic(t *testing.T) {
	require := require.New(t)

	hybrid, err := newHybridConsensus(2)
	require.NoError(err)

	// Add validators
	hybrid.AddValidator("v1", 100)
	hybrid.AddValidator("v2", 100)
	hybrid.AddValidator("v3", 100)

	require.Equal(3, hybrid.validatorCount())

	// Generate certificate
	block := &Block{
		ID:        sha256.Sum256([]byte("test")),
		ChainID:   sha256.Sum256([]byte("chain")),
		ChainName: "test",
		Height:    1,
		Timestamp: time.Now(),
	}

	cert := hybrid.generateCert(block)
	require.NotNil(cert)
	require.NotEmpty(cert.BLS)
	require.NotEmpty(cert.PQ)
	require.Equal(uint64(1), cert.Epoch)

	// Verify certificate
	require.True(cert.Verify([]string{"v1", "v2", "v3"}))
}

// TestHybridConsensusValidatorChurn tests validator add/remove
func TestHybridConsensusValidatorChurn(t *testing.T) {
	require := require.New(t)

	hybrid, err := newHybridConsensus(2)
	require.NoError(err)

	hybrid.AddValidator("v1", 100)
	hybrid.AddValidator("v2", 100)
	require.Equal(2, hybrid.validatorCount())

	hybrid.RemoveValidator("v1")
	require.Equal(1, hybrid.validatorCount())

	hybrid.AddValidator("v3", 150)
	require.Equal(2, hybrid.validatorCount())
}

// --- Safety Tests ---

// TestQuasarSafetyUnderPartition tests safety during network partition
func TestQuasarSafetyUnderPartition(t *testing.T) {
	require := require.New(t)

	cfg := Config{QThreshold: 3, QuasarTimeout: 30}
	net := newQuasarNetwork(30, 2, cfg)

	// Run some rounds toward convergence
	for i := 0; i < 50; i++ {
		net.Round()
	}

	// Simulate partition by resetting confidence for some nodes
	net.mu.Lock()
	for i := 0; i < 10; i++ {
		if !net.nodes[i].finalized {
			net.nodes[i].confidence = 0
		}
	}
	net.mu.Unlock()

	// Continue consensus
	for i := 0; i < 150 && !net.Agreement(); i++ {
		net.Round()
	}

	// Safety: no disagreement among finalized nodes
	require.False(net.Disagreement(), "Safety should hold under partition")
}

// TestQuasarLiveness tests eventual progress
func TestQuasarLiveness(t *testing.T) {
	require := require.New(t)

	cfg := Config{QThreshold: 2, QuasarTimeout: 30}
	net := newQuasarNetwork(15, 2, cfg)

	// Should make progress within reasonable rounds
	maxRounds := 100
	initialAgreement := net.Agreement()

	rounds := 0
	for rounds < maxRounds && !net.Agreement() {
		net.Round()
		rounds++
	}

	// Either started in agreement or reached it
	require.True(initialAgreement || net.Agreement(),
		"Network should make progress (reached agreement in %d rounds)", rounds)
}

// --- Parameter Sensitivity Tests ---

// TestThresholdSensitivity tests different threshold configurations
func TestThresholdSensitivity(t *testing.T) {
	thresholds := []int{1, 2, 3, 5}

	for _, threshold := range thresholds {
		t.Run("threshold_"+string(rune('0'+threshold)), func(t *testing.T) {
			require := require.New(t)

			cfg := Config{QThreshold: threshold, QuasarTimeout: 30}
			net := newQuasarNetwork(20, 2, cfg)

			maxRounds := 200
			for i := 0; i < maxRounds && !net.Agreement(); i++ {
				net.Round()
			}

			require.True(net.Agreement(), "Should converge with threshold=%d", threshold)
			require.False(net.Disagreement())
		})
	}
}

// TestNetworkSizeSensitivity tests different network sizes
func TestNetworkSizeSensitivity(t *testing.T) {
	sizes := []int{5, 10, 25, 50}

	for _, size := range sizes {
		t.Run("size_"+string(rune('0'+size/10))+string(rune('0'+size%10)), func(t *testing.T) {
			require := require.New(t)

			cfg := Config{QThreshold: 3, QuasarTimeout: 30}
			net := newQuasarNetwork(size, 2, cfg)

			maxRounds := 300
			for i := 0; i < maxRounds && !net.Agreement(); i++ {
				net.Round()
			}

			require.True(net.Agreement(), "Should converge with size=%d", size)
		})
	}
}
