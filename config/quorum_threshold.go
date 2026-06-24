// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_threshold.go — the SINGLE definition of the stake-weighted finality
// threshold, shared by the cert verifier and the live-set parameter sizer.
//
// THE ONE RULE (one place): a set of validators is a finality supermajority iff
// its summed stake STRICTLY EXCEEDS two-thirds of the total stake
// (Tendermint +⅔). TwoThirdsStakeFloor computes floor(2·total/3) overflow-safely;
// the verifier (engine/chain QuorumCert.VerifyWeighted) accepts iff
// `voted > TwoThirdsStakeFloor(total)`, and the parameter sizer derives the
// minimum vote COUNT that can reach the SAME predicate (WeightedSupermajorityThreshold).
// Because both read this one function, the count threshold the node sizes K/α to
// can never drift from the stake predicate the cert enforces — it is DERIVED from
// the verifier's rule, not a parallel hardcoded schedule.
//
// Lives in the config package (not engine/chain) so config.FeasibleParams can use
// it with no import cycle: engine/chain already imports config, so the verifier
// reaches config.TwoThirdsStakeFloor in the allowed direction.
package config

import (
	"sort"
	"time"

	"github.com/luxfi/constants"
)

// TwoThirdsStakeFloor returns floor(2·total/3) — the threshold a supermajority
// must STRICTLY EXCEED. Computed overflow-safely from `total` alone (3·voted and
// 2·total would overflow near 2^64 for a large total): floor(2·total/3) =
// 2·(total/3) + floor(2·(total mod 3)/3), and floor(2r/3) for r∈{0,1,2} is
// {0,0,1}. This is the SINGLE definition of the ⅔ floor — the cert verifier and
// the parameter sizer both call it, so the count threshold and the stake
// predicate can never disagree.
func TwoThirdsStakeFloor(total uint64) uint64 {
	q, r := total/3, total%3
	floor := 2 * q
	if r == 2 {
		floor++ // floor(2r/3): r∈{0,1}→0, r==2→1
	}
	return floor
}

// WeightedSupermajorityThreshold returns the MINIMUM number of votes whose
// cumulative stake can STRICTLY EXCEED two-thirds of the total stake — derived
// from the SAME predicate the cert verifier enforces (`cum > TwoThirdsStakeFloor`).
//
// "Minimum count that CAN reach ⅔" means: order validators heaviest-first and
// count until the running stake first exceeds the floor. Heaviest-first is the
// correct selection because α is the engine's COUNT gate; the cert SEPARATELY
// enforces the actual ⅔-by-stake predicate (VerifyWeighted) at finalize, so a
// count below the true stake quorum can never finalize. Setting α to the
// smallest count that *can* reach ⅔ means: below α even the heaviest voters
// cannot hold ⅔ (so finality is provably impossible — no point demanding more
// votes), and at α the heaviest voters do (so the count gate never blocks a
// legitimate ⅔-stake quorum). Lightest-first ("every α-subset exceeds ⅔") would
// over-constrain liveness on skewed stake and can be unsatisfiable for count ≤ N.
//
// For EQUAL stake (every weight equal and > 0) this collapses to the closed form
//
//	α = floor(2N/3) + 1
//
// which is what the live equal-stake networks use: N=5→4, N=11→8, N=21→15
// (15, not 14: 14/21 = 66.67% does NOT strictly exceed ⅔). See
// EqualStakeSupermajorityThreshold.
//
// Returns 0 for an empty set or zero total stake (the caller treats that as
// "no stake model" / fail-closed, mirroring VerifyWeighted's zero-total branch).
// Never returns more than len(weights).
func WeightedSupermajorityThreshold(weights []uint64) int {
	var total uint64
	for _, w := range weights {
		total += w
	}
	if total == 0 || len(weights) == 0 {
		return 0
	}
	floor := TwoThirdsStakeFloor(total)

	// Heaviest-first: copy + sort descending so we find the SMALLEST count that
	// can reach the floor. Copy so we never mutate the caller's slice.
	sorted := make([]uint64, len(weights))
	copy(sorted, weights)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })

	var cum uint64
	for i, w := range sorted {
		cum += w
		if cum > floor { // STRICT > ⅔ — the exact predicate VerifyWeighted uses
			return i + 1
		}
	}
	// Unreachable for total>0 (the full set sums to total > floor(2·total/3) for
	// any total ≥ 1), but be explicit and fail-closed to the full set.
	return len(sorted)
}

