# Lux Consensus Examples - Pedagogical Guide

This directory contains progressive examples teaching you how to use the Lux consensus and cross-chain infrastructure.

## Learning Path 🎓

The examples are designed to be completed in order, each building on concepts from the previous:

```
Basic Concepts → Networking → AI Integration → Advanced Consensus
     ↓              ↓               ↓                   ↓
   01-02          03-04           05-06                 07
```

## Examples Overview

### Level 1: Basic Concepts

**[01-simple-bridge](./01-simple-bridge/)** - Cross-Chain Bridge Basics
- Learn: Bridge creation, asset transfers, status monitoring
- Tech: Go, Lux DEX bridge
- Time: 15 minutes
- Prerequisites: None

**[02-ai-payment](./02-ai-payment/)** - AI Payment Validation
- Learn: AI decision making, confidence scoring, fraud detection
- Tech: Go, AI consensus agents
- Time: 20 minutes
- Prerequisites: Example 01

### Level 2: Networking & Communication

**[03-qzmq-networking](./03-qzmq-networking/)** - Quantum-Secure Messaging
- Learn: QZMQ protocol, quantum-resistant encryption, message routing
- Tech: Go, ZeroMQ, post-quantum crypto
- Time: 25 minutes
- Prerequisites: Example 01

**[04-grpc-service](./04-grpc-service/)** - gRPC API Integration
- Learn: Protocol buffers, gRPC services, API design
- Tech: Go, gRPC, Protocol Buffers
- Time: 30 minutes
- Prerequisites: Example 01, 03

### Level 3: Multi-Language Integration

**[05-python-client](./05-python-client/)** - Python Bridge Client
- Learn: Cross-language RPC, Python integration, async programming
- Tech: Python, gRPC, asyncio
- Time: 25 minutes
- Prerequisites: Example 04

**[06-nodejs-client](./06-nodejs-client/)** - Node.js TypeScript Client
- Learn: TypeScript integration, WebSocket connections, real-time updates
- Tech: TypeScript, Node.js, WebSocket
- Time: 25 minutes
- Prerequisites: Example 04

### Level 4: Advanced AI Consensus

**[07-ai-consensus](./07-ai-consensus/)** - Dynamic AI-Managed Consensus
- Learn: Multi-agent consensus, shared hallucinations, photon→quasar flow
- Tech: Go, AI agents, consensus protocols
- Time: 45 minutes
- Prerequisites: All previous examples

## Quick Start

### Run a Simple Example

```bash
# Example 1: Simple Bridge
cd 01-simple-bridge
go run main.go

# Run its tests
go test -v
```

### Run All Examples

```bash
# Test all Go examples
./run_all_tests.sh

# Or individually
for dir in 0*/; do
  cd "$dir"
  go test -v
  cd ..
done
```

## What You'll Learn

### Core Concepts

1. **Cross-Chain Transfers** (Ex 01)
   - How assets move between blockchains
   - Transfer lifecycle and confirmations
   - Exchange rates and liquidity

2. **AI Decision Making** (Ex 02)
   - How AI validates transactions
   - Confidence scoring and thresholds
   - Continuous learning from outcomes

3. **Quantum-Secure Networking** (Ex 03)
   - Post-quantum cryptography
   - Secure message routing
   - Performance vs security tradeoffs

4. **Service Communication** (Ex 04)
   - gRPC vs REST patterns
   - Protocol buffer schemas
   - Service mesh integration

5. **Language Interoperability** (Ex 05-06)
   - Go backend, Python/Node frontend
   - Cross-language type safety
   - Error handling across boundaries

6. **Advanced Consensus** (Ex 07)
   - Multi-agent coordination
   - Shared hallucinations
   - Photon→Quasar consensus flow

### Integration Patterns

Each example demonstrates a key integration pattern:

- **01**: Production bridge as dependency
- **02**: AI agent wrapping business logic
- **03**: QZMQ for secure transport layer
- **04**: gRPC for service boundaries
- **05-06**: Multi-language clients
- **07**: Full AI consensus orchestration

## Testing Philosophy

Each example includes:
- ✅ **Runnable Demo** (`main.go` or `main.py`) - See it work
- ✅ **Unit Tests** (`*_test.go`) - Verify behavior
- ✅ **Documentation** (`README.md`) - Understand why
- ✅ **Interactive** - Modify and experiment

