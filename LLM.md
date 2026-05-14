# Lux Consensus -- Agent Knowledge Base

**Repository**: github.com/luxfi/consensus
**Latest Tag**: v1.23.7
**Go**: 1.26.1

## Post-E2E-PQ State (current)

This is the `consensus-auth` worktree of `luxfi/consensus`. Source under
`config/`, `protocol/auth/`, `protocol/zchain/`, `protocol/quasar/` is
the same tree as the canonical `consensus/` checkout — every section
below describes the same package layout. Strict-PQ profile is the canon
here; classical-compat is opt-in only via
`ChainSecurityProfile.ForkClassicalCompatUnsafe`.

- `config/security_profile.go` — `ChainSecurityProfile` with 11 E2E
  fields (ProofPolicyID, ProofBackendID, ProofFormatID, VerifierID,
  HashSuiteID, SigSchemeID, IdentitySchemeID, WalletSchemeID,
  TxSchemeID, ContractSchemeID, KEMSchemeID, RecoverySchemeID).
- `config/pq_mode.go` — `PQMode` enum (`bls`, `corona`, `pulsar`,
  `quasar`, `mldsa`); `LUX_CONSENSUS_PQ_MODE` env knob. Canonical strict
  default: `PQModeQuasar` resolved against `ProofPolicySTARKFRISHA3PQ`.
- `config/validator_scheme.go` — `ValidatorSchemeID()` +
  `AcceptsValidatorScheme(presented, classicalCompatUnsafe)` cross-axis
  gate. Strict-PQ profiles refuse classical NodeIDScheme (0x90)
  regardless of operator flag.
- `protocol/auth/` — `TxAuthEnvelope`, `PQPermit`,
  `ContractAuthProfile`, SP 800-185 TupleHash256 digest.
- `protocol/zchain/` — `ZProofEnvelope`, `VerifierManifest`, per-backend
  registry (P3Q / SP1 / RISC0 / Stone / Stwo) under
  `ProofPolicySTARKFRISHA3PQ`.
- `protocol/zchain/registry/` — `PQKeyRecord`, 5 ops
  (`OpRegisterKey/Rotate/Revoke/AuthorizeSession/CommitTxAuthBatch`),
  `ZRegistryRoots` (7-root TupleHash256 → `EpochCommitment`),
  `VerifyAuthPassed` execution-hot-path verifier.
- `protocol/quasar/qblock.go` — HIP-0079 Q-Chain finality block with a
  single Pulsar-M threshold signature over the canonical transcript;
  `NetworkPolicyStrictPQ` refuses any block whose `ProofPolicyID` is
  not `ProofPolicySTARKFRISHA3PQ`, whose `HashSuiteID` is not
  `HashSuiteSHA3NIST`, or whose `FinalitySchemeID` is not Pulsar-M-65
  / Pulsar-M-87. Pulsar-M-44 is devnet-only.

### Cross-repo dependencies (this repo → others)
- `luxfi/crypto` → ML-DSA / SLH-DSA / Pulsar primitives
- `luxfi/pulsar` → R-LWE threshold (Q-Chain consensus path)
- `luxfi/pulsar-m` → M-LWE threshold ML-DSA (HIP-0079 finality)
- `luxfi/p3q` → STARK / FRI proof substrate for Z-Chain
  (`ProofBackendP3QSTARKFRISHA3`, 10 crates on crates.io)
- `luxfi/threshold` → BLS / FROST / CGGMP21 wiring (classical-compat
  only)
- `luxfi/ids` → `NodeIDScheme` wire enum
- `luxfi/zap` → wire protocol (not p2p)

---

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

### Quasar Certificate

`QuasarCert` (see `protocol/quasar/types.go`) carries up to three witness
slots; the canonical wire layout is fixed but the meaning of each slot
depends on the security profile.

```go
type QuasarCert struct {
    BLS        []byte    // Classical aggregate slot (refused under strict-PQ)
    Corona   []byte    // Lattice threshold slot — under strict-PQ this is Pulsar / Pulsar-M
    MLDSAProof []byte    // Z-Chain proof slot — see profile-dependent table below
    Epoch      uint64
    Finality   time.Time
    Validators int
}
```

Profile-dependent semantics of each slot:

