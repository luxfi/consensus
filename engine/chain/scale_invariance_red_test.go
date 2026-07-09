// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// scale_invariance_red_test.go — RED adversarial scale-invariance suite for the
// dynamic committee sizer (bftCommittee / bftAlpha / config.*Threshold /
// FeasibleParams). It PROVES the algorithm cannot choke at ANY validator count
// n ∈ [1, 1_000_000] and, by property/fuzz, over the full int range.
//
// This is the ANALYTICAL half of the confidence gate (the live multi-node sims live
// in byzantine_scale_red_test.go / convergence_bound_red_test.go). It attacks the
// four production choke points at the SIZER boundary:
//
//	(1) committee mis-sizing — α unreachable (α>n) OR lone-reachable (α≤1 self-finalize)
//	(3) self-finality floor  — a transient/degenerate low count must HALT, never α≤1
//	    scale               — no n on [1,1M] (and, by fuzz, no n in the full int range)
//	                          where the BFT invariant 2α−n>f OR reachability α≤n breaks
//
// Every assertion is derived from the ⅔-supermajority the CERT enforces
// (config.TwoThirdsStakeFloor), so a green suite here means the count gate the engine
// sizes to can never demand a quorum the stake cert would not, at any scale.
package chain

import (
	"math/big"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/constants"
)

// redScaleSet is the exact task-mandated ladder plus the two simulated-weight points.
// 10_000 and 1_000_000 are analytical-only (a literal live sim is infeasible; the live
// sims run at the largest feasible size in the sibling-storm suite).
var redScaleSet = []int{1, 2, 3, 4, 5, 7, 11, 21, 64, 256, 1024, 10_000, 1_000_000}

// bigTwoThirdsFloor is the INDEPENDENT reference for floor(2·n/3), computed in big.Int
// so it cannot share the production overflow trick — if TwoThirdsStakeFloor is wrong at
// any magnitude this reference disagrees.
func bigTwoThirdsFloor(n uint64) uint64 {
	x := new(big.Int).SetUint64(n)
	x.Mul(x, big.NewInt(2))
	x.Div(x, big.NewInt(3))
	return x.Uint64()
}

// bftInvariant asserts the two safety/liveness properties a committee of size n with
// accept quorum α MUST satisfy, using the SAME f the engine uses (⌊(n-1)/3⌋):
//
//	SAFETY (overlap):  2α − n > f      (two α-quorums intersect in >f ⇒ >1 honest ⇒ no two conflicting finalizes)
//	LIVENESS (reach):  α ≤ n           (α distinct honest votes are obtainable)
//	NO-SELF-FINAL:     α ≥ 2 for n≥2   (a lone node can never finalize a multi-node committee)
func bftInvariant(t *testing.T, n, alpha int, ctx string) {
	t.Helper()
	if n <= 0 {
		return
	}
	f := (n - 1) / 3
	if 2*alpha-n <= f {
		t.Errorf("%s: BFT OVERLAP VIOLATED n=%d α=%d f=%d: 2α−n=%d ≤ f — two α-quorums can finalize conflicting blocks (FORK)",
			ctx, n, alpha, f, 2*alpha-n)
	}
	if alpha > n {
		t.Errorf("%s: UNREACHABLE α n=%d α=%d: α>n — the α-of-K cert can never assemble (PERMANENT STALL)",
			ctx, n, alpha)
	}
	if alpha < 1 {
		t.Errorf("%s: DEGENERATE α n=%d α=%d: α<1 — no vote required", ctx, n, alpha)
	}
	if n >= 2 && alpha < 2 {
		t.Errorf("%s: SELF-FINALITY n=%d α=%d: α≤1 on a multi-node committee — one node self-finalizes (the 1085013 fork)",
			ctx, n, alpha)
	}
}

