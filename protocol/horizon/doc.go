// Package horizon houses DAG order-theory predicates.
//
// It answers reachability, LCA, and antichain queries, and provides small
// helpers for certificate/skip detection under a DAG model. In the metaphor,
// the "event horizon" is the boundary beyond which reordering cannot affect
// committed history; here, it's a precise predicate over the poset.
package horizon
