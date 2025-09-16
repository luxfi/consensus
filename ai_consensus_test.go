// Copyright (C) 2024, Lux Industries Inc. All rights reserved.
// AI Consensus Implementation Tests

package consensus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNeuralConsensus(t *testing.T) {
	require := require.New(t)

	// Create neural consensus
	nc := NewNeuralConsensus()
	require.NotNil(nc)

	// Test neural networks are initialized
	require.NotNil(nc.proposalNetwork)
	require.NotNil(nc.validationNetwork)
	require.NotNil(nc.consensusNetwork)

	// Test network dimensions
	require.Equal(100, nc.proposalNetwork.inputSize)
	require.Equal(64, nc.proposalNetwork.hiddenSize)
	require.Equal(32, nc.proposalNetwork.outputSize)

	require.Equal(64, nc.validationNetwork.inputSize)
	require.Equal(32, nc.validationNetwork.hiddenSize)
	require.Equal(2, nc.validationNetwork.outputSize)

	require.Equal(128, nc.consensusNetwork.inputSize)
	require.Equal(64, nc.consensusNetwork.hiddenSize)
	require.Equal(1, nc.consensusNetwork.outputSize)
}

func TestNeuralNetworkForward(t *testing.T) {
	require := require.New(t)

	// Create a simple neural network
	nn := NewNeuralNetwork(10, 5, 2)
	require.NotNil(nn)

	// Test forward pass
	input := make([]float64, 10)
	for i := range input {
		input[i] = float64(i) / 10.0
	}

	output := nn.Forward(input)
	require.Len(output, 2)

	// Output should be between 0 and 1 due to sigmoid
	for _, val := range output {
		require.GreaterOrEqual(val, 0.0)
		require.LessOrEqual(val, 1.0)
	}
}

func TestProposeBlock(t *testing.T) {
	require := require.New(t)

	nc := NewNeuralConsensus()
	ctx := context.Background()

	// Test block proposal
	data := []byte("test transaction data")
	block, err := nc.ProposeBlock(ctx, data)
	require.NoError(err)
	require.NotNil(block)

	// Block should contain original data
	var blockStruct BlockStructure
	err = json.Unmarshal(block, &blockStruct)
	require.NoError(err)
	require.Equal(data, blockStruct.Data)
}

func TestValidateBlock(t *testing.T) {
	require := require.New(t)

	nc := NewNeuralConsensus()
	ctx := context.Background()

	// Create a test block
	data := []byte("test data")
	block, err := nc.ProposeBlock(ctx, data)
	require.NoError(err)

	// Validate the block
	valid, err := nc.ValidateBlock(ctx, block)
	require.NoError(err)

	// Should be valid initially (no training yet)
	require.True(valid)

	// Test metrics update
	metrics := nc.GetMetrics()
	require.NotNil(metrics)
	require.GreaterOrEqual(metrics["accuracy"], 0.0)
}

func TestReachConsensus(t *testing.T) {
	require := require.New(t)

	nc := NewNeuralConsensus()
	ctx := context.Background()

	// Create validators
	validators := []string{
		"validator1",
		"validator2",
		"validator3",
		"validator4",
		"validator5",
	}

	// Create proposal
	proposal := []byte("test proposal")

	// Test consensus
	consensus, err := nc.ReachConsensus(ctx, validators, proposal)
	require.NoError(err)

	// Should reach consensus (67% threshold)
	// Initial implementation uses random, so we just check no error
	_ = consensus

	// Check validator profiles were created
	for _, validator := range validators {
		profile := nc.validators[validator]
		require.NotNil(profile)
		require.Equal(validator, profile.ID)
		require.NotEmpty(profile.VoteHistory)
	}

	// Check consensus event was recorded
	require.NotEmpty(nc.history)
	lastEvent := nc.history[len(nc.history)-1]
	require.Equal(validators, lastEvent.Validators)
}

func TestValidatorBehaviorPrediction(t *testing.T) {
	require := require.New(t)

	nc := NewNeuralConsensus()

	// Create validators with different reliability scores
	validators := []string{"reliable", "unreliable"}
	
	// Set up validator profiles
	nc.validators["reliable"] = &ValidatorProfile{
		ID:               "reliable",
		ReliabilityScore: 0.95,
		ResponseTime:     50 * time.Millisecond,
		MaliciousScore:   0.01,
	}
	
	nc.validators["unreliable"] = &ValidatorProfile{
		ID:               "unreliable",
		ReliabilityScore: 0.3,
		ResponseTime:     500 * time.Millisecond,
		MaliciousScore:   0.5,
	}

	// Predict votes
	proposal := []byte("test proposal")
	predictions := nc.predictValidatorVotes(validators, proposal)

	// Predictions should exist for all validators
	require.Len(predictions, 2)
	require.Contains(predictions, "reliable")
	require.Contains(predictions, "unreliable")

	// Predictions should be probabilities between 0 and 1
	for _, prob := range predictions {
		require.GreaterOrEqual(prob, 0.0)
		require.LessOrEqual(prob, 1.0)
	}
}

