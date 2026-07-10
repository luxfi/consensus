// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum.go — the two-tier quorum as a clean function of the live validator count n.
//
// Nova is a TWO-TIER finality engine, and the two tiers use DIFFERENT quorums:
//
//	LiveQuorum(n) = ⌊n/2⌋+1  — the majority Nova needs to ACCEPT a block (drive
//	  VM.Accept). It tolerates ⌊(n-1)/2⌋ crash faults — the MOST a net of n can lose
//	  and still make safe progress under the crash-fault model. This is what lets a
//	  small foundation net keep PRODUCING through node drops instead of stalling.
//
//	CertQuorum(n) = ⌊2n/3⌋+1 — the ⅔ BFT quorum for the TRAILING Quasar attestation
//	  (the signable, Byzantine-safe finality receipt for bridges/DEX). It NEVER gates
//	  Nova liveness; it certifies a block Nova already accepted, once ⌊2n/3⌋+1 stake
//	  is present. It tolerates ⌊(n-1)/3⌋ Byzantine (equivocating) validators.
//
// SAFETY MODEL — read this before touching the thresholds.
//
//   - Under CRASH faults (a validator stops; it never sends conflicting messages),
//     LiveQuorum is SAFE: any two majorities of n intersect, and a non-equivocating
//     node in the intersection cannot have voted for two conflicting blocks — so no
//     two conflicting blocks both reach a majority. Partitions are safe for the same
//     reason: at most one side of a partition can hold ⌊n/2⌋+1, so only one side
//     finalizes and the minority adopts it on heal. This is the model a foundation-run
//     net (all validators operated by one trusted party) actually lives in.
//
//   - Under BYZANTINE faults (a validator equivocates — votes A to one group, B to
//     another), LiveQuorum is NOT safe on a small net: one equivocator can push two
//     conflicting blocks each to a bare majority (n=5: {1,2,3}→A, {3,4,5}→B, node 3
//     equivocating). And β cannot fix this: β-confidence dilutes an adversary only
//     with RANDOM SUBSAMPLING (K<n), where different honest nodes query different
//     samples; on a small net K=n (everyone is polled every round), so the equivocator
//     sits in every poll and β buys nothing. The BYZANTINE-safe guarantee on these
//     nets is CertQuorum (the ⅔ overlap), delivered as the trailing attestation — NOT
//     LiveQuorum, and NOT β. Anything that must be Byzantine-final (a bridge withdrawal,
//     a DEX settlement) waits for the CertQuorum receipt, not the Nova accept.
//
// So: β is HYSTERESIS, not Byzantine safety, on the K=n deploy nets (see LiveBeta).
// When the validator set grows large and permissionless (K<n, real subsampling), the
// model flips: LiveQuorum should become the ⅔ Byzantine threshold and β the classical
// negl(β) metastable bound. That is a set-size migration, gated separately — this file
// is the small-trusted-net (n≤~21) regime the network deploys in today.
package chain

// LiveQuorum is the crash-fault-tolerant MAJORITY threshold ⌊n/2⌋+1 that Nova needs to
// accept a block. It tolerates ⌊(n-1)/2⌋ crashes — the maximum for a net of n. n<1 is
// treated as a single self-finalizing node (returns 1) so the function is total and a
// transiently-empty validator view never yields a zero threshold (which would let a lone
// node self-accept — the 1085013 fork class). See the SAFETY MODEL note above.
func LiveQuorum(n int) int {
	if n < 1 {
		return 1
	}
	return n/2 + 1
}

// CertQuorum is the ⅔ BFT threshold ⌊2n/3⌋+1 for the trailing Quasar finality cert. It
// tolerates ⌊(n-1)/3⌋ Byzantine validators and is the Byzantine-safe finality receipt.
// It never gates Nova's liveness. n<1 returns 1 (single-node self-attestation).
func CertQuorum(n int) int {
	if n < 1 {
		return 1
	}
	return (2*n)/3 + 1
}

// CrashTolerance is the number of simultaneous crash faults Nova's LiveQuorum survives
// while still finalizing: n − LiveQuorum(n) = ⌈n/2⌉−1. Beyond it, Nova PAUSES
// fail-closed (no majority ⇒ no accept ⇒ no fork) and resumes the instant a node
// returns. Exposed for the n∈{1..5} property tests and for operator dashboards.
func CrashTolerance(n int) int {
	if n < 2 {
		return 0
	}
	return n - LiveQuorum(n)
}

// ByzantineTolerance is the number of equivocating validators the CertQuorum (⅔)
// finality receipt survives: ⌊(n-1)/3⌋. This is 0 for n≤3 and 1 for n∈{4,5} — the
// honest bound on how much the trailing cert can prove on a tiny net.
func ByzantineTolerance(n int) int {
	if n < 1 {
		return 0
	}
	return (n - 1) / 3
}

// LiveBeta is the confidence depth: the number of CONSECUTIVE majority polls Nova
// requires before it accepts, as a function of the live count n.
//
//   - n==1: β=1 — a single node self-finalizes immediately; there are no peers to
//     confirm against and any wait would be a phantom-peer stall.
//   - 2≤n≤betaSmallNetMax: β=2 — minimal HYSTERESIS (one confirming round) so a
//     transient one-round majority flip does not finalize, WITHOUT the fragility of a
//     deep β at zero margin (with only ⌊n/2⌋+1 alive, a single flap during a long β
//     window resets the count and stalls liveness — so β must stay shallow exactly
//     when margin is thin). β here is NOT a Byzantine-safety knob (K=n ⇒ no subsampling
//     ⇒ β cannot dilute an equivocator); that is CertQuorum's job.
//
// For large permissionless nets (K<n) this must grow to the classical β≈15–20 so the
// metastable subsampling bound (negl(β)) holds — gated by the set-size migration noted
// in the file header, not applied to the small trusted deploy nets.
func LiveBeta(n int) int {
	if n <= 1 {
		return 1
	}
	return 2
}