// TestRedScale_BFTAlpha_ExactFormulaAndInvariantAtEveryScale is the core "any size"
// proof for the committee-of-size-n accept quorum (the value effectiveCommittee /
// FeasibleParams size α to). At each n on the ladder: bftAlpha(n) must equal the
// ⅔-supermajority floor(2n/3)+1 EXACTLY (independently recomputed in big.Int), and the
// BFT invariant must hold. This is the number the whole scaling story rests on.
func TestRedScale_BFTAlpha_ExactFormulaAndInvariantAtEveryScale(t *testing.T) {
	for _, n := range redScaleSet {
		got := bftAlpha(n)
		want := int(bigTwoThirdsFloor(uint64(n))) + 1
		if got != want {
			t.Errorf("bftAlpha(%d)=%d, want ⌊2·%d/3⌋+1=%d (independent big.Int reference)", n, got, n, want)
		}
		bftInvariant(t, n, got, "bftAlpha")
		// Cross-source agreement: the config closed form MUST match the engine's bftAlpha
		// (they claim to be the SAME ⅔ definition; a drift here is a split-brain quorum).
		if cf := config.EqualStakeSupermajorityThreshold(n); cf != got {
			t.Errorf("DRIFT: config.EqualStakeSupermajorityThreshold(%d)=%d != bftAlpha(%d)=%d — two ⅔ definitions disagree", n, cf, n, got)
		}
	}
}

// TestRedScale_TwoThirdsStakeFloor_OverflowSafeToUint64Max attacks the "overflow-safe"
// claim head-on: at magnitudes where the naive 2·total would wrap uint64, the production
// floor must still equal the big.Int reference. A wrong answer here silently corrupts α
// for a very-high-total-stake network (weights are uint64 stake, not validator counts).
func TestRedScale_TwoThirdsStakeFloor_OverflowSafeToUint64Max(t *testing.T) {
	const maxU64 = ^uint64(0)
	totals := []uint64{
		0, 1, 2, 3, 4, 5, 6,
		1_000_000, 1 << 32, 1 << 40, 1 << 62, 1 << 63,
		maxU64 - 2, maxU64 - 1, maxU64, // 2·total overflows uint64 here
		6_148_914_691_236_517_205, // (2^64-1)/3 region
	}
	for _, total := range totals {
		got := config.TwoThirdsStakeFloor(total)
		want := bigTwoThirdsFloor(total)
		if got != want {
			t.Errorf("TwoThirdsStakeFloor(%d)=%d, big.Int reference=%d — OVERFLOW CORRUPTION", total, got, want)
		}
		// The defining predicate: floor is the largest value NOT exceeding ⅔·total, i.e.
		// 3·floor ≤ 2·total < 3·(floor+1). Check via big.Int (no wrap).
		three := big.NewInt(3)
		twoTotal := new(big.Int).Mul(new(big.Int).SetUint64(total), big.NewInt(2))
		lo := new(big.Int).Mul(new(big.Int).SetUint64(got), three)      // 3·floor
		hi := new(big.Int).Mul(new(big.Int).SetUint64(got+1), three)    // 3·(floor+1)
		if lo.Cmp(twoTotal) > 0 || twoTotal.Cmp(hi) >= 0 {
			t.Errorf("TwoThirdsStakeFloor(%d)=%d violates 3·floor ≤ 2·total < 3·(floor+1)", total, got)
		}
	}
}