| Slot | Strict-PQ (`LUX_STRICT_E2E_PQ`) | Classical-compat (`ForkClassicalCompatUnsafe`) |
|------|-------------------------------|------------------------------------------------|
| `BLS` | refused (`verifyBLSAggregate` returns `ErrBLSForbiddenUnderStrictPQ`); BLS12-381 is not canonical under strict-PQ | 48-byte BLS12-381 aggregate (co-CDH) for legacy fast path only |
| `Corona` | Pulsar (R-LWE threshold) for Q-Chain consensus OR Pulsar-M (M-LWE threshold, output verifies under unmodified FIPS 204 ML-DSA.Verify) for HIP-0079 Q-blocks | same as strict-PQ; threshold-lattice is the same layer either way |
| `MLDSAProof` | `ZProofEnvelope` reference (see `protocol/zchain/proof_envelope.go`) — backend-agnostic STARK proof under `ProofPolicySTARKFRISHA3PQ` (0x10), produced by an `IsProductionPQ()` backend (RISC0 succinct, SP1 compressed, P3Q-SHA3, Stone, or Stwo) | legacy `PQModeQuasar` carried a ~192-byte Groth16 rollup of N × ML-DSA-65 verifications; that path is `BLSPlusGroth16`-era and is not produced on strict-PQ chains |

The Q-Chain finality lane (HIP-0079) does not carry a per-validator
ML-DSA roll-up at all — it rides a single Pulsar-M threshold signature
over a `QBlock` transcript (see `protocol/quasar/qblock.go`). The
`MLDSAProof` slot is used only by Z-Chain to anchor an `EpochCommitment`
(7-root TupleHash256) produced by the registry; the proof system that
produces that anchor is selected by `ProofBackendID` and gated by the
profile's `ProofPolicyID`.

Crypto: `luxfi/crypto/bls` (classical-compat only), `luxfi/crypto/mldsa`,
`luxfi/pulsar/threshold` (R-LWE), `luxfi/pulsar-m` (M-LWE, FIPS 204
output-interchangeable), `luxfi/p3q` (STARK/FRI proof substrate).

### PQ Mode Selection

`config/pq_mode.go` defines the configurable PQ mode enum, selectable via
the `LUX_CONSENSUS_PQ_MODE` env var or `Parameters.PQMode` field. Modes
name a *threshold + identity* stack; the proof system that backs Z-Chain
is a separate orthogonal axis (`ProofPolicyID` + `ProofBackendID` in
`ChainSecurityProfile`).

| Mode | Wire alias | Threshold + identity stack |
|------|------------|----------------------------|
| `PQModeBLS` | `bls` | BLS aggregate only (classical fast path; refused under strict-PQ) |
| `PQModeCorona` | `corona` | BLS + Corona academic (BLAKE3); trusted-dealer DKG, federation-only |
| `PQModePulsar` | `pulsar` | BLS + Pulsar.R (SHA-3 / SP 800-185); Pedersen DKG over R_q; public-chain ready |
| `PQModeQuasar` | `quasar` | BLS + Pulsar + ML-DSA-65 with a Z-Chain rollup. Default since the Hanzo-mesh switch. **The shape of the rollup depends on the profile**: pre-HIP-0078 chains rolled N × ML-DSA into a 192-byte Groth16 proof; post-HIP-0078 strict-PQ chains pin `ProofPolicySTARKFRISHA3PQ` and roll the registry's EpochCommitment through a STARK backend (P3Q / SP1 / RISC0 / Stone / Stwo) — same Z-Chain anchor slot, post-quantum proof system. |
| `PQModeMLDSA` | `mldsa` | BLS + per-validator raw ML-DSA-65 (audit grade, no threshold, no rollup). FIPS-approvable when BLS is dropped. |

There is no `BLSPlusGroth16` constant in `config/pq_mode.go`; "Groth16"
is the parse-alias for `PQModeQuasar` ("groth16", "bls-z", "bls-zk",
"bls-groth16", "z-chain", "pulsar-z" all parse to `PQModeQuasar`),
retained for one release for back-compat with older callers. Strict-PQ
deployments resolve `PQModeQuasar` against a STARK backend, not Groth16.

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
| **Z-Chain** | Anchors a per-epoch `EpochCommitment` (7-root TupleHash256, see `protocol/zchain/registry/roots.go`). Under strict-PQ this is a STARK proof (`ProofPolicySTARKFRISHA3PQ` 0x10, produced by a P3Q / SP1 / RISC0 / Stone / Stwo backend) referenced via the `MLDSAProof` slot of QuasarCert. Pre-HIP-0078 chains carried a 192-byte Groth16 rollup of N × ML-DSA verifications in this slot; that path is not produced on strict-PQ chains. |

