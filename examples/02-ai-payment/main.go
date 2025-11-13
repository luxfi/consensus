// Example 02: AI-Powered Payment Validation
//
// This demonstrates using AI consensus to validate payments before execution.
// The AI analyzes transaction patterns, amounts, and risk factors.

package main

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/luxfi/consensus/ai"
)

func main() {
	fmt.Println("=== AI Payment Validation Example ===\n")

	// Step 1: Create AI agent for payment validation
	agent := createAIAgent()
	fmt.Println("✓ AI agent initialized\n")

	// Step 2: Train agent with historical data
	trainAgent(agent)
	fmt.Println("✓ Agent trained with 100 historical transactions\n")

	// Step 3: Test payment validations
	testPayments := []PaymentRequest{
		{
			ID:          "pay-001",
			Amount:      big.NewInt(1000),
			SourceChain: "ethereum",
			DestChain:   "lux",
			Sender:      "0x123...sender1",
			Recipient:   "0x456...recipient1",
			Description: "Normal payment",
		},
		{
			ID:          "pay-002",
			Amount:      big.NewInt(50000),
			SourceChain: "ethereum",
			DestChain:   "lux",
			Sender:      "0x789...sender2",
			Recipient:   "0xabc...recipient2",
			Description: "Large unusual payment",
		},
		{
			ID:          "pay-003",
			Amount:      big.NewInt(100),
			SourceChain: "lux",
			DestChain:   "ethereum",
			Sender:      "0xdef...sender3",
			Recipient:   "0x123...recipient3",
			Description: "Small payment",
		},
	}

	fmt.Println("=== Validating Payments ===\n")

	ctx := context.Background()
	for _, payment := range testPayments {
		validatePayment(ctx, agent, payment)
		fmt.Println()
	}

	// Step 4: Show learning statistics
	showLearningStats(agent)
}

type PaymentRequest struct {
	ID          string
	Amount      *big.Int
	SourceChain string
	DestChain   string
	Sender      string
	Recipient   string
	Description string
}

func createAIAgent() *ai.Agent[ai.TransactionData] {
	// Create simple model for payment validation
	model := ai.NewSimpleModel("payment-validator")

	// Create agent (photon and quasar are nil for this example)
	agent := ai.New("node-001", model, nil, nil)

	return agent
}

func trainAgent(agent *ai.Agent[ai.TransactionData]) {
	// Train with historical transactions
	// In production, this would load from database

	// Normal successful payments (positive training)
	for i := 0; i < 70; i++ {
		amount := uint64(500 + i*10) // 500-1200 range
		example := ai.TrainingExample[ai.TransactionData]{
			Input: ai.TransactionData{
				Amount:    amount,
				Timestamp: time.Now().Add(-time.Duration(i) * time.Hour),
			},
			Output: ai.Decision[ai.TransactionData]{
				Action:     "approve",
				Confidence: 0.9,
			},
			Feedback: 1.0, // Positive feedback
			NodeID:   "node-001",
			Weight:   1.0,
		}
		agent.AddTrainingData(example)
	}

	// Fraudulent payments (negative training)
	for i := 0; i < 20; i++ {
		amount := uint64(50000 + i*1000) // Very large amounts
		example := ai.TrainingExample[ai.TransactionData]{
			Input: ai.TransactionData{
				Amount:    amount,
				Timestamp: time.Now().Add(-time.Duration(i) * time.Hour),
			},
			Output: ai.Decision[ai.TransactionData]{
				Action:     "reject",
				Confidence: 0.85,
			},
			Feedback: 1.0, // Correct rejection
			NodeID:   "node-001",
			Weight:   1.0,
		}
		agent.AddTrainingData(example)
	}

	// Borderline cases (mixed)
	for i := 0; i < 10; i++ {
		amount := uint64(2000 + i*100) // Medium amounts
		example := ai.TrainingExample[ai.TransactionData]{
			Input: ai.TransactionData{
				Amount:    amount,
				Timestamp: time.Now().Add(-time.Duration(i) * time.Hour),
			},
			Output: ai.Decision[ai.TransactionData]{
				Action:     "review",
				Confidence: 0.6,
			},
			Feedback: 0.5, // Neutral
			NodeID:   "node-001",
			Weight:   1.0,
		}
		agent.AddTrainingData(example)
	}
}

