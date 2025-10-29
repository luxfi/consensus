// Copyright (C) 2025, Lux Partners Limited All rights reserved.
// Cross-Chain AI Computation Demo

package ai

import (
	"context"
	"fmt"
	"math/big"
	"time"
)

// DemoXChainCompute demonstrates cross-chain AI computation with payments
func DemoXChainCompute() {
	fmt.Println("=== Cross-Chain AI Computation Demo ===")

	// Create node with marketplace enabled
	config := &IntegrationConfig{
		NodeID:            "demo-node-1",
		Enabled:           true,
		EnableMarketplace: true,
		SupportedChains: []*ChainConfig{
			{
				ChainID:        "ethereum",
				Name:           "Ethereum",
				NativeCurrency: "ETH",
				MinPayment:     big.NewInt(1000000), // 0.001 ETH
				GasMultiplier:  1.5,
				Enabled:        true,
			},
			{
				ChainID:        "polygon",
				Name:           "Polygon",
				NativeCurrency: "MATIC",
				MinPayment:     big.NewInt(100000), // 0.1 MATIC
				GasMultiplier:  0.8,
				Enabled:        true,
			},
			{
				ChainID:        "lux-x",
				Name:           "Lux X-Chain",
				NativeCurrency: "LUX",
				MinPayment:     big.NewInt(10000), // 0.01 LUX
				GasMultiplier:  1.0,
				Enabled:        true,
			},
		},
		PricePerUnit:    1000,
		MaxComputeUnits: 1000000,
	}

	node, err := NewNodeIntegration("demo-node-1", config)
	if err != nil {
		fmt.Printf("Failed to create node: %v\n", err)
		return
	}

	ctx := context.Background()

	// Demo 1: Cross-chain AI inference request
	fmt.Println("\n--- Demo 1: Cross-chain AI Inference ---")

	inferenceReq := &ComputeRequest{
		SourceChain: "ethereum",
		Requester:   "0x1234567890123456789012345678901234567890",
		JobType:     "inference",
		Data: map[string]interface{}{
			"model":  "blockchain-sentiment",
			"input":  "What is the future of DeFi?",
			"params": map[string]interface{}{"temperature": 0.7},
		},
		MaxPayment: big.NewInt(5000000), // 0.005 ETH
	}

	job, err := node.OfferCompute(ctx, inferenceReq)
	if err != nil {
		fmt.Printf("Failed to create inference job: %v\n", err)
		return
	}

	fmt.Printf("✅ Inference job created: %s\n", job.ID)
	fmt.Printf("   Source Chain: %s\n", job.SourceChain)
	fmt.Printf("   Compute Units: %s\n", job.ComputeUnits.String())
	fmt.Printf("   Payment Required: %s\n", job.PaymentAmount.String())

	// Simulate payment
	txHash := "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	if err := node.ProcessComputePayment(ctx, job.ID, txHash); err != nil {
		fmt.Printf("Failed to process payment: %v\n", err)
		return
	}

	fmt.Printf("✅ Payment processed: %s\n", txHash)

	// Wait for computation to complete
	time.Sleep(200 * time.Millisecond)

	// Demo 2: Cross-chain training request
	fmt.Println("\n--- Demo 2: Cross-chain AI Training ---")

	trainingReq := &ComputeRequest{
		SourceChain: "polygon",
		Requester:   "0x9876543210987654321098765432109876543210",
		JobType:     "training",
		Data: map[string]interface{}{
			"dataset":    "defi-transactions",
			"model_type": "transformer",
			"epochs":     10,
			"batch_size": 32,
		},
		MaxPayment: big.NewInt(50000000), // 50 MATIC
	}

	trainingJob, err := node.OfferCompute(ctx, trainingReq)
	if err != nil {
		fmt.Printf("Failed to create training job: %v\n", err)
		return
	}

	fmt.Printf("✅ Training job created: %s\n", trainingJob.ID)
	fmt.Printf("   Source Chain: %s\n", trainingJob.SourceChain)
	fmt.Printf("   Job Type: %s\n", trainingJob.JobType)

	// Demo 3: Cross-chain consensus decision
	fmt.Println("\n--- Demo 3: Cross-chain Consensus Decision ---")

	consensusReq := &ComputeRequest{
		SourceChain: "lux-x",
		Requester:   "lux1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqwrqsjw",
		JobType:     "consensus",
		Data: map[string]interface{}{
			"proposal_type": "upgrade",
			"proposal_data": map[string]interface{}{
				"version": "v2.0.0",
				"changes": []string{"improve-throughput", "add-privacy"},
				"risk":    "medium",
			},
		},
		MaxPayment: big.NewInt(1000000), // 0.1 LUX
	}

	consensusJob, err := node.OfferCompute(ctx, consensusReq)
	if err != nil {
		fmt.Printf("Failed to create consensus job: %v\n", err)
		return
	}

	fmt.Printf("✅ Consensus job created: %s\n", consensusJob.ID)

	// Demo 4: Marketplace statistics
	fmt.Println("\n--- Demo 4: Marketplace Statistics ---")

	stats := node.GetMarketplaceStats()
	fmt.Printf("✅ Marketplace Stats:\n")
	for key, value := range stats {
		fmt.Printf("   %s: %v\n", key, value)
	}

	// Demo 5: Cross-chain earnings settlement
	fmt.Println("\n--- Demo 5: Earnings Settlement ---")

	if err := node.SettleMarketplaceEarnings(ctx); err != nil {
		fmt.Printf("Failed to settle earnings: %v\n", err)
		return
	}

	fmt.Printf("✅ Earnings settled successfully\n")

	fmt.Println("\n=== Demo Complete ===")
	fmt.Println("✨ Cross-chain AI computation marketplace is working!")
	fmt.Println("🔗 Any chain can now pay for AI computation through X-Chain")
	fmt.Println("🧠 Decentralized inference, training, and consensus decisions")
	fmt.Println("💰 Automatic billing and settlement across chains")
}

