// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_guard.go — the engine's HONEST finality-regime introspection.
//
// Mode() reports, from the SAME fields that select and drive finalization, which
// finality regime the engine is actually operating under: single-validator
// (K==1, the sole validator's own accept is the 1-of-1 quorum) or quorum-finality
// (K>1 with a verified α-of-K cert path, a live cert gossiper, a stake-weighted
// supermajority rule, and — on a strict-PQ chain — a post-quantum witness). It is
// derived, never set independently, so it can never disagree with the path that
// actually finalizes a block.
//
// This is pure introspection: it answers "what finality does this engine deliver?"
// for monitoring, tests, and any consumer that needs the regime. It does NOT gate
// any other subsystem — there is no application activation switch braided into the
// consensus engine. (The 0x9999 native DEX is always-on and does NOT consult this:
// the DEX's correctness rests on its own intrinsic per-swap controls, not a runtime
// consensus-mode flag.)
package chain

// ConsensusMode names the finality regime an engine is operating under. It is
// derived from the engine's configuration, not set independently, so it can
// never disagree with the actual finalization path.
type ConsensusMode uint8

const (
	// ModeUnknown is the zero value — a degraded K>1 topology that cannot drive
	// cert-witnessed, stake-weighted (and, on strict-PQ, post-quantum) finality to
	// its followers. Not a regime any value-bearing consumer should treat as final.
	ModeUnknown ConsensusMode = iota

	// ModeSingleValidator is a K==1 engine: the sole validator's own accept IS
	// the 1-of-1 quorum (ForceAccept). Correct for --dev / localnet. A single
	// validator can author any history, so it is NOT a multi-party quorum-finality
	// regime.
	ModeSingleValidator

	// ModeQuorumFinality is a K>1 engine with a vote verifier: a block finalizes
	// ONLY on a verified α-of-K QuorumCert (no self-finality, no REJECT→ACCEPT
	// flip), distributed by a live cert gossiper, under a stake-weighted
	// supermajority, with a post-quantum witness on a strict-PQ chain.
	ModeQuorumFinality
)

// String renders the mode for logs/errors.
func (m ConsensusMode) String() string {
	switch m {
	case ModeSingleValidator:
		return "single-validator"
	case ModeQuorumFinality:
		return "quorum-finality"
	default:
		return "unknown"
	}
}

// Mode reports the engine's finality regime, derived from its live
// configuration:
//
//   - K<=1                                    → ModeSingleValidator
//   - K>1 with a vote verifier AND a cert
//     gossiper (the α-of-K topology is
//     actually reachable: votes collected,
//     certs distributed to followers)        → ModeQuorumFinality
//   - K>1 missing the verifier OR the cert
//     gossiper                               → ModeUnknown (degraded — a verifier
//     with no way to distribute the cert can leave followers unable to finalize;
//     the engine refuses Start without the verifier, and the introspection treats
//     this as not-quorum-finality so a degraded topology never reports finality).
//
// HIGH-4: ModeQuorumFinality REQUIRES a present quorum gossiper, not merely
// K>1 && verifier!=nil. Otherwise a node whose network layer never wired the
// vote/cert distribution would report "quorum-finality" while followers silently
// cannot finalize on a cert (freeze) — the topology must be live, not just the
// verifier present. Because the mode is computed from the SAME fields that select
// and distribute finality (verifier gates counting, certGossiper distributes the
// proof), an engine cannot report ModeQuorumFinality yet be unable to drive
// cert-witnessed finality to its followers.
//
// HIGH-4b (value-grade finality): ModeQuorumFinality ADDITIONALLY requires the
// finality-DELIVERING dependencies a value-bearing chain actually relies on, so the
// regime can NEVER be reported for a finalization path that does not deliver it:
//
//   - stakeSource != nil. A value/PoS chain finalizes on a STAKE-weighted
//     supermajority (cert.VerifyWeighted), not a raw voter count. Without a
//     StakeSource the engine falls back to count-α (cert.Verify), which a
//     low-stake voter coalition can satisfy while controlling a minority of stake.
//     The mode is computed from the SAME field finalizeWithCert reads to choose
//     VerifyWeighted vs Verify, so a "quorum-finality" report implies stake-weighted
//     finality is actually in force.
//   - when the chain profile is STRICT-PQ, a PQ CryptoWitnessSource != nil. A
//     strict-PQ chain's finality witness MUST be post-quantum; without the PQ
//     witness wired, the cert path cannot produce the PQ proof the profile demands,
//     so the engine must not claim a quorum-finality regime it cannot witness
//     post-quantum. (On a non-strict profile this requirement is vacuous.)
//
// All inputs are read under the lock from the SAME fields that select and drive
// finality, so the reported mode cannot disagree with the live finalization path.
func (t *Transitive) Mode() ConsensusMode {
	k := t.consensus.K()
	if k <= 1 {
		return ModeSingleValidator
	}
	t.mu.RLock()
	hasVerifier := t.voteVerifier != nil
	hasGossiper := t.certGossiper != nil
	hasStake := t.stakeSource != nil
	strictPQ := t.strictPQ
	// The PQ witness source is the node-layer hook that upgrades the engine cert into a
	// quasar.WeightedQuorumCert (the post-quantum finality witness). nil means "not
	// plumbed" — exactly the semantics ToQuasarCert relies on — so a strict-PQ chain
	// without it cannot witness finality post-quantum and the mode is not quorum-finality.
	hasPQWitness := t.cryptoWitness != nil
	t.mu.RUnlock()
	if !hasVerifier || !hasGossiper {
		return ModeUnknown // topology not live (no counting / no cert distribution)
	}
	if !hasStake {
		return ModeUnknown // count-α only; not the stake-weighted finality value grade requires
	}
	if strictPQ && !hasPQWitness {
		return ModeUnknown // strict-PQ chain cannot witness finality post-quantum
	}
	return ModeQuorumFinality
}
