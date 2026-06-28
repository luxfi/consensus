// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"math"
	"math/big"
	"testing"
)

// TestCaptureProbability_Exact pins the per-committee capture probability to its
// exact closed form, so a regression in the binomial math is caught to the bit.
//
//	n=8, t=7, f=1/3:  p = C(8,7)(1/3)^7(2/3) + C(8,8)(1/3)^8 = 16/6561 + 1/6561 = 17/6561
//	n=8, t=8, f=1/3:  p = (1/3)^8                            = 1/6561
func TestCaptureProbability_Exact(t *testing.T) {
	if got := CommitteeCaptureProbability(8, 7, 1, 3); got.Cmp(big.NewRat(17, 6561)) != 0 {
		t.Fatalf("p(n=8,t=7,f=1/3) = %s, want 17/6561", got.RatString())
	}
	if got := CommitteeCaptureProbability(8, 8, 1, 3); got.Cmp(big.NewRat(1, 6561)) != 0 {
		t.Fatalf("p(n=8,t=8,f=1/3) = %s, want 1/6561", got.RatString())
	}
	// Degenerate guards.
	if got := CommitteeCaptureProbability(8, 9, 1, 3); got.Sign() != 0 {
		t.Fatalf("p with t>n must be 0, got %s", got.RatString())
	}
	if got := CommitteeCaptureProbability(8, 0, 1, 3); got.Cmp(big.NewRat(1, 1)) != 0 {
		t.Fatalf("p with t<=0 must be 1, got %s", got.RatString())
	}
}

// TestAllOfR_EqualsPowExactly asserts the all-of-r special case m=r reduces to
// p^r EXACTLY — the cleanest invariant of the two-stage binomial. This is the
// Pulsar analogue of Avalanche's β confidence as pure repetition.
func TestAllOfR_EqualsPowExactly(t *testing.T) {
	p := CommitteeCaptureProbability(8, 7, 1, 3) // 17/6561
	for _, r := range []int{1, 3, 5, 8} {
		want := ratPow(p, r)
		got := SampledFailureProbability(p, r, r) // m = r
		if got.Cmp(want) != 0 {
			t.Fatalf("all-of-r mismatch r=%d: P_fail=%s want p^r=%s", r, got.RatString(), want.RatString())
		}
	}
}

// TestDefaultParams_FailureProbability checks the production default
// Pulsar-HYBRID-PQ-v1 (n=8,t=7,m=12,r=8,f=1/3) hits the documented ~2^-59.8
// finality-forgery bound, and that the per-committee capture is ~2^-8.6.
func TestDefaultParams_FailureProbability(t *testing.T) {
	capBits := PulsarHybridPQv1.CaptureBits()
	if capBits < 8.4 || capBits > 8.8 {
		t.Fatalf("default capture bits = %.3f, want ~8.6", capBits)
	}
	secBits := PulsarHybridPQv1.SecurityBits()
	if secBits < 58.0 || secBits > 62.0 {
		t.Fatalf("default r-of-m security bits = %.3f, want ~59.8", secBits)
	}
	t.Logf("Pulsar-HYBRID-PQ-v1: per-committee p ≈ 2^-%.2f, r-of-m P_fail ≈ 2^-%.2f", capBits, secBits)
}

// TestLivenessSlackCostsSecurity asserts m>r is STRICTLY weaker than all-of-r
// (m=r): the verifier accepting any r of m committees buys m−r committees of
// liveness slack at a precisely-quantified security cost.
func TestLivenessSlackCostsSecurity(t *testing.T) {
	p := PulsarHybridPQv1.CaptureProbability()
	allOfR := SampledFailureProbability(p, 8, 8)  // m=r=8 → p^8
	slack := SampledFailureProbability(p, 12, 8)  // m=12, r=8
	if slack.Cmp(allOfR) <= 0 {
		t.Fatalf("expected P_fail(m=12,r=8) > P_fail(m=8,r=8); got %s vs %s", slack.RatString(), allOfR.RatString())
	}
	allBits := -ratLog2(allOfR)
	slackBits := -ratLog2(slack)
	if allBits < 67.0 || allBits > 71.0 {
		t.Fatalf("all-of-r (p^8) bits = %.3f, want ~68.9", allBits)
	}
	t.Logf("all-of-r m=r=8: 2^-%.2f ; with m=12 slack: 2^-%.2f (cost %.2f bits)", allBits, slackBits, allBits-slackBits)
}