Run tests to prove the examples work:

```bash
# Go examples
go test -v

# Python examples
pytest -v

# Node.js examples
npm test
```

## Project Structure

```
examples/
├── README.md                 # This file
├── 01-simple-bridge/         # Basic bridge usage
│   ├── main.go              # Runnable demo
│   ├── bridge_test.go       # Tests
│   ├── go.mod               # Dependencies
│   └── README.md            # Guide
│
├── 02-ai-payment/           # AI validation
│   ├── main.go
│   ├── payment_test.go
│   └── README.md
│
├── 03-qzmq-networking/      # Quantum-secure messaging
│   ├── main.go
│   ├── qzmq_test.go
│   └── README.md
│
├── 04-grpc-service/         # gRPC service
│   ├── server/main.go
│   ├── client/main.go
│   ├── proto/bridge.proto
│   └── README.md
│
├── 05-python-client/        # Python integration
│   ├── client.py
│   ├── test_client.py
│   ├── requirements.txt
│   └── README.md
│
├── 06-nodejs-client/        # Node.js integration
│   ├── src/client.ts
│   ├── test/client.test.ts
│   ├── package.json
│   └── README.md
│
└── 07-ai-consensus/         # Advanced AI consensus
    ├── main.go
    ├── consensus_test.go
    └── README.md
```

## Tips for Learning

1. **Follow the Order**: Each example builds on previous concepts
2. **Run the Code**: Don't just read - execute and experiment
3. **Read the Tests**: Tests show expected behavior and edge cases
4. **Modify and Break**: Change values, see what fails, understand why
5. **Check Dependencies**: Each example lists required packages
6. **Ask Questions**: README files explain the "why" behind decisions

## Common Patterns

### Creating Clients

```go
// Pattern 1: Direct instantiation
bridge := lx.NewCrossChainBridge(config)

// Pattern 2: Builder pattern
engine := ai.NewBuilder().
    WithInference(model).
    WithDecision(threshold).
    Build()

// Pattern 3: Dependency injection
adapter := NewAIBridgeAdapter(bridge, agent, nodeID)
```

### Error Handling

```go
// Pattern 1: Immediate error check
result, err := client.Transfer(...)
if err != nil {
    return fmt.Errorf("transfer failed: %w", err)
}

// Pattern 2: Retry with backoff
for i := 0; i < maxRetries; i++ {
    if err := operation(); err == nil {
        break
    }
    time.Sleep(backoff * time.Duration(i))
}

// Pattern 3: Circuit breaker
if breaker.Allow() {
    err := operation()
    breaker.Record(err)
}
```

### Testing Strategies

```go
// Pattern 1: Table-driven tests
tests := []struct {
    name string
    input int
    want int
}{
    {"case1", 1, 2},
    {"case2", 2, 4},
}

// Pattern 2: Mocking dependencies
mock := &MockBridge{}
client := NewClient(mock)

// Pattern 3: Integration tests
if testing.Short() {
    t.Skip("skipping integration test")
}
```

## Troubleshooting

### "Cannot find package"
```bash
# Ensure you're in the example directory
cd examples/01-simple-bridge

# Initialize module
go mod init
go mod tidy
```

### "Connection refused"
```bash
# For examples requiring servers (04, 05, 06)
# Start the server first in one terminal
cd server && go run main.go

# Then run client in another terminal
cd ../client && go run main.go
```

### "Import cycle"
```bash
# Check go.mod has correct replace directives
replace github.com/luxfi/consensus => ../..
replace github.com/luxfi/dex => ../../../dex
```

## Contributing

Want to add an example? Follow these guidelines:

1. **Pedagogical**: Teach one concept clearly
2. **Progressive**: Build on previous examples
3. **Tested**: Include comprehensive tests
4. **Documented**: Explain the "why" not just "how"
5. **Runnable**: Should work out of the box

## Additional Resources

- [Lux Documentation](https://docs.lux.network)
- [AI Consensus Whitepaper](../../paper/)
- [DEX Bridge Documentation](../../../dex/docs/)
- [QZMQ Specification](../../utils/qzmq/README.md)

## Support

Questions? Check:
1. Example-specific README.md
2. Test files for usage patterns
3. Source code comments
4. Main project documentation

Happy learning! 🚀
