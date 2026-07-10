// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import "testing"

// TestQuorumTable_SmallNets pins the exact two-tier quorum table the v1.36 deploy
// requires at n∈{1..5} — the must-pass "scale smoothly from 1" contract.
func TestQuorumTable_SmallNets(t *testing.T) {
	// n → {liveQ, certQ, crashTol, byzTol, beta}
	want := map[int][5]int{
		1: {1, 1, 0, 0, 1}, // single node self-finalizes; no fault tolerance
		2: {2, 2, 0, 0, 2}, // both must agree; any drop pauses
		3: {2, 3, 1, 0, 2}, // majority 2 → tolerate 1 crash; ⅔ cert needs all 3
		4: {3, 3, 1, 1, 2}, // majority 3 → tolerate 1; majority == ⅔ here
		5: {3, 4, 2, 1, 2}, // majority 3 → tolerate 2 crashes; ⅔ cert needs 4
	}
	for n, w := range want {
		if got := LiveQuorum(n); got != w[0] {
			t.Errorf("n=%d LiveQuorum=%d want %d", n, got, w[0])
		}
		if got := CertQuorum(n); got != w[1] {
			t.Errorf("n=%d CertQuorum=%d want %d", n, got, w[1])
		}
		if got := CrashTolerance(n); got != w[2] {
			t.Errorf("n=%d CrashTolerance=%d want %d", n, got, w[2])
		}
		if got := ByzantineTolerance(n); got != w[3] {
			t.Errorf("n=%d ByzantineTolerance=%d want %d", n, got, w[3])
		}
		if got := LiveBeta(n); got != w[4] {
			t.Errorf("n=%d LiveBeta=%d want %d", n, got, w[4])
		}
	}
}

// TestQuorum_MajorityOverlap_CrashSafe proves the crash-fault safety invariant for
// LiveQuorum: any TWO majorities of n intersect (2·LiveQuorum(n) > n), so — with no
// equivocation — no two conflicting blocks can both reach a majority. Holds for every n.
func TestQuorum_MajorityOverlap_CrashSafe(t *testing.T) {
	for n := 1; n <= 1000; n++ {
		if 2*LiveQuorum(n) <= n {
			t.Fatalf("n=%d: majority overlap broken (2·%d ≤ %d) — LiveQuorum is not crash-safe",
				n, LiveQuorum(n), n)
		}
	}
}

// TestQuorum_CertOverlap_ByzantineSafe proves the ⅔ BFT invariant for CertQuorum: any
// two ⅔-quorums intersect in MORE than ByzantineTolerance(n) validators
// (2·CertQuorum(n) − n > f), so an equivocator set of size ≤ f can never make two
// conflicting certs. This is why the trailing cert is the Byzantine-safe receipt.
func TestQuorum_CertOverlap_ByzantineSafe(t *testing.T) {
	for n := 1; n <= 1000; n++ {
		overlap := 2*CertQuorum(n) - n
		if overlap <= ByzantineTolerance(n) {
			t.Fatalf("n=%d: ⅔ overlap %d ≤ f=%d — CertQuorum is not Byzantine-safe",
				n, overlap, ByzantineTolerance(n))
		}
	}
}

// TestQuorum_CertAtLeastLive: the trailing cert is never weaker than liveness
// acceptance (CertQuorum ≥ LiveQuorum), so a certified block was always Nova-accepted.
func TestQuorum_CertAtLeastLive(t *testing.T) {
	for n := 1; n <= 1000; n++ {
		if CertQuorum(n) < LiveQuorum(n) {
			t.Fatalf("n=%d: CertQuorum %d < LiveQuorum %d", n, CertQuorum(n), LiveQuorum(n))
		}
	}
}

// TestQuorum_CrashTolerance_IsMax confirms Nova tolerates the MAXIMUM crash faults for
// each n: CrashTolerance(n) alive-minus-quorum equals ⌈n/2⌉−1, and n−CrashTolerance
// still meets the majority (a net at exactly its tolerance still finalizes).
func TestQuorum_CrashTolerance_IsMax(t *testing.T) {
	for n := 1; n <= 1000; n++ {
		alive := n - CrashTolerance(n)
		if alive < LiveQuorum(n) {
			t.Fatalf("n=%d: at %d crashes only %d alive < majority %d — over-claimed tolerance",
				n, CrashTolerance(n), alive, LiveQuorum(n))
		}
		// One more crash must DROP below majority (tolerance is exactly maximal).
		if CrashTolerance(n) > 0 && (n-CrashTolerance(n)-1) >= LiveQuorum(n) {
			t.Fatalf("n=%d: tolerance %d is not maximal — %d alive still ≥ majority %d",
				n, CrashTolerance(n), n-CrashTolerance(n)-1, LiveQuorum(n))
		}
	}
}

// TestQuorum_ScalesFromOne: the functions are total and monotonic non-decreasing from
// n=1 up (no panics, no regressions) — the "smooth 1→N" property.
func TestQuorum_ScalesFromOne(t *testing.T) {
	prevLive, prevCert := 0, 0
	for n := 1; n <= 1000; n++ {
		l, c := LiveQuorum(n), CertQuorum(n)
		if l < prevLive || c < prevCert {
			t.Fatalf("n=%d: quorum regressed (live %d<%d or cert %d<%d)", n, l, prevLive, c, prevCert)
		}
		if l < 1 || c < 1 {
			t.Fatalf("n=%d: non-positive quorum (live=%d cert=%d)", n, l, c)
		}
		prevLive, prevCert = l, c
	}
}
