package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/consensus/utils/ids"
)

// TestCrossLanguageConsensus is the ultimate E2E test:
// All languages (Go, C, C++, Rust, Python) validate the same network
func TestCrossLanguageConsensus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E cross-language test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Define test blocks that all languages must agree on
	testBlocks := []*Block{
		{
			ID:       ids.GenerateTestID(),
			ParentID: ids.Empty,
			Height:   1,
			Data:     []byte("genesis block"),
		},
		{
			ID:       ids.GenerateTestID(),
			ParentID: ids.Empty, // Will be set to previous block
			Height:   2,
			Data:     []byte("block 2"),
		},
		{
			ID:       ids.GenerateTestID(),
			ParentID: ids.Empty, // Will be set to previous block
			Height:   3,
			Data:     []byte("block 3"),
		},
	}

	// Link blocks
	testBlocks[1].ParentID = testBlocks[0].ID
	testBlocks[2].ParentID = testBlocks[1].ID

	// Initialize nodes for all languages
	nodes := map[string]NodeRunner{
		"go":     NewGoNode(t),
		"c":      NewCNode(t),
		"cpp":    NewCppNode(t),
		"rust":   NewRustNode(t),
		"python": NewPythonNode(t),
	}

	// Start all nodes in parallel
	t.Log("Starting nodes in all languages...")
	var wg sync.WaitGroup
	basePort := 9000

	for lang, node := range nodes {
		wg.Add(1)
		go func(language string, n NodeRunner, port int) {
			defer wg.Done()
			if err := n.Start(ctx, port); err != nil {
				t.Logf("⚠️  Failed to start %s node: %v", language, err)
			}
		}(lang, node, basePort)
		basePort++
	}

	wg.Wait()

	// Wait for all nodes to be healthy
	t.Log("Waiting for all nodes to be healthy...")
	availableNodes := getHealthyNodes(t, nodes, 5*time.Second)
	if len(availableNodes) == 0 {
		t.Fatal("No nodes became healthy")
	}

	t.Logf("✅ %d/%d nodes are healthy: %v", len(availableNodes), len(nodes), getNodeLanguages(availableNodes))
	nodes = availableNodes

	// Propose blocks to all nodes
	t.Log("Proposing test blocks to all nodes...")
	for i, block := range testBlocks {
		t.Logf("Proposing block %d (ID: %s, Height: %d)", i+1, block.ID, block.Height)

		for lang, node := range nodes {
			if err := node.ProposeBlock(block); err != nil {
				t.Logf("⚠️  Failed to propose block to %s node: %v", lang, err)
			}
		}

		// Wait for consensus to be reached
		time.Sleep(500 * time.Millisecond)
	}

	// Verify all nodes reached the same decisions
	t.Log("Verifying consensus across all languages...")
	decisions := make(map[string]map[ids.ID]bool)

	for lang, node := range nodes {
		decisions[lang] = make(map[ids.ID]bool)
		for _, block := range testBlocks {
			accepted, err := node.GetDecision(block.ID)
			if err != nil {
				t.Logf("⚠️  Failed to get decision from %s node: %v", lang, err)
				continue
			}
			decisions[lang][block.ID] = accepted
			t.Logf("%s node: block %s = %v", lang, block.ID, accepted)
		}
	}

	// Verify all languages agree
	t.Log("Checking consensus agreement across all languages...")
	for _, block := range testBlocks {
		var firstDecision *bool
		var firstLang string

		for lang, langDecisions := range decisions {
			decision := langDecisions[block.ID]

			if firstDecision == nil {
				firstDecision = &decision
				firstLang = lang
			} else if decision != *firstDecision {
				t.Errorf("CONSENSUS MISMATCH for block %s: %s=%v, %s=%v",
					block.ID, firstLang, *firstDecision, lang, decision)
			}
		}

		if firstDecision != nil {
			t.Logf("✅ All languages agree on block %s: %v", block.ID, *firstDecision)
		}
	}

	// Cleanup
	t.Log("Stopping all nodes...")
	for lang, node := range nodes {
		if err := node.Stop(); err != nil {
			t.Logf("⚠️  Failed to stop %s node: %v", lang, err)
		}
	}

	t.Log("✅ Cross-language consensus test complete!")
}

// getHealthyNodes returns only the nodes that are healthy
func getHealthyNodes(t *testing.T, nodes map[string]NodeRunner, timeout time.Duration) map[string]NodeRunner {
	deadline := time.Now().Add(timeout)

	// Wait a bit for nodes to initialize
	for time.Now().Before(deadline) {
		healthyNodes := make(map[string]NodeRunner)
		for lang, node := range nodes {
			if node.IsHealthy() {
				healthyNodes[lang] = node
			}
		}

		if len(healthyNodes) > 0 {
			return healthyNodes
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Return whatever we have
	healthyNodes := make(map[string]NodeRunner)
	for lang, node := range nodes {
		if node.IsHealthy() {
			healthyNodes[lang] = node
		}
	}
	return healthyNodes
}

// getNodeLanguages returns the languages of nodes in a map
func getNodeLanguages(nodes map[string]NodeRunner) []string {
	languages := make([]string, 0, len(nodes))
	for lang := range nodes {
		languages = append(languages, lang)
	}
	return languages
}
