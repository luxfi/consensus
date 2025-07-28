# consensus

## Lux Quasar: Post Quantum Consensus Engine

Quasar is a revolutionary unified protocol that secures traditional consensus mechanisms (BFT, Snowman++) into a single quantum-resistant engine. Quasar uses lattice based post-quantum cryptography that delivers **2-round finality** with both classical and quantum security. Every chain in the Lux network - Q, C, X, M - is secured with Quasar. 

## Why Quasar?

Traditional consensus engines have limitations:
- Different engines for different chain types
- No native post-quantum security
- Complex multi-layer architecture

Quasar solves all these with **"One engine to rule them all"**:
- **Unified Protocol**: Same engine for DAG, linear, EVM, MPC chains
- **Quantum Finality**: Every block requires post quantum certificates
- **2-Round Total**: Nova (1 round) + Ringtail PQ (2 phases) = quantum finality
- **Zero Leaders**: Fully decentralized, leaderless, highly secure
- **Sub-Second Performance**: <1s finality with quantum security

## Architecture

Below is the flattened directory layout, grouping by high‑level concerns and cleanly separating core primitives, engine wiring, providers, and helpers.

```text
consensus/                  # Core photonic consensus stages
├── photon/                # Sampling (Photon)
├── wave/                  # Thresholding (Wave)
├── focus/                 # Confidence (Focus)
├── beam/                  # Linear finalizer (Beam)
├── flare/                 # DAG ordering (Flare)
└── nova/                  # DAG finalizer (Nova)

engine/                    # Full node engine layers
├── chain/                 # Photon→Wave→Beam pipeline (linear chain)
├── dag/                   # Photon→Wave→Beam→Flare→Nova pipeline (DAG chain)
└── pq/                    # Quasar dual-cert engine (post-quantum finality)

poll/                      # Photon sampling providers
quorum/                    # Wave threshold providers
confidence/                # Focus confidence providers
networking/                # P2P transport abstractions
config/                    # Parameter builders & validation
util/                      # Shared utilities (math, sets, timing)
test/                      # Integration & fuzz tests
examples/                  # User-facing sample programs
```

## Framework for Quasar Consensus

**Quasar** is built on two core components that provide both classical and quantum finality:

### 1. Nova DAG (Classical Finality)
- **Sampling**: k-peer sampling for vote collection
- **Confidence**: Build confidence d(T) through repeated sampling
- **Thresholds**: β₁ for early preference, β₂ for decision
- **Time**: ~600-700ms to classical finality

### 2. Ringtail PQ (Quantum Finality)
- **Phase I - Propose**: Sample DAG frontier, propose highest confidence
- **Phase II - Commit**: Aggregate threshold signatures if > α𝚌 agree
- **Certificates**: Lattice-based post-quantum certificates
- **Time**: ~200-300ms additional for quantum finality

## Dual-Certificate Finality

Every block header now contains:

```go
type CertBundle struct {
    BLSAgg  []byte  // 96B BLS aggregate signature
    RTCert  []byte  // ~3KB Ringtail certificate
}

// Block is only final when BOTH certificates are valid
isFinal := verifyBLS(blsAgg, quorum) && verifyRT(rtCert, quorum)
```

## Key Hierarchy

| Layer | Key Type | Purpose | Storage |
|-------|----------|---------|---------|
| Node-ID | ed25519 | P2P transport auth | `$HOME/.lux/node.key` |
| Validator-BLS | bls12-381 | Fast finality votes | `$HOME/.lux/bls.key` |
| Validator-RT | lattice | PQ finality shares | `$HOME/.lux/rt.key` |
| Wallet (EVM) | secp256k1 or Lamport | User tx signatures | In wallet |
| Wallet (X-Chain) | secp256k1 or Ringtail | UTXO locking | In wallet |

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
    RTRounds:        2,   // Ringtail rounds
  }

  // 2. Create Quasar engine
  nodeID := ids.GenerateNodeID()
  engine := quasar.NewQuasar(cfg, nodeID)

  // 3. Initialize with both BLS and RT keys
  engine.Initialize(ctx, blsKey, rtKey)

  // 4. Process vertices/blocks
  engine.AddVertex(ctx, vertex)

  // 5. Dual-certificate finality
  engine.SetFinalizedCallback(func(qBlock QBlock) {
    log.Printf("Q-block %d finalized with dual certificates\n", qBlock.Height)
  })
}
```

## Performance Parameters

| Symbol | Mainnet | Testnet | Dev-net |
|--------|---------|---------|----------|
| Validators (n) | 21 | 11 | 5 |
| Threshold (t) | 15 | 8 | 4 |
| Round delay (Δ) | 50ms | 25ms | 5ms |
| β (BLS rounds) | 6 | 5 | 4 |
| RT rounds | 2 | 2 | 2 |
| Expected latency | 400ms | 225ms | 45ms |

Both certificates complete within one consensus slot (~1s).

## Security Model

1. **Pre-quantum**: Attacker must corrupt ≥⅓ stake to fork
2. **Q-day (BLS broken)**: Attacker can forge BLS but not RT
   - Block fails dual-cert check
   - Consensus halts rather than accepting unsafe fork
3. **Post-quantum**: Security rests on lattice SVP (2¹⁶⁰ ops)

Attack window ≤ RT round time (≤50ms mainnet).

## Chain Integration

| Chain | Integration | Rule |
|-------|-------------|------|
| Q-Chain | Q-blocks as internal txs | All chains read Q-blocks |
| C-Chain | Every block has CertBundle | Invalid without dual certs |
| X-Chain | Vertex metadata references Q-block | Epoch sealed by RT cert |
| M-Chain | MPC rounds reference Q-block height | Custody requires PQ proof |

## Account-Level PQ Options

### EVM (C-Chain)
- Lamport-XMSS multisig contracts
- Gas: ~300k for VERIFY_LAMPORT
- EIP-4337 AA wrapper available

### X-Chain
- New PQTOutput type with Ringtail pubkey
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

# Analyze dual-certificate safety
consensus check --dual-cert
```

## Implementation Roadmap

| Priority | Task | Status |
|----------|------|--------|
| P0 | Quasar engine core (Nova + Ringtail) | ✓ Complete |
| P0 | Dual-certificate block headers | In Progress |
| P0 | RT key generation & registration | In Progress |
| P1 | Quasar service with precompute | Planned |
| P1 | PQ account options (Lamport/RT) | Planned |
| P2 | Fork-choice for dual-cert finality | Planned |

## Testing

```bash
# Unit tests with quantum scenarios
go test ./engine/quasar/...

# Benchmark dual-certificate performance
go test -bench=DualCert ./ringtail/

# Fuzz test certificate aggregation
go test -fuzz=Certificate ./testing/
```

## Summary

Quasar is not an add-on or extra layer - it IS the consensus engine for Lux. By combining Nova DAG with Ringtail PQ in a unified protocol, Quasar delivers:

- **2-round finality** with both classical and quantum security
- **Dual certificates** required for every block
- **One engine** for all chain types
- **Sub-second performance** even with PQ security

Welcome to the quantum era of consensus.

## License

BSD 3-Clause — free for academic & commercial use. See LICENSE.

## Acknowledgements

Quasar builds on the metastable consensus research while introducing the first production dual-certificate quantum-resistant protocol.
