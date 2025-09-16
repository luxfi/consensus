// Package engine houses the three consensus engines: chain, dag, and pq.
//
// Each engine orchestrates a specific transaction topology:
// - chain: Linear consensus for sequential blocks
// - dag: DAG-based consensus for parallel, causally-ordered vertices
// - pq: Post-quantum hardened consensus with quantum-safe certificates
//
// All engines share the same algorithmic pipeline but differ in their
// data structures and finality mechanisms.
package engine
