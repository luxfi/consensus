// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// AI Marketplace Bridge Integration Example
//
// This demonstrates how to integrate AI consensus with the production
// Lux DEX cross-chain bridge for AI compute payments.

package marketplace

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/luxfi/consensus/ai"
	"github.com/luxfi/dex/pkg/lx"
)

// AIBridgeAdapter wraps the production DEX bridge for AI-specific use cases
type AIBridgeAdapter struct {
	bridge *lx.CrossChainBridge
	agent  *ai.Agent[ai.TransactionData]
	nodeID string
}

// NewAIBridgeAdapter creates a bridge adapter integrated with AI consensus
func NewAIBridgeAdapter(
	bridge *lx.CrossChainBridge,
	agent *ai.Agent[ai.TransactionData],
	nodeID string,
) *AIBridgeAdapter {
	return &AIBridgeAdapter{
		bridge: bridge,
		agent:  agent,
		nodeID: nodeID,
	}
}

// AIPaymentRequest represents an AI compute payment request
type AIPaymentRequest struct {
	JobID         string
	SourceChain   string
	TargetChain   string
	Amount        *big.Int
	Recipient     string
	ComputeUnits  uint64
	ModelName     string
	RequireProof  bool
	MaxWaitTime   time.Duration
}

// AIPaymentResult contains the payment result and AI consensus decision
type AIPaymentResult struct {
	TxHash       string
	Amount       *big.Int
	Verified     bool
	Decision     *ai.Decision[ai.TransactionData]
	Confidence   float64
	BridgeStatus lx.BridgeStatus
}

// ProcessAIPayment handles an AI compute payment with consensus validation
//
// This demonstrates the integration pattern:
// 1. AI agent validates the payment request
// 2. Production bridge handles cross-chain transfer
// 3. AI consensus verifies the result
func (a *AIBridgeAdapter) ProcessAIPayment(ctx context.Context, req *AIPaymentRequest) (*AIPaymentResult, error) {
	// Step 1: AI consensus validates payment parameters
	txData := ai.TransactionData{
		From:      a.nodeID,
		To:        req.Recipient,
		Amount:    req.Amount.Uint64(),
		Fee:       calculateFee(req.Amount),
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"job_id":        req.JobID,
			"compute_units": req.ComputeUnits,
			"model":         req.ModelName,
		},
	}

	// AI makes decision about payment
	decision, err := a.agent.ProposeDecision(ctx, txData, map[string]interface{}{
		"type":          "ai_payment",
		"source_chain":  req.SourceChain,
		"target_chain":  req.TargetChain,
		"require_proof": req.RequireProof,
	})
	if err != nil {
		return nil, fmt.Errorf("AI consensus failed: %w", err)
	}

	// Check AI confidence threshold
	if decision.Confidence < 0.7 {
		return nil, fmt.Errorf("AI confidence too low: %.2f", decision.Confidence)
	}

	// Step 2: Use production bridge for actual transfer
	transfer := &lx.BridgeTransfer{
		Asset:            "LUX",
		Amount:           req.Amount,
		Fee:              big.NewInt(int64(txData.Fee)),
		SourceChain:      req.SourceChain,
		DestChain:        req.TargetChain,
		DestAddress:      req.Recipient,
		RequiredConfirms: 6,
		InitiatedAt:      time.Now(),
		ExpiryTime:       time.Now().Add(req.MaxWaitTime),
	}

	// Initiate transfer using production bridge
	transferID, err := a.bridge.InitiateTransfer(ctx, transfer)
	if err != nil {
		return nil, fmt.Errorf("bridge transfer failed: %w", err)
	}

	// Step 3: Wait for transfer completion and verify
	result := &AIPaymentResult{
		TxHash:       transferID,
		Amount:       req.Amount,
		Decision:     decision,
		Confidence:   decision.Confidence,
		BridgeStatus: lx.BridgeStatusPending,
	}

	// Monitor transfer status
	if err := a.monitorTransfer(ctx, transferID, req.MaxWaitTime, result); err != nil {
		return nil, fmt.Errorf("transfer monitoring failed: %w", err)
	}

	// Step 4: AI verifies the completed transfer
	if result.BridgeStatus == lx.BridgeStatusCompleted {
		result.Verified = true
		
		// Record successful payment in AI training data
		example := ai.TrainingExample[ai.TransactionData]{
			Input:    txData,
			Output:   *decision,
			Feedback: 1.0, // Positive feedback
			NodeID:   a.nodeID,
			Weight:   1.0,
			Context: map[string]interface{}{
				"transfer_id": transferID,
				"completed":   true,
			},
		}
		a.agent.AddTrainingData(example)
	} else {
		// Record failed payment for learning
		example := ai.TrainingExample[ai.TransactionData]{
			Input:    txData,
			Output:   *decision,
			Feedback: -0.5, // Negative feedback
			NodeID:   a.nodeID,
			Weight:   1.0,
			Context: map[string]interface{}{
				"transfer_id": transferID,
				"failed":      true,
				"status":      result.BridgeStatus,
			},
		}
		a.agent.AddTrainingData(example)
	}

	return result, nil
}

