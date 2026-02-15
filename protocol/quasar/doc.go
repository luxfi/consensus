// Package quasar implements Quasar consensus with up to three independent
// cryptographic signing paths running in parallel:
//
//   - BLS12-381 threshold signatures — classical fast-path (ECDL hardness)
//   - Ringtail (Ring-LWE) 2-round threshold — post-quantum lattice (Module-LWE)
//   - ML-DSA-65 (FIPS 204) identity signatures — post-quantum (Module-LWE + Module-SIS)
//
// Modes (each layer independently toggleable):
//
//	BLS-only:                  fastest classical consensus
//	BLS + ML-DSA:              dual PQ consensus (single-round PQ sigs)
//	BLS + Ringtail:            dual PQ consensus (2-round threshold)
//	BLS + Ringtail + ML-DSA:   triple consensus (all three hardness assumptions)
//
// Triple signing via [signer.TripleSignRound1] runs all three paths in parallel.
// An adversary must break ECDL AND Module-LWE AND Module-SIS simultaneously.
//
// Inter-node transport uses ZAP (github.com/luxfi/zap) with optional PQ-TLS 1.3
// (Go 1.26 ML-KEM-768 default). GPU acceleration is aspirational.
//
// See [QuasarCert] and [QuasarSignature] in types.go for the wire format.
package quasar
