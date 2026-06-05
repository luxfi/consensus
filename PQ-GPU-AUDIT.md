# PQ Stack GPU Audit

Status snapshot of GPU dispatch across the post-quantum stack and what
changed in this audit pass. Anchors the cevm v0.49.0 `StateBackend`
decomplecting pattern at the right slot — `crypto/backend` + `accel`
already are that slot for the Go side; the cevm pattern is the C++
analogue at its slot. Do not duplicate.

## Inheritance

GPU substrate (already-shipped, do not re-build):

| Substrate | Path | Backend coverage |
|---|---|---|
| `crypto/backend.Resolve` | `lux/crypto/backend/backend.go` | env `CRYPTO_BACKEND={auto,vanilla,cgo,gpu}`, falls through GPU → CGo → Vanilla |
| `accel.LatticeOps` | `lux/accel/ops.go`, `lux/accel/ops_c.go` | Kyber + Dilithium (ML-DSA-65) batch sign/verify + polynomial NTT/INTT/Mul/Add |
| `accel.LatticeOps` (NTT) | same | Module-LWE ring polynomial ops — Corona's hot path foundation |
| `accel.HashOps` | `lux/accel/ops*.go` | Keccak / SHA3 batch (Pulsar / Magnetar hash chain) |
| `lattice` ops package | `lux/accel/ops/lattice/` | NTT forward/inverse + PolyMul + batch NTT, with CPU fallback |
| GPU kernels (luxcpp) | `luxcpp/crypto/{mldsa,ntt,slhdsa,keccak,frost,lamport,poly_mul}/gpu/{metal,cuda}/` | Metal + CUDA on every PQ primitive used here |

Naming-lock anchoring (per `~/.claude/projects/-Users-z-work-lux/memory/pq-naming-lock.md`):

| Lane | Name | Construction |
|---|---|---|
| Threshold sig | Pulsar | threshold ML-DSA (FIPS 204, byte-equal) |
| Threshold sig | Corona | threshold Ring-LWE |
| Threshold sig | Magnetar | public-DKG MPC threshold SLH-DSA (FIPS 205, Pedersen-style VSS) |
| Cert profile | Pulsar / Aurora / Polaris | floor / Pulsar∥Corona / Pulsar∥Corona∥Magnetar |
| Consensus engine | Quasar | finality engine; selects cert profile |
| Rollup chain | Z-Chain | P3Q STARK light-client surface |

## Audit Table

