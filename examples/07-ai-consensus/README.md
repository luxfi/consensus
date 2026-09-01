# Example 07: Dynamic AI-Managed Consensus

**⚡ Advanced Example** - This brings together all previous concepts into a complete AI consensus system.

## What You'll Learn

- Multi-agent consensus coordination
- Shared hallucinations across nodes
- Photon→Quasar consensus flow
- Dynamic AI adaptation and learning
- Production-ready consensus patterns

## Prerequisites

**IMPORTANT**: Complete all previous examples first!
- Example 01: Cross-chain bridge basics
- Example 02: AI payment validation
- Example 03: Quantum-secure networking
- Example 04: gRPC services
- Example 05-06: Multi-language clients

## Concepts Demonstrated

### 1. Photon→Quasar Consensus Flow

```
Photon Phase:  Emit proposals at light speed
    ↓
Wave Phase:    Amplify through network
    ↓
Focus Phase:   Converge on best options
    ↓
Prism Phase:   Refract through DAG
    ↓
Horizon Phase: Finalize with quantum certificate
```

### 2. Shared Hallucinations

AI agents share "hallucinations" (model states) across the network:
- **Consensus**: Agents agree on shared reality
- **Evolution**: States improve through feedback
- **Diversity**: Multiple perspectives strengthen decisions

### 3. Multi-Agent Coordination

```
Agent A: Proposes → [Photon Broadcast]
Agent B: Validates →  ↓
Agent C: Votes →      [Quasar DAG]
Agent D: Finalizes →  ↓
                   [Horizon Certificate]
```

## Run the Example

### Terminal 1: Start Consensus Node 1
```bash
cd examples/07-ai-consensus
go run main.go --node=1 --port=5001
```

### Terminal 2: Start Consensus Node 2
```bash
go run main.go --node=2 --port=5002 --peer=localhost:5001
```

### Terminal 3: Start Consensus Node 3
```bash
go run main.go --node=3 --port=5003 --peer=localhost:5001
```

### Terminal 4: Submit Proposals
```bash
go run client/main.go --endpoint=localhost:5001
```

## Run the Tests

```bash
# Unit tests
go test -v

# Integration tests (requires multiple nodes)
go test -v -tags=integration

# Benchmark consensus throughput
go test -bench=. -benchmem
```

## Expected Output

**Node 1 (Proposer):**
```
=== AI Consensus Node #1 ===

Initializing:
✓ AI agent created (node-001)
✓ QZMQ publisher started on :5001
✓ Photon emitter initialized
✓ Quasar DAG engine ready

Phase 1: Photon Emission
→ Received proposal: Block #100
→ AI analyzing transaction patterns...
✓ Confidence: 0.89 (HIGH)
✓ Emitting photon proposal to network...

Phase 2: Wave Amplification
✓ Broadcast to 2 peer nodes
✓ Awaiting validation responses...

Phase 3: Focus Convergence
← Node-002: APPROVE (confidence: 0.92)
← Node-003: APPROVE (confidence: 0.87)
✓ Consensus achieved (3/3 nodes)

Phase 4: Prism Validation
✓ Adding to Quasar DAG...
✓ DAG validated: Block connects to parent
✓ No conflicts detected

Phase 5: Horizon Finalization
✓ Generating quantum certificate...
✓ Block #100 finalized
✓ Shared hallucination updated

Performance:
  Latency:   245ms
  Throughput: 4.08 blocks/sec
  AI CPU:    12%
  Network:   2.3 KB/sec
```

**Node 2 (Validator):**
```
=== AI Consensus Node #2 ===

Phase 1: Receive Proposal
← Photon received from node-001
  Block: #100
  Hash:  0x abc...

Phase 2: AI Validation
→ AI analyzing proposal...
  Transactions: 15
  Gas limit: 8M
  Confidence: 0.92
✓ Decision: APPROVE

Phase 3: Vote Broadcast
→ Broadcasting vote via QZMQ...
✓ Vote sent with ML-DSA signature

Phase 4: Learning
✓ Training from consensus outcome
✓ Model weights updated
✓ Shared hallucination synchronized
```

## Code Walkthrough

### 1. Create AI Consensus Agent

```go
// Create model for block validation
model := ai.NewSimpleModel("block-validator")

// Create agent with photon and quasar engines
agent := ai.New(nodeID, model, quasarEngine, photonEngine)

// Configure consensus parameters
agent.Configure(ai.Config{
    ConfidenceThreshold: 0.7,
    RequiredVotes:       2,
    TimeoutPhase:        5 * time.Second,
})
```

### 2. Propose Block

