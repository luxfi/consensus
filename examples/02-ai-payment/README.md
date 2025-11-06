# Example 02: AI-Powered Payment Validation

This example demonstrates how AI consensus validates cross-chain payments before execution.

## What It Shows

- AI agent analyzing payment requests
- Confidence scoring for transactions
- Fraud detection using AI
- Learning from payment outcomes

## Run the Example

```bash
cd examples/02-ai-payment
go run main.go
```

## Run the Tests

```bash
go test -v
```

## Expected Output

```
=== AI Payment Validation ===

Payment Request #1:
  Amount:   1000 LUX
  From:     ethereum
  To:       lux
  
AI Analysis:
  ✓ Confidence: 0.92 (HIGH)
  ✓ Risk Level: LOW
  ✓ Decision: APPROVE
  Reasoning: Normal transaction pattern, amount within limits

Payment Request #2:
  Amount:   50000 LUX
  From:     ethereum
  To:       lux
  
AI Analysis:
  ⚠ Confidence: 0.45 (LOW)
  ⚠ Risk Level: HIGH
  ✗ Decision: REJECT
  Reasoning: Unusual amount, potential fraud

=== Learning from Outcomes ===
✓ AI updated with 2 training examples
✓ Model weights adjusted based on feedback
```

## Key Concepts

1. **AI Decision Making**: Agent analyzes payment before execution
2. **Confidence Scoring**: Determines how sure the AI is
3. **Risk Assessment**: Identifies potentially fraudulent transactions
4. **Continuous Learning**: Improves from successful/failed payments

## Integration Pattern

```go
// 1. Create AI agent for payment validation
agent := ai.NewAgent(nodeID, model, quasar, photon)

// 2. Analyze payment request
decision, err := agent.ProposeDecision(ctx, txData, context)

// 3. Check confidence threshold
if decision.Confidence < 0.7 {
    // Reject low-confidence payments
    return errors.New("confidence too low")
}

// 4. Learn from outcome
agent.AddTrainingData(example)
```

## Play With It

Try modifying:
- Confidence thresholds
- Risk assessment criteria
- Training data feedback
- Transaction patterns
