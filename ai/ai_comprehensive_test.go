// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Comprehensive AI Package Tests - Achieve 80%+ Coverage

package ai

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// Mock implementations for testing
type mockModel struct {
	mu        sync.Mutex
	responses map[string]*SimpleDecision
	errors    map[string]error
	callCount int
}

func (m *mockModel) Decide(ctx context.Context, question string, data map[string]interface{}) (*SimpleDecision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++

	if err, ok := m.errors[question]; ok {
		return nil, err
	}

	if resp, ok := m.responses[question]; ok {
		return resp, nil
	}

	// Default response
	return &SimpleDecision{
		Action:     "proceed",
		Confidence: 0.85,
		Reasoning:  "Default decision",
		Data:       data,
		Timestamp:  time.Now(),
	}, nil
}

type mockLogger struct {
	mu     sync.Mutex
	infos  []string
	warns  []string
	errors []string
}

func (l *mockLogger) Info(msg string, fields ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, msg)
}

func (l *mockLogger) Warn(msg string, fields ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
}

func (l *mockLogger) Error(msg string, fields ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, msg)
}

func newTestAgent() (*SimpleAgent, *mockModel, *mockLogger) {
	model := &mockModel{
		responses: make(map[string]*SimpleDecision),
		errors:    make(map[string]error),
	}
	logger := &mockLogger{}
	agent := NewSimple(model, logger)
	return agent, model, logger
}

// === CORE FUNCTIONALITY TESTS ===

func TestNewSimple(t *testing.T) {
	model := &mockModel{responses: make(map[string]*SimpleDecision), errors: make(map[string]error)}
	logger := &mockLogger{}

	agent := NewSimple(model, logger)

	if agent == nil {
		t.Fatal("NewSimple returned nil")
	}

	if agent.model == nil {
		t.Error("Agent model is nil")
	}

	if agent.log == nil {
		t.Error("Agent logger is nil")
	}

	if agent.state == nil {
		t.Error("Agent state is nil")
	}

	// Check initial state
	state := agent.GetState()
	if state.chains == nil || len(state.chains) != 0 {
		t.Error("Initial chains map not properly initialized")
	}
	if state.disputes == nil || len(state.disputes) != 0 {
		t.Error("Initial disputes map not properly initialized")
	}
	if state.upgrades == nil || len(state.upgrades) != 0 {
		t.Error("Initial upgrades map not properly initialized")
	}
	if state.security == nil || state.security.ThreatLevel != "low" {
		t.Error("Initial security state not properly initialized")
	}
}

// === STATE MANAGEMENT TESTS ===

func TestUpdateChain(t *testing.T) {
	agent, _, _ := newTestAgent()

	chainID := "chain-1"
	height := uint64(1000)
	hash := "0xabcdef"
	validators := []string{"val1", "val2", "val3"}
	perf := &Performance{
		TPS:           1500.0,
		Latency:       100,
		FaultRate:     0.01,
		UpgradeNeeded: false,
	}

	agent.UpdateChain(chainID, height, hash, validators, perf)

	state := agent.GetState()
	chain, exists := state.chains[chainID]
	if !exists {
		t.Fatal("Chain not found after update")
	}

	if chain.Height != height {
		t.Errorf("Expected height %d, got %d", height, chain.Height)
	}
	if chain.Hash != hash {
		t.Errorf("Expected hash %s, got %s", hash, chain.Hash)
	}
	if len(chain.Validators) != len(validators) {
		t.Errorf("Expected %d validators, got %d", len(validators), len(chain.Validators))
	}
	if chain.Performance.TPS != perf.TPS {
		t.Errorf("Expected TPS %.2f, got %.2f", perf.TPS, chain.Performance.TPS)
	}
}

