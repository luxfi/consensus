# Lux Consensus -- Agent Knowledge Base

**Repository**: github.com/luxfi/consensus
**Latest Tag**: v1.22.85
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
| `quasar` | BLS + Pulsar + ML-DSA threshold signing | `signer`, `BLS`, `EpochManager`, `BundleSigner` |

### Consensus Flow

**Linear (Nova)**: Photon -> Wave -> Focus -> Ray -> Sink

**DAG (Nebula)**: Photon -> Wave (per frontier vertex) -> Flare (cert/skip) -> Horizon (safe prefix) -> Field (commit) -> Committer

### ConsensusCert envelope (CANONICAL — supersedes the Groth16 `QuasarCert` narrative below)

The finality certificate is threshold **CERTIFICATION**, not threshold signing.
One policy-gated envelope (`protocol/quasar/consensus_cert.go`,
`VerifyConsensusCert`) with per-leg evidence modes:
`EvidenceThresholdSig` (lattice O(1) — origin's `VerifyWithRealKeys`),
`EvidenceWeightedSigSet` (independent FIPS sigs + weighted quorum — the
`WeightedQuorumCert`; the **only** mode for SLH-DSA, never seed-reconstructed),
`EvidenceStarkCompressedSigSet` (P3Q, same predicate, audit-gated stub), and
`EvidenceClassicalAggregate` (BLS, classical-only, never satisfies a PQ leg).
Required legs are **policy-derived** (`CertPolicy.RequiredLegs`), never cert
bytes (the Red H3 anti-downgrade). One-sentence spec: *policy defines the
required legs; cert bytes provide per-leg evidence; each mode proves the same
weighted-quorum predicate over the same domain-separated message.* The
`MLDSAProof`/Groth16 3-tuple below is the SUPERSEDED prior design.

### Quasar Certificate (superseded — historical)

`QuasarCert` (see `protocol/quasar/types.go:40`) is a 3-tuple:

```go
type QuasarCert struct {
    BLS        []byte  // BLS12-381 aggregate, 48 bytes classical fast path
    Pulsar   []byte  // Ring-LWE threshold (PQ), O(1) after DKG
    MLDSAProof []byte  // Z-Chain Groth16 rolling up N × ML-DSA identity sigs, ~192 bytes
    Epoch      uint64
    Finality   time.Time
    Validators int
}
```

| Layer | Scheme | Hardness | Raw Size | In Cert |
|-------|--------|----------|----------|---------|
| BLS | BLS12-381 aggregate | co-CDH | 48 B | 48 B |
| ML-DSA | ML-DSA-65 (FIPS 204) | Module-LWE + MSIS | ~3309 B per validator | 192 B (Groth16) |
| Pulsar | Ring-LWE threshold | Module-LWE | O(1) after DKG | variable |

Modes (each layer independently toggleable):
- BLS-only: classical fast path
- BLS + Pulsar: dual PQ
- BLS + Pulsar + ML-DSA: full Quasar (`TripleSignRound1`)
- Full Quasar + Z-Chain ZKP: production mode (succinct certificate)

`IsTripleMode()` checks all three signing layers.
Crypto: `luxfi/crypto/bls`, `luxfi/crypto/mldsa`, `luxfi/corona/threshold`.

### PQ Mode Selection

`config/pq_mode.go` defines the configurable PQ mode enum, selectable via
the `LUX_CONSENSUS_PQ_MODE` env var or `Parameters.PQMode` field:

| Mode | Value | Description |
|------|-------|-------------|
| `BLSOnly` | bls | Classical fast path, smallest cert |
| `BLSPlusMLDSA` | bls-mldsa | BLS + per-validator ML-DSA-65 |
| `BLSPlusCorona` | bls-rt | BLS + Pulsar 2-round threshold |
| `BLSPlusGroth16` | bls-z | BLS + Z-Chain Groth16 rollup (placeholder) |
| `TripleQuantum` | triple | All three layers active |

`engine/pq.NewConsensus` resolves the mode via `config.PQModeFromEnv` and
exposes `PQMode()` getter. `bench/pq_modes_bench_test.go` covers all modes
with real signing (real BLS aggregate, real ML-DSA-65, real Pulsar
2-round threshold).

Bench (Apple M1, n=21):

| Mode | Sign | Agg | Verify | Cert | Storage 10K |
|------|------|-----|--------|------|-------------|
| bls | 312µs | 8.6ms | 714µs | 123 B | 1.17 MB |
| bls-mldsa | 369µs | 8.5ms | 3.4ms | 69 KB | 665 MB |
| bls-rt | 39ms | 3.3s | 1.6ms | 33 KB | 318 MB |

The Pulsar layer is implemented by **Pulsar** (`github.com/luxfi/pulsar/threshold`)
— Lux's variant with DKG2 (`pulsar/dkg2/`) and Pulsar-SHA3 hash suite
(`pulsar/hash/sp800_185.go`, KMAC over cSHAKE256). Pulsar params are
byte-identical to the original Pulsar: M=8, N=7, LogN=8 (ring degree
256), Q=0x1000000004A01 (48-bit NTT-friendly prime), Dbar=48, Kappa=23.

Cert-size honest accounting (production params, classical 2^142 /
quantum 2^130 security):
- Signature = (C: 1 ring.Poly) + (Z: Vector[ring.Poly] len 8) +
  (Delta: Vector[ring.Poly] len 8) = 17 polys × 256 coeffs × 8 bytes
  raw ≈ 34816 B; measured 33052 B native binary, 33221 B gob (gob
  bloat is only 1.01x — see `cert_size_compare_test.go`). Native
  encoder ships in `protocol/quasar/corona_gob.go` (replaces gob).
- 10K certs ≈ 315 MB native, 317 MB gob.
- The earlier "10 MB / 10K = 1 KB / cert" claim was a different
  parameter sweep (smaller ring + smaller Q) — would lose ~20 bits
  classical and ~15 bits quantum security. Not interchangeable with
  Pulsar production.
| triple | 40ms | 3.3s | 4.3ms | 102 KB | 981 MB |

### Chain Separation for Threshold Cryptography

**Per LP-134**: T-Chain MPC and FHE roles (originally per LP-7330) are split into M-Chain and F-Chain. T-Chain is reserved for `teleportvm` (LP-6332).

Quasar consensus lives here in `consensus/`, but the threshold-crypto ceremonies
that feed it are split across the primary network chains:

| Chain | Role |
|-------|------|
| **X-Chain** | *Verifies* already-signed UTXOs via Fx plugins (secp256k1fx, mldsafx, slhdsafx, ed25519fx, secp256r1fx...). Does not run MPC ceremonies. |
| **Q-Chain** | Runs the Pulsar 2-round threshold for *consensus only* (this repo's `protocol/quasar/` emits those rounds). Not a general MPC host. |
| **M-Chain** | (was T-Chain MPC per LP-7330; superseded by LP-134) Runs MPC ceremonies (CGGMP21, FROST, Pulsar-general) for bridge custody of external wallets. |
| **F-Chain** | (was T-Chain FHE per LP-7330; superseded by LP-134) Runs TFHE bootstrap-key generation and FHE compute (encrypted EVM). |
| **T-Chain** | Now reserved for `teleportvm` (LP-6332): unified bridge + relay + oracle. |
| **Z-Chain** | Rolls N per-validator ML-DSA identity sigs into a single 192-byte Groth16 proof per epoch (the `MLDSAProof` field). |

**Why `MLDSAProof` and not `ThresholdMLDSA`**: threshold ML-DSA has no FIPS
standard; research constructions hit a rejection-sampling circular dependency
(see `~/work/lux/proofs/quasar-cert-soundness.tex` App. A). Quasar takes the
non-threshold path — each validator signs individually, Z-Chain compresses via
Groth16 over BLS12-381.

### Formal Proofs (LP-105 + Proof Sketch)

The paper + proof sketch carry the soundness/liveness/PQ-safety arguments:

- `~/work/lux/papers/lp-105-quasar-consensus.tex`:
  - §5 Chain Separation
  - §6 QuasarCert Formal Definition (Def 6.1, 6.2)
  - Thm 7.5 Soundness
  - Thm 7.6 Parallel Liveness
  - Thm 7.7 Post-Quantum Safety
- `~/work/lux/proofs/quasar-cert-soundness.tex`:
  - App B — ML-DSA-65 R1CS constraint count (~2^22.5 per verification; per-cert
    amortized to ~2^20 via shared-matrix optimization for n=21 validators)
  - App C — Static vs adaptive corruption (Fischlin / erasure hybrids)
  - App D — Trusted-setup ceremony (Bowe-Gabizon-Miers), PLONK upgrade path
  - App E — Pulsar parameter tightness: classical 2^142, quantum 2^130 via
    BDGL sieving + Grover speedup

### Domain separation

All ML-DSA/SLH-DSA callers bind signatures to a context string per FIPS 204/205:

| Context | Used by | File |
|---------|---------|------|
| `lux-x-chain-utxo-v1` | UTXO Fx plugins | `utxo/mldsafx`, `utxo/slhdsafx` |
| `lux-evm-precompile-mldsa-v1` | EVM precompile | `precompile/mldsa/contract.go` |
| `lux-evm-precompile-slhdsa-v1` | EVM precompile | `precompile/slhdsa/contract.go` |
| `lux-reshare-v1` | Key resharing HKDF | `threshold/mpc` |
| `lux-wave-v1` | Wave voting | `consensus/protocol/wave` |

No collisions. See `crypto/mldsa.SignCtx`/`VerifySignatureCtx` (same for slhdsa).

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
bench/                Benchmarks (ZAP throughput, Lux vs prior-gen EVM)
version/              Re-exports github.com/luxfi/version
```

## Performance (Measured)

All numbers below are measured on Apple M1 Max (10 cores, darwin/arm64), CPU
path only unless noted. See `BENCHMARKS.md` for full raw output and reproduce
commands.

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
| Quasar | QuantumHash | 435 ns |
| Quasar | Validator add | 328 us |

### QuasarCert Components (measured CPU path)

Per-component CPU costs for QuasarCert production and verification:

| Operation | Time | Source |
|-----------|------|--------|
| BLS sign (single) | 350 us | `crypto/bls BenchmarkSign` |
| BLS verify (single) | 820 us | `crypto/bls BenchmarkVerify` |
| BLS aggregate 100 sigs | 5.3 ms | `protocol/quasar BenchmarkBLSAggregation/100` |
| BLS aggregated verify (100 signers) | 875 us | `protocol/quasar BenchmarkBLSAggregatedVerification/100` |
| ML-DSA-65 sign | 495 us | `crypto/mldsa BenchmarkMLDSA_Sign` |
| ML-DSA-65 verify | 181 us | `crypto/mldsa BenchmarkMLDSA_Verify` |
| ML-DSA-65 verify (via Fx) | 254 us | `utxo/mldsafx BenchmarkMLDSA65Verify` |
| ML-DSA-65 verify (cached) | 3 us | `utxo/mldsafx BenchmarkMLDSA65VerifyCached` |
| SLH-DSA-192f verify | 1.92 ms | `utxo/slhdsafx BenchmarkSLH192fVerify` |
| Quasar full block (BLS+ML-DSA+Pulsar) | 1.85 ms | `protocol/quasar BenchmarkQuasarBlockProcessing` |

**QuasarCert verify (approx CPU, single cert, n=21 validators):**
- BLS aggregate verify: ~875 us (constant in signer count)
- Groth16 proof verify: ~1-3 ms (pairing-dominated, not yet in our bench harness — see App B of proof sketch)
- Pulsar threshold verify: variable, amortized O(1) after DKG
- Total: ~2-5 ms per cert, GPU batch can amortize 10-100x across certs

**Note on the stale 357 us claim in older papers:** The "357 us epoch finality"
from earlier Lux drafts (lux-triple-proof-consensus, lux-master-security-model,
lux-performance-security-tradeoffs) does not match any measured operation in
the current code. Closest real candidates: BLS single keygen (350 us),
ML-DSA-65 sign (495 us), Groth16 proof size is 192 B but prover time is
~400 ms CPU / ~5-15 ms GPU for the full ML-DSA-65 verification circuit
(App B). Papers should quote the measured 2-5 ms CPU QuasarCert verify.

### Signature Schemes Benchmark (crypto + utxo Fx)

| Scheme | Single Verify | Cached | Ratio vs secp256k1 |
|--------|---------------|--------|-------------------|
| secp256k1 (C, native) | 658 ns | — | 1.0x |
| P-256 (Go stdlib) | 121 us | 1.0 us | 184x |
| Ed25519 (Go stdlib) | 205 us | 1.1 us | 312x |
| ML-DSA-44 | 140 us | — | 213x |
| ML-DSA-65 | 250 us | 3.0 us | 380x |
| ML-DSA-87 | 420 us | — | 638x |
| SLH-DSA-SHA2-192f | 1.92 ms | 131 us | 2912x |
| BLS (single verify) | 820 us | — | 1246x |

`CostPerSignature` values in UTXO Fx plugins are benchmarked-proportional.

### GPU Primitives (Metal, Apple M1 Max)

| Operation | Time | Throughput |
|-----------|------|------------|
| MatMul (dense) | 399 us | 20.0 GB/s |
| Add (elementwise) | 336 us | 238 MB/s |
| NTT (N=8, CPU fallback) | 461 ns | — |
| PolyMul (N=8, CPU fallback) | 1.5 us | — |
| FieldMul | 2.2 us | — |

GPU batch verify kicks in at ≥64 signatures (`accel.BLSBatchVerifyThreshold`).
Below that, the CPU single-verify path is faster due to kernel dispatch
overhead. Raw Metal dispatch is ~100 us minimum; the break-even for ML-DSA is
around 64 signatures.

### EVM (evmgpu core, CPU only)

| Operation | Time |
|-----------|------|
| InsertChain empty block | 171 us (5844 blocks/sec) |
| InsertChain value-tx block | 246 us (4067 blocks/sec) |

ring-call benchmarks (ring200, ring1000) currently hit a pre-existing
nil-pointer in `core/types.Header.Hash` at bench_test.go:306 when running full
chain-read; unrelated to consensus correctness but flagged for evmgpu repo.

### Lux vs prior-gen EVM (bench/)
ZAP deserialization: 157x faster than protobuf (21 ns vs 3231 ns, zero allocs).
End-to-end throughput: 11.5M TPS (Lux) vs 246K (prior-gen baseline).
Run: `GOWORK=off go test -v -run TestLuxVsLegacy_EndToEnd -bench=. ./bench/`

## Key Technical Notes

### Test Status
- All tests pass except `TestQuantumBundle_ChainIntegrity` which is flaky
  (Pulsar threshold signing nondeterminism -- passes on retry)
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
- `github.com/luxfi/corona` -- Ring-LWE threshold signatures
- `github.com/luxfi/zap` -- Zero-copy wire protocol
- `github.com/luxfi/ids` -- ID types
- `github.com/luxfi/version` -- Version management

### ValidatorSchemeID — cross-axis NodeIDScheme gate

`config/validator_scheme.go` (v1.23.4) owns the rule that pins which
`NodeIDScheme` byte a peer / proposer / validator / mempool sender
MUST present on this chain.

- `ChainSecurityProfile.ValidatorSchemeID()` reads through
  `IdentitySchemeID` (one byte, one source of truth). Returns
  `SigSchemeMLDSA65` (0x42) on the canonical strict-PQ profile.
- `ChainSecurityProfile.AcceptsValidatorScheme(presented, classicalCompatUnsafe)`
  is the cross-axis check the node-side `peer.SchemeGate` funnels every
  inbound NodeID through. Strict-PQ profiles refuse classical
  (0x90) regardless of the operator flag (defence-in-depth); permissive
  profiles honour the flag.
- `ErrValidatorSchemeMismatch` is the typed error consumers match
  against; mirrors `ids.ErrNodeIDSchemeMismatch` at the wire-decode layer.

The wire bytes are shared with `luxfi/ids.NodeIDScheme`. A test pins
the alignment in literal form so a renumber in either enum trips CI.

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
