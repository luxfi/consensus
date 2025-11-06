// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Comprehensive Tests for AI Models

package ai

import (
	"context"
	"testing"
	"time"
)

// Mock FeatureExtractor for BlockData
type mockBlockFeatureExtractor struct{}

func (e *mockBlockFeatureExtractor) Extract(data BlockData) map[string]float64 {
	return map[string]float64{
		"height":   float64(data.Height),
		"tx_count": float64(len(data.Transactions)),
		"size":     float64(data.Size),
		"gas_used": float64(data.GasUsed),
	}
}

func (e *mockBlockFeatureExtractor) Names() []string {
	return []string{"height", "tx_count", "size", "gas_used"}
}

// === SimpleModel Tests ===

func TestNewSimpleModel(t *testing.T) {
	extractor := &mockBlockFeatureExtractor{}
	model := NewSimpleModel[BlockData]("node-1", extractor)

	if model == nil {
		t.Fatal("NewSimpleModel returned nil")
	}

	if model.nodeID != "node-1" {
		t.Errorf("Expected nodeID 'node-1', got '%s'", model.nodeID)
	}

	if model.learningRate != 0.01 {
		t.Errorf("Expected learning rate 0.01, got %f", model.learningRate)
	}

	if model.bias != 0.0 {
		t.Error("Initial bias should be 0.0")
	}

	if len(model.weights) != 0 {
		t.Error("Initial weights should be empty")
	}

	if len(model.history) != 0 {
		t.Error("Initial history should be empty")
	}
}

func TestSimpleModelDecide(t *testing.T) {
	extractor := &mockBlockFeatureExtractor{}
	model := NewSimpleModel[BlockData]("node-1", extractor)

	// Set some weights
	model.weights["height"] = 0.5
	model.weights["tx_count"] = 1.0
	model.bias = 0.1

	data := BlockData{
		Height:       100,
		Transactions: make([]string, 10),
		Size:         2048,
		GasUsed:      50000,
		Timestamp:    time.Now(),
	}

	ctx := context.Background()
	decision, err := model.Decide(ctx, data, make(map[string]interface{}))

	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}

	if decision == nil {
		t.Fatal("Decision is nil")
	}

	if decision.ID == "" {
		t.Error("Decision ID not set")
	}

	if decision.ProposerID != "node-1" {
		t.Errorf("Expected proposer 'node-1', got '%s'", decision.ProposerID)
	}

	if decision.Confidence < 0 || decision.Confidence > 1 {
		t.Errorf("Confidence should be between 0 and 1, got %f", decision.Confidence)
	}

	if decision.Action != "approve" && decision.Action != "reject" {
		t.Errorf("Unexpected action: %s", decision.Action)
	}

	if decision.Reasoning == "" {
		t.Error("Reasoning should not be empty")
	}

	if decision.Timestamp.IsZero() {
		t.Error("Timestamp not set")
	}
}

func TestSimpleModelDecide_ApproveAction(t *testing.T) {
	extractor := &mockBlockFeatureExtractor{}
	model := NewSimpleModel[BlockData]("node-1", extractor)

	// Set weights that will result in positive score
	model.weights["tx_count"] = 1.0
	model.bias = 10.0 // Strong positive bias

	data := BlockData{
		Height:       100,
		Transactions: make([]string, 10),
		Size:         2048,
		GasUsed:      50000,
		Timestamp:    time.Now(),
	}

	ctx := context.Background()
	decision, err := model.Decide(ctx, data, nil)

	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}

	if decision.Action != "approve" {
		t.Errorf("Expected action 'approve', got '%s'", decision.Action)
	}
}

func TestSimpleModelDecide_RejectAction(t *testing.T) {
	extractor := &mockBlockFeatureExtractor{}
	model := NewSimpleModel[BlockData]("node-1", extractor)

	// Set weights that will result in negative score
	model.weights["tx_count"] = -1.0
	model.bias = -10.0 // Strong negative bias

	data := BlockData{
		Height:       100,
		Transactions: make([]string, 10),
		Size:         2048,
		GasUsed:      50000,
		Timestamp:    time.Now(),
	}

	ctx := context.Background()
	decision, err := model.Decide(ctx, data, nil)

	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}

	if decision.Action != "reject" {
		t.Errorf("Expected action 'reject', got '%s'", decision.Action)
	}
}