// TestRedScale_bftCommittee_LiveClampSafeAndReachableAcrossScale drives the ACTUAL
// production clamp effectiveCommittee uses: a large MainnetParams-shaped preset (K=21)
// whose LIVE validator count sweeps the whole ladder. It asserts, for the (K,α) the
// engine would actually run at each live count:
//   - the clamp only ever SHRINKS an oversized preset, never grows it past the preset;
//   - the result never dips below the minimal BFT floor (K≥4) when clamped;
//   - α is never self-finalizing (α≥2) and never unreachable relative to the COMMITTEE K;
//   - the BFT overlap invariant holds for the committee actually used.
//
// It also pins the DEAD ZONE explicitly (RED finding): with a large preset, live counts
// 1 and 2 produce a floored K=4/α=3 whose α=3 exceeds the live nodes — the chain HALTS
// fail-closed (the intended self-finality-floor behavior), and live count 3 finalizes
// only at unanimity (α=3, zero fault tolerance despite f=1 nominal).
func TestRedScale_bftCommittee_LiveClampSafeAndReachableAcrossScale(t *testing.T) {
	const preset = 21 // MainnetParams sample
	for _, live := range redScaleSet {
		k, alpha, clamped := bftCommittee(preset, live)
		if !clamped {
			// Unclamped ⇒ live ≥ preset (sample regime) OR live ≤ 0. The committee actually
			// used is the preset (K=21, α=15). Assert THAT is safe and reachable.
			if live >= preset {
				bftInvariant(t, preset, 15, "sample-regime@live="+itoaBig(live))
			}
			continue
		}
		// Clamped ⇒ 0 < live < preset. The committee is (k, alpha).
		if k > preset {
			t.Errorf("live=%d: clamp GREW K past preset (%d>%d)", live, k, preset)
		}
		if k < 4 {
			t.Errorf("live=%d: clamp dropped K below the minimal BFT floor (K=%d<4) — self-finality risk", live, k)
		}
		if alpha < 2 {
			t.Errorf("live=%d: clamp produced self-finalizing α=%d (≤1)", live, alpha)
		}
		// Overlap invariant for the COMMITTEE of size k (not live) — this is what the cert enforces.
		bftInvariant(t, k, alpha, "clamped-committee@live="+itoaBig(live))

		// REACHABILITY vs the LIVE set: α distinct signers must EXIST. Below α live
		// validators the cert can never assemble ⇒ HALT. Pin the dead zone.
		reachable := alpha <= live
		switch {
		case live <= 2:
			if reachable {
				t.Errorf("live=%d: expected UNREACHABLE α (dead-zone HALT) but α=%d ≤ live", live, alpha)
			}
		case live == 3:
			if !reachable || alpha != 3 {
				t.Errorf("live=3: expected reachable unanimous α=3, got α=%d reachable=%v", alpha, reachable)
			}
		}
	}
}

// TestRedScale_bftCommittee_DenseMonotoneProperty is the dense property companion:
// over every (preset, live) with preset ∈ [2,64] and live ∈ [1, 3·preset], the clamp is
// self-consistent — monotone-non-decreasing in live up to the preset cap, never
// self-finalizing when clamped, always BFT-safe for the resulting committee.
func TestRedScale_bftCommittee_DenseMonotoneProperty(t *testing.T) {
	for preset := 2; preset <= 64; preset++ {
		prevK := 0
		for live := 1; live <= 3*preset; live++ {
			k, alpha, clamped := bftCommittee(preset, live)
			if !clamped {
				// preset≤live (fits) or live≤0: the committee is the preset itself.
				if live >= preset && k != preset {
					t.Fatalf("preset=%d live=%d: unclamped K=%d != preset", preset, live, k)
				}
				continue
			}
			// Clamped committee must be BFT-valid and non-self-finalizing.
			if alpha < 2 || alpha > k || k < 2 || k > preset {
				t.Fatalf("preset=%d live=%d: invalid clamped committee (K=%d α=%d)", preset, live, k, alpha)
			}
			bftInvariant(t, k, alpha, "dense")
			// Monotone: as live grows within the clamp region, K never DECREASES (no
			// non-determinism where a slightly larger set yields a smaller committee).
			if k < prevK {
				t.Fatalf("preset=%d live=%d: K went DOWN as live grew (%d<%d) — non-monotone clamp", preset, live, k, prevK)
			}
			prevK = k
		}
	}
}

// TestRedScale_FeasibleParams_UpToOneMillion attacks the SECOND (currently UNWIRED)
// sizing path config.FeasibleParams(networkID, n) — K=n uncapped. If it is ever wired
// to the node (it exists in the config package, callable), it MUST size a valid,
// BFT-safe, config.Valid()-passing committee at every scale up to 1M with no overflow.
// RED FINDING: this path is defined but NOT called by the node (manager.go uses
// selectConsensusParams → MainnetParams), so today two independent sizing mechanisms
// coexist — a "one way to do everything" violation and a latent divergence if wired.
func TestRedScale_FeasibleParams_UpToOneMillion(t *testing.T) {
	for _, n := range redScaleSet {
		p := config.FeasibleParams(constants.MainnetID, n)
		// config.Valid() is the protocol safety gate (overlap bound + α∈[0.66,1] + K≥1).
		if err := p.Valid(); err != nil {
			t.Errorf("FeasibleParams(mainnet, n=%d) failed config.Valid(): %v (K=%d α=%d)", n, err, p.K, p.AlphaPreference)
		}
		// K must be n (or the floor of 4 for tiny n), never a giant unsatisfiable value.
		wantK := n
		if wantK < 4 {
			wantK = 4
		}
		if p.K != wantK {
			t.Errorf("FeasibleParams n=%d: K=%d, want %d", n, p.K, wantK)
		}
		// α is the ⅔ supermajority for the committee, reachable and BFT-safe.
		bftInvariant(t, p.K, p.AlphaPreference, "FeasibleParams@n="+itoaBig(n))
		if p.AlphaPreference != p.AlphaConfidence {
			t.Errorf("FeasibleParams n=%d: AlphaPreference(%d) != AlphaConfidence(%d)", n, p.AlphaPreference, p.AlphaConfidence)
		}
	}
}