// EqualStakeSupermajorityThreshold returns the minimum vote count for an
// EQUAL-stake set of n validators whose cumulative stake strictly exceeds ⅔ —
// i.e. WeightedSupermajorityThreshold over n unit weights. It is the closed form
//
//	α = floor(2n/3) + 1     (n ≥ 1)
//
// and is the threshold the live equal-stake networks (all current Lux nets) size
// α to. n≤0 yields 1 (a degenerate single-acceptor floor). The result equals the
// heaviest-first general computation for unit weights, so the two definitions
// cannot diverge — proven in TestEqualStakeMatchesWeighted.
func EqualStakeSupermajorityThreshold(n int) int {
	if n <= 0 {
		return 1
	}
	return int(TwoThirdsStakeFloor(uint64(n))) + 1
}

// liveTimingFor returns the (blockTime, roundTimeout) appropriate for a network.
// Timing legitimately varies by deployment — localhost dev nets have ~zero
// network latency and run 1ms blocks / 5ms rounds for throughput; WAN production
// nets use larger budgets so a round survives real propagation jitter. This is
// the ONLY axis FeasibleParams varies by network; K and α are sized purely from
// the live validator set. (Mainnet 200ms/400ms and testnet 100ms/225ms match the
// retired MainnetParams/TestnetParams timing exactly, so the only behavioral
// change vs. the old per-tier presets is the dynamic K/α.)
func liveTimingFor(networkID uint32) (time.Duration, time.Duration) {
	switch networkID {
	case constants.MainnetID:
		return 200 * time.Millisecond, 400 * time.Millisecond
	case constants.TestnetID:
		return 100 * time.Millisecond, 225 * time.Millisecond
	default: // devnet / localnet / sovereign L1s: localhost-fast
		return 1 * time.Millisecond, 5 * time.Millisecond
	}
}

// FeasibleParams returns the consensus parameters for a sybil-protected value
// network sized to the LIVE validator set of n validators — ONE function for
// mainnet, testnet, devnet, localnet and every sovereign L1 (it replaces the
// per-tier MainnetParams/TestnetParams/DefaultParams/local split). The committee
// is exactly the live set so the proposer's self-filtered K-sample still polls
// every peer and α has real Byzantine slack:
//
//	K = n                                    (sample every live validator)
//	α = max( EqualStakeSupermajorityThreshold(n),   // strict >⅔ stake (the cert floor)
//	         bftQuorumFloor(K) )                     // 2α−K ≥ f+1 overlap (never below)
//
// α is the strict-⅔ stake threshold DERIVED from the cert verifier's rule
// (TwoThirdsStakeFloor), clamped UP to the BFT overlap floor so it can never dip
// below the safety bound config.Valid() enforces. For equal stake the strict-⅔
// value already dominates the overlap floor (e.g. N=21 → 15 > 14), so the clamp
// only binds on pathologically skewed stake; either way α is never below the
// stake-cert threshold and never below the overlap floor. AlphaPreference and
// AlphaConfidence are both set to α; BetaVirtuous stays small for fast
// confirmation and BetaRogue is held at/above K.
//
// n<1 is treated as the minimal Byzantine committee (n=4): the validator set is
// not yet known and we fail to a SAFE small committee (K=4/α=3, f=1), never to a
// giant unsatisfiable one (the old Default K=20/α=14 froze small nets at height
// 0). Timing varies by network (localhost-fast vs. WAN); K/α are purely live-set
// derived.
func FeasibleParams(networkID uint32, n int) Parameters {
	k := n
	if k < 4 {
		k = 4 // minimal BFT committee (f=1); fail to a safe small set, never a dead large one
	}

	// α = strict-⅔ stake threshold (equal-stake closed form, = heaviest-first
	// general computation for unit weights), clamped up to the BFT overlap floor.
	alpha := EqualStakeSupermajorityThreshold(k)
	if floor := (Parameters{K: k}).bftQuorumFloor(); alpha < floor {
		alpha = floor
	}
	if alpha > k {
		alpha = k
	}

	blockTime, roundTO := liveTimingFor(networkID)

	p := DefaultParams() // inherits processing/parents/batch knobs
	p.K = k
	p.AlphaPreference = alpha
	p.AlphaConfidence = alpha
	// Float α (legacy Quasar-compat field) tracks the integer ratio but is
	// floored at the 0.66 minimum config.Valid() requires. α/K for the live nets
	// is ≥0.71 (4/5, 8/11, 15/21), comfortably above the floor.
	p.Alpha = float64(alpha) / float64(k)
	if p.Alpha < 0.66 {
		p.Alpha = 0.66
	}
	if p.Alpha > 1.0 {
		p.Alpha = 1.0
	}
	p.Beta = 2
	p.BetaVirtuous = 2
	if p.BetaRogue < k {
		p.BetaRogue = k // rogue confidence stays at/above the committee size
	}
	p.BlockTime = blockTime
	p.RoundTO = roundTO
	return p
}