func TestSimpleModelProposeDecision(t *testing.T) {
	extractor := &mockBlockFeatureExtractor{}
	model := NewSimpleModel[BlockData]("node-1", extractor)

	data := BlockData{
		Height:       100,
		Transactions: make([]string, 10),
		Size:         2048,
		GasUsed:      50000,
		Timestamp:    time.Now(),
	}

	ctx := context.Background()
	proposal, err := model.ProposeDecision(ctx, data)

	if err != nil {
		t.Fatalf("ProposeDecision failed: %v", err)
	}

	if proposal == nil {
		t.Fatal("Proposal is nil")
	}

	if proposal.ID == "" {
		t.Error("Proposal ID not set")
	}

	if proposal.NodeID != "node-1" {
		t.Errorf("Expected node 'node-1', got '%s'", proposal.NodeID)
	}

	if proposal.Decision == nil {
		t.Fatal("Proposal decision is nil")
	}

	if len(proposal.Evidence) == 0 {
		t.Error("Proposal should have evidence")
	}

	if proposal.Weight != 1.0 {
		t.Errorf("Expected weight 1.0, got %f", proposal.Weight)
	}

	if proposal.Timestamp.IsZero() {
		t.Error("Proposal timestamp not set")
	}
}

func TestSimpleModelLearn(t *testing.T) {
	extractor := &mockBlockFeatureExtractor{}
	model := NewSimpleModel[BlockData]("node-1", extractor)

	blockData := BlockData{
		Height:       100,
		Transactions: make([]string, 10),
		Size:         2048,
		GasUsed:      50000,
		Timestamp:    time.Now(),
	}

	example := TrainingExample[BlockData]{
		Input: blockData,
		Output: Decision[BlockData]{
			ID:         "test-decision",
			Action:     "approve",
			Data:       blockData,
			Confidence: 0.9,
			Reasoning:  "Test decision",
			Context:    make(map[string]interface{}),
			Timestamp:  time.Now(),
			ProposerID: "node-1",
		},
		Feedback: 1.0, // Positive feedback
		NodeID:   "node-1",
		Weight:   1.0,
		Context:  make(map[string]interface{}),
	}

	initialBias := model.bias
	initialWeights := make(map[string]float64)
	for k, v := range model.weights {
		initialWeights[k] = v
	}

	examples := []TrainingExample[BlockData]{example}
	model.Learn(examples)

	// Weights should have changed
	if model.bias == initialBias && len(model.weights) == 0 {
		t.Error("Model weights should have been updated")
	}

	// History should contain the example
	if len(model.history) != 1 {
		t.Errorf("Expected 1 history entry, got %d", len(model.history))
	}
}

func TestSimpleModelLearnBatch(t *testing.T) {
	extractor := &mockBlockFeatureExtractor{}
	model := NewSimpleModel[BlockData]("node-1", extractor)

	blockData1 := BlockData{
		Height:       100,
		Transactions: make([]string, 10),
		Size:         2048,
		GasUsed:      50000,
		Timestamp:    time.Now(),
	}

	blockData2 := BlockData{
		Height:       200,
		Transactions: make([]string, 5),
		Size:         1024,
		GasUsed:      25000,
		Timestamp:    time.Now(),
	}

	examples := []TrainingExample[BlockData]{
		{
			Input: blockData1,
			Output: Decision[BlockData]{
				ID:         "decision-1",
				Action:     "approve",
				Data:       blockData1,
				Confidence: 0.9,
				Reasoning:  "Good block",
				Context:    make(map[string]interface{}),
				Timestamp:  time.Now(),
				ProposerID: "node-1",
			},
			Feedback: 1.0,
			NodeID:   "node-1",
			Weight:   1.0,
			Context:  make(map[string]interface{}),
		},
		{
			Input: blockData2,
			Output: Decision[BlockData]{
				ID:         "decision-2",
				Action:     "reject",
				Data:       blockData2,
				Confidence: 0.8,
				Reasoning:  "Suspicious block",
				Context:    make(map[string]interface{}),
				Timestamp:  time.Now(),
				ProposerID: "node-1",
			},
			Feedback: -1.0,
			NodeID:   "node-1",
			Weight:   1.0,
			Context:  make(map[string]interface{}),
		},
	}

	model.Learn(examples)

	if len(model.history) != 2 {
		t.Errorf("Expected 2 history entries, got %d", len(model.history))
	}
}