func TestUpdateChainMultipleTimes(t *testing.T) {
	agent, _, _ := newTestAgent()

	chainID := "chain-1"

	// First update
	agent.UpdateChain(chainID, 100, "hash1", []string{"val1"}, &Performance{TPS: 100})

	// Second update
	agent.UpdateChain(chainID, 200, "hash2", []string{"val1", "val2"}, &Performance{TPS: 200})

	state := agent.GetState()
	chain := state.chains[chainID]

	if chain.Height != 200 {
		t.Errorf("Expected height 200, got %d", chain.Height)
	}
	if chain.Hash != "hash2" {
		t.Error("Chain hash not updated")
	}
	if len(chain.Validators) != 2 {
		t.Error("Validators not updated")
	}
}

func TestAddDispute(t *testing.T) {
	agent, _, _ := newTestAgent()

	disputeID := "dispute-1"
	disputeType := "fork"
	chainID := "chain-1"
	parties := []string{"validator-a", "validator-b"}
	evidence := []string{"block-a", "block-b"}

	agent.AddDispute(disputeID, disputeType, chainID, parties, evidence)

	state := agent.GetState()
	dispute, exists := state.disputes[disputeID]
	if !exists {
		t.Fatal("Dispute not found after adding")
	}

	if dispute.Type != disputeType {
		t.Errorf("Expected type %s, got %s", disputeType, dispute.Type)
	}
	if dispute.ChainID != chainID {
		t.Errorf("Expected chainID %s, got %s", chainID, dispute.ChainID)
	}
	if dispute.Status != "open" {
		t.Errorf("Expected status 'open', got %s", dispute.Status)
	}
	if len(dispute.Parties) != 2 {
		t.Error("Parties not properly stored")
	}
	if len(dispute.Evidence) != 2 {
		t.Error("Evidence not properly stored")
	}
}

func TestAddUpgrade(t *testing.T) {
	agent, _, _ := newTestAgent()

	upgradeID := "upgrade-1"
	upgradeType := "protocol"
	chainID := "chain-1"
	version := "v2.0.0"
	changes := []string{"improve consensus", "add feature X"}
	risk := "medium"

	agent.AddUpgrade(upgradeID, upgradeType, chainID, version, changes, risk)

	state := agent.GetState()
	upgrade, exists := state.upgrades[upgradeID]
	if !exists {
		t.Fatal("Upgrade not found after adding")
	}

	if upgrade.Type != upgradeType {
		t.Errorf("Expected type %s, got %s", upgradeType, upgrade.Type)
	}
	if upgrade.Version != version {
		t.Errorf("Expected version %s, got %s", version, upgrade.Version)
	}
	if upgrade.Risk != risk {
		t.Errorf("Expected risk %s, got %s", risk, upgrade.Risk)
	}
	if upgrade.Status != "proposed" {
		t.Errorf("Expected status 'proposed', got %s", upgrade.Status)
	}
	if len(upgrade.Changes) != 2 {
		t.Error("Changes not properly stored")
	}
}

func TestUpdateSecurity(t *testing.T) {
	agent, _, _ := newTestAgent()

	threatLevel := "high"
	threats := []string{"ddos", "51% attack"}
	mitigations := map[string]string{
		"ddos":       "rate limiting",
		"51% attack": "increase validators",
	}

	agent.UpdateSecurity(threatLevel, threats, mitigations)

	state := agent.GetState()
	if state.security.ThreatLevel != threatLevel {
		t.Errorf("Expected threat level %s, got %s", threatLevel, state.security.ThreatLevel)
	}
	if len(state.security.ActiveThreats) != 2 {
		t.Error("Threats not properly stored")
	}
	if len(state.security.Mitigations) != 2 {
		t.Error("Mitigations not properly stored")
	}
	if state.security.LastScan.IsZero() {
		t.Error("LastScan not set")
	}
}

func TestGetState(t *testing.T) {
	agent, _, _ := newTestAgent()

	// Add some data
	agent.UpdateChain("chain-1", 100, "hash", []string{"val1"}, &Performance{TPS: 100})
	agent.AddDispute("d1", "fork", "chain-1", []string{"a", "b"}, []string{"e1"})

	// Get state multiple times
	state1 := agent.GetState()
	state2 := agent.GetState()

	// Should be separate copies
	if state1 == state2 {
		t.Error("GetState returned same instance instead of copy")
	}

	// But should have same data
	if len(state1.chains) != len(state2.chains) {
		t.Error("State copies have different chain counts")
	}
	if len(state1.disputes) != len(state2.disputes) {
		t.Error("State copies have different dispute counts")
	}
}

