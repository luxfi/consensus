// Package consensus defines a minimal set of orthogonal primitives for
// probabilistic agreement.
//
// The terminology follows a physics / cosmology metaphor:
//
//	photon   — the atomic unit: select K "rays" (a committee sample)
//	wave     — per-round thresholds (α_pref, α_conf) and phase shifts (FPC)
//	focus    — β consecutive rounds concentrate confidence into a decision
//	prism    — geometric views of a DAG: frontiers, cuts, refractions
//	horizon  — order-theoretic structure: reachability, antichains, cert/skip
//
// Together these stages form the optics of consensus: light is emitted (photon),
// amplified and interfered (wave), reinforced (focus), bent through a DAG
// (prism), and ultimately anchored against the limits of visibility (horizon).
//
// Engines (chain, dag, quasar) compose these primitives into full protocols,
// while remaining modular and substitutable.
package consensus
