# Lux Consensus -- Agent Knowledge Base

**Last Updated**: 2026-04-12
**Repository**: github.com/luxfi/consensus
**Latest Tag**: v1.22.84
**Go**: 1.26.1

## Quasar Family of Consensus

The consensus system provides two modes (linear and DAG) with optional
post-quantum finality. All sub-protocols live in `protocol/`.

### Sub-Protocols (protocol/)

| Package | Role | Key Types |
|---------|------|-----------|
| `photon` | K-of-N committee selection, luminance tracking | `Emitter`, `Luminance` |
| `wave` | Threshold voting + FPC | `Wave[T]`, `Config`, `Photon[T]` |
| `wave/fpc` | Fast Probabilistic Consensus selector | `Selector` |
| `focus` | Beta consecutive successes counter | `Tracker[ID]`, `Confidence[ID]`, `WindowedConfidence[ID]` |
| `prism` | DAG geometry: cuts, frontiers, uniform sampling | `Cut[T]`, `Engine`, `Proposal` |
| `horizon` | DAG reachability, LCA, transitive closure, skip lists | `TransitiveClosure`, `LowestCommonAncestor`, `SkipList` |
| `flare` | DAG cert/skip via 2f+1 quorum | `Flare`, `HasCertificate`, `HasSkip` |
| `ray` | Linear chain finality driver | `Driver[T]`, `Source[T]`, `Sink[T]` |
| `field` | DAG finality driver with safe-prefix commit | `Driver[V]`, `Store[V]`, `Proposer[V]`, `Committer[V]` |
| `nova` | Linear chain mode (wraps ray) | `Nova[T]` |
| `nebula` | DAG mode (wraps field) | `Nebula[V]` |
| `chain` | Block interface primitives | `Block`, `ChainState` |
| `quasar` | BLS + Ringtail + ML-DSA threshold signing | `signer`, `BLS`, `EpochManager`, `BundleSigner` |

### Consensus Flow

**Linear (Nova)**: Photon -> Wave -> Focus -> Ray -> Sink

**DAG (Nebula)**: Photon -> Wave (per frontier vertex) -> Flare (cert/skip) -> Horizon (safe prefix) -> Field (commit) -> Committer

### Quasar Certificate

Validators sign with BLS, ML-DSA, and Ringtail. Z-Chain generates a single
Groth16 SNARK proving all three are valid (~192 bytes).

| Layer | Scheme | Hardness | Size |
|-------|--------|----------|------|
| BLS | BLS12-381 aggregate | ECDL | 48 bytes |
| ML-DSA | ML-DSA-65 (FIPS 204) | Module-LWE + SIS | ~3.3KB raw |
| Ringtail | Ring-LWE threshold | Module-LWE | variable |
| **ZKP** | **Groth16 / BN254** | — | **~192 bytes** |

Certificate = BLS aggregate (48 B) + Z-Chain ZKP (~192 B) = ~240 bytes total.
Compression: N * (48 + 3309 + RT) → 240 bytes regardless of validator count.

Modes (each layer independently toggleable):
- BLS-only: classical fast path
- BLS + Ringtail: dual PQ
- BLS + Ringtail + ML-DSA: full Quasar (`TripleSignRound1`)
- Full Quasar + Z-Chain ZKP: production mode (succinct certificate)

`IsTripleMode()` checks all three signing layers.
Crypto: `luxfi/crypto/bls`, `luxfi/crypto/mldsa`, `luxfi/ringtail/threshold`.

### Transport

Inter-node: ZAP (`github.com/luxfi/zap`), NOT p2p or gRPC/protobuf.

## Package Layout

```
consensus.go          Root facade, type aliases, NewChain/NewDAG/NewPQ
config/               Parameter presets (single, local, testnet, mainnet)
core/                 Core interfaces, dag structures
  dag/                DAG store, event horizon, ordering
engine/               Consensus engines (Chain, DAG, PQ)
  chain/              Linear chain engine
  dag/                DAG engine
  pq/                 Post-quantum engine
  interfaces/         State enum (Unknown..Stopped)
protocol/             All Quasar sub-protocols (see table above)
types/                Block, Vote, Config, Decision, bag/
runtime/              VM wiring (chain IDs, validators)
pkg/wire/             Wire credentials (ML-DSA-44/65/87, BLS, Ed25519)
bench/                Benchmarks (ZAP throughput, Lux vs Avalanche)
version/              Re-exports github.com/luxfi/version
```

## Performance (Measured)

### ZAP Wire Protocol (bench/)
| Config | Throughput |
|--------|------------|
| Single connection | 114K TPS |
| 20 parallel connections | 376K TPS |
| 50 conns + batch 1000 | 20.26M TPS |

### Protocol Microbenchmarks
| Component | Operation | Time/Op |
|-----------|-----------|---------|
| Wave | Vote round | 3.38 us |
| Photon | K-of-N selection | 3.03 us |
| Luminance | Brightness update | 72 ns |

### Lux vs Avalanche (bench/)
ZAP deserialization: 157x faster than protobuf (21 ns vs 3231 ns, zero allocs).
End-to-end throughput: 11.5M TPS (Lux) vs 246K (Avalanche).
Run: `GOWORK=off go test -v -run TestLuxVsAvalanche_EndToEnd -bench=. ./bench/`

## Key Technical Notes

### Test Status
- All tests pass except `TestQuantumBundle_ChainIntegrity` which is flaky
  (Ringtail threshold signing nondeterminism -- passes on retry)
- Build: `GOWORK=off go build ./...`
- Tests: `GOWORK=off go test -count=1 -short -timeout 300s ./...`

### SDK Status (Honest Assessment)
- **Go**: Production-ready (protocol/, engine/, core/)
- **Python** (`pkg/python/`): Only complete non-Go SDK with real consensus logic
- **C** (`pkg/c/`): Data structures only, not real consensus
- **Rust** (`pkg/rust/`): FFI wrapper around C, not native
- **C++** (`pkg/cpp/`): Stub

### Dependencies (Critical)
- `github.com/luxfi/crypto` -- BLS, ML-DSA, threshold signing
- `github.com/luxfi/ringtail` -- Ring-LWE threshold signatures
- `github.com/luxfi/zap` -- Zero-copy wire protocol
- `github.com/luxfi/ids` -- ID types
- `github.com/luxfi/version` -- Version management

### Bag Package
Canonical location: `types/bag`. All repos should import from here.

### Version
Managed via `github.com/luxfi/version` (re-exported in `version/`).
Do not hardcode version strings in this repo.

## Rules

1. ALWAYS use `GOWORK=off` for go commands in this repo
2. NEVER bump packages above v1.x.x
3. NEVER use go-ethereum or ava-labs packages -- use luxfi
4. Update THIS file (LLM.md) with significant discoveries
5. CLAUDE.md and AGENTS.md are symlinks to LLM.md -- do not commit them
6. Show tests passing, do not just claim "done"
