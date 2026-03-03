# Quasar Consensus — Measured Benchmarks

**Hardware**: Apple M1 Max (10 cores, darwin/arm64)
**Date**: 2026-04-13
**Build**: Go 1.26.1, CPU path only (no `-tags accel` / no CGO GPU)

All numbers below are raw `go test -bench` output. Reproduce with the commands
at the end of each section.

## 1. BLS12-381 (crypto/bls)

```
pkg: github.com/luxfi/crypto/bls
BenchmarkNewSecretKey-10           1000000       1056 ns/op
BenchmarkSign-10                      3592     350089 ns/op
BenchmarkVerify-10                    1563     819843 ns/op
BenchmarkAggregatePublicKeys-10       2719     481776 ns/op
BenchmarkAggregateSignatures-10       2306     542316 ns/op
```

Reproduce:
```
cd ~/work/lux/crypto/bls && go test -bench=. -benchtime=1s -run=^$
```

## 2. ML-DSA-65 (crypto/mldsa)

```
pkg: github.com/luxfi/crypto/mldsa
BenchmarkMLDSA_Sign-10             5208    494486 ns/op
BenchmarkMLDSA_Verify-10          14754    180723 ns/op
BenchmarkMLDSA_KeyGeneration-10   10000    202173 ns/op
```

Reproduce:
```
cd ~/work/lux/crypto && go test -bench=. -benchtime=2s -run=^$ ./mldsa/...
```

## 3. UTXO Fx Plugins (with LRU verify cache)

```
pkg: github.com/luxfi/utxo/mldsafx
BenchmarkMLDSA65Verify-10          9712    253761 ns/op
BenchmarkMLDSA65VerifyCached-10  880334      2963 ns/op

pkg: github.com/luxfi/utxo/slhdsafx
BenchmarkSLH192fVerify-10          2762   1916073 ns/op
BenchmarkSLH192fVerifyCached-10   32167    131072 ns/op

pkg: github.com/luxfi/utxo/ed25519fx
BenchmarkEd25519Verify-10         10000    205551 ns/op
BenchmarkEd25519VerifyCached-10 2289220      1144 ns/op

pkg: github.com/luxfi/utxo/secp256r1fx
BenchmarkP256Verify-10            21979    121321 ns/op
BenchmarkP256VerifyCached-10    2347353      1032 ns/op

pkg: github.com/luxfi/utxo/secp256k1fx
BenchmarkSecp256k1Verify-10     4237567       658.1 ns/op
```

Reproduce:
```
cd ~/work/lux/utxo && go test -bench=. -benchtime=2s -run=^$ \
  ./mldsafx/... ./slhdsafx/... ./ed25519fx/... ./secp256r1fx/... ./secp256k1fx/...
```

## 4. ML-DSA EVM Precompile

```
pkg: github.com/luxfi/precompile/mldsa
BenchmarkMLDSAVerify_SmallMessage-10           10000   247749 ns/op
BenchmarkMLDSAVerify_LargeMessage-10            8485   274320 ns/op
BenchmarkMLDSAVerify_AllModes/ML-DSA-44-10     15764   140178 ns/op
BenchmarkMLDSAVerify_AllModes/ML-DSA-65-10     10000   250112 ns/op
BenchmarkMLDSAVerify_AllModes/ML-DSA-87-10      6062   419649 ns/op
```

Reproduce:
```
cd ~/work/lux/precompile && go test -bench=. -benchtime=2s -run=^$ ./mldsa/...
```

## 5. Quasar Protocol (consensus/protocol/quasar)

```
pkg: github.com/luxfi/consensus/protocol/quasar
BenchmarkQuasarSigVariableMessageSize/verify_msg_512_bytes-10    1172  1017823 ns/op
BenchmarkQuasarSigVariableMessageSize/sign_msg_1024_bytes-10     1460   808500 ns/op
BenchmarkQuasarSigVariableMessageSize/verify_msg_1024_bytes-10    949  1089206 ns/op
BenchmarkBLSAggregation/4_signatures-10                          5877   209808 ns/op
BenchmarkBLSAggregation/8_signatures-10                          2966   409945 ns/op
BenchmarkBLSAggregation/16_signatures-10                         1406   832009 ns/op
BenchmarkBLSAggregation/32_signatures-10                          724  1656841 ns/op
BenchmarkBLSAggregation/64_signatures-10                          369  3288973 ns/op
BenchmarkBLSAggregation/100_signatures-10                         232  5297844 ns/op
BenchmarkBLSAggregatedVerification/4_signers-10                  1579   820998 ns/op
BenchmarkBLSAggregatedVerification/8_signers-10                  1294   815583 ns/op
BenchmarkBLSAggregatedVerification/16_signers-10                 1509   876531 ns/op
BenchmarkBLSAggregatedVerification/32_signers-10                 1506   832595 ns/op
BenchmarkBLSAggregatedVerification/64_signers-10                 1390   776815 ns/op
BenchmarkBLSAggregatedVerification/100_signers-10                1609   875227 ns/op
BenchmarkQuasarBlockProcessing-10                                 674  1849772 ns/op
BenchmarkQuasarQuantumHash-10                                 2884668      434.7 ns/op
BenchmarkQuasarValidatorAddition-10                              3861   327629 ns/op
```

