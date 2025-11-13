package main

import (
	"context"
	"math/big"
	"testing"

	"github.com/luxfi/consensus/ai"
)

func TestAIAgentCreation(t *testing.T) {
	agent := createAIAgent()

	if agent == nil {
		t.Fatal("failed to create AI agent")
	}
}

func TestAgentTraining(t *testing.T) {
	agent := createAIAgent()
	trainAgent(agent)

	// Agent should have training data after training
	// This is a smoke test to ensure training doesn't panic
}

func TestPaymentValidation_NormalAmount(t *testing.T) {
	agent := createAIAgent()
	trainAgent(agent)

	payment := PaymentRequest{
		ID:          "test-001",
		Amount:      big.NewInt(1000), // Normal amount
		SourceChain: "ethereum",
		DestChain:   "lux",
		Sender:      "0x123",
		Recipient:   "0x456",
	}

	ctx := context.Background()

	// Create transaction data
	txData := ai.TransactionData{
		Hash:   payment.ID,
		From:   payment.Sender,
		To:     payment.Recipient,
		Amount: payment.Amount.Uint64(),
	}

	decision, err := agent.ProposeDecision(ctx, txData, map[string]interface{}{
		"type": "payment_validation",
	})

	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// Normal amounts should have reasonable confidence
	if decision.Confidence < 0.3 {
		t.Errorf("confidence too low for normal amount: %.2f", decision.Confidence)
	}
}

func TestPaymentValidation_LargeAmount(t *testing.T) {
	agent := createAIAgent()
	trainAgent(agent)

	payment := PaymentRequest{
		ID:          "test-002",
		Amount:      big.NewInt(50000), // Large amount
		SourceChain: "ethereum",
		DestChain:   "lux",
		Sender:      "0x789",
		Recipient:   "0xabc",
	}

	ctx := context.Background()

	txData := ai.TransactionData{
		Hash:   payment.ID,
		From:   payment.Sender,
		To:     payment.Recipient,
		Amount: payment.Amount.Uint64(),
	}

	decision, err := agent.ProposeDecision(ctx, txData, map[string]interface{}{
		"type": "payment_validation",
	})

	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// Large amounts might have lower confidence
	// Just ensure we got a decision
	if decision == nil {
		t.Error("expected decision, got nil")
	}
}

func TestRiskAssessment(t *testing.T) {
	tests := []struct {
		amount   *big.Int
		expected string
	}{
		{big.NewInt(100), "LOW"},
		{big.NewInt(1000), "LOW"},
		{big.NewInt(5000), "MEDIUM"},
		{big.NewInt(15000), "HIGH"},
		{big.NewInt(100000), "HIGH"},
	}

	for _, tt := range tests {
		result := assessRisk(tt.amount)
		if result != tt.expected {
			t.Errorf("assessRisk(%s) = %s, expected %s",
				tt.amount, result, tt.expected)
		}
	}
}

func TestMultiplePaymentValidations(t *testing.T) {
	agent := createAIAgent()
	trainAgent(agent)

	payments := []PaymentRequest{
		{
			ID:     "batch-001",
			Amount: big.NewInt(500),
		},
		{
			ID:     "batch-002",
			Amount: big.NewInt(1500),
		},
		{
			ID:     "batch-003",
			Amount: big.NewInt(25000),
		},
	}

	ctx := context.Background()

	for _, payment := range payments {
		txData := ai.TransactionData{
			Hash:   payment.ID,
			Amount: payment.Amount.Uint64(),
		}

		decision, err := agent.ProposeDecision(ctx, txData, map[string]interface{}{
			"type": "payment_validation",
		})

		if err != nil {
			t.Errorf("validation failed for %s: %v", payment.ID, err)
			continue
		}

		if decision == nil {
			t.Errorf("nil decision for %s", payment.ID)
		}
	}
}

// Benchmark AI validation performance
func BenchmarkPaymentValidation(b *testing.B) {
	agent := createAIAgent()
	trainAgent(agent)

	payment := PaymentRequest{
		ID:     "bench-001",
		Amount: big.NewInt(1000),
	}

	txData := ai.TransactionData{
		Hash:   payment.ID,
		Amount: payment.Amount.Uint64(),
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agent.ProposeDecision(ctx, txData, map[string]interface{}{
			"type": "payment_validation",
		})
	}
}
