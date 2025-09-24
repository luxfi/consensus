// Package pq implements post-quantum hardened consensus.
//
// PQ uses quantum-safe cryptographic primitives throughout the consensus
// pipeline: ML-KEM for key exchange, ML-DSA for signatures, and hybrid
// certificates from the quasar overlay. The topology can be linear or DAG,
// but all operations are quantum-resistant for long-term security.
package pq