// === DECISION TESTS ===

func TestShouldUpgrade_Success(t *testing.T) {
	agent, model, logger := newTestAgent()

	chainID := "chain-1"
	agent.UpdateChain(chainID, 1000, "hash", []string{"val1"}, &Performance{
		TPS:           500,
		Latency:       200,
		FaultRate:     0.05,
		UpgradeNeeded: true,
	})

	expectedQuestion := "Should chain chain-1 upgrade based on current performance and issues?"
	model.responses[expectedQuestion] = &SimpleDecision{
		Action:     "upgrade_to_v2",
		Confidence: 0.92,
		Reasoning:  "Performance is degraded, upgrade recommended",
		Timestamp:  time.Now(),
	}

	ctx := context.Background()
	decision, err := agent.ShouldUpgrade(ctx, chainID)

	if err != nil {
		t.Fatalf("ShouldUpgrade failed: %v", err)
	}

	if decision.Action != "upgrade_to_v2" {
		t.Errorf("Expected action 'upgrade_to_v2', got '%s'", decision.Action)
	}
	if decision.Confidence != 0.92 {
		t.Errorf("Expected confidence 0.92, got %.2f", decision.Confidence)
	}

	// Check logging
	if len(logger.infos) == 0 {
		t.Error("No info logs recorded")
	}
}

