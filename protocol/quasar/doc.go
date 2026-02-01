// Package quasar implements Quasar triple consensus: BLS + Ringtail + ML-DSA.
//
// Three independent cryptographic hardness assumptions run in parallel:
//
//   - BLS12-381 aggregate signatures — classical fast-path (48-byte proof)
//   - Ringtail (Ring-LWE) threshold signatures — post-quantum lattice path
//   - ML-DSA-65 (FIPS 204) identity proofs — post-quantum, rolled into ZK proof
//
// Each layer can be enabled independently. BLS-only gives fastest classical
// consensus. BLS + Ringtail gives PQ-safe threshold finality. Full triple
// mode adds ML-DSA ZK rollup (~200-byte STARK proving N ML-DSA sigs valid).
//
// All layers support GPU acceleration via github.com/luxfi/crypto for BLS
// pairing, lattice computations, and ZK proof generation.
//
// Inter-node transport uses ZAP (github.com/luxfi/zap) for zero-copy
// consensus messages — not github.com/luxfi/p2p.
//
// See QuasarCert and QuasarSignature in types.go for the wire format.
package quasar
