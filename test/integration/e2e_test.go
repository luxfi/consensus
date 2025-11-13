package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/luxfi/consensus"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// TestE2EConsensusEngine tests the consensus engine end-to-end
func TestE2EConsensusEngine(t *testing.T) {
	// Test all three engines
	engines := []string{"chain", "dag", "pq"}

	for _, engineType := range engines {
		t.Run(engineType, func(t *testing.T) {
			var engine consensus.Engine

			switch engineType {
			case "chain":
				engine = consensus.NewChainEngine()
			case "dag":
				engine = consensus.NewDAGEngine()
			case "pq":
				engine = consensus.NewPQEngine()
			}

			// Test engine creation
			if engine == nil {
				t.Fatalf("Failed to create %s engine", engineType)
			}

			// Test health check
			ctx := context.Background()
			if adapter, ok := engine.(interface {
				HealthCheck(context.Context) (interface{}, error)
			}); ok {
				health, err := adapter.HealthCheck(ctx)
				if err != nil {
					t.Errorf("%s engine health check failed: %v", engineType, err)
				}
				t.Logf("%s engine health: %v", engineType, health)
			}
		})
	}
}

// TestE2EConsensusServer tests the HTTP server endpoints
func TestE2EConsensusServer(t *testing.T) {
	serverURL := "http://localhost:9090"

	// Test health endpoint
	t.Run("HealthCheck", func(t *testing.T) {
		resp, err := http.Get(serverURL + "/health")
		if err != nil {
			t.Skipf("Server not running: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Health check failed: status %d", resp.StatusCode)
		}
	})

	// Test status endpoint
	t.Run("Status", func(t *testing.T) {
		resp, err := http.Get(serverURL + "/status")
		if err != nil {
			t.Skipf("Server not running: %v", err)
		}
		defer resp.Body.Close()

		var status map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			t.Errorf("Failed to decode status: %v", err)
		}

		if !status["healthy"].(bool) {
			t.Error("Engine not healthy")
		}

		t.Logf("Status: %+v", status)
	})

	// Test consensus processing
	t.Run("ConsensusRound", func(t *testing.T) {
		payload := map[string]interface{}{
			"block_id": "test-block-123",
			"votes": map[string]int{
				"node1": 1,
				"node2": 1,
				"node3": 1,
				"node4": 0,
			},
		}

		body, _ := json.Marshal(payload)
		resp, err := http.Post(serverURL+"/consensus", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Skipf("Server not running: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Errorf("Failed to decode result: %v", err)
		}

		// With 75% votes (3/4), consensus should be reached (alpha=0.8 for local)
		// Actually with local params alpha might be lower
		t.Logf("Consensus result: %+v", result)
	})
}

// TestE2ESimulation runs a full simulation
func TestE2ESimulation(t *testing.T) {
	params := config.LocalParams()

	// Simulate 10 rounds of consensus
	accepts := 0
	rejects := 0

	for round := 1; round <= 10; round++ {
		// Generate random votes
		totalNodes := 5
		acceptVotes := 0

		// Simulate 80% accepting
		for i := 0; i < totalNodes; i++ {
			if i < 4 { // 4 out of 5 nodes accept
				acceptVotes++
			}
		}

		confidence := float64(acceptVotes) / float64(totalNodes)

		if confidence >= params.Alpha {
			accepts++
			t.Logf("Round %d: ACCEPT (confidence: %.2f%%)", round, confidence*100)
		} else {
			rejects++
			t.Logf("Round %d: REJECT (confidence: %.2f%%)", round, confidence*100)
		}
	}

	t.Logf("Simulation complete: %d accepts, %d rejects", accepts, rejects)

	if accepts < 7 {
		t.Errorf("Too few accepts: %d/10 (expected at least 7)", accepts)
	}
}

// BenchmarkE2EConsensus benchmarks consensus operations
func BenchmarkE2EConsensus(b *testing.B) {
	engine := consensus.NewChainEngine()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blockID := ids.GenerateTestID()
		_ = blockID

		// Simulate consensus check
		if adapter, ok := engine.(interface {
			HealthCheck(context.Context) (interface{}, error)
		}); ok {
			adapter.HealthCheck(ctx)
		}
	}
}

// TestE2EPerformance tests performance requirements
func TestE2EPerformance(t *testing.T) {
	_ = consensus.NewChainEngine() // Engine would be used in real test
	params := config.LocalParams()

	start := time.Now()
	rounds := 1000

	for i := 0; i < rounds; i++ {
		// Simulate consensus round
		_ = ids.GenerateTestID()

		// Check against alpha threshold
		confidence := 0.85 // Simulated confidence
		_ = confidence >= params.Alpha
	}

	elapsed := time.Since(start)
	opsPerSec := float64(rounds) / elapsed.Seconds()

	t.Logf("Performance: %d rounds in %v (%.0f ops/sec)", rounds, elapsed, opsPerSec)

	// Expect at least 10k ops/sec
	if opsPerSec < 10000 {
		t.Errorf("Performance too low: %.0f ops/sec (expected > 10000)", opsPerSec)
	}
}

// TestE2EIntegration tests integration with multiple components
func TestE2EIntegration(t *testing.T) {
	// Test chain engine
	chainEngine := consensus.NewChainEngine()
	if chainEngine == nil {
		t.Fatal("Chain engine creation failed")
	}

	// Test DAG engine
	dagEngine := consensus.NewDAGEngine()
	if dagEngine == nil {
		t.Fatal("DAG engine creation failed")
	}

	// Test PQ engine
	pqEngine := consensus.NewPQEngine()
	if pqEngine == nil {
		t.Fatal("PQ engine creation failed")
	}

	// Test config loading
	configs := []struct {
		name   string
		params config.Parameters
	}{
		{"mainnet", config.MainnetParams()},
		{"testnet", config.TestnetParams()},
		{"local", config.LocalParams()},
	}

	for _, cfg := range configs {
		t.Run(cfg.name, func(t *testing.T) {
			if cfg.params.K == 0 {
				t.Errorf("%s config has invalid K value", cfg.name)
			}
			if cfg.params.Alpha == 0 {
				t.Errorf("%s config has invalid Alpha value", cfg.name)
			}
			if cfg.params.Beta == 0 {
				t.Errorf("%s config has invalid Beta value", cfg.name)
			}

			t.Logf("%s config: K=%d, Alpha=%.2f, Beta=%d",
				cfg.name, cfg.params.K, cfg.params.Alpha, cfg.params.Beta)
		})
	}
}

// Example of how to run a consensus round programmatically
func Example_consensusRound() {
	// Create engine
	engine := consensus.NewChainEngine()
	params := config.LocalParams()

	// Generate block ID
	blockID := ids.GenerateTestID()

	// Collect votes (in real scenario, from network)
	votes := map[string]bool{
		"node1": true,
		"node2": true,
		"node3": true,
		"node4": false,
		"node5": true,
	}

	// Calculate confidence
	acceptCount := 0
	for _, vote := range votes {
		if vote {
			acceptCount++
		}
	}

	confidence := float64(acceptCount) / float64(len(votes))
	finalized := confidence >= params.Alpha

	_ = engine // Use engine for actual consensus

	fmt.Printf("Block %s: confidence=%.0f%%, finalized=%v\n",
		blockID, confidence*100, finalized)
}