// DemoUsageScenarios shows practical usage scenarios
func DemoUsageScenarios() {
	fmt.Println("=== Cross-Chain AI Usage Scenarios ===")

	scenarios := []struct {
		name        string
		chain       string
		useCase     string
		description string
	}{
		{
			name:        "DeFi Risk Assessment",
			chain:       "ethereum",
			useCase:     "inference",
			description: "Ethereum DeFi protocol pays Lux nodes to analyze smart contract risks",
		},
		{
			name:        "Gaming AI Training",
			chain:       "polygon",
			useCase:     "training",
			description: "Polygon gaming dApp pays to train AI models on player behavior data",
		},
		{
			name:        "Cross-Chain Governance",
			chain:       "lux-x",
			useCase:     "consensus",
			description: "Lux X-Chain pays for AI-powered governance decision analysis",
		},
		{
			name:        "NFT Valuation",
			chain:       "ethereum",
			useCase:     "inference",
			description: "NFT marketplace pays for AI-powered artwork valuation",
		},
		{
			name:        "Fraud Detection",
			chain:       "polygon",
			useCase:     "inference",
			description: "Payment processor pays for real-time transaction fraud detection",
		},
		{
			name:        "Yield Optimization",
			chain:       "ethereum",
			useCase:     "inference",
			description: "Yield farming protocol pays for AI-optimized strategy recommendations",
		},
	}

	for i, scenario := range scenarios {
		fmt.Printf("\n%d. %s (%s)\n", i+1, scenario.name, scenario.chain)
		fmt.Printf("   Use Case: %s\n", scenario.useCase)
		fmt.Printf("   Description: %s\n", scenario.description)
	}

	fmt.Println("\n=== Key Benefits ===")
	fmt.Println("🌍 Global AI compute marketplace")
	fmt.Println("⚡ Pay-per-use model with any cryptocurrency")
	fmt.Println("🔒 Decentralized and trustless payments")
	fmt.Println("🎯 Specialized AI agents for different blockchain operations")
	fmt.Println("📊 Transparent pricing and resource allocation")
	fmt.Println("🔄 Automatic cross-chain settlement")
}
