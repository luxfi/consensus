# Dual-Certificate Finality Implementation

## Overview

Lux Quasar consensus achieves quantum-resistant finality through dual certificates - every block requires both BLS (classical) and Ringtail (post-quantum) signatures. This document details the implementation across the consensus and node packages.

## Architecture

### Consensus Package (`/consensus/`)

The consensus package provides the building blocks:

1. **PQ Engine** (`engine/pq/`)
   - THE production consensus engine
   - Manages dual-certificate finality
   - Integrates Nova (classical) + Ringtail (quantum)

2. **Ringtail Layer** (`ringtail/`)
   - `certificate.go` - CertBundle with BLS + RT certificates
   - `keys.go` - Ringtail key management
   - `service.go` - Quasar service orchestration
   - `precompute.go` - RT share precomputation

### Node Package (`/node/consensus/`)

The node integrates these components:

1. **Quasar Engine** (`engine/quasar/`)
   - Adapts PQ engine to node interfaces
   - Handles network messages
   - Manages Q-blocks

2. **Q-Chain** (`qchain/`)
   - Embeds Q-blocks in Q-Chain
   - Provides network-wide finality
   - Chain listeners for cross-chain sync

## Key Components

### CertBundle Structure

```go
type CertBundle struct {
    BLSAgg  []byte  // 96B BLS aggregate signature
    RTCert  []byte  // ~3KB Ringtail certificate
    Round   uint64  // Consensus round
    Height  uint64  // Block height
}
```

### Dual-Certificate Validation

```go
// Block is only final when BOTH certificates are valid
isFinal := verifyBLS(blsAgg, quorum) && verifyRT(rtCert, quorum)
```

### Key Hierarchy

| Key Type | Purpose | Storage |
|----------|---------|---------|
| Node-ID (ed25519) | P2P transport | `$HOME/.lux/node.key` |
| Validator-BLS | Fast finality | `$HOME/.lux/bls.key` |
| Validator-RT | PQ finality | `$HOME/.lux/rt.key` |

## Consensus Flow

1. **Nova DAG** (Round 1)
   - k-peer sampling builds confidence d(T)
   - Vertices become preferred at β₁, decided at β₂
   - ~600-700ms to classical consensus

2. **Ringtail PQ** (Rounds 2-3)
   - Phase I: Validators propose highest-confidence vertex
   - Phase II: Aggregate threshold signatures if > α𝚌 agree
   - ~200-300ms additional for quantum finality

3. **Q-Block Creation**
   - Contains finalized vertices + dual certificates
   - Embedded as Q-Chain internal transaction
   - All chains read Q-blocks for finality

## Performance Optimization

### RT Precomputation

The Quasar service precomputes Ringtail shares to hide lattice computation latency:

```go
// Precomputer maintains 20-50 shares ready in RAM
precomputer := NewPrecomputer(rtKey, runtime.NumCPU())

// Workers generate shares on spare cores
// ~50-100ms per share, done in background

// Fast path: bind precomputed share to block
share := precomputer.GetShare()
boundShare := bindShare(share, blockHash)
```

### Pipeline Architecture

```
BLS signing (hot path) ──┐
                         ├──> Dual-cert finality
RT precompute (parallel) ┘
```

## Security Model

### Attack Scenarios

1. **Pre-quantum**: Attacker needs ≥⅓ stake to fork
2. **Q-day (BLS broken)**:
   - Attacker can forge BLS but not RT
   - Block fails `isFinal` check
   - Consensus halts rather than accepting unsafe fork
3. **Post-quantum**: Security rests on lattice SVP (2¹⁶⁰ ops)

### Attack Window

- RT round time ≤50ms on mainnet
- Attacker must break both BLS and lattice in this window
- Effectively impossible even with quantum computer

## Implementation Status

### Complete ✓
- [x] PQ engine with dual-certificate logic
- [x] CertBundle structure and validation
- [x] Ringtail key management
- [x] Certificate creation and aggregation
- [x] Quasar service with precomputation
- [x] Q-Chain integration design

### In Progress
- [x] Integration with luxfi/crypto/ringtail for actual lattice crypto
- [ ] PQ account options (Lamport, XMSS)
- [ ] Fork-choice rules for dual-cert finality

### Future Work
- [ ] Dynamic precompute pool sizing
- [ ] Certificate compression
- [ ] Light client proofs for Q-blocks

## Usage Example

```go
// Create PQ engine with dual keys
engine := pq.New(params, nodeID)
engine.Initialize(ctx, blsKey, rtKey, validators)

// Add block to consensus
engine.Add(ctx, blockID, parentID, height, data)

// Both certificates created automatically
// Finality only when BOTH are valid

// Check dual-certificate finality
if engine.Finalized() {
    bundle, _ := engine.GetCertBundle(height)
    // bundle contains both BLS and RT certificates
}
```

## Summary

The dual-certificate implementation ensures Lux remains secure against both classical and quantum attacks. By requiring both BLS and Ringtail signatures for every block, we achieve:

- **Sub-second finality** even with PQ security
- **Graceful degradation** if one signature scheme is compromised
- **Zero additional latency** through precomputation
- **Unified protocol** for all chain types

Welcome to the quantum era of consensus - where every block is quantum-immortal.
