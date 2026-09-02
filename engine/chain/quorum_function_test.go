// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"testing"

	"github.com/luxfi/consensus/config"
)

// TestNovaQuorum_Table pins the Nova-tier thresholds at n∈{1..5} — the "scale
// smoothly from 1" contract for β-ignition / local-execution acceptance.
func TestNovaQuorum_Table(t *testing.T) {
	want := map[int][3]int{ // n → {NovaQuorum, CrashTolerance, NovaBeta}
		1: {1, 0, 1}, // lone node self-ignites; no fault tolerance
		2: {2, 0, 2}, // both must agree; any drop pauses Nova
		3: {2, 1, 2}, // majority 2 → tolerate 1 crash
		4: {3, 1, 2}, // majority 3 → tolerate 1 crash
		5: {3, 2, 2}, // majority 3 → tolerate 2 crashes (the deploy)
	}
	for n, w := range want {
		if got := NovaQuorum(n); got != w[0] {
			t.Errorf("n=%d NovaQuorum=%d want %d", n, got, w[0])
		}
		if got := CrashTolerance(n); got != w[1] {
			t.Errorf("n=%d CrashTolerance=%d want %d", n, got, w[1])
		}
		if got := NovaBeta(n); got != w[2] {
			t.Errorf("n=%d NovaBeta=%d want %d", n, got, w[2])
		}
	}
}

// TestNovaQuorum_MajorityOverlap_CrashSafe: two Nova majorities always intersect
// (2·NovaQuorum(n) > n), so — with no equivocation — no two conflicting blocks can
// both ignite to Nova. Holds for every n.
func TestNovaQuorum_MajorityOverlap_CrashSafe(t *testing.T) {
	for n := 1; n <= 1000; n++ {
		if 2*NovaQuorum(n) <= n {
			t.Fatalf("n=%d: majority overlap broken (2·%d ≤ %d) — Nova is not crash-safe",
				n, NovaQuorum(n), n)
		}
	}
}

// TestNova_Below_Quasar proves the two rungs are DISTINCT authorization tiers and are
// never collapsed: Nova (local, majority) never requires MORE than Quasar (export,
// ⅔-stake), and sits STRICTLY below it wherever majority < ⅔ (n=3, n=5, and generally).
// The Quasar threshold is read from its ONE definition (config), never re-expressed.
func TestNova_Below_Quasar(t *testing.T) {
	strictlySeparatedAt := map[int]bool{3: true, 5: true} // deploy cases we assert are strict
	for n := 1; n <= 1000; n++ {
		quasar := config.TwoThirdsCount(n) // ⌊2n/3⌋+1, the single ⅔ definition
		nova := NovaQuorum(n)
		if nova > quasar {
			t.Fatalf("n=%d: Nova majority %d exceeds Quasar ⅔ %d — tiers collapsed", n, nova, quasar)
		}
		if strictlySeparatedAt[n] && nova >= quasar {
			t.Fatalf("n=%d: Nova %d not strictly below Quasar %d — no distinct local vs export boundary",
				n, nova, quasar)
		}
	}
	// The deploy case, spelled out: n=5 tolerates 2 crashes at Nova (3-of-5) but the
	// exportable Quasar cert still needs 4-of-5 — the whole reason to keep them apart.
	if NovaQuorum(5) != 3 || config.TwoThirdsCount(5) != 4 {
		t.Fatalf("n=5 deploy: Nova=%d Quasar=%d want 3 and 4", NovaQuorum(5), config.TwoThirdsCount(5))
	}
}

// TestFinalityLadder: the ladder is strictly ordered and the authorization boundaries
// are exactly Nova (local) / Quasar (export) / Horizon (irreversible) — Photon and Wave
// authorize nothing. This is the anti-semantic-collapse contract as code.
func TestFinalityLadder(t *testing.T) {
	if !(Photon < Wave && Wave < Nova && Nova < Quasar && Quasar < Horizon) {
		t.Fatal("finality ladder is not strictly ordered Photon<Wave<Nova<Quasar<Horizon")
	}
	for _, f := range []Finality{Photon, Wave} {
		if f.AuthorizesLocalExecution() || f.AuthorizesExport() || f.AuthorizesIrreversibleSettlement() {
			t.Fatalf("%s must authorize nothing (pre-ignition)", f)
		}
	}
	if !Nova.AuthorizesLocalExecution() || Nova.AuthorizesExport() {
		t.Fatal("Nova must authorize LOCAL execution only — never export")
	}
	if !Quasar.AuthorizesExport() || Quasar.AuthorizesIrreversibleSettlement() {
		t.Fatal("Quasar must authorize export — but not irreversible settlement")
	}
	if !Horizon.AuthorizesIrreversibleSettlement() {
		t.Fatal("Horizon must authorize irreversible settlement")
	}
	// Names are the lowercase ontology verbatim (RPC/metrics contract).
	for f, name := range map[Finality]string{Photon: "photon", Wave: "wave", Nova: "nova", Quasar: "quasar", Horizon: "horizon"} {
		if f.String() != name {
			t.Errorf("%d.String()=%q want %q", f, f.String(), name)
		}
	}
}
