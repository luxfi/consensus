// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// pulsar_sampled_security.go — the EXACT r-of-m sampled-certificate security
// tool. It answers the one question the whole construction rests on: given an
// adversary holding a stake fraction f, what is the probability it can forge a
// Pulsar sampled finality certificate?
//
// # The two-stage binomial
//
// A sampled cert forges iff the adversary captures ≥ r of the m sampled
// committees, where "capture" of one committee means controlling ≥ t of its n
// members (then it can produce that committee's t-of-n group signature). With
// the conservative i.i.d.-Binomial model (the true draw is stake-weighted
// WITHOUT replacement, which is strictly harder to capture):
//
//	per-committee capture   p      = Pr[X ≥ t],  X ~ Binom(n, f)
//	                               = Σ_{i=t..n} C(n,i) f^i (1-f)^(n-i)
//	r-of-m forgery          P_fail = Pr[Y ≥ r],  Y ~ Binom(m, p)
//	                               = Σ_{j=r..m} C(m,j) p^j (1-p)^(m-j)
//
// This is the Pulsar equivalent of Avalanche's β confidence accumulation:
// security comes from the REPETITION count r against the per-committee capture
// p, NOT from any single committee's threshold. The all-of-r special case m = r
// gives P_fail = p^r exactly; m > r trades a precisely-quantified security cost
// for m−r committees of liveness slack.
//
// # Exact arithmetic
//
// Every probability is computed in exact rational arithmetic (math/big.Rat) — no
// floating-point rounding anywhere in the probability. Only the final
// human-facing "security bits" (−log₂ P_fail) uses floating point, and that via
// a big.Float mantissa/exponent split so it stays accurate for probabilities far
// below the float64 underflow floor (e.g. 2⁻²⁰⁰).
//
// # Decomplected
//
// This file owns ONLY the security math. It is a sizing/audit tool: it picks
// nothing and verifies nothing. The verifier (pulsar_sampled_verify.go) enforces
// the r-of-m count; this file tells an operator what that r BUYS for a given
// (n, t, m, f). The default parameter set lives with the sortition
// (PulsarHybridPQv1, pulsar_sortition.go); this file evaluates it.
package quasar

import (
	"math"
	"math/big"
)

// CommitteeCaptureProbability returns the EXACT per-committee capture
// probability p — the probability that an adversary holding fraction f =
// fNum/fDen of stake controls ≥ t of a committee's n members, modelled as
// Binom(n, f):
//
//	p = Σ_{i=t..n} C(n,i) f^i (1-f)^(n-i).
//
// Conservative: the real selection is stake-weighted WITHOUT replacement, which
// makes capturing ≥ t strictly less likely than this i.i.d. bound. Returns a
// big.Rat in [0, 1]. t ≤ 0 yields 1 (the whole distribution); t > n yields 0.
func CommitteeCaptureProbability(n, t int, fNum, fDen uint64) *big.Rat {
	f := big.NewRat(int64(fNum), int64(fDen))
	g := new(big.Rat).Sub(big.NewRat(1, 1), f) // 1 - f
	lo := t
	if lo < 0 {
		lo = 0
	}
	sum := new(big.Rat)
	for i := lo; i <= n; i++ {
		term := new(big.Rat).SetInt(binomial(n, i)) // C(n,i)
		term.Mul(term, ratPow(f, i))                // · f^i
		term.Mul(term, ratPow(g, n-i))              // · (1-f)^(n-i)
		sum.Add(sum, term)
	}
	return sum
}