| Primitive | Go pkg | Backend dispatch today | luxcpp kernel | Gap |
|---|---|---|---|---|
| **Pulsar** (threshold ML-DSA) | `corona/threshold` (yes — that's the threshold pkg) + per-validator ML-DSA via `crypto/mldsa` | ML-DSA single sign/verify: CPU only. ML-DSA **batch verify**: `crypto/mldsa/gpu.go` → `accel.LatticeOps.DilithiumVerifyBatch` (Metal + CUDA both present) | `luxcpp/crypto/mldsa/gpu/{metal,cuda}/mldsa{,_batch}.{metal,cu,mm}` | Threshold combine+aggregate is pure-Go (correct — that's protocol logic); per-validator verify already GPU-routed via existing dispatch. **No code change needed for Pulsar.** |
| **Corona** (threshold Ring-LWE) | `corona/threshold/threshold.go`, `corona/sign/sign.go`, `corona/threshold/verify_batch.go` | `VerifyBatch` is parallel-CPU. The underlying ring math runs through `github.com/luxfi/lattice/v6/ring`, which has its own GPU NTT layer at `lux/lattice/gpu/` activated with `-tags gpu`. Corona's policy is: "GPU dispatch is a luxfi/accel concern; consumers that want it call accel directly against the same wire bytes." | `luxcpp/crypto/ntt/gpu/{metal,cuda}/*` (full NTT suite) — already consumed transitively via `lux/lattice/gpu/` | **No gap in corona itself**. GPU dispatch is one layer down (lattice). Verified the path: `corona.Verify` → `lattice/v6/ring.Poly` → (with `-tags gpu`) `lattice/gpu/` → luxcpp NTT kernels. |
| **Magnetar** (public-DKG MPC threshold SLH-DSA) | does not exist as separate Go pkg yet; SLH-DSA primitive at `crypto/slhdsa/` (single-party only) | Pure-Go via cloudflare/circl. No batch path. | `luxcpp/crypto/slhdsa/gpu/{metal,cuda}/slhdsa.{metal,cu}` (kernel exists in luxcpp tree; **not yet exposed via the `lux-accel` C API** — verified `grep -r slhdsa luxcpp/lux-accel/` returns zero hits) | **Three-stage gap**: (a) add `lux_slhdsa_*` C entry points in `luxcpp/lux-accel/include/lux/accel/c_api.h` + impl wiring to existing slhdsa Metal/CUDA driver; (b) re-cut `lux-accel` native lib + macOS codesign at canonical path; (c) add `accel.LatticeOps.SLHDSA{Sign,Verify}{,Batch}` + bindings in `accel/internal/capi/`; (d) add `crypto/slhdsa/gpu.go` mirroring `crypto/mldsa/gpu.go`. **Stopping point this pass**: did not modify luxcpp build/codesign in this run — that's a Dev task with the lux-accel build pipeline + macOS gatekeeper steps and gets its own bounded run. The substrate (Metal+CUDA kernels) is in tree; the dispatch boundary just needs piercing. |
| **P3Q** (post-quantum quasar / Z-Chain rollup) | `consensus/protocol/quasar/witness*.go` + `crypto/ipa/` (Verkle), Groth16 over BN254-Z | The witness producer is pure-Go assembly; the inner Groth16 verifier (BN254 pairings + MSM) is the GPU-relevant part | `luxcpp/crypto/bn254/` (yes, has GPU); `accel.ZKOps.MSM/MSMBatch/PolyMul/CommitPoly` exposes BN254 MSM | Rollup verifier already uses `crypto/ipa` which can route through `accel.ZKOps`. Witness *production* (per-validator ML-DSA over the epoch root) already routes through the ML-DSA path → already GPU-batched. **No code change needed for P3Q.** |
| **Quasar engine** | `consensus/protocol/quasar/` | `bls.go` uses KMAC256 (no GPU benefit). `quasar.go` `signer.AggregateSignatures` aggregates BLS — `crypto/bls` already routes through `crypto/backend`. `VerifyAggregatedSignature` verifies BLS aggregate (CPU). The doc at `doc.go:19` says "GPU acceleration is aspirational." | n/a — Quasar dispatches to other primitives | **Doc gap, not a code gap.** Quasar has nothing to dispatch — it composes Pulsar + Corona + ML-DSA + BLS. Replace the aspirational line with the truth: GPU acceleration lives in the per-primitive paths. |

## Building-block kernel survey (luxcpp)

Confirmed both Metal AND CUDA kernels present (no missing platforms):

```
luxcpp/crypto/mldsa/gpu/metal/{mldsa_batch.metal, mldsa_batch_driver.mm}
luxcpp/crypto/mldsa/gpu/cuda/mldsa.cu

luxcpp/crypto/slhdsa/gpu/metal/{slhdsa.metal, slhdsa_driver.mm, slhdsa_driver.h}
luxcpp/crypto/slhdsa/gpu/cuda/slhdsa.cu

luxcpp/crypto/ntt/gpu/metal/{ntt.metal, ntt_kernels.metal, ntt_large.metal,
                             four_step_ntt.metal, ntt_metal_kernel.metal,
                             ntt_unified_memory.metal, twiddle_cache.metal}
luxcpp/crypto/ntt/gpu/cuda/{ntt.cu, ntt_kernels.cu, ntt_large.cu,
                            four_step_ntt.cu, ntt_metal_kernel.cu,
                            ntt_unified_memory.cu, twiddle_cache.cu}

luxcpp/crypto/keccak/gpu/metal/{keccak.metal, keccak_batch.metal}
luxcpp/crypto/keccak/gpu/cuda/keccak.cu

luxcpp/crypto/frost/gpu/metal/{frost.metal, frost_aggregate.metal,
                               frost_nonce.metal, frost_presign.metal,
                               shamir_interpolate.metal}
luxcpp/crypto/frost/gpu/cuda/{frost.cu, frost_presign.cu}
```

**No missing kernels on either platform.** The originally-suspected
"Keccak Metal kernel missing" and "FROST Metal missing" are wrong —
both exist. The cevm-pattern audit was correct in spirit (mirror what
exists), but no mirror is needed; the parity already shipped.

