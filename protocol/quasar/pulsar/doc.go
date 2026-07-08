// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package pulsar wires the luxfi/pulsar threshold ML-DSA primitives into
// Quasar as a certificate profile. It is the consensus-SIDE adapter: Quasar
// consumes Pulsar, Pulsar never consumes Quasar.
//
// Decomplected ownership (the load-bearing boundary of this package):
//
//	Consensus owns ORCHESTRATION:
//	  - validator-set membership and committee sampling (prism.Cut)
//	  - session binding to the consensus round (PulsarSession)
//	  - canonical, non-grindable nonce selection from the pool
//	  - QC aggregation of z-shares, signer bitmap, ConsensusCert assembly
//	  - retries, coarse aborts, fallback policy, slashing inputs
//
//	Pulsar owns CRYPTO:
//	  - the two-round shape (Round1 binds the nonce; Round2 emits a
//	    proof-carrying z partial)
//	  - z aggregation modulo q, the public hint recovery (FindHint over the
//	    public w' = A·z − c·t1·2^d), and the final FIPS 204 ML-DSA signature
//
// The crypto-side contract is luxfi/pulsar's RoundSigner interface. The final
// FIPS 204 signature byte production is secret-bearing (it needs the module
// matrix A, t1, the challenge, and the per-coefficient hint), so it lives
// ONLY inside package pulsar — never here. This package therefore drives the
// orchestration it owns end to end, and delegates the signature-assembly step
// to a pluggable pulsarlib.RoundSigner CORE. The core is fail-closed until the
// lib ships a sound Boundary-Cleared / Carry-Elimination (BCC/CEF) signer with
// externally-reviewed ZK proofs (pulsar.ProductionBCCSigningReady). Until then
// Finalize returns a structurally-complete ConsensusCert with an EMPTY
// Signature and reports ErrProfileNotReady, and the Quasar orchestrator falls
// back to the Corona (Ring-LWE) profile. No secret residual is ever opened on
// this path.
//
// NonceCerts are produced by a BACKGROUND validator NonceTranscript
// subprotocol that fills a NonceCertPool — never inline on the block hot path.
// An empty pool yields ErrNonceCertPoolEmpty (a fallback signal), never an
// inline nonce.
package pulsar
