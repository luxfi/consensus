# Lux Consensus v1.22.0 Documentation

Welcome to Lux Consensus - a next-generation Byzantine fault-tolerant consensus engine with post-quantum security.

[![CI Status](https://github.com/luxfi/consensus/actions/workflows/ci.yml/badge.svg)](https://github.com/luxfi/consensus/actions)
[![Coverage](https://img.shields.io/badge/coverage-96%25-brightgreen)](https://github.com/luxfi/consensus)
[![Go Version](https://img.shields.io/badge/go-1.24.5-blue)](https://go.dev)
[![Release](https://img.shields.io/badge/release-v1.22.0-green)](https://github.com/luxfi/consensus/releases/tag/v1.22.0)

## What is Lux Quasar?

**Lux Quasar** is a unified consensus engine that delivers **2-round finality** with both classical Byzantine fault tolerance and quantum-resistant security. Unlike traditional blockchains that require different consensus mechanisms for different chain types, Quasar provides **"One engine to rule them all"**.

### Why It Matters

**Speed**: Sub-second finality (< 1 second) for all chain types

**Security**: Post-quantum security using lattice-based cryptography

**Simplicity**: One consensus protocol for DAG, linear, EVM, and MPC chains

**Resilience**: Leaderless, fully decentralized Byzantine fault tolerance

## Quick Navigation

### 🚀 Getting Started
- **[Getting Started Tutorial](./tutorials/GETTING_STARTED.md)** - Build your first consensus node
- **[Installation Guide](./tutorials/INSTALLATION.md)** - Quick setup for all SDKs

### 📚 Core Concepts
- **[Byzantine Consensus Explained](./concepts/BYZANTINE_CONSENSUS.md)** - Learn BFT and metastable consensus
- **[Quasar Architecture](./concepts/QUASAR_ARCHITECTURE.md)** - How Lux achieves quantum finality
- **[Protocol Overview](./concepts/PROTOCOL.md)** - Wave, Focus, Photon, and Prism

### 💻 SDK Documentation

| SDK | Installation | Status | Coverage | Throughput |
|-----|--------------|--------|----------|------------|
| **[Go](./sdks/GO.md)** | `go get github.com/luxfi/consensus@v1.22.0` | ✅ Production | 96% | 111K blocks/sec |
| **[Python](./sdks/PYTHON.md)** | `pip install lux-consensus` | ✅ Production | 100% | 6.7M blocks/sec |
| **[Rust](./sdks/RUST.md)** | `cargo add lux-consensus` | ✅ Production | 100% | 1.6M blocks/sec |
| **[C](./sdks/C.md)** | `make install` | ✅ Production | 100% | 111K blocks/sec |
| **[C++](./sdks/CPP.md)** | `cmake && make install` | 🔶 Beta | 95% | 9M blocks/sec |

### 📊 Performance & Benchmarks
- **[Comprehensive Benchmarks](./BENCHMARKS.md)** - All SDK performance metrics
- **[Network Performance](./benchmarks/NETWORK.md)** - Real-world multi-node results
- **[Optimization Guide](./benchmarks/OPTIMIZATION.md)** - Tuning for your use case

### 📖 Advanced Topics
- **[White Paper](../paper/)** - Academic paper (LaTeX source)
- **[Protocol Specification](./specs/PROTOCOL.md)** - Formal protocol definition
- **[Security Model](./specs/SECURITY.md)** - Byzantine and quantum threat analysis
- **[API Reference](./api/)** - Complete API documentation

## What You'll Learn

### For Beginners
If you're new to blockchain consensus, start with:
1. **[Byzantine Consensus Explained](./concepts/BYZANTINE_CONSENSUS.md)** - Understand the fundamentals
2. **[Getting Started Tutorial](./tutorials/GETTING_STARTED.md)** - Build a working example
3. **[Go SDK Guide](./sdks/GO.md)** - Simplest SDK to get started

### For Developers
If you want to integrate Lux consensus:
1. Choose your SDK: **[Go](./sdks/GO.md)**, **[Python](./sdks/PYTHON.md)**, **[Rust](./sdks/RUST.md)**, **[C](./sdks/C.md)**, or **[C++](./sdks/CPP.md)**
2. Review **[Examples](../examples/)** for your use case
3. Check **[Benchmarks](./BENCHMARKS.md)** for performance expectations

### For Researchers
If you're studying consensus protocols:
1. Read the **[White Paper](../paper/)** for formal analysis
2. Study **[Protocol Specification](./specs/PROTOCOL.md)** for implementation details
3. Explore **[Security Model](./specs/SECURITY.md)** for threat analysis

## Key Features

### ⚛️ Quantum-Resistant Security
Every block requires both:
- **BLS signature aggregation** (classical finality)
- **Lattice-based certificates** (post-quantum security)

Attack resistance: 2^160 operations (post-quantum security level)

### 🌊 Metastable Consensus
Unlike traditional consensus requiring 51% or 67% agreement, Lux uses **metastable consensus**:
- Sample k validators (typically k=21)
- Build confidence through repeated sampling
- Achieve finality through threshold voting (α, β parameters)

### 🎯 Multiple Consensus Engines
One protocol, multiple engines:
- **Wave**: Threshold voting with confidence tracking
- **Focus**: Confidence accumulation for decisions
- **Prism**: Conflict set resolution
- **Nova**: Linear chain consensus
- **Nebula**: DAG-based consensus
- **Quasar**: Post-quantum overlay

### 🚀 Performance
- **Finality**: < 1 second (all chains)
- **Throughput**: 111K - 6.7M operations/sec (SDK dependent)
- **Memory**: < 200 bytes per block
- **Network**: Leaderless, fully decentralized

## Architecture Overview

```
Lux Consensus Architecture

┌─────────────────────────────────────────────────────┐
│                    Application Layer                 │
│  (Your blockchain, DAG, or distributed system)      │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│              Consensus Interface                     │
│  Single-import API: consensus.NewChain()            │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│                Quasar Engine                         │
│  ┌──────────────┐      ┌──────────────┐            │
│  │  Nova DAG    │      │ PQ Certs     │            │
│  │  (Classical) │──────│ (Quantum)    │            │
│  └──────────────┘      └──────────────┘            │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│              Core Protocols                          │
│  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐      │
│  │ Wave   │ │ Focus  │ │ Photon │ │ Prism  │      │
│  │ Vote   │ │ Track  │ │ Sample │ │ Resolve│      │
│  └────────┘ └────────┘ └────────┘ └────────┘      │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│              Network Layer (QZMQ)                    │
│  Post-quantum transport with Kyber/Dilithium        │
└─────────────────────────────────────────────────────┘
```

## Quick Start Example

```go
package main

import (
    "context"
    "github.com/luxfi/consensus"
)

func main() {
    // Create consensus chain with defaults
    chain := consensus.NewChain(consensus.DefaultConfig())
    
    // Start the consensus engine
    ctx := context.Background()
    chain.Start(ctx)
    defer chain.Stop()
    
    // Add a block
    block := &consensus.Block{
        ID:       consensus.NewID(),
        ParentID: consensus.GenesisID,
        Height:   1,
        Payload:  []byte("Hello, Quantum Consensus!"),
    }
    
    if err := chain.Add(ctx, block); err != nil {
        panic(err)
    }
    
    // Block achieves quantum finality automatically
    // Both BLS and lattice certificates are validated
}
```

## SDK Test Coverage Matrix

| Test Category | Go | Python | Rust | C | C++ |
|--------------|----|----|------|---|-----|
| Block Addition | ✅ | ✅ | ✅ | ✅ | ✅ |
| Vote Processing | ✅ | ✅ | ✅ | ✅ | ✅ |
| Finalization | ✅ | ✅ | ✅ | ✅ | ✅ |
| Engine Types | ✅ | ✅ | ✅ | ✅ | ✅ |
| Batch Operations | ✅ | ✅ | ✅ | ✅ | ✅ |
| Concurrent Access | ✅ | ✅ | ✅ | ✅ | ✅ |
| Memory Safety | ✅ | ✅ | ✅ | ✅ | 🔶 |
| Edge Cases | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Total Coverage** | **96%** | **100%** | **100%** | **100%** | **95%** |

## Performance Summary

### Single Operation Latency

| SDK | Block Addition | Vote Processing | Finalization Check |
|-----|----------------|-----------------|-------------------|
| **Go** | 121 ns | 530 ns | 213 ns |
| **Python** | 149 ns | 128 ns | 76 ns |
| **Rust** | 611 ns | 639 ns | 660 ns |
| **C** | 8,968 ns | 46,396 ns | 320 ns |
| **C++** | ~800 ns | ~700 ns | ~200 ns |

### Batch Throughput (10,000 operations)

| SDK | Blocks/Second | Votes/Second |
|-----|---------------|--------------|
| **Go** | 8.25M | 1.89M |
| **Python** | 6.7M | 7.8M |
| **Rust** | 3.9B | 6.6B |
| **C** | 111K | 21K |
| **C++** | 9M | 8M |

**Note**: See **[BENCHMARKS.md](./BENCHMARKS.md)** for detailed analysis and interpretation.

## Community & Support

- **GitHub**: [luxfi/consensus](https://github.com/luxfi/consensus)
- **Documentation**: [docs.lux.network](https://docs.lux.network)
- **Discord**: [Lux Community](https://discord.gg/lux)
- **Twitter**: [@luxblockchain](https://twitter.com/luxblockchain)

## Contributing

We welcome contributions! See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.

## License

BSD 3-Clause - Free for academic and commercial use. See [LICENSE](../LICENSE).

---

**Ready to build quantum-resistant consensus systems?** Start with the **[Getting Started Tutorial](./tutorials/GETTING_STARTED.md)**!
