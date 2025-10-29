// Copyright (C) 2025, Lux Partners Limited All rights reserved.
// AI Package Tests - Validate Orthogonal Design

package ai

import (
	"context"
	"testing"
)

func TestOrthogonalDesign(t *testing.T) {
	ctx := context.Background()

	// Test: Exactly one way to build
	engine, err := NewBuilder().
		WithInference("test_inference", nil).
		WithDecision("test_decision", nil).
		Build()

	if err != nil {
		t.Fatalf("Failed to build engine: %v", err)
	}

	// Test: Exactly one way to process
	input := Input{
		Type: InputBlock,
		Data: map[string]interface{}{"test": "data"},
	}

	output, err := engine.Process(ctx, input)
	if err != nil {
		t.Fatalf("Failed to process: %v", err)
	}

	if output.Type != OutputDecision {
		t.Errorf("Expected decision output, got %s", output.Type)
	}
}

func TestComposability(t *testing.T) {
	// Test: Different compositions work identically
	compositions := []Builder{
		NewBuilder().WithInference("inf1", nil),
		NewBuilder().WithDecision("dec1", nil),
		NewBuilder().WithLearning("learn1", nil),
		NewBuilder().WithCoordination("coord1", nil),
		NewBuilder().
			WithInference("inf2", nil).
			WithDecision("dec2", nil).
			WithLearning("learn2", nil).
			WithCoordination("coord2", nil),
	}

	for i, builder := range compositions {
		engine, err := builder.Build()
		if err != nil {
			t.Errorf("Composition %d failed to build: %v", i, err)
			continue
		}

		// Same interface regardless of composition
		modules := engine.ListModules()
		if len(modules) == 0 && i == len(compositions)-1 {
			t.Errorf("Full composition should have modules")
		}

		// Same processing interface
		input := Input{Type: InputQuery, Data: map[string]interface{}{}}
		_, err = engine.Process(context.Background(), input)
		if err != nil {
			t.Errorf("Composition %d failed to process: %v", i, err)
		}
	}
}

func TestSingleInterface(t *testing.T) {
	// Test: All modules implement exactly the same interface
	modules := []Module{
		NewInferenceModule("test1", nil),
		NewDecisionModule("test2", nil),
		NewLearningModule("test3", nil),
		NewCoordinationModule("test4", nil),
	}

	for _, module := range modules {
		// Same lifecycle interface
		if err := module.Initialize(context.Background(), Config{}); err != nil {
			t.Errorf("Module %s failed to initialize: %v", module.ID(), err)
		}

		if err := module.Start(context.Background()); err != nil {
			t.Errorf("Module %s failed to start: %v", module.ID(), err)
		}

		// Same processing interface
		input := Input{Type: InputQuery, Data: map[string]interface{}{}}
		_, err := module.Process(context.Background(), input)
		if err != nil {
			t.Errorf("Module %s failed to process: %v", module.ID(), err)
		}

		if err := module.Stop(context.Background()); err != nil {
			t.Errorf("Module %s failed to stop: %v", module.ID(), err)
		}
	}
}

func BenchmarkOrthogonalProcessing(b *testing.B) {
	engine, _ := NewBuilder().
		WithInference("bench_inf", nil).
		WithDecision("bench_dec", nil).
		Build()

	input := Input{
		Type: InputBlock,
		Data: map[string]interface{}{"size": 1024},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Process(ctx, input)
	}
}