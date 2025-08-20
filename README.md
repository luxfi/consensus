# Lux Consensus

[![CI Status](https://github.com/luxfi/consensus/actions/workflows/ci.yml/badge.svg)](https://github.com/luxfi/consensus/actions)
[![Coverage](https://img.shields.io/badge/coverage-96%25-brightgreen)](https://github.com/luxfi/consensus)
[![Go Version](https://img.shields.io/badge/go-1.24.5-blue)](https://go.dev)

## Lux Quasar: Post Quantum Consensus Engine with Photonic Selection

Quasar upgrades traditional consensus mechanisms with a Quantum Finality
engine. Quasar combines traditional BLS signature aggregation with parallel
lattice encryption to deliver **2-round finality** with both classical and 
quantum security. Every chain in the Lux primary network - Q, C, X - run Quasar, 
reaching Quantum Finality, in < 1 second.

## Why Quasar?

Traditional consensus engines have limitations:
- Different engines for different chain types
- No native post-quantum security
- Complex multi-layer architecture

Quasar solves all these with **"One engine to rule them all"**:
- **Unified Protocol**: Same engine for DAG, linear, EVM, MPC chains
- **Quantum Finality**: Every block requires post quantum certificates
- **2-Round Total**: BLS Signatures (1 round) + Lattice Signatures (2 phases) = quantum finality
- **Zero Leaders**: Fully decentralized, leaderless, highly secure
- **Sub-Second Performance**: <1s finality with quantum security

## 🚀 Recent Updates (December 2024)

### Photon/Emitter Refactoring Complete ✅
- Replaced `Sampler/Sample` pattern with light-themed `Emitter/Emit`
- Implemented luminance tracking (10-1000 lux range) for node selection
- Performance-based weighting adjusts selection probability
- **96%+ test coverage maintained**
- **CI fully green** with all lint checks passing

## Quick Start

### Installation
```bash
go get github.com/luxfi/consensus
```

### Basic Usage
```go
import (
    "github.com/luxfi/consensus/photon"
    "github.com/luxfi/consensus/core/wave"
    "github.com/luxfi/consensus/config"
)

// Create photon emitter for peer selection
emitter := photon.NewUniformEmitter(peers, photon.DefaultEmitterOptions())

// Initialize wave consensus
cfg := config.DefaultParams()
engine := wave.New(cfg, emitter, transport)

// Start consensus
engine.Start(ctx, blockID)
```

### Running Tests
```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run benchmarks
go test -bench=. ./...
```

## Architecture

### Core Consensus Components

```text
consensus/
├── photon/                # 🌟 K-of-N committee selection (NEW)
│   ├── emitter.go        # Light emission-based peer selection
│   └── luminance.go      # Node brightness tracking (lux units)
│
├── core/
│   ├── wave/             # 🌊 Wave consensus mechanism
│   │   └── engine.go     # Threshold voting (α, β parameters)
│   ├── dag/              # 📊 DAG structure & ordering
│   │   ├── flare/        # Certificate generation
│   │   └── horizon/      # Frontier management
│   └── focus/            # 🎯 Confidence tracking
│
├── protocol/
│   ├── quasar/          # ⚛️ Post-quantum security
│   │   └── ringtail.go  # Quantum-resistant signatures
│   ├── nebula/          # ☁️ State sync protocol
│   └── nova/            # ⭐ Parallel chain support
│
├── qzmq/                # 🔐 Post-quantum transport
│   ├── session.go       # Hybrid key exchange
│   └── messages.go      # Wire protocol
│
└── engine/              # 🎮 Consensus engines
    ├── chain/          # Linear blockchain
    └── dag/            # DAG-based chains
```

## Framework for Quasar Consensus

**Quasar** is built on two core components that provide both classical and quantum finality:

### 1. Nova DAG (Classical Finality)
- **Sampling**: k-peer sampling for vote collection
- **Confidence**: Build confidence d(T) through repeated sampling
- **Thresholds**: β₁ for early preference, β₂ for decision
- **Time**: ~600-700ms to classical finality

### 2. Quasar (Quantum Finality)
- **Phase I - Propose**: Sample DAG frontier, propose highest confidence
- **Phase II - Commit**: Aggregate threshold signatures if > α𝚌 agree
- **Certificates**: Lattice-based post-quantum certificates
- **Time**: ~200-300ms additional for quantum finality

## Quantum Finality

Every block header now contains:

```go
type CertBundle struct {
    BLSAgg  []byte  // 96B BLS aggregate signature
    PQCert  []byte  // ~3KB lattice certificate
}

// Block is only final when BOTH certificates are valid
isFinal := verifyBLS(blsAgg, quorum) && verifyPQ(pqCert, quorum)
```

## Key Hierarchy

| Layer | Key Type | Purpose | Storage |
|-------|----------|---------|---------|
| Node-ID | ed25519 | P2P transport auth | `$HOME/.lux/node.key` |
| Validator-BLS | bls12-381 | Fast finality votes | `$HOME/.lux/bls.key` |
| Validator-PQ | lattice | PQ finality shares | `$HOME/.lux/rt.key` |
| Wallet (EVM) | secp256k1 or Lamport | User tx signatures | In wallet |
| Wallet (X-Chain) | secp256k1 or Dilithium | UTXO locking | In wallet |

The same `rt.key` registered on Q-Chain is reused by all chains - no extra onboarding.

## Quick Start

```go
package main

import (
  "context"
  "log"

  "github.com/luxfi/consensus/config"
  "github.com/luxfi/consensus/engine/quasar"
  "github.com/luxfi/ids"
)

func main() {
  // 1. Configure Quasar parameters
  cfg := config.Parameters{
    K:               21,  // Validators to sample
    AlphaPreference: 15,  // Preference threshold
    AlphaConfidence: 18,  // Confidence threshold
    Beta:            8,   // Finalization rounds
    QRounds:         2,   // Quantum rounds
  }

  // 2. Create Quasar engine
  nodeID := ids.GenerateNodeID()
  engine := quasar.NewQuasar(cfg, nodeID)

  // 3. Initialize with both BLS and PQ keys
  engine.Initialize(ctx, blsKey, pqKey)

  // 4. Process vertices/blocks
  engine.AddVertex(ctx, vertex)

  // 5. Dual-certificate finality
  engine.SetFinalizedCallback(func(qBlock QBlock) {
    log.Printf("Q-block %d finalized with quantum certificates\n", qBlock.Height)
  })
}
```

## Performance Metrics

### Consensus Performance
| Network | Validators | Finality | Block Time | Configuration |
|---------|-----------|----------|------------|---------------|
| **Mainnet** | 21 | 9.63s | 200ms | Production ready |
| **Testnet** | 11 | 6.3s | 100ms | Testing network |
| **Local** | 5 | 3.69s | 10ms | Development |
| **X-Chain** | 5 | 5ms | 1ms | 100Gbps networks |

### Benchmark Results (Apple M1 Max)
| Component | Operation | Time/Op | Memory | Allocations |
|-----------|-----------|---------|--------|-------------|
| **Wave Consensus** | Vote Round | 3.38μs | 2.3KB | 8 allocs |
| **Photon Emitter** | K-of-N Selection | 3.03μs | 3.0KB | 2 allocs |
| **Luminance** | Brightness Update | 72ns | 0B | 0 allocs |
| **Quasar** | Phase I | 0.33ns | 0B | 0 allocs |
| **Quasar** | Phase II | 40.7ns | 0B | 0 allocs |

Both BLS and post-quantum certificates complete within one consensus slot.

## Security Model

1. **Pre-quantum**: Attacker must corrupt ≥⅓ stake to fork
2. **Q-day (BLS broken)**: Attacker can forge BLS but not lattice
   - Block fails quantum check
   - Consensus halts rather than accepting unsafe fork
3. **Post-quantum**: Security rests on lattice SVP (2¹⁶⁰ ops)

Attack window ≤ PQ round time (≤50ms mainnet).

## Chain Integration

| Chain | Integration | Rule |
|-------|-------------|------|
| Q-Chain | Q-blocks as internal txs | All chains read Q-blocks |
| C-Chain | Every block has CertBundle | Invalid without quantum certs |
| X-Chain | Vertex metadata references Q-block | Epoch sealed by quantum cert |
| M-Chain | MPC rounds reference Q-block height | Custody requires PQ proof |

## Account-Level PQ Options

### EVM (C-Chain)
- Lamport-XMSS multisig contracts
- Gas: ~300k for VERIFY_LAMPORT
- EIP-4337 AA wrapper available

### X-Chain
- New LatticeOutput type with PQ pubkey
- Spend verifies single RT signature (~1.8KB)
- CLI: `lux-wallet generate --pq`

## Research Tools

```bash
# Interactive parameter tuning
consensus params tune

# Distributed benchmark with Quasar
consensus bench --engine quasar

# Simulate quantum state progression
consensus sim --quantum

# Analyze quantum-certificate safety
consensus check --quantum-cert
```

## Implementation Roadmap

| Priority | Task | Status |
|----------|------|--------|
| P0 | Quasar engine core (Nova + Quasar) | ✓ Complete |
| P0 | Quantum-certificate block headers | ✓ Complete |
| P0 | Quantum key generation & registration | In Progress |
| P1 | Quasar service with precompute | Planned |
| P1 | PQ account options (Lamport) | ✓ Complete |
| P1 | PQ account options (Dilithium) | Planned |

## Testing

```bash
# Unit tests with quantum scenarios
go test ./engine/quasar/...

# Benchmark dual-certificate performance
go test -bench=QuantumCert ./lattice/

# Fuzz test certificate aggregation
go test -fuzz=Certificate ./testing/
```

## Summary

Quasar is not an add-on or extra layer - it IS the consensus engine for Lux. By combining Nova DAG with Quasar PQ in a unified protocol, Quasar delivers:

- **2-round finality** with both classical and quantum security
- **Dual certificates** required for every block
- **One engine** for all chain types
- **Sub-second performance** even with PQ security

Welcome to the quantum era of consensus.

## License

BSD 3-Clause — free for academic & commercial use. See LICENSE.
# Trigger benchmark run to test gh-pages