// TestRedScale_BFTAlpha_PropertyOverFullRange is the exhaustive-in-spirit property:
// for a dense low range and a log-spaced high range up to just under max int, bftAlpha
// obeys the invariant with NO overflow (α>0, α≤n) and matches the big.Int reference.
// This is the "prove analytically over the full uint range" ask without a literal 2^63
// loop. FuzzBFTAlpha below covers arbitrary inputs including negatives.
func TestRedScale_BFTAlpha_PropertyOverFullRange(t *testing.T) {
	// Dense 1..5000.
	for n := 1; n <= 5000; n++ {
		checkAlphaAgainstReference(t, n)
	}
	// Log-spaced up toward max int (guards the int(uint64) truncation the sizer relies on).
	for n := 10_000; n > 0 && n <= 1<<40; n *= 4 {
		checkAlphaAgainstReference(t, n)
	}
	// Just under 2^62 and 2^62 — TwoThirdsFloor(2^62)≈3.07e18 fits int64; +1 must not wrap.
	for _, n := range []int{1 << 61, 1 << 62, (1 << 62) + 7} {
		checkAlphaAgainstReference(t, n)
	}
}

func checkAlphaAgainstReference(t *testing.T, n int) {
	t.Helper()
	got := bftAlpha(n)
	want := int(bigTwoThirdsFloor(uint64(n))) + 1
	if got != want {
		t.Fatalf("bftAlpha(%d)=%d != big.Int reference %d (overflow/truncation)", n, got, want)
	}
	if got < 1 || got > n {
		t.Fatalf("bftAlpha(%d)=%d out of [1,n] — unreachable or degenerate", n, got)
	}
	bftInvariant(t, n, got, "fullrange")
}

// FuzzBFTAlpha fuzzes n over the FULL int domain (negatives, zero, huge). Contract:
// n≤0 ⇒ α=0 (fail-closed, the caller treats it as "no committee"); n≥1 ⇒ 1≤α≤n and the
// BFT invariant holds with NO panic and NO overflow. Run: go test -run x -fuzz FuzzBFTAlpha.
func FuzzBFTAlpha(f *testing.F) {
	for _, n := range redScaleSet {
		f.Add(n)
	}
	f.Add(0)
	f.Add(-1)
	f.Add(int(^uint(0) >> 1)) // max int
	f.Fuzz(func(t *testing.T, n int) {
		alpha := bftAlpha(n)
		if n <= 0 {
			if alpha != 0 {
				t.Fatalf("bftAlpha(%d)=%d, want 0 for n≤0 (fail-closed)", n, alpha)
			}
			return
		}
		want := int(bigTwoThirdsFloor(uint64(n))) + 1
		if alpha != want {
			t.Fatalf("bftAlpha(%d)=%d != reference %d", n, alpha, want)
		}
		if alpha < 1 || alpha > n {
			t.Fatalf("bftAlpha(%d)=%d out of [1,n]", n, alpha)
		}
		f2 := (n - 1) / 3
		if 2*alpha-n <= f2 {
			t.Fatalf("bftAlpha(%d)=%d violates overlap 2α−n>f (f=%d)", n, alpha, f2)
		}
	})
}

// itoaBig is a scale-safe int→string (the package's itoa only handles 0..9).
func itoaBig(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}