Reproduce:
```
cd ~/work/lux/consensus && GOWORK=off go test \
  -bench='^BenchmarkQuasar|^BenchmarkBLS' \
  -benchtime=1s -run=^$ -timeout=120s ./protocol/quasar/
```

## 6. GPU Primitives (Metal, CPU fallback for small N)

```
pkg: github.com/luxfi/gpu (Metal backend)
BenchmarkMatMul-10     3151   399456 ns/op   20027.26 MB/s
BenchmarkAdd-10        4346   335961 ns/op     238.12 MB/s
  Backend: metal, Device: Apple M1 Max (metal)

pkg: github.com/luxfi/accel/ops/zk (CPU fallback, N=8)
BenchmarkNTT_N8-10        5022608      460.9 ns/op
BenchmarkPolyMul_N8-10    1606084     1546   ns/op
BenchmarkFieldMul-10      1000000     2248   ns/op

pkg: github.com/luxfi/accel (no-CGO default path)
BenchmarkNoCGOPath/DefaultSession_Stub-10  516712508    2.446 ns/op
BenchmarkNoCGOPath/Available_Check-10       44443218   27.03  ns/op
BenchmarkNoCGOPath/Backends_List-10         40341445   29.89  ns/op
```

Note: `accel.BLSBatchVerifyThreshold = 64`. Below 64 sigs, CPU is faster due
to ~100 μs Metal dispatch overhead. Above 64, GPU batch verify benefits.

Reproduce:
```
cd ~/work/lux/gpu && go test -bench=. -benchtime=1s -run=^$ .
cd ~/work/lux/accel && go test -bench=. -benchtime=2s -run=^$ ./ops/zk/...
cd ~/work/lux/accel && go test -bench=. -benchtime=1s -run=^$ .
```

## 7. EVM (evmgpu core, CPU only)

```
pkg: github.com/luxfi/evmgpu/core
BenchmarkInsertChain_empty_memdb-10      8019    171136 ns/op  163423 B/op  602 allocs/op
BenchmarkInsertChain_valueTx_memdb-10    5487    245878 ns/op   75289 B/op  881 allocs/op
```

Translated: 5844 empty blocks/sec; 4067 value-tx blocks/sec.

Reproduce (working benches only):
```
cd ~/work/lux/evmgpu && go test \
  -bench='BenchmarkInsertChain_empty_memdb|BenchmarkInsertChain_valueTx_memdb' \
  -benchtime=1s -run=^$ -timeout=120s ./core/
```

ring-call benchmarks currently crash with a pre-existing nil-pointer in
`core/types.Header.Hash` at `core/bench_test.go:306` — unrelated to consensus.

## 8. Derived QuasarCert Numbers

**Per-cert production (n=21 validators, 1 cert/epoch):**

| Component | Ops | CPU Cost |
|-----------|-----|----------|
| Per-validator BLS sign | 21 × 350 us | 7.35 ms total (parallel: 350 us wall) |
| BLS aggregation | 1 × (21 × 53 us) | 1.11 ms |
| Per-validator ML-DSA sign | 21 × 495 us | 10.4 ms total (parallel: 495 us wall) |
| Z-Chain Groth16 prover | 1 × ~400 ms | 400 ms CPU / 5-15 ms GPU (est) |
| Ringtail round | 1 × variable | ~ms range |
| **Total wall-clock** | | **~400-500 ms CPU** (Groth16-dominated) |

**Per-cert verification (light client):**

| Component | Cost |
|-----------|------|
| BLS aggregated verify (100 signers) | 875 us (constant) |
| Groth16 verify (3 pairings on BLS12-381) | ~1-3 ms CPU, ~200-500 us GPU |
| Ringtail verify | variable |
| **Total** | **~2-5 ms CPU** |

This supersedes the `357 us` claim in older papers. Stale sections to
correct:

- `papers/lp-105-quasar-consensus/sections/05-chain-separation-for-threshold-cryptogra.tex:92`
- `papers/lp-105-quasar-consensus/sections/07-security-analysis.tex:264`
- `papers/lux-triple-proof-consensus/sections/*`
- `papers/lux-quasar-consensus/sections/*`
- `papers/lux-performance-security-tradeoffs/sections/*`
- `papers/lux-master-security-model/sections/*`

See `~/work/lux/proofs/quasar-cert-soundness.tex` Appendix B for the ML-DSA-65
Groth16 circuit analysis (~2^22.5 R1CS constraints per verification).
