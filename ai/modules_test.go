// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// AI Modules Tests

package ai

import (
	"context"
	"testing"
	"time"
)

// === Inference Module Tests ===

func TestInferenceModuleProcess_AllInputTypes(t *testing.T) {
	module := NewInferenceModule("test-inference", nil)
	ctx := context.Background()

	tests := []struct {
		name      string
		inputType InputType
		expectErr bool
	}{
		{"Block input", InputBlock, false},
		{"Proposal input", InputProposal, false},
		{"Vote input", InputVote, false},
		{"Query input", InputQuery, false},
		{"Unknown input", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := Input{
				Type: tt.inputType,
				Data: map[string]interface{}{
					"test": "data",
				},
			}

			output, err := module.Process(ctx, input)

			if tt.expectErr {
				if err == nil {
					t.Error("Expected error for unknown input type")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if output.Type == "" {
					t.Error("Expected non-empty output type")
				}
			}
		})
	}
}

// === Model Learn Tests ===

func TestSimpleModelLearn_HistoryBounding(t *testing.T) {
	extractor := &mockBlockFeatureExtractor{}
	model := NewSimpleModel[BlockData]("test-node", extractor)

	// Create more than 10000 examples to trigger history bounding
	examples := make([]TrainingExample[BlockData], 10005)
	for i := 0; i < 10005; i++ {
		examples[i] = TrainingExample[BlockData]{
			Input: BlockData{
				Height:    uint64(i),
				Timestamp: time.Now(),
			},
			Output: Decision[BlockData]{
				Action:     "approve",
				Confidence: 0.9,
			},
			Feedback: 1.0,
			NodeID:   "node-1",
			Weight:   1.0,
		}
	}

	err := model.Learn(examples)
	if err != nil {
		t.Fatalf("Learn() error = %v", err)
	}

	// Verify history was bounded to 10000
	state := model.GetState()
	historySize, ok := state["history_size"].(int)
	if !ok {
		t.Fatal("history_size not found in state")
	}

	if historySize != 10000 {
		t.Errorf("Expected history size 10000 after bounding, got %d", historySize)
	}
}