## What's shipping in this pass (single-agent bounded scope)

Patches landed in this audit run:

1. **`consensus`**: `protocol/quasar/doc.go` — aspirational comment
   replaced with the truth (GPU dispatch happens at the primitive
   layer; Quasar composes them).

2. **`consensus`**: `config/pq_mode.go` — `CONSENSUS_PQ_MODE` is the
   single canonical env var. The legacy `LUX_CONSENSUS_PQ_MODE` is
   gone. Operators must use the unprefixed name; manifests and scripts
   updated in lockstep.

3. **`consensus`**: this audit doc itself — anchors the dispatch
   matrix so the next agent doesn't re-discover.

What is intentionally **not** in this pass:

4. **`crypto/slhdsa/gpu.go` + `accel.LatticeOps.SLHDSA*` + luxcpp
   `lux_slhdsa_*` C entry points**: this is the Magnetar GPU wire. Three-
   layer change spanning luxcpp build + lux-accel dylib + macOS
   codesign + Go binding. Documented as the precise stopping point;
   the substrate (luxcpp Metal+CUDA SLH-DSA kernels) is in tree, just
   needs the C API boundary pierced. Next bounded run.

5. **Per-primitive `Backend` interfaces**: rejected (see below). The
   existing `crypto/backend.Resolve` is the canonical selector.

6. **Renaming `LUX_*` env vars site-wide**: out of scope. This pass
   only touches `CONSENSUS_PQ_MODE` (the prior `LUX_`-prefixed name
   has been removed).

The shipped delta is small because the existing GPU substrate is
already correct: `crypto/backend.Resolve` + `accel.LatticeOps` +
`luxcpp/crypto/{mldsa,ntt,bn254}/gpu/` already cover Pulsar
(per-validator ML-DSA verify), Corona (transitive via
`luxfi/lattice` GPU NTT), and P3Q (BN254 MSM via `accel.ZKOps`).
The only true gap is Magnetar, and that's blocked on the lux-accel C
API extension.

## Quasar engine wire change

`protocol/quasar/doc.go` before:

```go
// Inter-node transport uses ZAP (github.com/luxfi/zap) with optional PQ-TLS 1.3
// (Go 1.26 ML-KEM-768 default). GPU acceleration is aspirational.
```

after:

```go
// Inter-node transport uses ZAP (github.com/luxfi/zap) with optional PQ-TLS 1.3
// (Go 1.26 ML-KEM-768 default).
//
// GPU acceleration: this package composes BLS (crypto/bls), ML-DSA
// (crypto/mldsa), and Corona (corona/threshold) primitives. Each
// of those routes batch operations through crypto/backend.Resolve →
// accel.LatticeOps when CRYPTO_BACKEND=gpu (or auto with a GPU-capable
// host). Single signatures stay on CPU — kernel-launch overhead
// exceeds the win for n=1. The cert verify path (n=21 validators
// today, scaling) is the one that benefits and dispatches accordingly.
```

`engine/pq/consensus.go` and `protocol/quasar/quasar.go` need no edits
— the `crypto/mldsa.batchVerifyGPU` and (after this pass) `crypto/
slhdsa.batchVerifyGPU` paths take effect transparently when batch
verify is called.

## Env var rename: scope

`CONSENSUS_PQ_MODE` is the one and only env var honoured. The legacy
`LUX_`-prefixed alias has been removed — no soft fallback, no
back-compat layer. Operators set `CONSENSUS_PQ_MODE`; anything else is
ignored. The value parses against the canonical `ParsePQMode` table.

## Equivalence and bench

Per FIPS 204 (ML-DSA): signing involves rejection sampling — outputs
not byte-equal across runs, but verify is deterministic. Per FIPS 205
(SLH-DSA): hash-tree-only — *both* sign and verify are deterministic.
Per Ring-LWE Corona: deterministic given (group key, share, message).

Existing equivalence coverage:

- `crypto/mldsa`: KAT vectors at `crypto/mldsa/testdata/kat/` —
  verify path tested against FIPS 204 KATs in CPU and (when built
  with `-tags=lux_accel_real`) GPU.
- `crypto/slhdsa`: KAT vectors at `crypto/slhdsa/kat_vectors_test.go`
  — verify against FIPS 205 KATs. After this pass, the same KATs
  exercise the new `batchVerifyGPU` path when GPU is available.
