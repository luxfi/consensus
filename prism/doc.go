// Package prism provides DAG geometry: frontiers, cuts, and refractions.
//
// Given a partial order (the transaction/vertex DAG), Prism helps project
// that poset into slices that are easy to vote on and schedule. "Refraction"
// is the deterministic projection into sub-slices; "frontier" returns a
// maximal antichain; "cut" selects a thin slice across causal layers.
//
// Prism is DAG-only; linear chains never need antichains or cuts.
package prism