func TestShouldUpgrade_ChainNotFound(t *testing.T) {
	agent, _, _ := newTestAgent()

	ctx := context.Background()
	_, err := agent.ShouldUpgrade(ctx, "nonexistent")

	if err == nil {
		t.Error("Expected error for nonexistent chain")
	}
	if err.Error() != "chain nonexistent not found" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestShouldUpgrade_ModelError(t *testing.T) {
	agent, model, _ := newTestAgent()

	chainID := "chain-1"
	agent.UpdateChain(chainID, 1000, "hash", []string{"val1"}, &Performance{TPS: 500})

	expectedQuestion := "Should chain chain-1 upgrade based on current performance and issues?"
	model.errors[expectedQuestion] = errors.New("model unavailable")

	ctx := context.Background()
	_, err := agent.ShouldUpgrade(ctx, chainID)

	if err == nil {
		t.Error("Expected error from model failure")
	}
}

func TestResolveDispute_Success(t *testing.T) {
	agent, model, logger := newTestAgent()

	disputeID := "dispute-1"
	agent.AddDispute(disputeID, "fork", "chain-1", []string{"a", "b"}, []string{"evidence1"})

	expectedQuestion := "How should dispute dispute-1 be resolved based on evidence?"
	model.responses[expectedQuestion] = &SimpleDecision{
		Action:     "choose_fork_a",
		Confidence: 0.88,
		Reasoning:  "Fork A has stronger evidence",
		Timestamp:  time.Now(),
	}

	ctx := context.Background()
	decision, err := agent.ResolveDispute(ctx, disputeID)

	if err != nil {
		t.Fatalf("ResolveDispute failed: %v", err)
	}

	if decision.Action != "choose_fork_a" {
		t.Errorf("Expected action 'choose_fork_a', got '%s'", decision.Action)
	}

	// Check that dispute was marked as resolved
	state := agent.GetState()
	dispute := state.disputes[disputeID]
	if dispute.Status != "resolved" {
		t.Errorf("Expected dispute status 'resolved', got '%s'", dispute.Status)
	}
	if dispute.Resolution == "" {
		t.Error("Resolution not set")
	}
	if dispute.ResolvedAt.IsZero() {
		t.Error("ResolvedAt not set")
	}

	// Check logging
	if len(logger.infos) == 0 {
		t.Error("No info logs recorded")
	}
}

func TestResolveDispute_NotFound(t *testing.T) {
	agent, _, _ := newTestAgent()

	ctx := context.Background()
	_, err := agent.ResolveDispute(ctx, "nonexistent")

	if err == nil {
		t.Error("Expected error for nonexistent dispute")
	}
}

func TestResolveFork_Success(t *testing.T) {
	agent, model, logger := newTestAgent()

	chainID := "chain-1"
	forks := []string{"fork-a", "fork-b"}

	// Add state for both forks
	agent.UpdateChain("fork-a", 1000, "hash-a", []string{"val1"}, &Performance{TPS: 1000})
	agent.UpdateChain("fork-b", 1001, "hash-b", []string{"val1", "val2"}, &Performance{TPS: 1200})

	expectedQuestion := "Which fork should chain chain-1 follow?"
	model.responses[expectedQuestion] = &SimpleDecision{
		Action:     "fork-b",
		Confidence: 0.95,
		Reasoning:  "Fork B has higher performance and more validators",
		Timestamp:  time.Now(),
	}

	ctx := context.Background()
	decision, err := agent.ResolveFork(ctx, chainID, forks)

	if err != nil {
		t.Fatalf("ResolveFork failed: %v", err)
	}

	if decision.Action != "fork-b" {
		t.Errorf("Expected action 'fork-b', got '%s'", decision.Action)
	}

	// Check logging
	if len(logger.infos) == 0 {
		t.Error("No info logs recorded")
	}
}

func TestResolveFork_ModelError(t *testing.T) {
	agent, model, _ := newTestAgent()

	chainID := "chain-1"
	forks := []string{"fork-a", "fork-b"}

	expectedQuestion := "Which fork should chain chain-1 follow?"
	model.errors[expectedQuestion] = errors.New("insufficient data")

	ctx := context.Background()
	_, err := agent.ResolveFork(ctx, chainID, forks)

	if err == nil {
		t.Error("Expected error from model failure")
	}
}

func TestCheckSecurity_Success(t *testing.T) {
	agent, model, logger := newTestAgent()

	chainID := "chain-1"
	agent.UpdateChain(chainID, 1000, "hash", []string{"val1"}, &Performance{TPS: 1000})
	agent.UpdateSecurity("medium", []string{"ddos"}, map[string]string{"ddos": "rate limit"})

	expectedQuestion := "What security actions should be taken for chain chain-1?"
	model.responses[expectedQuestion] = &SimpleDecision{
		Action:     "increase_rate_limit",
		Confidence: 0.87,
		Reasoning:  "DDoS attack detected, strengthen defenses",
		Timestamp:  time.Now(),
	}

	ctx := context.Background()
	decision, err := agent.CheckSecurity(ctx, chainID)

	if err != nil {
		t.Fatalf("CheckSecurity failed: %v", err)
	}

	if decision.Action != "increase_rate_limit" {
		t.Errorf("Expected action 'increase_rate_limit', got '%s'", decision.Action)
	}

	// Check logging
	if len(logger.infos) == 0 {
		t.Error("No info logs recorded")
	}
}

func TestCheckSecurity_ModelError(t *testing.T) {
	agent, model, _ := newTestAgent()

	chainID := "chain-1"
	agent.UpdateChain(chainID, 1000, "hash", []string{"val1"}, &Performance{TPS: 1000})

	expectedQuestion := "What security actions should be taken for chain chain-1?"
	model.errors[expectedQuestion] = errors.New("analysis failed")

	ctx := context.Background()
	_, err := agent.CheckSecurity(ctx, chainID)

	if err == nil {
		t.Error("Expected error from model failure")
	}
}

// === CONCURRENCY TESTS ===

func TestConcurrentUpdates(t *testing.T) {
	agent, _, _ := newTestAgent()

	var wg sync.WaitGroup
	numGoroutines := 10
	updatesPerGoroutine := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < updatesPerGoroutine; j++ {
				chainID := "chain-1"
				height := uint64(j)
				agent.UpdateChain(chainID, height, "hash", []string{"val1"}, &Performance{TPS: 1000})
			}
		}(i)
	}

	wg.Wait()

	// Should not crash and state should be accessible
	state := agent.GetState()
	if state.chains["chain-1"] == nil {
		t.Error("Chain state lost during concurrent updates")
	}
}