func TestSimpleModelGetWeights(t *testing.T) {
	extractor := &mockBlockFeatureExtractor{}
	model := NewSimpleModel[BlockData]("node-1", extractor)

	model.weights["feature1"] = 0.5
	model.weights["feature2"] = 1.0
	model.bias = 0.3

	weights := model.GetWeights()

	if len(weights) != 2 {
		t.Errorf("Expected 2 weights, got %d", len(weights))
	}

	if weights["feature1"] != 0.5 {
		t.Error("Weight not correctly retrieved")
	}
}

func TestSimpleModelSetWeights(t *testing.T) {
	extractor := &mockBlockFeatureExtractor{}
	model := NewSimpleModel[BlockData]("node-1", extractor)

	newWeights := map[string]float64{
		"feature1": 2.0,
		"feature2": 3.0,
	}

	model.SetWeights(newWeights)

	if model.weights["feature1"] != 2.0 {
		t.Error("Weights not properly set")
	}

	if model.weights["feature2"] != 3.0 {
		t.Error("Weights not properly set")
	}
}

func TestSimpleModelGetHistory(t *testing.T) {
	extractor := &mockBlockFeatureExtractor{}
	model := NewSimpleModel[BlockData]("node-1", extractor)

	// Add some history
	blockData := BlockData{
		Height:       100,
		Transactions: make([]string, 10),
		Size:         2048,
		GasUsed:      50000,
		Timestamp:    time.Now(),
	}

	example := TrainingExample[BlockData]{
		Input: blockData,
		Output: Decision[BlockData]{
			ID:         "test-decision",
			Action:     "approve",
			Data:       blockData,
			Confidence: 0.9,
			Reasoning:  "Test",
			Context:    make(map[string]interface{}),
			Timestamp:  time.Now(),
			ProposerID: "node-1",
		},
		Feedback: 1.0,
		NodeID:   "node-1",
		Weight:   1.0,
		Context:  make(map[string]interface{}),
	}
	examples := []TrainingExample[BlockData]{example}
	model.Learn(examples)

	if len(model.history) != 1 {
		t.Errorf("Expected 1 history entry, got %d", len(model.history))
	}
}

// === Utility Function Tests ===

func TestSigmoid(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
		epsilon  float64
	}{
		{0.0, 0.5, 0.001},
		{1000.0, 1.0, 0.001}, // Large positive should approach 1
		{-1000.0, 0.0, 0.001}, // Large negative should approach 0
		{1.0, 0.731, 0.01},
		{-1.0, 0.268, 0.01},
	}

	for _, test := range tests {
		result := sigmoid(test.input)
		if result < test.expected-test.epsilon || result > test.expected+test.epsilon {
			t.Errorf("sigmoid(%f) = %f, expected ~%f", test.input, result, test.expected)
		}
		if result < 0 || result > 1 {
			t.Errorf("sigmoid(%f) = %f, should be in [0, 1]", test.input, result)
		}
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID()
	id2 := generateID()

	if id1 == "" {
		t.Error("generateID returned empty string")
	}

	if id1 == id2 {
		t.Error("generateID should produce unique IDs")
	}

	if len(id1) < 10 {
		t.Error("Generated ID seems too short")
	}
}

// === Feature Extractor Tests ===

func TestMockFeatureExtractor(t *testing.T) {
	extractor := &mockBlockFeatureExtractor{}

	data := BlockData{
		Height:       1000,
		Transactions: make([]string, 20),
		Size:         4096,
		GasUsed:      100000,
		Timestamp:    time.Now(),
	}

	features := extractor.Extract(data)

	if len(features) != 4 {
		t.Errorf("Expected 4 features, got %d", len(features))
	}

	if features["height"] != 1000.0 {
		t.Errorf("Expected height 1000, got %f", features["height"])
	}

	if features["tx_count"] != 20.0 {
		t.Errorf("Expected tx_count 20, got %f", features["tx_count"])
	}

	names := extractor.Names()
	if len(names) != 4 {
		t.Errorf("Expected 4 feature names, got %d", len(names))
	}
}