```go
// Create block data
blockData := ai.BlockData{
    Height:       100,
    Transactions: txs,
    Validator:    nodeID,
}

// AI proposes with Photon→Quasar flow
decision, err := agent.ProposeDecision(ctx, blockData, context)

// This internally executes all 5 phases:
// 1. Photon emission
// 2. Wave amplification  
// 3. Focus convergence
// 4. Prism validation
```

### 3. Validate Proposal

```go
// Receive proposal from peer
proposal := <-proposals

// AI validates
confidence, err := agent.ValidateProposal(proposal)

// Vote based on confidence
if confidence > 0.7 {
    vote := createVote(proposal.ID, "APPROVE", confidence)
    qzmq.Publish("consensus/votes", vote)
}
```

### 4. Learn from Outcomes

```go
// After finalization, train AI
example := ai.TrainingExample{
    Input:    blockData,
    Output:   decision,
    Feedback: feedback, // 1.0 if accepted, -1.0 if rejected
}

agent.AddTrainingData(example)

// Synchronize with network
agent.SyncSharedMemory(ctx)
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                 AI Consensus System                      │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌────────────┐    ┌────────────┐    ┌────────────┐   │
│  │  Agent 1   │    │  Agent 2   │    │  Agent 3   │   │
│  │            │    │            │    │            │   │
│  │  • Photon  │───▶│  • Photon  │───▶│  • Photon  │   │
│  │  • Quasar  │◀───│  • Quasar  │◀───│  • Quasar  │   │
│  │  • Model   │    │  • Model   │    │  • Model   │   │
│  └─────┬──────┘    └─────┬──────┘    └─────┬──────┘   │
│        │                  │                  │          │
│        └──────────┬───────┴──────────────────┘          │
│                   │                                      │
│           ┌───────▼────────┐                           │
│           │ Shared Memory  │                           │
│           │ • Hallucinations│                           │
│           │ • Weights      │                           │
│           │ • Training Data │                           │
│           └────────────────┘                           │
│                                                          │
│  Transport: QZMQ (Quantum-Secure ZeroMQ)                │
│  Consensus: Photon→Quasar Flow                          │
│  Learning:  Distributed Gradient Descent                │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

## Performance Metrics

| Metric | Value | Notes |
|--------|-------|-------|
| Consensus Latency | 200-300ms | Including AI validation |
| Throughput | 3-5 blocks/sec | With 3 nodes |
| AI CPU Usage | 10-15% | Per node |
| Network Bandwidth | 2-5 KB/sec | With quantum security |
| Scalability | Linear to 10 nodes | Tested configuration |

## Security Properties

✅ **Quantum-Resistant**: All network communication via QZMQ  
✅ **Byzantine Fault Tolerant**: Handles up to (n-1)/3 malicious nodes  
✅ **AI-Verified**: Every proposal validated by multiple AI agents  
✅ **Cryptographically Provable**: ML-DSA signatures on all messages  
✅ **Forward Secure**: Past consensus cannot be compromised  

## Customization

### 1. Change AI Model

```go
// Use different model types
model := ai.NewNeuralModel(layers)        // Neural network
model := ai.NewLLMModel(llmConfig)        // Large language model
model := ai.NewEnsembleModel(models...)   // Ensemble of models
```

### 2. Adjust Consensus Parameters

```go
config := ai.Config{
    ConfidenceThreshold: 0.8,     // Higher = more strict
    RequiredVotes:       3,        // Minimum votes needed
    TimeoutPhase:        10 * time.Second,
    MaxProposalsPerSec:  10,
}
```

### 3. Add Custom Phases

```go
// Extend the consensus flow
agent.AddModule("custom-validator", customModule)
agent.AddModule("fraud-detector", fraudModule)
```

## Production Deployment

This example demonstrates the core patterns. For production:

1. **Persistent Storage**: Add database for consensus history
2. **Monitoring**: Integrate Prometheus metrics
3. **High Availability**: Deploy with Kubernetes
4. **Security Hardening**: HSM for key management
5. **Performance Tuning**: Adjust based on workload

## Troubleshooting

**"Consensus timeout"**
- Check network connectivity between nodes
- Verify QZMQ ports are open
- Increase timeout values

**"Low confidence scores"**
- AI may need more training data
- Check model configuration
- Verify input data quality

**"DAG conflicts"**
- Ensure nodes are synchronized
- Check for network partitions
- Verify quasar configuration

## Next Steps

You've completed all examples! 🎉

Now you can:
1. Build your own consensus system
2. Integrate with production blockchain
3. Contribute to Lux consensus
4. Deploy multi-chain infrastructure

## References

- [AI Consensus Whitepaper](../../paper/)
- [Photon Protocol Spec](../../protocol/photon/)
- [Quasar DAG Spec](../../protocol/quasar/)
- [QZMQ Documentation](../../utils/qzmq/)
- [Production Deployment Guide](../../docs/DEPLOYMENT.md)