func TestConcurrentReads(t *testing.T) {
	agent, _, _ := newTestAgent()

	// Setup initial state
	agent.UpdateChain("chain-1", 1000, "hash", []string{"val1"}, &Performance{TPS: 1000})
	agent.AddDispute("d1", "fork", "chain-1", []string{"a", "b"}, []string{"e1"})

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				state := agent.GetState()
				if state == nil {
					t.Error("GetState returned nil during concurrent access")
				}
			}
		}()
	}

	wg.Wait()
}

func TestConcurrentReadWrite(t *testing.T) {
	agent, _, _ := newTestAgent()

	var wg sync.WaitGroup
	numReaders := 10
	numWriters := 5

	// Readers
	wg.Add(numReaders)
	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				state := agent.GetState()
				_ = state
			}
		}()
	}

	// Writers
	wg.Add(numWriters)
	for i := 0; i < numWriters; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				agent.UpdateChain("chain-1", uint64(j), "hash", []string{"val1"}, &Performance{TPS: 1000})
			}
		}(i)
	}

	wg.Wait()

	// Should not deadlock or crash
	state := agent.GetState()
	if state == nil {
		t.Error("State corrupted after concurrent read/write")
	}
}

// === EDGE CASE TESTS ===

func TestEmptyValidators(t *testing.T) {
	agent, _, _ := newTestAgent()

	agent.UpdateChain("chain-1", 100, "hash", []string{}, &Performance{TPS: 100})

	state := agent.GetState()
	chain := state.chains["chain-1"]
	if chain == nil {
		t.Fatal("Chain not created")
	}
	if len(chain.Validators) != 0 {
		t.Error("Empty validators not handled")
	}
}

func TestNilPerformance(t *testing.T) {
	agent, _, _ := newTestAgent()

	// Should not crash with nil performance
	agent.UpdateChain("chain-1", 100, "hash", []string{"val1"}, nil)

	state := agent.GetState()
	chain := state.chains["chain-1"]
	if chain == nil {
		t.Fatal("Chain not created with nil performance")
	}
}

func TestEmptyDisputeEvidence(t *testing.T) {
	agent, _, _ := newTestAgent()

	agent.AddDispute("d1", "fork", "chain-1", []string{"a"}, []string{})

	state := agent.GetState()
	dispute := state.disputes["d1"]
	if dispute == nil {
		t.Fatal("Dispute not created with empty evidence")
	}
	if len(dispute.Evidence) != 0 {
		t.Error("Empty evidence not handled")
	}
}

// === BENCHMARK TESTS ===

func BenchmarkUpdateChain(b *testing.B) {
	agent, _, _ := newTestAgent()
	perf := &Performance{TPS: 1000, Latency: 100, FaultRate: 0.01}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agent.UpdateChain("chain-1", uint64(i), "hash", []string{"val1"}, perf)
	}
}

func BenchmarkGetState(b *testing.B) {
	agent, _, _ := newTestAgent()

	// Setup some state
	for i := 0; i < 100; i++ {
		agent.UpdateChain("chain-1", uint64(i), "hash", []string{"val1"}, &Performance{TPS: 1000})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = agent.GetState()
	}
}

func BenchmarkShouldUpgrade(b *testing.B) {
	agent, _, _ := newTestAgent()
	agent.UpdateChain("chain-1", 1000, "hash", []string{"val1"}, &Performance{TPS: 1000})

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = agent.ShouldUpgrade(ctx, "chain-1")
	}
}

func BenchmarkConcurrentAccess(b *testing.B) {
	agent, _, _ := newTestAgent()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			agent.UpdateChain("chain-1", 1000, "hash", []string{"val1"}, &Performance{TPS: 1000})
			_ = agent.GetState()
		}
	})
}
