// Package quasar implements Quasar consensus with up to three independent
// cryptographic signing paths running in parallel:
//
//   - BLS12-381 threshold signatures — classical fast-path (ECDL hardness)
//   - Corona (Ring-LWE) 2-round threshold — post-quantum lattice (Module-LWE)
//   - ML-DSA-65 (FIPS 204) identity signatures — post-quantum (Module-LWE + Module-SIS)
//
// Modes (each layer independently toggleable):
//
//	BLS-only:                  fastest classical consensus
//	BLS + ML-DSA:              dual PQ consensus (single-round PQ sigs)
//	BLS + Corona:               dual PQ consensus (2-round threshold)
//	BLS + Corona + ML-DSA:   Quasar (all three hardness assumptions)
//
// Quasar signing via [signer.TripleSignRound1] runs all three paths in parallel.
// An adversary must break ECDL AND Module-LWE AND Module-SIS simultaneously.
//
// Inter-node transport uses ZAP (github.com/luxfi/zap) with optional PQ-TLS 1.3
// (Go 1.26 ML-KEM-768 default).
//
// GPU acceleration: this package composes BLS (crypto/bls), ML-DSA
// (crypto/mldsa), Corona (corona/threshold), and SLH-DSA (crypto/slhdsa)
// primitives. Each routes batch operations through crypto/backend.Resolve
// → accel.LatticeOps when CRYPTO_BACKEND=gpu (or auto with a GPU-capable
// host). Single signatures stay on CPU — kernel-launch overhead exceeds
// the win for n=1. The aggregated cert verify path (n=21 validators
// today) is the one that benefits and dispatches accordingly.
//
// See ../../PQ-GPU-AUDIT.md for the per-primitive dispatch matrix.
//
// See [QuasarCert] and [QuasarSignature] in types.go for the wire format.
package quasar