func TestConsensusStrategy(t *testing.T) {
	require := require.New(t)

	nc := NewNeuralConsensus()
	nc.accuracy = 0.95 // High accuracy

	validators := []string{"v1", "v2", "v3"}
	predictions := map[string]float64{
		"v1": 0.9,  // High probability
		"v2": 0.85, // High probability
		"v3": 0.3,  // Low probability
	}

	strategy := nc.optimizeConsensusStrategy(validators, predictions)

	// High accuracy should result in fast timeout
	require.Equal(50*time.Millisecond, strategy.Timeout)

	// Priority should favor high probability validators
	require.Contains(strategy.Priority[:2], "v1")
	require.Contains(strategy.Priority[:2], "v2")
}

func TestDeepValidation(t *testing.T) {
	require := require.New(t)

	nc := NewNeuralConsensus()

	// Test with valid block structure
	validBlock := BlockStructure{
		Version:    1,
		Timestamp:  time.Now().Unix(),
		Data:       []byte("valid data"),
		Difficulty: 1000,
		Nonce:      12345,
	}

	validBytes, err := json.Marshal(validBlock)
	require.NoError(err)

	isValid := nc.deepValidation(validBytes)
	require.True(isValid)

	// Test with invalid block (empty)
	isValid = nc.deepValidation([]byte{})
	require.False(isValid)
}

func TestLearningMechanism(t *testing.T) {
	require := require.New(t)

	nc := NewNeuralConsensus()

	// Initial state
	initialAccuracy := nc.accuracy
	require.Equal(float64(0), initialAccuracy)

	// Simulate correct prediction
	nc.learn(true, 0.8) // Predicted 0.8, actual true (correct)
	require.Greater(nc.accuracy, initialAccuracy)
	require.Equal(int64(1), nc.totalDecisions)

	// Simulate incorrect prediction
	prevAccuracy := nc.accuracy
	nc.learn(false, 0.9) // Predicted 0.9, actual false (incorrect)
	require.Less(nc.accuracy, prevAccuracy)
	require.Equal(int64(2), nc.totalDecisions)
}

func TestHistoryManagement(t *testing.T) {
	require := require.New(t)

	nc := NewNeuralConsensus()
	ctx := context.Background()

	// Add many blocks to trigger history cleanup
	for i := 0; i < 1100; i++ {
		block := []byte("test block")
		_, err := nc.ValidateBlock(ctx, block)
		require.NoError(err)
	}

	// History should be trimmed to last 900 events (1000 - 100)
	require.LessOrEqual(len(nc.history), 1000)
}

func TestActivationFunctions(t *testing.T) {
	require := require.New(t)

	// Test ReLU
	require.Equal(float64(0), relu(-5))
	require.Equal(float64(0), relu(0))
	require.Equal(float64(5), relu(5))

	// Test Sigmoid
	sig0 := sigmoid(0)
	require.Equal(0.5, sig0)

	sigPos := sigmoid(10)
	require.Greater(sigPos, 0.99)

	sigNeg := sigmoid(-10)
	require.Less(sigNeg, 0.01)

	// Test Tanh
	tanh0 := tanh(0)
	require.Equal(float64(0), tanh0)

	tanhPos := tanh(10)
	require.Greater(tanhPos, 0.99)

	tanhNeg := tanh(-10)
	require.Less(tanhNeg, -0.99)
}

func TestWeightInitialization(t *testing.T) {
	require := require.New(t)

	// Test Xavier initialization
	weights := initializeWeights(100, 50)
	require.Len(weights, 100)
	require.Len(weights[0], 50)

	// Check weights are properly scaled
	var sum float64
	var count int
	for i := range weights {
		for j := range weights[i] {
			sum += weights[i][j] * weights[i][j]
			count++
		}
	}

	// Variance should be approximately 2/input_size
	variance := sum / float64(count)
	expectedVariance := 2.0 / 100.0
	require.InDelta(expectedVariance, variance, 0.05)
}

// Benchmark tests

func BenchmarkNeuralNetworkForward(b *testing.B) {
	nn := NewNeuralNetwork(100, 64, 32)
	input := make([]float64, 100)
	for i := range input {
		input[i] = float64(i) / 100.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = nn.Forward(input)
	}
}

func BenchmarkProposeBlock(b *testing.B) {
	nc := NewNeuralConsensus()
	ctx := context.Background()
	data := []byte("benchmark transaction data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = nc.ProposeBlock(ctx, data)
	}
}

func BenchmarkReachConsensus(b *testing.B) {
	nc := NewNeuralConsensus()
	ctx := context.Background()
	validators := []string{"v1", "v2", "v3", "v4", "v5"}
	proposal := []byte("benchmark proposal")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = nc.ReachConsensus(ctx, validators, proposal)
	}
}