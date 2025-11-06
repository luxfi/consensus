// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.

/*
Package consensus provides advanced consensus mechanisms for blockchain systems,
featuring AI-powered validation and quantum-secure networking.

# Overview

The Lux consensus package implements multiple consensus engines with AI integration,
supporting Chain (linear), DAG (parallel), and PQ (post-quantum) consensus algorithms.
It features dynamic AI-managed consensus through the Photon→Quasar flow and shared
hallucinations across network nodes.

# Architecture

The consensus system is organized into several key components:

  - ai/          AI consensus agents and models
  - engine/      Consensus engine implementations (chain, dag, pq)
  - protocol/    Consensus protocols (photon, quasar)
  - core/        Core interfaces and types
  - utils/       Utilities (qzmq for quantum-secure messaging)
  - examples/    Progressive tutorial examples

# AI Consensus

The AI consensus system allows machine learning models to participate in consensus:

	// Create AI model for block validation
	model := ai.NewSimpleModel("block-validator")

	// Create consensus agent
	agent := ai.New(nodeID, model, quasarEngine, photonEngine)

	// AI proposes decision following Photon→Quasar flow
	decision, err := agent.ProposeDecision(ctx, blockData, context)

AI agents can use various model types:
  - SimpleModel:  Feedforward neural network
  - LLMModel:     Large language model integration
  - NeuralModel:  Custom neural architectures
  - EnsembleModel: Combination of multiple models

# Consensus Engines

Multiple consensus engines are available:

	// Chain consensus (linear blockchain)
	chainEngine := chain.NewEngine(db, validators)

	// DAG consensus (parallel processing)
	dagEngine := dag.NewEngine(config)

	// Post-quantum consensus
	pqEngine := pq.NewEngine(quantumConfig)

Each engine implements the core consensus.Engine interface and can be hot-swapped
at runtime for dynamic consensus algorithm selection.

# Photon→Quasar Flow

The Photon→Quasar consensus flow provides fast, quantum-secure finalization:

  1. Photon Phase:  Emit proposals at light speed
  2. Wave Phase:    Amplify through network via QZMQ
  3. Focus Phase:   Converge on best options using AI
  4. Prism Phase:   Refract through DAG for validation
  5. Horizon Phase: Finalize with quantum certificate

Example:

	// Photon emits proposal
	proposal, err := photon.Emit(blockData)

	// Quasar validates through DAG
	cert, err := quasar.Finalize(proposal)

# Quantum-Secure Networking

All network communication uses QZMQ (Quantum-Secure ZeroMQ) with post-quantum
cryptography:

	// Create quantum-secure publisher
	pub, err := qzmq.NewPublisher(config)

	// Messages signed with Dilithium, encrypted with Kyber
	err = pub.Send(qzmq.Message{
		From: nodeID,
		Data: proposalData,
		Type: qzmq.TypeConsensus,
	})

QZMQ provides:
  - Dilithium signatures (post-quantum)
  - Kyber encryption (quantum-resistant)
  - Forward secrecy
  - Replay protection

# Shared Hallucinations

AI agents share "hallucinations" (model states) across the network for distributed
learning and consensus:

	// Update shared hallucination
	agent.AddTrainingData(example)

	// Synchronize with network
	err := agent.SyncSharedMemory(ctx)

	// Get hallucination
	hallucination, exists := agent.GetSharedHallucination(id)

Shared hallucinations enable:
  - Distributed model training
  - Consensus on AI decisions
  - Evolutionary model improvement
  - Byzantine fault tolerance

# Cross-Chain Integration

The consensus system integrates with the Lux DEX for cross-chain operations:

	import "github.com/luxfi/dex/pkg/lx"

	// Use production bridge
	bridge := lx.NewCrossChainBridge(config)

	// Wrap with AI validation
	adapter := NewAIBridgeAdapter(bridge, agent, nodeID)

	// Process cross-chain payment with AI consensus
	result, err := adapter.ProcessAIPayment(ctx, request)

# Performance

Benchmark results (Apple M1 Max):

  Operation          Latency    Throughput
  ─────────────────  ─────────  ────────────
  Model Inference    1.5 μs     666K ops/sec
  Consensus Vote     529 ns     1.9M ops/sec
  Feature Extract    37 ns      27M ops/sec
  Sigmoid            5.6 ns     179M ops/sec
  Photon Emit        245 ms     4 proposals/sec
  Full Consensus     200-300 ms 3-5 blocks/sec

# Examples

Progressive tutorial examples are provided in examples/:

  01-simple-bridge      Cross-chain bridge basics
  02-ai-payment        AI payment validation
  03-qzmq-networking   Quantum-secure messaging
  04-grpc-service      gRPC API integration
  05-python-client     Python integration
  06-nodejs-client     TypeScript integration
  07-ai-consensus      Dynamic AI consensus

Each example includes runnable code, comprehensive tests, and detailed documentation.

See examples/README.md for the complete learning path.

# Testing

The package includes extensive tests:

  - Unit tests (co-located with source)
  - Integration tests (test/integration/)
  - Benchmarks (benchmarks/)
  - Example tests (examples/*/test*.go)

Run tests:

	go test ./...                    # All tests
	go test -v ./ai/                 # AI package tests
	go test -bench=. ./ai/           # Benchmarks
	go test -tags=integration ./...  # Integration tests

# Documentation

Additional documentation:

  - README.md              Project overview
  - LLM.md                 AI assistant knowledge base
  - REFACTORING_FINAL.md   Recent refactoring summary
  - EXAMPLES_COMPLETE.md   Tutorial system overview
  - paper/                 Academic whitepaper
  - examples/README.md     Learning path guide

# Security

Security features:

  - Post-quantum cryptography (Dilithium, Kyber)
  - Quantum-secure networking (QZMQ)
  - Byzantine fault tolerance
  - Cryptographic consensus proofs
  - Forward secrecy
  - Replay attack protection

# Contributing

See CONTRIBUTING.md for development guidelines.

# License

Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
See LICENSE for details.
*/
package consensus
