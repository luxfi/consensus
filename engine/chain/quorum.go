// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum.go — the NOVA-tier thresholds, as a clean function of the live count n.
//
// This file owns ONLY the Nova (β-ignition / local-execution) rung of the ladder
// in finality.go. The QUASAR rung — the ⅔-stake certificate — has exactly ONE
// definition and it is NOT here: it is config.TwoThirdsStakeFloor (the strict >2/3
// stake predicate, overflow-safe) and config.EqualStakeSupermajorityThreshold (its
// equal-stake vote count ⌊2n/3⌋+1), enforced by QuorumCert.VerifyWeighted over
// DISTINCT signers. Nova must never re-express that threshold; it deliberately sits
// BELOW it (majority < ⅔), because Nova authorizes local execution while Quasar
// authorizes export.
package chain

// NovaQuorum is the majority ⌊n/2⌋+1 the sampler needs to IGNITE a block to Nova
// (acceptance, local execution). It tolerates ⌊(n-1)/2⌋ crash faults — the MOST a net
// of n can lose and still make safe local progress under the crash-fault model
// (foundation-run, non-equivocating validators): any two majorities intersect and a
// non-equivocating node cannot back two conflicting blocks, and a partition lets at
// most one side hold a majority. It is DELIBERATELY below the Quasar ⅔ stake gate —
// a single equivocator can fork a bare majority, so Nova is never a Byzantine
// guarantee (that is Quasar's job). n<1 → 1 (a lone node self-ignites; never 0, which
// would let a transiently-empty validator view self-accept — the 1085013 fork class).
func NovaQuorum(n int) int {
	if n < 1 {
		return 1
	}
	return n/2 + 1
}

// CrashTolerance is the number of simultaneous crash faults Nova ignition survives:
// n − NovaQuorum(n) = ⌈n/2⌉−1. Beyond it the sampler cannot reach a majority, so Nova
// PAUSES fail-closed (no ignition ⇒ no acceptance ⇒ no fork) and resumes the instant a
// node returns. Feeds the degraded-mode signal (responsive stake below NovaQuorum ⇒
// degraded, novaHeight stalls, quasarHeight stalls further behind).
func CrashTolerance(n int) int {
	if n < 2 {
		return 0
	}
	return n - NovaQuorum(n)
}

// NovaBeta is the β confidence depth: the number of CONSECUTIVE majority sampling
// rounds required to IGNITE Nova.
//
//   - n==1: β=1 — a lone node ignites immediately; there are no peers to confirm
//     against and any wait is a phantom-peer stall.
//   - 2≤n: β=2 — one confirming round of HYSTERESIS so a transient one-round majority
//     flip cannot ignite, WITHOUT the fragility of a deep β at zero margin (with only
//     ⌊n/2⌋+1 alive, a single flap during a long β window resets the count and stalls
//     liveness exactly when margin is thin). β is NOT a Byzantine-safety knob on a K=n
//     net (no random subsampling ⇒ β cannot dilute an equivocator; that is the Quasar
//     cert's role).
//
// Large permissionless nets (K<n, real subsampling) raise β to the classical negl(β)
// metastable depth — a set-size migration, not applied to the small trusted deploy.
func NovaBeta(n int) int {
	if n <= 1 {
		return 1
	}
	return 2
}