// === Block Feature Extractor Tests ===

func TestBlockFeatures(t *testing.T) {
	extractor := NewBlockFeatures()

	if extractor == nil {
		t.Fatal("NewBlockFeatures returned nil")
	}

	names := extractor.Names()
	if len(names) == 0 {
		t.Error("BlockFeatures should have feature names")
	}

	// Test with BlockData
	blockData := BlockData{
		Height:     500,
		ParentHash: "0xabc",
		Timestamp:  time.Now(),
		TxCount:    100,
		Size:       2048,
		GasUsed:    50000,
	}

	features := extractor.Extract(blockData)
	if len(features) == 0 {
		t.Error("Extract should return features")
	}
}

// === Transaction Feature Extractor Tests ===

func TestTransactionFeatures(t *testing.T) {
	extractor := NewTransactionFeatures()

	if extractor == nil {
		t.Fatal("NewTransactionFeatures returned nil")
	}

	names := extractor.Names()
	if len(names) == 0 {
		t.Error("TransactionFeatures should have feature names")
	}

	// Test with TransactionData
	txData := TransactionData{
		From:      "0x123",
		To:        "0x456",
		Amount:    1000000,
		GasPrice:  50,
		GasLimit:  21000,
		Nonce:     10,
		Timestamp: time.Now(),
	}

	features := extractor.Extract(txData)
	if len(features) == 0 {
		t.Error("Extract should return features")
	}
}

// === Upgrade Feature Extractor Tests ===

func TestUpgradeFeatures(t *testing.T) {
	extractor := NewUpgradeFeatures()

	if extractor == nil {
		t.Fatal("NewUpgradeFeatures returned nil")
	}

	names := extractor.Names()
	if len(names) == 0 {
		t.Error("UpgradeFeatures should have feature names")
	}

	upgradeData := UpgradeData{
		Version:     "v2.0.0",
		Changes:     []string{"feature1", "feature2", "bugfix1"},
		Risk:        "medium",
		TestResults: []string{"test1: pass", "test2: pass", "test3: fail"},
		Timestamp:   time.Now(),
	}

	features := extractor.Extract(upgradeData)
	if len(features) == 0 {
		t.Error("Extract should return features")
	}
}

// === Benchmark Tests ===

func BenchmarkSimpleModelDecide(b *testing.B) {
	extractor := &mockBlockFeatureExtractor{}
	model := NewSimpleModel[BlockData]("node-1", extractor)
	model.weights["tx_count"] = 1.0
	model.bias = 0.5

	data := BlockData{
		Height:       100,
		Transactions: make([]string, 10),
		Size:         2048,
		GasUsed:      50000,
		Timestamp:    time.Now(),
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = model.Decide(ctx, data, nil)
	}
}

func BenchmarkSimpleModelLearn(b *testing.B) {
	extractor := &mockBlockFeatureExtractor{}
	model := NewSimpleModel[BlockData]("node-1", extractor)

	blockData := BlockData{
		Height:       100,
		Transactions: make([]string, 10),
		Size:         2048,
		GasUsed:      50000,
		Timestamp:    time.Now(),
	}

	example := TrainingExample[BlockData]{
		Input: blockData,
		Output: Decision[BlockData]{
			ID:         "bench-decision",
			Action:     "approve",
			Data:       blockData,
			Confidence: 0.9,
			Reasoning:  "Benchmark",
			Context:    make(map[string]interface{}),
			Timestamp:  time.Now(),
			ProposerID: "node-1",
		},
		Feedback: 1.0,
		NodeID:   "node-1",
		Weight:   1.0,
		Context:  make(map[string]interface{}),
	}
	examples := []TrainingExample[BlockData]{example}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model.Learn(examples)
	}
}

func BenchmarkFeatureExtraction(b *testing.B) {
	extractor := &mockBlockFeatureExtractor{}
	data := BlockData{
		Height:       100,
		Transactions: make([]string, 10),
		Size:         2048,
		GasUsed:      50000,
		Timestamp:    time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = extractor.Extract(data)
	}
}

func BenchmarkSigmoid(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = sigmoid(float64(i % 100))
	}
}
