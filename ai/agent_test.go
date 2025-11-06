// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Agentic AI Consensus - Tests

package ai

import (
	"context"
	"testing"
	"time"
)

// === MOCK IMPLEMENTATIONS ===

// mockModel implements Model[T] for testing
type mockAgentModel[T ConsensusData] struct {
	proposeErr       error
	validateConf     float64
	validateErr      error
	learnErr         error
	updateWeightsErr error
	state            map[string]interface{}
	stateErr         error
}

func (m *mockAgentModel[T]) Decide(ctx context.Context, input T, context map[string]interface{}) (*Decision[T], error) {
	return &Decision[T]{
		ID:         generateID(),
		Action:     "approve",
		Data:       input,
		Confidence: 0.8,
		Reasoning:  "mock decision",
		Context:    context,
		Timestamp:  time.Now(),
	}, nil
}

func (m *mockAgentModel[T]) ProposeDecision(ctx context.Context, input T) (*Proposal[T], error) {
	if m.proposeErr != nil {
		return nil, m.proposeErr
	}
	
	decision, _ := m.Decide(ctx, input, nil)
	return &Proposal[T]{
		ID:         generateID(),
		NodeID:     "test-node",
		Decision:   decision,
		Evidence:   []Evidence[T]{},
		Weight:     1.0,
		Confidence: 0.8,
		Timestamp:  time.Now(),
	}, nil
}

func (m *mockAgentModel[T]) ValidateProposal(proposal *Proposal[T]) (float64, error) {
	if m.validateErr != nil {
		return 0, m.validateErr
	}
	return m.validateConf, nil
}

func (m *mockAgentModel[T]) Learn(examples []TrainingExample[T]) error {
	return m.learnErr
}

func (m *mockAgentModel[T]) UpdateWeights(gradients []float64) error {
	return m.updateWeightsErr
}

func (m *mockAgentModel[T]) GetState() map[string]interface{} {
	if m.state != nil {
		return m.state
	}
	return map[string]interface{}{
		"weights": map[string]float64{"test": 1.0},
		"bias":    0.5,
	}
}

func (m *mockAgentModel[T]) LoadState(state map[string]interface{}) error {
	return m.stateErr
}

// For testing, we'll use nil for quasar and photon since
// most functions don't actually use them (or we mock their behavior)

// === AGENT CONSTRUCTOR TESTS ===

func TestNewAgent(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	quasar := nil
	photon := nil

	agent := New("test-node", model, quasar, photon)

	if agent == nil {
		t.Fatal("New() returned nil")
	}

	if agent.nodeID != "test-node" {
		t.Errorf("Expected nodeID 'test-node', got '%s'", agent.nodeID)
	}

	if agent.model == nil {
		t.Error("Agent model is nil")
	}

	if agent.quasar == nil {
		t.Error("Agent quasar is nil")
	}

	if agent.photon == nil {
		t.Error("Agent photon is nil")
	}

	if agent.hallucinations == nil {
		t.Error("hallucinations map is nil")
	}

	if agent.weights == nil {
		t.Error("weights map is nil")
	}

	if agent.usage == nil {
		t.Error("usage map is nil")
	}

	if agent.memory == nil {
		t.Error("memory is nil")
	}

	if agent.consensus == nil {
		t.Error("consensus is nil")
	}
}

// === PROPOSE DECISION TESTS ===

func TestAgentProposeDecision_Success(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	input := BlockData{
		Height:       100,
		Hash:         "0x123",
		Timestamp:    time.Now(),
		TxCount:      10,
		Size:         1024,
		GasUsed:      50000,
	}

	ctx := context.Background()
	decision, err := agent.ProposeDecision(ctx, input, nil)

	if err != nil {
		t.Fatalf("ProposeDecision() error = %v", err)
	}

	if decision == nil {
		t.Fatal("decision is nil")
	}

	if decision.Action != "approve" {
		t.Errorf("Expected action 'approve', got '%s'", decision.Action)
	}

	if decision.Confidence <= 0 {
		t.Errorf("Expected positive confidence, got %f", decision.Confidence)
	}

	// Check consensus state
	if agent.consensus.Phase != PhaseHorizon {
		t.Errorf("Expected phase Horizon, got %s", agent.consensus.Phase)
	}

	if agent.consensus.Finalized == nil {
		t.Error("consensus.Finalized is nil")
	}
}