// ValidateForLiveValueNetwork validates value/PoS parameters against the LIVE
// validator count — the live-aware companion to ValidateForValueNetwork. It is
// the manager's fail-closed backstop after FeasibleParams sizes K=liveN.
//
// Why a live-aware form exists: the static ValidateForValueNetwork enforces fixed
// tier floors (mainnet K≥11, testnet K≥5) that assume the network HAS that many
// validators. When the live set is smaller (e.g. 5 validators on mainnet), a
// fixed K≥11 floor makes quorum UNREACHABLE — the committee cannot sample
// validators that do not exist — so the floor stops protecting safety and instead
// causes a finality outage. The live-aware floor reads the tier minimum as an
// ASPIRATION capped by reality: you must sample at least min(tierFloor, liveN)
// validators. With K = liveN (FeasibleParams' choice) this always holds, while a
// genuinely tiny set still runs at K = liveN — the only safe committee available.
//
// The protocol SAFETY invariant is unchanged and still enforced: Valid() asserts
// the overlap bound 2α−K ≥ ⌊(K−1)/3⌋+1, and value nets still require f≥1 (K≥4).
// Only the decentralization-aspiration numeric floor becomes live-relative.
//
//	K==1            → rejected (single-validator is a separate explicit regime)
//	K<4 (f=0)       → rejected (no Byzantine tolerance)
//	K < min(tierFloor, liveN) → rejected (under-sized vs. the live set)
//	otherwise (overlap bound holds) → admitted
func (p Parameters) ValidateForLiveValueNetwork(networkID uint32, liveN int) error {
	// Valid() (overlap bound + α∈[0.66,1] + K≥1) is the protocol safety gate.
	if err := p.Valid(); err != nil {
		return err
	}
	// Value across independent parties requires at least single-fault tolerance.
	if p.ByzantineFaultTolerance() < 1 {
		return errKTooLowForValueLive(p, networkID)
	}
	// Tier aspiration, capped by the live set: you must sample at least the
	// smaller of the tier floor and however many validators actually exist.
	tierFloor := 0
	switch networkID {
	case constants.MainnetID:
		tierFloor = 11
	case constants.TestnetID:
		tierFloor = 5
	}
	if tierFloor > 0 {
		effective := tierFloor
		if liveN > 0 && liveN < tierFloor {
			effective = liveN
		}
		if p.K < effective {
			return errKBelowLiveFloor(p, networkID, liveN, effective)
		}
	}
	return nil
}