**Why a Z-Chain rollup slot rather than `ThresholdMLDSA`**: threshold
ML-DSA has no FIPS standard; research constructions hit a rejection-
sampling circular dependency (see
`~/work/lux/proofs/quasar-cert-soundness.tex` App. A). Quasar takes the
non-threshold path — each validator signs individually, Z-Chain rolls
the registry state into a single proof. Pre-HIP-0078 chains used
Groth16 over BLS12-381 for that proof (~192 B, pairing-based,
classical). Strict-PQ chains (post-HIP-0078) pin
`ProofPolicySTARKFRISHA3PQ` and produce the rollup with a P3Q-family
STARK backend (`luxfi/p3q@0.0.1`, 10 crates on crates.io); see
`protocol/zchain/proof_envelope.go` for the `ZProofEnvelope` wire
format and `protocol/zchain/backend_registry.go` for the dispatch into
`IsProductionPQ()` backends.

Pulsar-M (M-LWE threshold, output-interchangeable with FIPS 204
ML-DSA.Verify) is a separate option for **Q-Chain finality** itself —
see `protocol/quasar/qblock.go`, which signs the Q-block transcript with
a single Pulsar-M threshold signature rather than N raw ML-DSA sigs.

### Formal Proofs (LP-105 + Proof Sketch)

The paper + proof sketch carry the soundness/liveness/PQ-safety arguments:

- `~/work/lux/papers/lp-105-quasar-consensus.tex`:
  - §5 Chain Separation
  - §6 QuasarCert Formal Definition (Def 6.1, 6.2)
  - Thm 7.5 Soundness
  - Thm 7.6 Parallel Liveness
  - Thm 7.7 Post-Quantum Safety
- `~/work/lux/proofs/quasar-cert-soundness.tex` (pre-HIP-0078 / Groth16
  era — kept verbatim as historical context for the soundness argument;
  the constraint-count and trusted-setup appendices apply only to the
  legacy Groth16 Z-Chain rollup, NOT to the strict-PQ STARK path that
  replaces it):
  - App B — ML-DSA-65 R1CS constraint count (~2^22.5 per verification;
    per-cert amortized to ~2^20 via shared-matrix optimization for n=21
    validators) [legacy Groth16 path only]
  - App C — Static vs adaptive corruption (Fischlin / erasure hybrids)
    [applies to both eras]
  - App D — Trusted-setup ceremony (Bowe-Gabizon-Miers), PLONK upgrade
    path [legacy Groth16 path only; strict-PQ uses transparent
    FRI-based STARKs with no trusted setup]
  - App E — Pulsar parameter tightness: classical 2^142, quantum 2^130
    via BDGL sieving + Grover speedup [applies to both eras]

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
bench/                Benchmarks (ZAP throughput, Lux vs Avalanche)
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
- BLS aggregate verify: ~875 us (classical-compat profile only —
  refused under strict-PQ)
- Z-Chain proof verify: profile-dependent. Under
  `ProofPolicySTARKFRISHA3PQ` the cost is set by the backend
  (`ProofBackendID`); the verifier-manifest dispatch in
  `protocol/zchain/verifier_manifest.go` carries the per-backend gate.
  Pre-HIP-0078 Groth16 was ~1-3 ms (pairing-dominated, not in this
  repo's bench harness); STARK backends trade a larger proof for
  transparent setup and PQ soundness.
- Pulsar threshold verify: variable, amortized O(1) after DKG.
- Total: ~2-5 ms per cert classical-compat, similar order under
  strict-PQ depending on backend. GPU batch amortizes 10-100x across
  certs.

**Note on the stale 357 us claim in older papers:** The "357 us epoch
finality" from earlier Lux drafts (lux-triple-proof-consensus,
lux-master-security-model, lux-performance-security-tradeoffs) is
pre-HIP-0077 era and does not match any measured operation in the
current code. Closest real candidates: BLS single keygen (350 us),
ML-DSA-65 sign (495 us). The Z-Chain rollup figures in older papers
(192 B Groth16 proof / 400 ms CPU prover) describe the legacy
PQModeQuasar witness path and do not apply to strict-PQ chains, which
use a STARK proof system through P3Q / SP1 / RISC0 / Stone / Stwo.
Papers should quote the measured 2-5 ms CPU QuasarCert verify and name
the active `ProofPolicyID` + `ProofBackendID` explicitly.

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

### Lux vs Avalanche (bench/)
ZAP deserialization: 157x faster than protobuf (21 ns vs 3231 ns, zero allocs).
End-to-end throughput: 11.5M TPS (Lux) vs 246K (Avalanche).
Run: `GOWORK=off go test -v -run TestLuxVsAvalanche_EndToEnd -bench=. ./bench/`

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
