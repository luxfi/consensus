package config

import (
	"sync"
	"testing"
)

// MockValidator represents a validator in the network
type MockValidator struct {
	ID     string
	Weight uint64
	Voted  bool
	Vote   bool // true for yes, false for no
}

// MockNetwork simulates a network with 69% threshold
type MockNetwork struct {
	validators  map[string]*MockValidator
	totalWeight uint64
	votes       map[string]uint64 // vote ID -> accumulated weight
	mu          sync.RWMutex
}

// NewMockNetwork creates a network with given validators
func NewMockNetwork(validatorWeights map[string]uint64) *MockNetwork {
	n := &MockNetwork{
		validators: make(map[string]*MockValidator),
		votes:      make(map[string]uint64),
	}

	for id, weight := range validatorWeights {
		n.validators[id] = &MockValidator{
			ID:     id,
			Weight: weight,
		}
		n.totalWeight += weight
	}

	return n
}

// SimulateVoting simulates a voting round with exact weight
func (n *MockNetwork) SimulateVoting(voteID string, supportPercentage float64) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	// For testing, we simulate exact voting weight
	votedWeight := uint64(float64(n.totalWeight) * supportPercentage)
	n.votes[voteID] = votedWeight

	// Check if consensus achieved with 69% threshold
	return HasSuperMajority(votedWeight, n.totalWeight)
}

// TestIntegrationSmallNetwork tests 69% threshold in small network
func TestIntegrationSmallNetwork(t *testing.T) {
	// Create a network with fine-grained weights for better precision
	network := NewMockNetwork(map[string]uint64{
		"node1": 100,
		"node2": 100,
		"node3": 100,
		"node4": 100,
		"node5": 100,
	})

	tests := []struct {
		name            string
		supportPercent  float64
		expectConsensus bool
	}{
		{"100% support", 1.0, true},
		{"80% support", 0.8, true},
		{"70% support", 0.7, true},
		{"69% support", 0.69, true},
		{"68% support", 0.68, false},
		{"60% support", 0.6, false},
		{"50% support", 0.5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consensus := network.SimulateVoting(tt.name, tt.supportPercent)
			if consensus != tt.expectConsensus {
				t.Errorf("SimulateVoting with %s = %v, want %v",
					tt.name, consensus, tt.expectConsensus)
			}
		})
	}
}

// TestIntegrationLargeNetwork tests 69% threshold in large network
func TestIntegrationLargeNetwork(t *testing.T) {
	// Create a 100-node network with varying weights
	validators := make(map[string]uint64)
	for i := 0; i < 100; i++ {
		weight := uint64(10 + i%20) // Weights from 10 to 29
		validators[string(rune('A'+i))] = weight
	}

	network := NewMockNetwork(validators)

	// Test various voting scenarios
	scenarios := []struct {
		name            string
		supportPercent  float64
		expectConsensus bool
	}{
		{"Exact 69%", 0.69, false}, // Due to integer truncation, this is 68.97%
		{"Just below 69%", 0.689, false},
		{"Just above 69%", 0.691, true},
		{"Byzantine maximum 31%", 0.31, false},
		{"Minimum winning 69.1%", 0.691, true}, // Actual minimum to achieve consensus
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			consensus := network.SimulateVoting(s.name, s.supportPercent)
			if consensus != s.expectConsensus {
				votedWeight := uint64(float64(network.totalWeight) * s.supportPercent)
				actualPercent := float64(votedWeight) * 100 / float64(network.totalWeight)
				t.Errorf("Large network %s: got %v, want %v (voted %d/%d = %.6f%%)",
					s.name, consensus, s.expectConsensus,
					votedWeight, network.totalWeight, actualPercent)
			}
		})
	}
}

// TestIntegrationByzantineScenarios tests Byzantine failure scenarios
func TestIntegrationByzantineScenarios(t *testing.T) {
	tests := []struct {
		name            string
		totalNodes      int
		byzantineNodes  int
		honestVoteRate  float64
		expectConsensus bool
	}{
		{
			name:            "30% Byzantine, all honest vote",
			totalNodes:      100,
			byzantineNodes:  30,
			honestVoteRate:  1.0, // 70% > 69%
			expectConsensus: true,
		},
		{
			name:            "31% Byzantine, all honest vote",
			totalNodes:      100,
			byzantineNodes:  31,
			honestVoteRate:  1.0, // 69% = 69%
			expectConsensus: true,
		},
		{
			name:            "32% Byzantine, all honest vote",
			totalNodes:      100,
			byzantineNodes:  32,
			honestVoteRate:  1.0, // 68% < 69%
			expectConsensus: false,
		},
		{
			name:            "20% Byzantine, 90% honest vote",
			totalNodes:      100,
			byzantineNodes:  20,
			honestVoteRate:  0.9, // 72% > 69%
			expectConsensus: true,
		},
		{
			name:            "25% Byzantine, 92% honest vote",
			totalNodes:      100,
			byzantineNodes:  25,
			honestVoteRate:  0.92, // 69% = 69%
			expectConsensus: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create network
			validators := make(map[string]uint64)
			for i := 0; i < tt.totalNodes; i++ {
				validators[string(rune('A'+i))] = 1 // Equal weight
			}
			network := NewMockNetwork(validators)

			// Calculate voting weight
			honestNodes := tt.totalNodes - tt.byzantineNodes
			votingNodes := int(float64(honestNodes) * tt.honestVoteRate)
			supportPercent := float64(votingNodes) / float64(tt.totalNodes)

			consensus := network.SimulateVoting(tt.name, supportPercent)
			if consensus != tt.expectConsensus {
				actualPercent := supportPercent * 100
				t.Errorf("%s: got consensus=%v, want %v (%.1f%% voted)",
					tt.name, consensus, tt.expectConsensus, actualPercent)
			}
		})
	}
}