func TestAgentProposeDecision_PhotonError(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	photon := &mockPhoton{emitErr: &testError{msg: "photon broadcast failed"}}
	agent := New("test-node", model, nil, photon)

	input := BlockData{Height: 100, Timestamp: time.Now()}
	ctx := context.Background()

	_, err := agent.ProposeDecision(ctx, input, nil)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if err.Error() != "wave broadcast failed: photon broadcast failed" {
		t.Errorf("Unexpected error: %v", err)
	}
}

// === TRAINING DATA TESTS ===

func TestAgentAddTrainingData(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	// Set initial weight for node
	agent.weights["node-1"] = 0.8

	example := TrainingExample[BlockData]{
		Input: BlockData{
			Height:    100,
			Timestamp: time.Now(),
		},
		Output: Decision[BlockData]{
			Action:     "approve",
			Confidence: 0.9,
		},
		Feedback: 1.0,
		NodeID:   "node-1",
	}

	agent.AddTrainingData(example)

	// Check training data was added
	if len(agent.trainingData) != 1 {
		t.Errorf("Expected 1 training example, got %d", len(agent.trainingData))
	}

	// Check weight was applied
	if agent.trainingData[0].Weight != 0.8 {
		t.Errorf("Expected weight 0.8, got %f", agent.trainingData[0].Weight)
	}

	// Check shared memory
	if len(agent.memory.trainingQueue) != 1 {
		t.Errorf("Expected 1 example in memory queue, got %d", len(agent.memory.trainingQueue))
	}

	// Check usage tracking
	usageKey := "approve_node-1"
	if agent.usage[usageKey] != 1 {
		t.Errorf("Expected usage count 1 for key %s, got %d", usageKey, agent.usage[usageKey])
	}
}

func TestAgentAddTrainingData_NewNode(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	example := TrainingExample[BlockData]{
		Input:    BlockData{Height: 100, Timestamp: time.Now()},
		Output:   Decision[BlockData]{Action: "approve"},
		Feedback: 1.0,
		NodeID:   "new-node",
	}

	agent.AddTrainingData(example)

	// Check default weight was applied
	if agent.trainingData[0].Weight != 0.1 {
		t.Errorf("Expected default weight 0.1, got %f", agent.trainingData[0].Weight)
	}
}

// === SYNC SHARED MEMORY TESTS ===

func TestAgentSyncSharedMemory_TooSoon(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	// Set last sync to now
	agent.memory.lastSync = time.Now()

	ctx := context.Background()
	err := agent.SyncSharedMemory(ctx)

	if err != nil {
		t.Errorf("SyncSharedMemory() error = %v", err)
	}

	// Should return nil (too soon to sync)
}

func TestAgentSyncSharedMemory_Success(t *testing.T) {
	model := &mockAgentModel[BlockData]{
		state: map[string]interface{}{
			"test_param": 1.5,
		},
	}
	agent := New("test-node", model, nil, nil)

	// Set last sync to past
	agent.memory.lastSync = time.Now().Add(-1 * time.Minute)

	// Add training examples
	agent.memory.trainingQueue = []TrainingExample[BlockData]{
		{
			Input:    BlockData{Height: 100, Timestamp: time.Now()},
			Output:   Decision[BlockData]{Action: "approve"},
			Feedback: 1.0,
		},
	}

	ctx := context.Background()
	err := agent.SyncSharedMemory(ctx)

	if err != nil {
		t.Fatalf("SyncSharedMemory() error = %v", err)
	}

	// Check training queue was cleared
	if len(agent.memory.trainingQueue) != 0 {
		t.Errorf("Expected empty training queue, got %d items", len(agent.memory.trainingQueue))
	}

	// Check lastSync was updated
	if time.Since(agent.memory.lastSync) > time.Second {
		t.Error("lastSync was not updated")
	}
}