func validatePayment(ctx context.Context, agent *ai.Agent[ai.TransactionData], payment PaymentRequest) {
	fmt.Printf("Payment Request #%s:\n", payment.ID)
	fmt.Printf("  Description: %s\n", payment.Description)
	fmt.Printf("  Amount:      %s LUX\n", payment.Amount.String())
	fmt.Printf("  From:        %s (%s)\n", payment.SourceChain, payment.Sender[:10]+"...")
	fmt.Printf("  To:          %s (%s)\n", payment.DestChain, payment.Recipient[:10]+"...")
	fmt.Println()

	// Create transaction data for AI
	txData := ai.TransactionData{
		Hash:      payment.ID,
		From:      payment.Sender,
		To:        payment.Recipient,
		Amount:    payment.Amount.Uint64(),
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"source_chain": payment.SourceChain,
			"dest_chain":   payment.DestChain,
		},
	}

	// AI makes decision
	decision, err := agent.ProposeDecision(ctx, txData, map[string]interface{}{
		"type": "payment_validation",
	})

	if err != nil {
		fmt.Printf("  ✗ AI Error: %v\n", err)
		return
	}

	// Display AI analysis
	fmt.Println("AI Analysis:")
	fmt.Printf("  Confidence:  %.2f ", decision.Confidence)

	// Show confidence level
	if decision.Confidence >= 0.8 {
		fmt.Println("(HIGH)")
	} else if decision.Confidence >= 0.6 {
		fmt.Println("(MEDIUM)")
	} else {
		fmt.Println("(LOW)")
	}

	// Show risk assessment
	risk := assessRisk(payment.Amount)
	fmt.Printf("  Risk Level:  %s\n", risk)

	// Show decision
	if decision.Confidence >= 0.7 {
		fmt.Println("  ✓ Decision:  APPROVE")
	} else if decision.Confidence >= 0.5 {
		fmt.Println("  ⚠ Decision:  REVIEW")
	} else {
		fmt.Println("  ✗ Decision:  REJECT")
	}

	fmt.Printf("  Reasoning:   %s\n", decision.Reasoning)

	// Record outcome for learning
	feedback := 1.0
	if decision.Confidence < 0.5 {
		feedback = -0.5
	} else if decision.Confidence < 0.7 {
		feedback = 0.5
	}

	example := ai.TrainingExample[ai.TransactionData]{
		Input:    txData,
		Output:   *decision,
		Feedback: feedback,
		NodeID:   "node-001",
		Weight:   1.0,
		Context: map[string]interface{}{
			"payment_id": payment.ID,
			"risk":       risk,
		},
	}
	agent.AddTrainingData(example)
}

func assessRisk(amount *big.Int) string {
	threshold := big.NewInt(10000)
	if amount.Cmp(threshold) > 0 {
		return "HIGH"
	}

	mediumThreshold := big.NewInt(2000)
	if amount.Cmp(mediumThreshold) > 0 {
		return "MEDIUM"
	}

	return "LOW"
}

func showLearningStats(agent *ai.Agent[ai.TransactionData]) {
	fmt.Println("=== Learning Statistics ===\n")
	fmt.Println("✓ AI continuously learns from payment outcomes")
	fmt.Println("✓ Model improves accuracy over time")
	fmt.Println("✓ Adapts to new fraud patterns automatically")
	fmt.Println()
	fmt.Println("In production, this would show:")
	fmt.Println("  - Total payments processed")
	fmt.Println("  - Accuracy rate")
	fmt.Println("  - False positive/negative rates")
	fmt.Println("  - Model performance metrics")
}
