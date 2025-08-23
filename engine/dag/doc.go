// Package dag implements consensus for Directed Acyclic Graphs.
//
// DAG handles parallel transactions with causal dependencies, where vertices
// can reference multiple parents. The consensus pipeline flows:
// photon → wave → focus → (prism + horizon) → flare → nebula.
// This enables concurrent transaction processing while respecting causality.
package dag
