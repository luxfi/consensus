// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// AI Demo - Orthogonal, Composable Architecture

package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

// RunDemo demonstrates the single, composable way to use AI
func RunDemo() {
	fmt.Println("=== Lux AI: One Way To Do Everything ===")

	demoOrthogonalComposition()
}

func demoOrthogonalComposition() {
	fmt.Println("\n1. Orthogonal Module Composition")
	fmt.Println("--------------------------------")

	ctx := context.Background()

	// Single way to build AI engine with composable modules
	engine, err := NewBuilder().
		WithInference("llm_inference", map[string]interface{}{
			"model_path": "/models/consensus.gguf",
			"max_tokens": 4096,
		}).
		WithDecision("consensus_decision", map[string]interface{}{
			"strategy":  "weighted_voting",
			"threshold": 0.67,
		}).
		WithLearning("adaptive_learning", map[string]interface{}{
			"learning_rate": 0.01,
			"memory_size":   10000,
		}).
		WithCoordination("network_coord", map[string]interface{}{
			"broadcast_mode": "gossip",
			"retry_count":    3,
		}).
		Build()

	if err != nil {
		fmt.Printf("❌ Failed to build engine: %v\n", err)
		return
	}

	fmt.Println("✅ Built AI engine with 4 orthogonal modules")

	// List composed modules
	modules := engine.ListModules()
	fmt.Printf("📦 Modules: ")
	for i, module := range modules {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("%s(%s)", module.ID(), module.Type())
	}
	fmt.Println()

	// Single way to process any input
	fmt.Println("\n2. Single Processing Pipeline")
	fmt.Println("-----------------------------")

	// Example: Process a block
	blockInput := Input{
		Type: InputBlock,
		Data: map[string]interface{}{
			"id":        "block_123",
			"height":    12345,
			"timestamp": 1699123456,
			"txs":       []string{"tx1", "tx2", "tx3"},
		},
	}

	output, err := engine.Process(ctx, blockInput)
	if err != nil {
		fmt.Printf("❌ Processing failed: %v\n", err)
		return
	}

	fmt.Printf("✅ Processed block through pipeline: %s → %s\n",
		blockInput.Type, output.Type)

	// Show result
	resultJSON, _ := json.MarshalIndent(output.Data, "", "  ")
	fmt.Printf("📊 Result:\n%s\n", resultJSON)

	// Example: Process a proposal
	fmt.Println("\n3. Same Pipeline, Different Input")
	fmt.Println("---------------------------------")

	proposalInput := Input{
		Type: InputProposal,
		Data: map[string]interface{}{
			"id":          "prop_456",
			"title":       "Increase block size limit",
			"description": "Proposal to increase max block size from 1MB to 2MB",
			"proposer":    "validator_node_7",
		},
	}

	output2, err := engine.Process(ctx, proposalInput)
	if err != nil {
		fmt.Printf("❌ Processing failed: %v\n", err)
		return
	}

	fmt.Printf("✅ Processed proposal through same pipeline: %s → %s\n",
		proposalInput.Type, output2.Type)

	// Show modular architecture
	fmt.Println("\n4. Modular Architecture")
	fmt.Println("-----------------------")
	fmt.Println("🔧 Inference  → Analyze input (LLM, Neural, etc.)")
	fmt.Println("⚖️  Decision   → Make decisions based on analysis")
	fmt.Println("🧠 Learning   → Adapt from outcomes (async)")
	fmt.Println("🌐 Coordination → Broadcast results (async)")
	fmt.Println()
	fmt.Println("✨ Each concern is orthogonal and composable")
	fmt.Println("📐 Exactly one way to: configure, process, manage")
	fmt.Println("🔀 Any combination of modules works together")
}

// DemoCustomComposition shows how to create custom compositions
func DemoCustomComposition() {
	fmt.Println("\n=== Custom Compositions ===")

	// Inference-only engine for analysis
	analysisEngine, _ := NewBuilder().
		WithInference("fast_inference", map[string]interface{}{
			"model": "lightweight",
		}).
		Build()

	// Decision-only engine for governance
	governanceEngine, _ := NewBuilder().
		WithDecision("dao_voting", map[string]interface{}{
			"quorum": 0.5,
		}).
		Build()

	// Full-featured engine for validators
	validatorEngine, _ := NewBuilder().
		WithInference("full_llm", nil).
		WithDecision("consensus_algo", nil).
		WithLearning("validator_learning", nil).
		WithCoordination("validator_network", nil).
		Build()

	fmt.Printf("🔬 Analysis Engine: %d modules\n", len(analysisEngine.ListModules()))
	fmt.Printf("🏛️  Governance Engine: %d modules\n", len(governanceEngine.ListModules()))
	fmt.Printf("✅ Validator Engine: %d modules\n", len(validatorEngine.ListModules()))

	fmt.Println("\n💡 Same interface, different compositions!")
}