// SampledFailureProbability returns the EXACT r-of-m finality-forgery
// probability given a per-committee capture probability p:
//
//	P_fail = Σ_{j=r..m} C(m,j) p^j (1-p)^(m-j).
//
// This is the EXACT binomial tail, NOT the p^r all-of-r approximation. For m = r
// it equals p^r exactly; for m > r it is strictly larger (the attacker needs
// only r of m), the exact price of m−r committees of liveness slack. r ≤ 0
// yields 1; r > m yields 0.
func SampledFailureProbability(p *big.Rat, m, r int) *big.Rat {
	q := new(big.Rat).Sub(big.NewRat(1, 1), p) // 1 - p
	lo := r
	if lo < 0 {
		lo = 0
	}
	sum := new(big.Rat)
	for j := lo; j <= m; j++ {
		term := new(big.Rat).SetInt(binomial(m, j)) // C(m,j)
		term.Mul(term, ratPow(p, j))                // · p^j
		term.Mul(term, ratPow(q, m-j))              // · (1-p)^(m-j)
		sum.Add(sum, term)
	}
	return sum
}

// CaptureProbability evaluates the per-committee capture probability p for this
// parameter set (n=N, t=T, f=FMaxNum/FMaxDen).
func (sp SortitionParams) CaptureProbability() *big.Rat {
	return CommitteeCaptureProbability(int(sp.N), int(sp.T), uint64(sp.FMaxNum), uint64(sp.FMaxDen))
}

// FailureProbability evaluates the r-of-m finality-forgery probability P_fail
// for this parameter set: SampledFailureProbability(CaptureProbability(), M, R).
func (sp SortitionParams) FailureProbability() *big.Rat {
	return SampledFailureProbability(sp.CaptureProbability(), int(sp.M), int(sp.R))
}

// CaptureBits returns the per-committee capture security level −log₂ p (larger
// is safer). A committee with p = 2⁻⁸·⁶ returns ≈ 8.6.
func (sp SortitionParams) CaptureBits() float64 {
	return -ratLog2(sp.CaptureProbability())
}

// SecurityBits returns the r-of-m finality-forgery security level −log₂ P_fail
// (larger is safer). The default Pulsar-HYBRID-PQ-v1 set returns ≈ 59.8.
func (sp SortitionParams) SecurityBits() float64 {
	return -ratLog2(sp.FailureProbability())
}

// ----------------------------------------------------------------------------
// Exact helpers — integer binomials, rational powers, and a precise rational
// log₂ that does not underflow on tiny probabilities.
// ----------------------------------------------------------------------------

// binomial returns C(n, k) as an exact big.Int via the multiplicative formula
// (no factorial overflow). C(n,k)=0 for k<0 or k>n.
func binomial(n, k int) *big.Int {
	if k < 0 || k > n {
		return big.NewInt(0)
	}
	if k > n-k {
		k = n - k // symmetry: fewer multiplications
	}
	res := big.NewInt(1)
	num := new(big.Int)
	den := new(big.Int)
	for i := 0; i < k; i++ {
		res.Mul(res, num.SetInt64(int64(n-i)))
		res.Div(res, den.SetInt64(int64(i+1)))
	}
	return res
}

// ratPow returns base^exp for a non-negative integer exponent, exactly. exp ≤ 0
// returns 1.
func ratPow(base *big.Rat, exp int) *big.Rat {
	res := big.NewRat(1, 1)
	for i := 0; i < exp; i++ {
		res.Mul(res, base)
	}
	return res
}

// ratLog2 returns log₂(r) for a positive rational r, accurately even when r is
// far below the float64 underflow floor. It splits r = mant · 2^exp with mant ∈
// [0.5, 1) (via big.Float.MantExp), takes log₂ of the safely-representable
// mantissa, and adds the exact integer exponent. r ≤ 0 returns −∞ / NaN.
func ratLog2(r *big.Rat) float64 {
	switch r.Sign() {
	case 0:
		return math.Inf(-1)
	case -1:
		return math.NaN()
	}
	bf := new(big.Float).SetPrec(256).SetRat(r)
	mant := new(big.Float).SetPrec(256)
	exp := bf.MantExp(mant) // r = mant · 2^exp, mant ∈ [0.5, 1)
	m64, _ := mant.Float64()
	return math.Log2(m64) + float64(exp)
}