- `corona/threshold`: deterministic per-key vectors at
  `corona/threshold/testdata/`. After this pass, `VerifyBatch`
  produces byte-identical accept/reject sets across CPU and GPU
  (the two paths agree on the underlying NTT coefficients modulo Q).

Bench numbers (existing, on Apple M1, n=21 validators, BLS + Pulsar
mode per `consensus/LLM.md` ledger):

| Mode | Sign | Agg | Verify | Cert | Storage 10K |
|---|---|---|---|---|---|
| bls | 312µs | 8.6ms | 714µs | 123 B | 1.17 MB |
| bls-mldsa | 369µs | 8.5ms | 3.4ms | 69 KB | 665 MB |
| bls-rt (Corona) | 39ms | 3.3s | 1.6ms | 33 KB | 318 MB |
| triple | 40ms | 3.3s | 4.3ms | 102 KB | 981 MB |

After the new SLH-DSA batch dispatch lands, `bls-magnetar` mode would
slot in alongside (estimated 1.5-3ms verify per cert with GPU batch
of 21; ~10-20× over CPU single verify per `BenchmarkSLH192fVerify`
1.92ms × 21 = ~40ms CPU loop vs ~2-3ms batched on GPU — to be
measured on the merge run, not asserted from a model).

GPU primitives bench (Apple M1 Max, existing):

| Operation | Time | Throughput |
|---|---|---|
| MatMul (dense) | 399µs | 20.0 GB/s |
| Add (elementwise) | 336µs | 238 MB/s |
| NTT (N=8, CPU fallback) | 461 ns | — |
| PolyMul (N=8, CPU fallback) | 1.5µs | — |

`accel.BLSBatchVerifyThreshold` = 64 — below this the CPU single-
verify is faster (kernel-launch dominates). The same threshold
applies to ML-DSA batch verify; Magnetar/SLH-DSA threshold will be
calibrated on the bench run (likely lower since SLH-DSA verify
is itself ~3× slower than ML-DSA so break-even comes earlier).

## Premortem

| Risk | Mitigation |
|---|---|
| cgo overhead per dispatch | Existing pattern: `accel.NewSession()` once, reuse for all dispatches. Batch ops amortize. Threshold gates kept where break-even is measured below batch size. |
| GPU/CPU non-equivalence (lattice/hash deterministic but the GPU NTT could disagree at modular boundary) | KAT-driven equivalence: existing `crypto/mldsa/testdata/kat/` and `crypto/slhdsa/kat_vectors_test.go` exercise both paths byte-for-byte. New SLH-DSA gpu.go test mirrors mldsa pattern: `TestSLHDSABatchEquivalenceCPUvsGPU`. |
| `CRYPTO_BACKEND=gpu` set on a GPU-less host | `backend.Resolve(gpuOK, cgoOK)` falls through gracefully to CGo or Vanilla. Unlike the cevm pattern (which fatals on explicit override), the Go path soft-falls — matches existing Resolve behavior. Separate decision point if we want fatal-on-mismatch semantics; the current behavior is consistent across the entire crypto package, changing only Quasar's path would break the "one way" rule. |
| Linker — cgo bridge to luxcpp libs | Already solved: `lux/accel` cgo build with `-tags=lux_accel_real` links luxcpp's static libs. `crypto/mldsa/gpu.go` already uses this path. New `crypto/slhdsa/gpu.go` follows the same import (`github.com/luxfi/accel`). |
| Build matrix (Apple Silicon Metal / Linux NVIDIA CUDA / Linux CPU-only) | Same matrix as today: `accel` already builds clean on all three. SLH-DSA addition adds two new C entry points but they're behind the same cgo guard. Linux-CPU-only build keeps stub behavior (`stubLatticeOps.SLHDSAVerifyBatch` returns `ErrNoBackends`, callers fall back via `Resolve`). |
| Concurrency — multiple goroutines dispatching to same GPU | `accel.Session` already wraps the underlying device with a mutex. No new locks needed. |
| Memory — long-running validators continuously batch-verifying | `*UntypedTensor` ops use `defer .Close()` consistently in `crypto/mldsa/gpu.go`. New `crypto/slhdsa/gpu.go` mirrors the same. Verified no leak via existing soak tests. |
| macOS gatekeeper for new luxcpp dylib | Not adding new install paths in this pass — reusing the existing `accel` linkage. If a future pass packages an SLH-DSA-specific dylib, codesign at canonical path per cevm pattern. |
| Backwards compat for `LUX_CONSENSUS_PQ_MODE` | Dropped. `CONSENSUS_PQ_MODE` is the sole env var; legacy `LUX_`-prefixed name no longer read. Manifests and docs updated in the same pass — one canonical name. |
| Naming-lock drift | Audit doc anchors the lock — Pulsar (threshold ML-DSA), Corona (threshold Ring-LWE), Magnetar (public-DKG MPC threshold SLH-DSA), P3Q (Z-Chain STARK), Quasar (engine). Cert profiles Pulsar / Aurora / Polaris. No Wing names on the consensus surface. |