func TestAgentSyncSharedMemory_LoadStateError(t *testing.T) {
	model := &mockAgentModel[BlockData]{
		stateErr: &testError{msg: "load state failed"},
	}
	agent := New("test-node", model, nil, nil)
	agent.memory.lastSync = time.Now().Add(-1 * time.Minute)

	ctx := context.Background()
	err := agent.SyncSharedMemory(ctx)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if err.Error() != "failed to load aggregated state: load state failed" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestAgentSyncSharedMemory_LearnError(t *testing.T) {
	model := &mockAgentModel[BlockData]{
		learnErr: &testError{msg: "learn failed"},
	}
	agent := New("test-node", model, nil, nil)
	agent.memory.lastSync = time.Now().Add(-1 * time.Minute)
	agent.memory.trainingQueue = []TrainingExample[BlockData]{
		{Input: BlockData{Height: 100, Timestamp: time.Now()}},
	}

	ctx := context.Background()
	err := agent.SyncSharedMemory(ctx)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if err.Error() != "shared learning failed: learn failed" {
		t.Errorf("Unexpected error: %v", err)
	}
}

// === NODE WEIGHT TESTS ===

func TestAgentUpdateNodeWeight_Increase(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	// Set initial weight
	agent.weights["node-1"] = 1.0

	// Update with good performance (>1.0)
	agent.UpdateNodeWeight("node-1", 2.0)

	// Weight should increase
	newWeight := agent.weights["node-1"]
	if newWeight <= 1.0 {
		t.Errorf("Expected weight > 1.0, got %f", newWeight)
	}

	// Check it was updated in memory too
	if agent.memory.nodeWeights["node-1"] != newWeight {
		t.Error("Weight not updated in memory")
	}
}

func TestAgentUpdateNodeWeight_Decrease(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	agent.weights["node-1"] = 1.0

	// Update with poor performance (0.0)
	agent.UpdateNodeWeight("node-1", 0.0)

	// Weight should decrease
	newWeight := agent.weights["node-1"]
	if newWeight >= 1.0 {
		t.Errorf("Expected weight < 1.0, got %f", newWeight)
	}
}

func TestAgentUpdateNodeWeight_MinClamp(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	agent.weights["node-1"] = 0.05

	// Update with very poor performance
	agent.UpdateNodeWeight("node-1", 0.0)

	// Weight should be clamped to minimum
	newWeight := agent.weights["node-1"]
	if newWeight < 0.01 {
		t.Errorf("Expected weight >= 0.01, got %f", newWeight)
	}
}

func TestAgentUpdateNodeWeight_MaxClamp(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	agent.weights["node-1"] = 9.5

	// Update with excellent performance
	agent.UpdateNodeWeight("node-1", 15.0)

	// Weight should be clamped to maximum
	newWeight := agent.weights["node-1"]
	if newWeight > 10.0 {
		t.Errorf("Expected weight <= 10.0, got %f", newWeight)
	}
}

// === HALLUCINATION TESTS ===

func TestAgentGetSharedHallucination_Exists(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	// Add a hallucination
	testID := "test-hallucination"
	agent.hallucinations[testID] = &Hallucination[BlockData]{
		ID:         testID,
		ModelID:    "test-model",
		Confidence: 0.9,
		State:      map[string]interface{}{"test": 1},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	hallucination, exists := agent.GetSharedHallucination(testID)

	if !exists {
		t.Fatal("Expected hallucination to exist")
	}

	if hallucination == nil {
		t.Fatal("hallucination is nil")
	}

	if hallucination.ID != testID {
		t.Errorf("Expected ID %s, got %s", testID, hallucination.ID)
	}
}

func TestAgentGetSharedHallucination_NotExists(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	hallucination, exists := agent.GetSharedHallucination("nonexistent")

	if exists {
		t.Error("Expected hallucination to not exist")
	}

	if hallucination != nil {
		t.Error("Expected nil hallucination")
	}
}

// === AGGREGATE MODEL STATES TESTS ===

func TestAggregateModelStates(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	// Add model states from multiple nodes
	agent.memory.modelStates["node-1"] = map[string]interface{}{
		"param1": 2.0,
		"param2": 4.0,
	}
	agent.memory.modelStates["node-2"] = map[string]interface{}{
		"param1": 4.0,
		"param2": 2.0,
	}

	// Set equal weights
	agent.memory.nodeWeights["node-1"] = 1.0
	agent.memory.nodeWeights["node-2"] = 1.0

	// Aggregate
	aggregated := agent.aggregateModelStates()

	// Check averaged values (should be 3.0 for both params)
	if param1, ok := aggregated["param1"].(float64); !ok || param1 != 3.0 {
		t.Errorf("Expected param1 = 3.0, got %v", aggregated["param1"])
	}

	if param2, ok := aggregated["param2"].(float64); !ok || param2 != 3.0 {
		t.Errorf("Expected param2 = 3.0, got %v", aggregated["param2"])
	}
}

func TestAggregateModelStates_WeightedAverage(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	// Add states with different weights
	agent.memory.modelStates["node-1"] = map[string]interface{}{
		"param": 2.0,
	}
	agent.memory.modelStates["node-2"] = map[string]interface{}{
		"param": 8.0,
	}

	// Node 2 has 3x weight of node 1
	agent.memory.nodeWeights["node-1"] = 1.0
	agent.memory.nodeWeights["node-2"] = 3.0

	aggregated := agent.aggregateModelStates()

	// Weighted average: (2*1 + 8*3) / (1+3) = 26/4 = 6.5
	if param, ok := aggregated["param"].(float64); !ok || param != 6.5 {
		t.Errorf("Expected param = 6.5, got %v", aggregated["param"])
	}
}

func TestAggregateModelStates_DefaultWeight(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	// Add state without setting weight
	agent.memory.modelStates["node-1"] = map[string]interface{}{
		"param": 5.0,
	}

	aggregated := agent.aggregateModelStates()

	// Should use default weight of 0.1
	// Result: 5.0 * 0.1 / 0.1 = 5.0
	if param, ok := aggregated["param"].(float64); !ok || param != 5.0 {
		t.Errorf("Expected param = 5.0, got %v", aggregated["param"])
	}
}

// === CONCURRENT ACCESS TESTS ===

func TestAgentConcurrentAddTraining(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			example := TrainingExample[BlockData]{
				Input:    BlockData{Height: uint64(id), Timestamp: time.Now()},
				Output:   Decision[BlockData]{Action: "approve"},
				Feedback: 1.0,
				NodeID:   "test-node",
			}
			agent.AddTrainingData(example)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	if len(agent.trainingData) != numGoroutines {
		t.Errorf("Expected %d training examples, got %d", numGoroutines, len(agent.trainingData))
	}
}

func TestAgentConcurrentUpdateWeight(t *testing.T) {
	model := &mockAgentModel[BlockData]{}
	agent := New("test-node", model, nil, nil)

	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(performance float64) {
			agent.UpdateNodeWeight("test-node", performance)
			done <- true
		}(float64(i) / 10.0)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Should not panic and should have a valid weight
	weight := agent.weights["test-node"]
	if weight < 0.01 || weight > 10.0 {
		t.Errorf("Weight out of bounds: %f", weight)
	}
}

// Test error type
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
