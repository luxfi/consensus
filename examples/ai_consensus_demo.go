// Copyright (C) 2024, Lux Industries Inc. All rights reserved.
// AI Consensus Demo - Shows how to use AI-powered consensus

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/luxfi/consensus"
)

func main() {
	fmt.Println("=== Lux AI Consensus Demo ===\n")

	// Demo 1: Neural Consensus
	demoNeuralConsensus()

	fmt.Println("\n" + strings.Repeat("=", 50) + "\n")

	// Demo 2: LLM Consensus with Evolution
	demoLLMConsensus()

	fmt.Println("\n" + strings.Repeat("=", 50) + "\n")

	// Demo 3: Multi-Backend AI Engine
	demoAIEngine()
}

func demoNeuralConsensus() {
	fmt.Println("1. Neural Consensus Demo")
	fmt.Println("------------------------")

	// Create neural consensus instance
	nc := consensus.NewNeuralConsensus()
	fmt.Println("✓ Created neural consensus with 3 neural networks")

	ctx := context.Background()

	// Propose a block
	data := []byte("Sample transaction data for neural consensus")
	block, err := nc.ProposeBlock(ctx, data)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("✓ Proposed block with %d bytes\n", len(block))

	// Validate the block
	valid, err := nc.ValidateBlock(ctx, block)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("✓ Block validation: %v\n", valid)

	// Reach consensus with validators
	validators := []string{
		"validator-1",
		"validator-2", 
		"validator-3",
		"validator-4",
		"validator-5",
	}

	consensusReached, err := nc.ReachConsensus(ctx, validators, block)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("✓ Consensus reached: %v\n", consensusReached)

	// Show metrics
	metrics := nc.GetMetrics()
	fmt.Printf("\nNeural Consensus Metrics:\n")
	fmt.Printf("  - Accuracy: %.2f%%\n", metrics["accuracy"].(float64)*100)
	fmt.Printf("  - Total Decisions: %d\n", metrics["total_decisions"])
	fmt.Printf("  - Validators: %d\n", metrics["validators"])
}

func demoLLMConsensus() {
	fmt.Println("2. LLM Consensus with Evolution Demo")
	fmt.Println("------------------------------------")

	// Configure LLM consensus
	config := &consensus.LLMConfig{
		ModelPath:     "/models/lux-consensus.gguf",
		ModelSize:     7000000000, // 7B parameters
		ContextWindow: 4096,
		Quantization:  "int8",
	}

	llm := consensus.NewLLMConsensus(config)
	fmt.Println("✓ Created LLM consensus with evolutionary capabilities")

	ctx := context.Background()

	// Establish node relationships
	err := llm.EstablishRelationship(ctx, "peer-node-1", "partner")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Established partnership with peer-node-1")

	// Process governance proposal
	proposal := &consensus.Proposal{
		ID:          "prop-001",
		Type:        "parameter_update",
		Description: "Increase block size to 2MB",
		Parameters: map[string]interface{}{
			"block_size": 2097152,
		},
		Deadline: time.Now().Add(24 * time.Hour),
	}

	err = llm.ProcessGovernanceProposal(ctx, proposal)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Processed governance proposal with LLM analysis")

	// Trigger evolution
	err = llm.Evolve(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Node evolved to next generation")

	// Exchange value with peer
	err = llm.ExchangeValue(ctx, "peer-node-1", "knowledge", 10.5)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Exchanged knowledge with peer")
}

func demoAIEngine() {
	fmt.Println("3. Multi-Backend AI Engine Demo")
	fmt.Println("-------------------------------")

	// Create AI engine configuration
	config := &consensus.AIEngineConfig{
		Backend: consensus.BackendAI,
		AI: &consensus.AIConfig{
			EnablePrediction:    true,
			EnableOptimization:  true,
			EnableValidation:    true,
			ConfidenceThreshold: 0.95,
			ConsensusThreshold:  0.67,
			EnableLearning:      true,
			LearningRate:        0.001,
		},
		Performance: consensus.PerformanceConfig{
			CacheSize:       1000,
			BatchProcessing: true,
			ParallelOps:     8,
			MaxLatency:      100 * time.Millisecond,
		},
		Debug: true,
	}

	ctx := context.Background()

	// Create AI consensus engine
	engine, err := consensus.NewAIConsensusEngine(ctx, config)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Created AI consensus engine with full AI backend")

	// Propose block through AI engine
	data := []byte("AI-optimized transaction batch")
	block, err := engine.ProposeBlock(ctx, data)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("✓ AI proposed optimized block (%d bytes)\n", len(block))

	// Get engine metrics
	metrics := engine.GetMetrics()
	fmt.Printf("\nAI Engine Metrics:\n")
	fmt.Printf("  - Blocks Proposed: %d\n", metrics.BlocksProposed)
	fmt.Printf("  - Blocks Validated: %d\n", metrics.BlocksValidated)
	fmt.Printf("  - Consensus Reached: %d\n", metrics.ConsensusReached)
	fmt.Printf("  - Prediction Accuracy: %.2f%%\n", metrics.PredictionAccuracy*100)

	// Switch backend dynamically
	err = engine.SwitchBackend(ctx, consensus.BackendMLX)
	if err != nil {
		// MLX backend might not be available
		fmt.Println("  Note: MLX backend not available (requires Apple Silicon)")
	} else {
		fmt.Println("✓ Switched to MLX backend for hardware acceleration")
	}

	// Shutdown
	err = engine.Shutdown()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Cleanly shut down AI consensus engine")
}

func prettyPrint(label string, data interface{}) {
	bytes, _ := json.MarshalIndent(data, "", "  ")
	fmt.Printf("%s:\n%s\n", label, string(bytes))
}