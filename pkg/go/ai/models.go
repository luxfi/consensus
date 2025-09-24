// Copyright (C) 2024, Lux Industries Inc. All rights reserved.
// Practical AI Models - Simple, Effective, No Bullshit

package ai

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// SimpleModel is a basic but effective AI model for consensus decisions
type SimpleModel[T ConsensusData] struct {
	weights    map[string]float64 // feature -> weight
	bias       float64
	learningRate float64
	features   FeatureExtractor[T]
	history    []TrainingExample[T]
	nodeID     string
}

// FeatureExtractor converts consensus data to features for ML
type FeatureExtractor[T ConsensusData] interface {
	Extract(data T) map[string]float64
	Names() []string
}

// NewSimpleModel creates a practical AI model
func NewSimpleModel[T ConsensusData](nodeID string, extractor FeatureExtractor[T]) *SimpleModel[T] {
	return &SimpleModel[T]{
		weights:      make(map[string]float64),
		bias:         0.0,
		learningRate: 0.01,
		features:     extractor,
		history:      make([]TrainingExample[T], 0),
		nodeID:       nodeID,
	}
}

// Decide makes a decision based on current model state
func (m *SimpleModel[T]) Decide(ctx context.Context, input T, context map[string]interface{}) (*Decision[T], error) {
	features := m.features.Extract(input)

	// Simple linear decision function
	score := m.bias
	for feature, value := range features {
		weight := m.weights[feature]
		score += weight * value
	}

	// Convert to probability
	confidence := sigmoid(score)

	// Determine action based on score
	action := "approve"
	if score < 0 {
		action = "reject"
	}

	// Generate reasoning
	reasoning := m.generateReasoning(features, score, action)

	decision := &Decision[T]{
		ID:          generateID(),
		Action:      action,
		Data:        input,
		Confidence:  confidence,
		Reasoning:   reasoning,
		Context:     context,
		Timestamp:   time.Now(),
		ProposerID:  m.nodeID,
	}

	return decision, nil
}

// ProposeDecision creates a proposal for consensus
func (m *SimpleModel[T]) ProposeDecision(ctx context.Context, input T) (*Proposal[T], error) {
	decision, err := m.Decide(ctx, input, make(map[string]interface{}))
	if err != nil {
		return nil, err
	}

	proposal := &Proposal[T]{
		ID:          generateID(),
		NodeID:      m.nodeID,
		Decision:    decision,
		Evidence:    []Evidence[T]{{
			Data:      input,
			NodeID:    m.nodeID,
			Weight:    1.0,
			Timestamp: time.Now(),
		}},
		Weight:      1.0,
		Confidence:  decision.Confidence,
		Timestamp:   time.Now(),
	}

	return proposal, nil
}

// ValidateProposal validates another node's proposal
func (m *SimpleModel[T]) ValidateProposal(proposal *Proposal[T]) (float64, error) {
	if proposal.Decision == nil {
		return 0.0, fmt.Errorf("proposal has no decision")
	}

	// Re-evaluate the same input
	ourDecision, err := m.Decide(context.Background(), proposal.Decision.Data, proposal.Decision.Context)
	if err != nil {
		return 0.0, err
	}

	// Compare decisions
	agreement := 0.0
	if ourDecision.Action == proposal.Decision.Action {
		agreement = 1.0
	}

	// Weight by confidence similarity
	confidenceDiff := math.Abs(ourDecision.Confidence - proposal.Decision.Confidence)
	confidenceWeight := 1.0 - confidenceDiff

	// Combined confidence
	validation := agreement * confidenceWeight * ourDecision.Confidence

	return validation, nil
}

// Learn updates the model with training examples
func (m *SimpleModel[T]) Learn(examples []TrainingExample[T]) error {
	for _, example := range examples {
		if err := m.learnExample(example); err != nil {
			return fmt.Errorf("learning failed for example: %w", err)
		}
	}

	m.history = append(m.history, examples...)

	// Keep history bounded
	if len(m.history) > 10000 {
		m.history = m.history[len(m.history)-10000:]
	}

	return nil
}

// UpdateWeights applies gradient updates
func (m *SimpleModel[T]) UpdateWeights(gradients []float64) error {
	featureNames := m.features.Names()

	if len(gradients) != len(featureNames)+1 { // +1 for bias
		return fmt.Errorf("gradient size mismatch: got %d, expected %d", len(gradients), len(featureNames)+1)
	}

	// Update feature weights
	for i, feature := range featureNames {
		m.weights[feature] -= m.learningRate * gradients[i]
	}

	// Update bias
	m.bias -= m.learningRate * gradients[len(featureNames)]

	return nil
}

// GetState returns current model state
func (m *SimpleModel[T]) GetState() map[string]interface{} {
	return map[string]interface{}{
		"weights":       m.weights,
		"bias":          m.bias,
		"learning_rate": m.learningRate,
		"node_id":       m.nodeID,
		"history_size":  len(m.history),
		"last_update":   time.Now(),
	}
}

// LoadState loads model state
func (m *SimpleModel[T]) LoadState(state map[string]interface{}) error {
	if weights, ok := state["weights"].(map[string]interface{}); ok {
		m.weights = make(map[string]float64)
		for k, v := range weights {
			if val, ok := v.(float64); ok {
				m.weights[k] = val
			}
		}
	}

	if bias, ok := state["bias"].(float64); ok {
		m.bias = bias
	}

	if lr, ok := state["learning_rate"].(float64); ok {
		m.learningRate = lr
	}

	return nil
}

