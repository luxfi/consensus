// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package zchain is the consensus-side reference implementation of HIP-0078
// (Z-Chain — Post-Quantum Identity & Attestation Rollup).
//
// The package owns ONE thing: the canonical proof envelope that lets SP1,
// RISC0, P3Q, Stone, and Stwo (and any future PQ proof backend) live behind
// a single verification interface. Backends do NOT self-assert safety —
// every "is this proof OK on this chain" decision flows through
//
//	VerifyZProofUnderProfile(profile, registry, input, proof)
//
// where:
//
//   - profile        — the chain-wide allow-list (config.ChainSecurityProfile)
//   - registry       — the in-process verifier-manifest registry (this package)
//   - input          — the canonical public inputs (HashZPublicInputs binds them)
//   - proof          — the on-the-wire envelope (UnmarshalZProofEnvelope decodes)
//
// Fail-closed in this order (see verify.go for the typed errors):
//
//  1. profile-axis pins  (ProfileID, HashSuiteID, ProofPolicyID, backend, format)
//  2. soundness / hash-width floors
//  3. transparency / pairing / KZG / trusted-setup / classical-wrapper bans
//  4. manifest lookup    (registry must hold a manifest for proof.VerifierID)
//  5. manifest equality  (backend / program / verifier-key)
//  6. public-inputs binding (HashZPublicInputs(input) == proof.PublicInputsHash)
//  7. backend dispatch   (backend verifier called only after every cheap check passes)
//
// Nothing in this package depends on luxfi/sp1, luxfi/risc0, luxfi/p3q,
// luxfi/stone, or luxfi/stwo — those bind their concrete backend verifiers
// against the BackendVerifier interface from outside the consensus module.
//
// Wire formats: see proof_envelope.go for the deterministic big-endian
// codec. Every byte the envelope contains is bound into TranscriptHash()
// via TupleHash256(... , customization="ZCHAIN-PROOF-ENVELOPE") so a
// post-sign mutation breaks signature verification, not just envelope
// equality. Hash output is 48 bytes (SHA3-384 wire width) per HIP-0078.
package zchain