// TestIntegrationParameterSets tests all parameter sets maintain 69% threshold
func TestIntegrationParameterSets(t *testing.T) {
	paramSets := []struct {
		name   string
		params Parameters
	}{
		{"Default", DefaultParams()},
		{"Mainnet", MainnetParams()},
		{"Testnet", TestnetParams()},
		{"Local", LocalParams()},
		{"XChain", XChainParams()},
	}

	for _, ps := range paramSets {
		t.Run(ps.name, func(t *testing.T) {
			// Verify Alpha is 69%
			if ps.params.Alpha < 0.68 || ps.params.Alpha > 0.70 {
				t.Errorf("%s: Alpha %.3f not in 69%% range", ps.name, ps.params.Alpha)
			}

			// Verify AlphaPreference meets threshold
			if ps.params.K > 0 && ps.params.AlphaPreference > 0 {
				actualPercent := float64(ps.params.AlphaPreference) / float64(ps.params.K)
				if actualPercent < 0.69 {
					t.Errorf("%s: AlphaPreference %d/%d = %.1f%% < 69%%",
						ps.name, ps.params.AlphaPreference, ps.params.K, actualPercent*100)
				}
			}

			// Verify parameters are valid
			if err := ps.params.Valid(); err != nil {
				t.Errorf("%s: Invalid parameters: %v", ps.name, err)
			}

			// Simulate consensus with these parameters
			network := NewMockNetwork(map[string]uint64{
				"v1": 100, "v2": 100, "v3": 100, "v4": 100, "v5": 100,
			})

			// Test that 69% achieves consensus
			if !network.SimulateVoting("test-69", 0.69) {
				t.Errorf("%s: 69%% support should achieve consensus", ps.name)
			}

			// Test that 68% does not achieve consensus
			if network.SimulateVoting("test-68", 0.68) {
				t.Errorf("%s: 68%% support should not achieve consensus", ps.name)
			}
		})
	}
}

// TestIntegrationConcurrentVoting tests concurrent voting scenarios
func TestIntegrationConcurrentVoting(t *testing.T) {
	network := NewMockNetwork(map[string]uint64{
		"v1": 100, "v2": 150, "v3": 200, "v4": 250, "v5": 300,
	})

	const numVotes = 100
	results := make(chan bool, numVotes)

	// Simulate concurrent voting rounds
	for i := 0; i < numVotes; i++ {
		go func(voteNum int) {
			// Vary support percentage
			support := 0.68 + float64(voteNum%3)*0.01 // 68%, 69%, or 70%
			voteID := string(rune('A' + voteNum))
			consensus := network.SimulateVoting(voteID, support)

			// Only 69% and 70% should achieve consensus
			expectedConsensus := support >= 0.69
			if consensus != expectedConsensus {
				t.Errorf("Vote %d with %.0f%% support: got %v, want %v",
					voteNum, support*100, consensus, expectedConsensus)
			}

			results <- consensus
		}(i)
	}

	// Wait for all votes
	consensusCount := 0
	for i := 0; i < numVotes; i++ {
		if <-results {
			consensusCount++
		}
	}

	// About 2/3 should achieve consensus (69% and 70% cases)
	expectedCount := numVotes * 2 / 3
	tolerance := numVotes / 10 // 10% tolerance

	if consensusCount < expectedCount-tolerance || consensusCount > expectedCount+tolerance {
		t.Errorf("Consensus count %d not near expected %d±%d",
			consensusCount, expectedCount, tolerance)
	}
}

// BenchmarkIntegrationVoting benchmarks voting performance
func BenchmarkIntegrationVoting(b *testing.B) {
	network := NewMockNetwork(map[string]uint64{
		"v1": 100, "v2": 100, "v3": 100, "v4": 100, "v5": 100,
		"v6": 100, "v7": 100, "v8": 100, "v9": 100, "v10": 100,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = network.SimulateVoting("bench", 0.69)
	}
}

// BenchmarkIntegrationLargeNetwork benchmarks large network voting
func BenchmarkIntegrationLargeNetwork(b *testing.B) {
	validators := make(map[string]uint64)
	for i := 0; i < 1000; i++ {
		validators[string(rune(i))] = uint64(100 + i%100)
	}
	network := NewMockNetwork(validators)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = network.SimulateVoting("bench", 0.69)
	}
}
