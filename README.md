# Lux Consensus

> Lux is not merely adding post-quantum signatures to a chain; it defines a hybrid finality architecture for DAG-native consensus, with protocol-agnostic threshold lifecycle, post-quantum threshold sealing, and cross-chain propagation of Horizon finality.

See [LP-105 §Claims and evidence](https://github.com/luxfi/lps/blob/main/LP-105-lux-stack-lexicon.md#claims-and-evidence) for the canonical claims/evidence table and the ten architectural commitments — single source of truth.

[![CI Status](https://github.com/luxfi/consensus/actions/workflows/ci.yml/badge.svg)](https://github.com/luxfi/consensus/actions)
[![Go Version](https://img.shields.io/badge/go-1.26-blue)](https://go.dev)

## Quasar Family of Consensus

Quasar is the consensus engine for the Lux network. It provides unified
consensus for linear chains (P-Chain, C-Chain) and DAG chains (X-Chain) with
post-quantum finality via three independent cryptographic signing paths.

### Sub-Protocols

| Protocol | Role | Package |
|----------|------|---------|
| **Photon** | K-of-N committee selection via Fisher-Yates sampling with luminance-weighted reputation | `protocol/photon` |
| **Wave** | Per-round threshold voting with FPC (Fast Probabilistic Consensus) | `protocol/wave` |
| **Focus** | Confidence accumulation: beta consecutive successes = local finality | `protocol/focus` |
| **Nova** | Linear chain consensus mode (P-Chain, C-Chain) -- wraps Ray | `protocol/nova` |
| **Nebula** | DAG consensus mode (X-Chain) -- wraps Field | `protocol/nebula` |
| **Prism** | DAG geometry: frontiers, cuts, uniform peer sampling | `protocol/prism` |
| **Horizon** | DAG order-theory: reachability, LCA, transitive closure, skip lists | `protocol/horizon` |
| **Flare** | DAG certificate/skip detection via 2f+1 quorum | `protocol/flare` |
| **Ray** | Linear chain finality driver: Wave + Focus + Sink | `protocol/ray` |
| **Field** | DAG finality driver: Wave + safe-prefix commit | `protocol/field` |
| **Quasar** | BLS + Pulsar + ML-DSA threshold signing, epoch management | `protocol/quasar` |

### Signing Modes

Each cryptographic layer is independently toggleable:

| Mode | Layers | Use Case |
|------|--------|----------|
| BLS-only | BLS12-381 threshold | Fastest classical consensus |
| BLS + ML-DSA | BLS + FIPS 204 ML-DSA-65 | Dual with PQ identity proof |
| BLS + Pulsar | BLS + Ring-LWE 2-round threshold | Dual with PQ threshold proof |
| BLS + Pulsar + ML-DSA | All three in parallel | Full Quasar (triple mode) |

`IsTripleMode()` returns true when all three paths are configured.
`TripleSignRound1` runs BLS + Pulsar + ML-DSA signing in parallel goroutines.

### PQ Mode Selection (Configurable)

The active mode is selectable at runtime via `config.PQMode` or the
`LUX_CONSENSUS_PQ_MODE` env var. Defined in `config/pq_mode.go`:

| Mode | Env value | Maps to wire policy |
|------|-----------|---------------------|
| `BLSOnly` | `bls` | `PolicyQuorum` |
| `BLSPlusMLDSA` | `bls-mldsa` | `PolicyPQ` |
| `BLSPlusCorona` | `bls-rt` | `PolicyPZ` |
| `BLSPlusGroth16` | `bls-z` | `PolicyQuantum` (placeholder) |
| `TripleQuantum` | `triple` | `PolicyQuantum` |

Run `bench/pq_modes_bench_test.go` for measured per-mode signing /
aggregation / verify costs and cert sizes. Storage and cert size
trade-offs:

| Mode | Sign | Agg | Verify | Cert | Storage 10K |
|------|------|-----|--------|------|-------------|
| bls | 312 µs | 8.6 ms | 714 µs | 123 B | 1.17 MB |
| bls-mldsa | 369 µs | 8.5 ms | 3.4 ms | 69 KB | 665 MB |
| bls-rt | 39 ms | 3.3 s | 1.6 ms | 33 KB | 318 MB |
| triple | 40 ms | 3.3 s | 4.3 ms | 102 KB | 981 MB |

Pulsar / triple include the full 2-round Pulsar threshold protocol
(`github.com/luxfi/pulsar/threshold` — Pulsar is Lux's variant with
DKG2 and Pulsar-SHA3 hash suite). Production parameters M=8, N=7,
LogN=8 (ring degree 256), Q=2^48-ish: classical 2^142, quantum 2^130.
Cert-size floor of ~33 KB is set by the (C, Z, Delta) ring polynomials;
`protocol/quasar/cert_size_compare_test.go` measures and verifies this.

## Architecture

```
consensus/
  protocol/
    photon/       Committee selection (Emitter + Luminance)
    wave/         Threshold voting + FPC
      fpc/        Fast Probabilistic Consensus selector
    focus/        Confidence counter (beta tracker)
    prism/        DAG cuts, frontiers, uniform sampling
    horizon/      DAG reachability, LCA, skip lists
    flare/        DAG cert/skip classification (2f+1)
    ray/          Linear chain driver (Nova uses this)
    field/        DAG finality driver (Nebula uses this)
    nova/         Linear chain consensus mode
    nebula/       DAG consensus mode
    chain/        Block interface primitives
    quasar/       BLS + Pulsar + ML-DSA threshold signing
  engine/         Consensus engine (Chain, DAG, PQ wrappers)
  core/           Core types, DAG structures
  types/          Block, Vote, Config, Bag
  config/         Parameter presets (single, local, testnet, mainnet)
  runtime/        VM wiring (chain IDs, validators, logging)
  pkg/wire/       Wire protocol credentials (ML-DSA-44/65/87 + BLS + Ed25519)
  bench/          Benchmarks
  version/        Re-exports github.com/luxfi/version
```

## Consensus Flow

### Linear Chains (Nova)

```
Photon (select committee) -> Wave (threshold vote) -> Focus (count beta) -> Ray (decide) -> Sink
```

### DAG Chains (Nebula)

```
Photon (select committee) -> Wave (threshold vote per frontier vertex)
  -> Flare (cert/skip) -> Horizon (safe prefix) -> Field (commit ordered prefix) -> Committer
```

### Quasar PQ Finality

After consensus decision, the Quasar signing layer produces threshold certificates:

1. BLS: single-round threshold share via `crypto/threshold`
2. Pulsar: 2-round Ring-LWE threshold via `luxfi/corona/threshold`
3. ML-DSA-65: single-round FIPS 204 identity signature

All three run in parallel. A block is quantum-final when all configured certificate
layers are valid.

## Key Hierarchy

| Layer | Key Type | Purpose |
|-------|----------|---------|
| Node-ID | Ed25519 | P2P transport auth |
| Validator-BLS | BLS12-381 | Classical finality votes |
| Validator-PQ | Pulsar (Ring-LWE) | PQ finality shares |
| Validator-ID | ML-DSA-65 (FIPS 204) | PQ identity attestation |
| Wallet (EVM) | secp256k1 | User transaction signatures |
| Wallet (X-Chain) | secp256k1 | UTXO locking |

## Quick Start

```go
import "github.com/luxfi/consensus"

chain := consensus.NewChain(consensus.DefaultConfig())

ctx := context.Background()
if err := chain.Start(ctx); err != nil {
    log.Fatal(err)
}
defer chain.Stop()

block := &consensus.Block{
    ID:       consensus.NewID(),
    ParentID: consensus.GenesisID,
    Height:   1,
    Payload:  []byte("Hello, Lux!"),
}
if err := chain.Add(ctx, block); err != nil {
    log.Fatal(err)
}
```

## Configuration

```go
// Auto-configure based on validator count
params := consensus.GetConfig(21) // mainnet defaults

// Or use presets
consensus.SingleValidatorParams() // 1 node, dev
consensus.LocalParams()           // 5 nodes, local
consensus.TestnetParams()         // 11 nodes, testnet
consensus.MainnetParams()         // 21 nodes, production
```

## Benchmarks

Measured on Apple M1 Max:

| Component | Operation | Time/Op |
|-----------|-----------|---------|
| Wave | Vote round | 3.38 us |
| Photon | K-of-N selection | 3.03 us |
| Luminance | Brightness update | 72 ns |

ZAP wire protocol (measured, `bench/`):

| Configuration | Throughput |
|--------------|------------|
| Single connection | 114K TPS |
| 20 parallel connections | 376K TPS |
| 50 conns + batch 1000 | 20.26M TPS |

## Testing

```bash
GOWORK=off go test ./...
GOWORK=off go test -bench=. ./bench/
```

## Security Model

1. **Pre-quantum**: Attacker must corrupt >= 1/3 stake to fork
2. **Q-day (BLS broken)**: Lattice certificates prevent unsafe finalization
3. **Post-quantum**: Security rests on Module-LWE (Pulsar) + Module-LWE/SIS (ML-DSA)

## License

BSD 3-Clause. See LICENSE.
