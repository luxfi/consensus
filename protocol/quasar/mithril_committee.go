// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

// mithril_committee.go — the size bound on a Pulsar (ML-DSA / FIPS-204) signing
// committee when its group key is produced by the DEALERLESS Replicated Secret
// Sharing DKG (Mithril, ePrint 2026/013; luxfi/dkg/rss, luxfi/pulsar v1.5.0+).
//
// # The bound
//
// Mithril RSS splits the threshold key into C(N, N−T+1) short subset secrets.
// That replicated-share count grows combinatorially and the local
// rejection-sampling cost grows with the committee, so the dealerless,
// stock-FIPS-204-signable construction is viable ONLY for a SMALL committee:
//
//	2 ≤ T ≤ N ≤ MithrilMaxCommittee   (= rss.MaxParties = 6).
//
// This file is the ONE consensus-side place that bound is enforced; the
// canonical numbers live in luxfi/dkg/rss (DRY — never re-typed here).
//
// # Why a small Pulsar signing committee is SOUND (the security argument)
//
// The consensus quorum is NOT the signing committee. Lux finality is
// Avalanche/Snow-family: safety and liveness come from repeated randomized
// SUBSAMPLING of the full N>1000 validator set, never from every validator
// co-signing every certificate. The Pulsar committee does not decide anything —
// it only emits compact post-quantum EVIDENCE of a digest the consensus has
// ALREADY finalized. So the committee needs exactly one property: it must be
// UNFORGEABLE (an adversary controlling < T members cannot produce a valid group
// signature). The dealerless RSS DKG gives precisely that: a coalition of ≤ T−1
// parties is disjoint from at least one whole M-subset whose fresh short secret
// information-theoretically masks the key, and the threshold-signed output is a
// standard ML-DSA signature whose EUF-CMA reduces to Module-LWE/Module-SIS.
//
// Broad economic/Byzantine security is preserved by four independent mechanisms,
// none of which depends on the committee being large:
//
//  1. Avalanche subsampling decides the digest over the full validator set; the
//     committee signs an already-final value.
//  2. Committees are epoch-sampled and ROTATING (stake-weighted + VRF), so a
//     transient corruption of one small committee does not persist.
//  3. High-value finality requires MULTIPLE independent committee certificates
//     (e.g. 2/3 of groups — DefaultGroupQuorum), so one committee is never a
//     single point of forgery.
//  4. The Quasar cert is dual-PQ AND-mode: a Pulsar (ML-DSA) leg AND a Corona
//     (Ringtail) leg must both verify, so forging finality requires breaking
//     two independent threshold schemes, not one small committee.
//
// Hence a Pulsar signing committee of N ≤ 6 with t ≈ ⌈2N/3⌉ is sound: it is the
// compact-evidence emitter, and the chain's security budget lives elsewhere.

import (
	"fmt"

	"github.com/luxfi/dkg/rss"
)

const (
	// MithrilMaxCommittee is the largest Pulsar signing committee the dealerless
	// RSS keygen admits. Canonical source: rss.MaxParties (= 6).
	MithrilMaxCommittee = rss.MaxParties

	// MithrilMinThreshold is the smallest meaningful threshold (T ≥ 2 — a
	// 1-of-N "threshold" would let any single party forge).
	MithrilMinThreshold = 2
)

// ValidateMithrilSigningCommittee enforces 2 ≤ T ≤ N ≤ MithrilMaxCommittee for a
// Pulsar dealerless-RSS signing committee. Two layered gates, fail-closed:
//
//  1. The consensus POLICY cap N ≤ MithrilMaxCommittee (= rss.MaxParties = 6).
//     Lux deliberately operates SMALL signing committees (see the design note
//     above): the chain's security budget lives in Avalanche subsampling +
//     rotation + multi-committee certs, not in a large signing group. This cap
//     is the consensus operating point, not a crypto limit — luxfi/dkg/rss
//     itself admits any norm-viable committee up to MaxBitmaskParties = 63.
//  2. The canonical crypto bound rss.ValidateCommittee (2 ≤ T ≤ N and
//     τ·C(N,N−T+1)·η < γ2) — the one definition of "stock-FIPS-204-signable",
//     never re-typed here (DRY).
//
// The policy cap is applied first and explicitly: rss loosened its own gate from
// a hard N ≤ 6 to the norm bound (dkg ≥ v0.3.4), so consensus must assert its
// small-committee operating point itself rather than free-ride on rss.
// An out-of-range committee is rejected, never silently resized to a weaker mode.
func ValidateMithrilSigningCommittee(t, n int) error {
	if n > MithrilMaxCommittee {
		return fmt.Errorf("quasar: Pulsar dealerless-RSS signing committee N=%d exceeds the consensus small-committee cap %d", n, MithrilMaxCommittee)
	}
	if err := rss.ValidateCommittee(t, n); err != nil {
		return fmt.Errorf("quasar: Pulsar dealerless-RSS signing committee: %w", err)
	}
	return nil
}

// RecommendedMithrilCommittee returns the default Pulsar RSS signing-committee
// size and threshold: 4-of-6 — a 2/3+ Byzantine threshold at the maximum viable
// committee size, maximising the masking-subset count C(6,3)=20 while remaining
// comfortably stock-FIPS-204-signable.
func RecommendedMithrilCommittee() (t, n int) { return 4, MithrilMaxCommittee }

// ClampMithrilCommitteeSize returns the largest admissible Pulsar RSS committee
// size ≤ the requested size, never exceeding MithrilMaxCommittee and never below
// MithrilMinThreshold+1. Used by the grouped-epoch sortition to keep every
// signing group within the dealerless-RSS viability bound.
func ClampMithrilCommitteeSize(requested int) int {
	if requested > MithrilMaxCommittee {
		return MithrilMaxCommittee
	}
	if requested < MithrilMinThreshold+1 {
		return MithrilMinThreshold + 1
	}
	return requested
}
