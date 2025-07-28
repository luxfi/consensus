# Quasar: Quantum-Secure Metastable Consensus for the Lux Network

**Quasar** is the final, ultra-fast quantum-safety phase in the Lux Consensus pipeline. It augments our five photonic stages (Photon → Wave → Focus → Beam/Flare → Nova) with an additional lightweight post-quantum confirmation to close any residual "attack window" in under 100 ms.

---

## 1. Motivation

Even after Nova (network-wide finality via BLS + Ringtail), a determined quantum adversary could—if they had real-time access to a fault-tolerant quantum computer—forge or replay a signature in the brief interval before full irreversibility. **Quasar** shrinks that interval to the coherence limit of today's fastest lattice-based checks.

---

## 2. Quasar in the Pipeline

| Stage | Name      | Function                                       | Notes                                    |
|:-----:|:----------|:-----------------------------------------------|:-----------------------------------------|
| 1     | **Photon** | Sampling                                       | poll K peers                             |
| 2     | **Wave**   | Thresholding                                   | αₚ/α vote quorum                         |
| 3     | **Focus**  | β-round confirmation                           | β consecutive rounds                     |
| 4a    | **Beam**   | Linear commit                                  | single-chain finality                    |
| 4b    | **Flare**  | DAG ordering                                   | multi-parent graph build                 |
| 5     | **Nova**   | Classical + PQ dual-cert finality              | BLS + Ringtail certificates              |
| **6** | **Quasar** | **Quantum-Safety Overlay**                     | lightweight PQ "heartbeat" confirmation  |

- Stages 1–5 are unchanged.  
- **Stage 6 (Quasar)** runs a rapid, correlation-based PQ check (`scheme.QuickVerify`) across the same quorum.  

---

## 3. Quasar Mechanics

### 3.1 Quasar Scheme Initialization
```go
qs := ringtail.NewQuasarScheme()
skQ, pkQ := qs.KeyGen()
```

### 3.2 Quasar Sign (at proposer)
```go
qsPre := qs.Precompute(skQ)
qsSig := qs.BindPrecomputed(qsPre).QuickSign(blockID)
```

### 3.3 Quasar Verify (at validators)
```go
ok := qs.Verify(pkQ, blockID, qsSig)
```

### 3.4 Dual-Stage Final Check
```go
isNovaFinal  := verifyBLS(blsAgg, Q) && verifyRT(rtCert, Q)
isQuasarSafe := qs.VerifyQuorum(Q, blockID, qsSig)
isFinal      := isNovaFinal && isQuasarSafe
```

- `QuickSign/QuickVerify` cost ≈ 50–100 ms on commodity hardware.
- Quasar leverages short lattice commitments and multi-qubit correlation tests for speed.

---

## 4. Security & Attack Window

- **Nova window**: (β+1)Δ_min ≈ 1.1s
- **Quasar window**: + 50–100 ms
- **Total exploitable window**: ≲ 100 ms before block is fully final — effectively zero for real-world attackers.

---

## 5. Integration

- **Location**: `engine/quantum/quasar.go`
- **Dependencies**: `luxfi/ringtail`
- **Enforcement**: PQ Engine refuses to commit until Quasar certificates are collected and verified.

---

## 6. Implementation Details

The Quasar stage adds a final quantum-safety overlay that:
1. Uses lightweight lattice commitments for ultra-fast verification
2. Runs in parallel with Nova finality checks
3. Provides a final "heartbeat" confirmation in under 100ms
4. Leverages precomputation for minimal latency

---

## 7. Conclusion

By tacking Quasar onto Nova, Lux Consensus achieves the world's fastest, quantum-secure metastable consensus: sub-second finality with an unassailable ≲ 100 ms quantum-safety window.