# Lux Consensus Documentation

## Overview

The Lux Consensus framework provides high-performance, Byzantine fault-tolerant consensus implementations across multiple languages. This documentation covers installation, usage, and integration for C, C++, Rust, Python, and Go implementations.

## Quick Links

- [C Implementation](./c/README.md) - High-performance native implementation
- [C++ MLX Implementation](./cpp/README.md) - Modern C++ with MLX extensions
- [Rust Implementation](./rust/README.md) - Memory-safe systems programming
- [Python Implementation](./python/README.md) - Rapid prototyping and research
- [Go Implementation](./go/README.md) - Production blockchain integration

## Performance

Our consensus implementations achieve:
- **14,000+ votes/second** in real-world testing
- **625M votes/second** theoretical at 40 Gbps
- **12.5B votes/second** theoretical at 800 Gbps
- **Sub-10s finality** on mainnet
- **3.69s finality** in local testing

## Consensus Engines

All implementations support these consensus engines:

### Snowball
Classic Snowball consensus with Byzantine fault tolerance.
- K parameter: number of consecutive successes needed
- Alpha: quorum size for each query
- Beta: confidence threshold for decision

### Avalanche
DAG-based consensus with conflict set tracking.
- Maintains directed acyclic graph of decisions
- Handles concurrent conflicting transactions
- Optimal for high-throughput scenarios

### Snowflake
Simplified consensus for single-choice decisions.
- Binary consensus protocol
- Minimal state overhead
- Fast finality for simple decisions

### DAG (Directed Acyclic Graph)
Full DAG consensus with vertex ordering.
- Topological ordering of transactions
- Conflict resolution via graph traversal
- Maximum parallelism in processing

### Chain
Linear chain consensus for ordered blocks.
- Sequential block processing
- Fork choice rule based on weight
- Compatible with traditional blockchains

### PostQuantum
Quantum-resistant consensus using lattice cryptography.
- ML-KEM-768/1024 for key encapsulation
- ML-DSA-44/65 for signatures
- Future-proof security guarantees

## Protocol Specifications

### Binary Vote Protocol (8 bytes)
```
┌─────────────┬────────────┬──────────┬───────────┬──────────┐
│ Engine Type │ Node ID    │ Block ID │ Vote Type │ Reserved │
│ (1 byte)    │ (2 bytes)  │ (2 bytes)│ (1 byte)  │ (2 bytes)│
└─────────────┴────────────┴──────────┴───────────┴──────────┘
```

### Message Types
- `VOTE_PREFER` (0x01) - Preference vote
- `VOTE_ACCEPT` (0x02) - Acceptance vote
- `VOTE_REJECT` (0x03) - Rejection vote
- `QUERY_STATE` (0x04) - State query
- `SYNC_BLOCKS` (0x05) - Block synchronization

## ZeroMQ Transport

All implementations use ZeroMQ for high-performance messaging:

```c
// Publisher pattern for broadcasting votes
void *publisher = zmq_socket(context, ZMQ_PUB);
zmq_bind(publisher, "tcp://*:5555");

// Subscriber pattern for receiving votes
void *subscriber = zmq_socket(context, ZMQ_SUB);
zmq_connect(subscriber, "tcp://localhost:5555");
zmq_setsockopt(subscriber, ZMQ_SUBSCRIBE, "", 0);
```

## Benchmarking

Run benchmarks for any implementation:

```bash
# C implementation
make bench-c

# Rust implementation
cargo bench

# Go implementation
go test -bench=. ./...

# Python implementation
python benchmarks/run_benchmarks.py

# Full matrix test
make test-matrix
```

## Integration Examples

### Basic Integration (Go)
```go
import "github.com/luxfi/consensus/engine/core"

params := core.ConsensusParams{
    K:               20,
    AlphaPreference: 15,
    AlphaConfidence: 15,
    Beta:            20,
}

consensus, err := core.NewCGOConsensus(params)
if err != nil {
    log.Fatal(err)
}

// Add block to consensus
consensus.Add(block)

// Record vote
consensus.RecordPoll(blockID, true)

// Check if accepted
if consensus.IsAccepted(blockID) {
    fmt.Println("Block accepted!")
}
```

### Network Integration (C)
```c
#include "consensus.h"
#include "network.h"

// Initialize consensus
consensus_t* consensus = consensus_new(SNOWBALL);

// Set up network
network_t* net = network_new("tcp://0.0.0.0:5555");

// Process incoming votes
while (running) {
    vote_msg_t vote;
    if (network_recv(net, &vote, sizeof(vote)) > 0) {
        consensus_process_vote(consensus, &vote);
    }
}
```

## Testing

Comprehensive test coverage across all implementations:

```bash
# Unit tests
make test

# Integration tests
make test-integration

# Parity tests (cross-language verification)
make test-parity

# Byzantine fault tolerance tests
make test-byzantine

# Performance regression tests
make test-perf
```

## Security Considerations

- All implementations use secure random number generation
- Byzantine fault tolerance up to f < n/3 malicious nodes
- Post-quantum security available via PostQuantum engine
- Network messages authenticated via HMAC-SHA256
- TLS 1.3 support for encrypted transport

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for development guidelines.

## License

Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.