// Private methods

func (m *SimpleModel[T]) learnExample(example TrainingExample[T]) error {
	features := m.features.Extract(example.Input)

	// Current prediction
	score := m.bias
	for feature, value := range features {
		score += m.weights[feature] * value
	}

	prediction := sigmoid(score)

	// Target based on feedback
	target := (example.Feedback + 1.0) / 2.0 // convert -1,1 to 0,1

	// Error
	error := target - prediction

	// Gradient descent update
	for feature, value := range features {
		gradient := error * prediction * (1 - prediction) * value * example.Weight
		m.weights[feature] += m.learningRate * gradient
	}

	// Update bias
	biasGradient := error * prediction * (1 - prediction) * example.Weight
	m.bias += m.learningRate * biasGradient

	return nil
}

func (m *SimpleModel[T]) generateReasoning(features map[string]float64, score float64, action string) string {
	// Find most influential features
	topFeatures := make([]string, 0)
	for feature, value := range features {
		weight := m.weights[feature]
		influence := math.Abs(weight * value)

		if influence > 0.1 { // threshold for significance
			topFeatures = append(topFeatures, fmt.Sprintf("%s(%.2f)", feature, influence))
		}
	}

	reasoning := fmt.Sprintf("Decision to %s based on score %.3f. Key factors: %v",
		action, score, topFeatures)

	return reasoning
}

// === FEATURE EXTRACTORS ===

// BlockFeatureExtractor extracts features from block data
type BlockFeatureExtractor struct{}

func (e *BlockFeatureExtractor) Extract(data BlockData) map[string]float64 {
	now := time.Now()
	age := now.Sub(data.Timestamp).Seconds()

	return map[string]float64{
		"height":           float64(data.Height),
		"tx_count":         float64(len(data.Transactions)),
		"age_seconds":      age,
		"age_normalized":   sigmoid(age / 3600), // normalize by hour
		"hash_complexity":  hashComplexity(data.Hash),
		"time_since_last":  float64(now.Unix() - data.Timestamp.Unix()),
	}
}

func (e *BlockFeatureExtractor) Names() []string {
	return []string{"height", "tx_count", "age_seconds", "age_normalized", "hash_complexity", "time_since_last"}
}

// TransactionFeatureExtractor extracts features from transaction data
type TransactionFeatureExtractor struct{}

func (e *TransactionFeatureExtractor) Extract(data TransactionData) map[string]float64 {
	return map[string]float64{
		"amount":         float64(data.Amount),
		"fee":            float64(data.Fee),
		"fee_ratio":      float64(data.Fee) / math.Max(float64(data.Amount), 1),
		"data_size":      float64(len(fmt.Sprintf("%v", data.Data))),
		"age_seconds":    time.Since(data.Timestamp).Seconds(),
		"from_entropy":   addressEntropy(data.From),
		"to_entropy":     addressEntropy(data.To),
	}
}

func (e *TransactionFeatureExtractor) Names() []string {
	return []string{"amount", "fee", "fee_ratio", "data_size", "age_seconds", "from_entropy", "to_entropy"}
}

// UpgradeFeatureExtractor extracts features from upgrade data
type UpgradeFeatureExtractor struct{}

func (e *UpgradeFeatureExtractor) Extract(data UpgradeData) map[string]float64 {
	riskScore := 0.0
	switch data.Risk {
	case "low":
		riskScore = 0.25
	case "medium":
		riskScore = 0.5
	case "high":
		riskScore = 0.75
	case "critical":
		riskScore = 1.0
	}

	return map[string]float64{
		"change_count":     float64(len(data.Changes)),
		"risk_score":       riskScore,
		"test_count":       float64(len(data.TestResults)),
		"age_hours":        time.Since(data.Timestamp).Hours(),
		"version_entropy":  versionEntropy(data.Version),
	}
}

func (e *UpgradeFeatureExtractor) Names() []string {
	return []string{"change_count", "risk_score", "test_count", "age_hours", "version_entropy"}
}

// === UTILITY FUNCTIONS ===

func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

func generateID() string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d_%f", time.Now().UnixNano(), rand.Float64())))
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func hashComplexity(hash string) float64 {
	if len(hash) == 0 {
		return 0
	}

	complexity := 0.0
	for _, char := range hash {
		if char >= '0' && char <= '9' {
			complexity += 0.1
		} else if char >= 'a' && char <= 'f' {
			complexity += 0.2
		}
	}

	return complexity / float64(len(hash))
}

func addressEntropy(address string) float64 {
	if len(address) == 0 {
		return 0
	}

	charCount := make(map[rune]int)
	for _, char := range address {
		charCount[char]++
	}

	entropy := 0.0
	length := float64(len(address))

	for _, count := range charCount {
		if count > 0 {
			p := float64(count) / length
			entropy -= p * math.Log2(p)
		}
	}

	return entropy
}

func versionEntropy(version string) float64 {
	// Simple version entropy based on semantic versioning patterns
	if len(version) == 0 {
		return 0
	}

	entropy := 0.0
	for i, char := range version {
		if char == '.' {
			entropy += 0.5
		} else if char >= '0' && char <= '9' {
			entropy += 0.1 * float64(i)
		}
	}

	return entropy / float64(len(version))
}