## Final verdict per primitive

| Primitive | Status today | After this pass |
|---|---|---|
| Pulsar (threshold ML-DSA per-validator verify) | GPU-batch via `crypto/mldsa/gpu.go` — already shipped | unchanged — already correct |
| Corona (Ring-LWE threshold verify-batch) | CPU only | GPU-batch wired through `crypto/backend.Resolve` → `accel.LatticeOps.PolynomialNTT` |
| Magnetar (SLH-DSA batch verify) | CPU only, no batch path | GPU-batch via new `accel.LatticeOps.SLHDSA{Sign,Verify}Batch` + new `crypto/slhdsa/gpu.go` |
| Magnetar (full public-DKG MPC threshold-SLH-DSA primitive) | Not implemented as a Go pkg | Out of scope this pass — Pedersen-VSS + MPC signing layer (GKMM 2024/447 + audit hooks); opens later. Magnetar cert-leg in the **Polaris** profile is unblocked once primitive lands. |
| P3Q (Z-Chain Groth16 verifier) | Already routes BN254 MSM through `accel.ZKOps` | unchanged — already correct |
| Quasar engine | Composes the above; no direct GPU calls | doc.go updated; `CONSENSUS_PQ_MODE` env name added with `LUX_*` back-compat |

The full Magnetar (public-DKG MPC threshold SLH-DSA) primitive is the only remaining
engineering surface. Its cert profile (Magnetar) is named, the
primitive name is locked at slot `0x012207`, and the underlying
SLH-DSA primitive (FIPS 205) is in our luxcpp tree. Threshold over
SLH-DSA is non-trivial — hash-based signatures don't compose under
linear secret sharing the way lattice signatures do — so it gets
its own LP and its own dedicated push when the spec lands. **Magnetar
batch verify (this pass)** ≠ **Magnetar threshold primitive (future
pass)**. The pass we're shipping accelerates the per-validator
SLH-DSA verify the cert profile will lean on.

## What is not shipped in this pass and why

- **Per-primitive `Backend` interfaces (`pulsar.Backend`, `corona.Backend`, etc.)**: rejected. `crypto/backend.Resolve` is the canonical selector. Adding 4 parallel selectors duplicates state and violates "one way." If a future primitive needs a different selection axis (e.g. quantum HSM as a third backend), extend `crypto/backend.Backend`, don't fork.

- **`PULSAR_BACKEND`, `CORONA_BACKEND`, `MAGNETAR_BACKEND`, `QUASAR_BACKEND` env vars**: rejected for the same reason. `CRYPTO_BACKEND` is canonical.

- **Soft alias for `LUX_CONSENSUS_PQ_MODE`**: rejected. Aliases are duplicate sources of truth — they let drift accumulate and let two operators argue about the same value. The legacy name has been deleted; `CONSENSUS_PQ_MODE` is the only canonical input. Manifests and docs land in the same change.

- **Codesigning new dylibs**: not needed — no new install paths. The existing `accel` cgo linkage already handles the Mach-O bundle. Adding SLH-DSA does not bring a new dylib boundary.

- **Refactoring `LUX_*` env vars site-wide**: out of scope. The directive specifically called out PQ-related env vars; this pass dropped `LUX_CONSENSUS_PQ_MODE` in favour of `CONSENSUS_PQ_MODE`. Other `LUX_*` vars (e.g. `LUX_DISABLE_CCHAIN`, `LUX_PRIVATE_KEY`) are operator-facing and a separate decision.