// monitorTransfer waits for transfer completion with timeout
func (a *AIBridgeAdapter) monitorTransfer(
	ctx context.Context,
	transferID string,
	maxWait time.Duration,
	result *AIPaymentResult,
) error {
	deadline := time.Now().Add(maxWait)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("transfer timeout after %v", maxWait)
			}

			// Query transfer status from production bridge
			transfer, err := a.bridge.GetTransfer(transferID)
			if err != nil {
				continue // Retry on error
			}

			result.BridgeStatus = transfer.Status

			// Check completion
			if transfer.Status == lx.BridgeStatusCompleted {
				return nil
			}
			if transfer.Status == lx.BridgeStatusFailed {
				return fmt.Errorf("transfer failed")
			}
		}
	}
}

// VerifyAIPayment verifies a payment using both bridge and AI consensus
func (a *AIBridgeAdapter) VerifyAIPayment(
	ctx context.Context,
	transferID string,
	expectedAmount *big.Int,
) (*AIPaymentResult, error) {
	// Get transfer from production bridge
	transfer, err := a.bridge.GetTransfer(transferID)
	if err != nil {
		return nil, fmt.Errorf("bridge query failed: %w", err)
	}

	// Verify amount matches
	if transfer.Amount.Cmp(expectedAmount) < 0 {
		return nil, fmt.Errorf("amount mismatch: got %s, expected %s",
			transfer.Amount.String(), expectedAmount.String())
	}

	// Create transaction data for AI verification
	txData := ai.TransactionData{
		Hash:      transferID,
		Amount:    transfer.Amount.Uint64(),
		Timestamp: transfer.CompletedAt,
	}

	// AI consensus verifies the transaction
	decision, err := a.agent.ProposeDecision(ctx, txData, map[string]interface{}{
		"type":   "payment_verification",
		"status": transfer.Status,
	})
	if err != nil {
		return nil, fmt.Errorf("AI verification failed: %w", err)
	}

	return &AIPaymentResult{
		TxHash:       transferID,
		Amount:       transfer.Amount,
		Verified:     decision.Action == "approve",
		Decision:     decision,
		Confidence:   decision.Confidence,
		BridgeStatus: transfer.Status,
	}, nil
}

// GetExchangeRate queries exchange rate from production bridge
func (a *AIBridgeAdapter) GetExchangeRate(ctx context.Context, fromChain, toChain string) (*big.Int, error) {
	// Use production bridge's exchange rate system
	// This is implemented in lx.CrossChainBridge with real market data
	return a.bridge.GetExchangeRate(ctx, fromChain, toChain)
}

// calculateFee computes fee based on amount (simplified)
func calculateFee(amount *big.Int) uint64 {
	// 0.1% fee
	fee := new(big.Int).Div(amount, big.NewInt(1000))
	return fee.Uint64()
}

// Example: How to use the AI bridge adapter
//
//	// Create production bridge (from DEX project)
//	bridge := lx.NewCrossChainBridge(config)
//
//	// Create AI consensus agent
//	agent := ai.NewAgent(nodeID, model, quasar, photon)
//
//	// Wrap with AI adapter
//	aiBridge := marketplace.NewAIBridgeAdapter(bridge, agent, nodeID)
//
//	// Process AI compute payment with consensus
//	result, err := aiBridge.ProcessAIPayment(ctx, &marketplace.AIPaymentRequest{
//		JobID:        "job123",
//		SourceChain:  "ethereum",
//		TargetChain:  "lux",
//		Amount:       big.NewInt(1000000),
//		Recipient:    "0x...",
//		ComputeUnits: 1000,
//		ModelName:    "gpt-4",
//		MaxWaitTime:  5 * time.Minute,
//	})
