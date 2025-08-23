// Package fpc implements Fast Probabilistic Consensus thresholds.
//
// For round "phase" and committee size k, it picks a θ ∈ [θ_min, θ_max] and
// returns α = ⌈θ·k⌉ for both preference and confidence. The PRF makes θ stable
// for a given phase, testable, and deterministic in simulations.
package fpc