// TestFailureMonotonicity: P_fail decreases in r (harder to capture more) and
// increases in m (more committees to capture r of), for a fixed p.
func TestFailureMonotonicity(t *testing.T) {
	p := CommitteeCaptureProbability(8, 7, 1, 3)
	// Decreasing in r at fixed m.
	prev := SampledFailureProbability(p, 12, 1)
	for r := 2; r <= 12; r++ {
		cur := SampledFailureProbability(p, 12, r)
		if cur.Cmp(prev) > 0 {
			t.Fatalf("P_fail must be non-increasing in r: r=%d %s > r=%d %s", r, cur.RatString(), r-1, prev.RatString())
		}
		prev = cur
	}
	// Increasing in m at fixed r.
	prev = SampledFailureProbability(p, 8, 8)
	for m := 9; m <= 16; m++ {
		cur := SampledFailureProbability(p, m, 8)
		if cur.Cmp(prev) < 0 {
			t.Fatalf("P_fail must be non-decreasing in m: m=%d %s < m=%d %s", m, cur.RatString(), m-1, prev.RatString())
		}
		prev = cur
	}
}

// TestAlternativeFamilies tabulates the alternative parameter families and
// asserts their EXACT per-committee capture bits and all-of-r security bits.
//
// The expected figures are hand-derived from the closed form and confirmed by
// this exact-rational tool. For n=16 they SUPERSEDE the rougher (and, for n=16,
// incorrect) estimates that circulated in the design notes: e.g. n=16,t=12 is
// p = 34113/3^16 ≈ 2^-10.30 (not 2^-7.9), and n=16,t=14 is p = 513/3^16 ≈
// 2^-16.36 (not 2^-13.5) — the n=16 families are STRONGER per committee than the
// notes claimed. The n=8 figures match the notes (cross-check). all-of-r bits =
// r · capBits exactly (proven independently by TestAllOfR_EqualsPowExactly).
func TestAlternativeFamilies(t *testing.T) {
	type fam struct {
		name           string
		n, tt, r       int
		wantCapBits    float64 // ±0.2 (exact closed form)
		wantAllOfRBits float64 // ±0.3 (= r·capBits)
	}
	fams := []fam{
		{"n8_t7_r8", 8, 7, 8, 8.59, 68.74},
		{"n8_t8_r5", 8, 8, 5, 12.68, 63.40},
		{"n16_t12_r6", 16, 12, 6, 10.30, 61.81},
		{"n16_t14_r4", 16, 14, 4, 16.36, 65.43},
		{"n16_t14_r5", 16, 14, 5, 16.36, 81.78},
	}
	for _, f := range fams {
		p := CommitteeCaptureProbability(f.n, f.tt, 1, 3)
		capBits := -ratLog2(p)
		allOfR := -ratLog2(ratPow(p, f.r)) // m=r
		t.Logf("%-12s n=%-2d t=%-2d r=%-2d : per-committee 2^-%.2f, all-of-r 2^-%.2f",
			f.name, f.n, f.tt, f.r, capBits, allOfR)
		if math.Abs(capBits-f.wantCapBits) > 0.2 {
			t.Errorf("%s capture bits = %.3f, want ~%.2f", f.name, capBits, f.wantCapBits)
		}
		if math.Abs(allOfR-f.wantAllOfRBits) > 0.3 {
			t.Errorf("%s all-of-r bits = %.3f, want ~%.2f", f.name, allOfR, f.wantAllOfRBits)
		}
	}
}

// TestBinomialExact spot-checks the exact integer binomial against known values.
func TestBinomialExact(t *testing.T) {
	cases := map[[2]int]int64{
		{8, 7}: 8, {8, 8}: 1, {12, 8}: 495, {16, 12}: 1820, {16, 14}: 120,
		{8, 0}: 1, {8, 9}: 0, {8, -1}: 0,
	}
	for nk, want := range cases {
		if got := binomial(nk[0], nk[1]); got.Int64() != want {
			t.Fatalf("C(%d,%d) = %d, want %d", nk[0], nk[1], got.Int64(), want)
		}
	}
}
