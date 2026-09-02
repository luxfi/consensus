// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// ladder_floor_test.go — the rungs and the Nova floors, at their boundaries.
//
// Two things live here. The LADDER's names and its three authorization
// predicates: the names are what RPC status and metrics carry verbatim, so a
// rename is an interface break and not a cosmetic one, and the predicates are
// the boundary a bridge gates on. And the NOVA floors as functions of the live
// count n — read at n = 0 and n = 1, where the whole reason they are written as
// functions and not as constants is that a transiently-empty validator view must
// not be allowed to self-accept.
package chain

import "testing"

// TestTheLadderNamesAreTheOntology holds each rung's name to the lowercase
// astrophysics word. These strings are the RPC status and the metric label; a
// generic finality word appearing here is a change every consumer sees.
func TestTheLadderNamesAreTheOntology(t *testing.T) {
	for _, row := range []struct {
		rung Finality
		name string
	}{
		{Photon, "photon"},
		{Wave, "wave"},
		{Nova, "nova"},
		{Quasar, "quasar"},
		{Horizon, "horizon"},
	} {
		if got := row.rung.String(); got != row.name {
			t.Fatalf("rung %d is %q, want %q", row.rung, got, row.name)
		}
	}
	// A rung off the ladder names itself as such rather than as the last rung it
	// happens to sit above — a wire-decoded byte out of range must not print as
	// "horizon" and read as irreversible.
	if got := Finality(200).String(); got != "unknown" {
		t.Fatalf("a rung off the ladder is %q, want %q", got, "unknown")
	}
}

// TestTheAuthorizationBoundariesAreDistinct is THE INVARIANT read as a table:
// each of the three predicates fires at exactly one rung and not one rung lower.
// Collapsing any two of them is how a block below Quasar leaves the chain.
func TestTheAuthorizationBoundariesAreDistinct(t *testing.T) {
	for _, row := range []struct {
		rung                        Finality
		local, export, irreversible bool
	}{
		{Photon, false, false, false},
		{Wave, false, false, false},
		{Nova, true, false, false},
		{Quasar, true, true, false},
		{Horizon, true, true, true},
	} {
		if got := row.rung.AuthorizesLocalExecution(); got != row.local {
			t.Fatalf("%s authorizes local execution = %v, want %v", row.rung, got, row.local)
		}
		if got := row.rung.AuthorizesExport(); got != row.export {
			t.Fatalf("%s authorizes export = %v, want %v", row.rung, got, row.export)
		}
		if got := row.rung.AuthorizesIrreversibleSettlement(); got != row.irreversible {
			t.Fatalf("%s authorizes irreversible settlement = %v, want %v", row.rung, got, row.irreversible)
		}
	}
}

// TestNovaQuorumNeverReturnsZero is the 1085013 guard stated directly: an
// unresolved or empty validator view reports n<1, and a quorum of zero would let
// a node whose set momentarily read empty accept its own block. The floor is one.
func TestNovaQuorumNeverReturnsZero(t *testing.T) {
	for _, n := range []int{-1000, -1, 0} {
		if q := NovaQuorum(n); q != 1 {
			t.Fatalf("NovaQuorum(%d) = %d, want 1 — an unresolved set must never quorum at zero", n, q)
		}
	}
	for _, row := range []struct{ n, want int }{
		{1, 1}, {2, 2}, {3, 2}, {4, 3}, {5, 3}, {6, 4}, {21, 11},
	} {
		if q := NovaQuorum(row.n); q != row.want {
			t.Fatalf("NovaQuorum(%d) = %d, want %d", row.n, q, row.want)
		}
	}
}

// TestNovaSignerFloorSaturates holds the difference between the two rungs'
// count floors. Nova's asks only "is this more than one party?" before a block
// may drive local execution the chain can still reorg away, so it saturates at
// the majority of the minimum BFT committee and does not grow with the set. It
// is capped by the live majority so a genuinely small chain stays satisfiable.
func TestNovaSignerFloorSaturates(t *testing.T) {
	ceiling := NovaQuorum(minBFTCommittee)
	for _, row := range []struct{ n, want int }{
		{0, 1}, {1, 1}, {2, 2}, {3, 2}, {4, 3}, {5, 3}, {100, 3}, {10000, 3},
	} {
		if got := NovaSignerFloor(row.n); got != row.want {
			t.Fatalf("NovaSignerFloor(%d) = %d, want %d", row.n, got, row.want)
		}
	}
	if NovaSignerFloor(10000) != ceiling {
		t.Fatalf("the floor must saturate at NovaQuorum(minBFTCommittee)=%d", ceiling)
	}
	// And it never exceeds the majority it is capped by, at any size.
	for n := 0; n <= 64; n++ {
		if f, q := NovaSignerFloor(n), NovaQuorum(n); f > q {
			t.Fatalf("NovaSignerFloor(%d)=%d exceeds NovaQuorum(%d)=%d — a small chain cannot ignite", n, f, n, q)
		}
	}
}

// TestCrashToleranceIsCeilHalfMinusOne states the degraded-mode signal against
// the closed form its documentation claims — ⌈n/2⌉−1 — computed here rather than
// taken from NovaQuorum, so the two derivations have to agree. Reading it back
// as `n - NovaQuorum(n)` would restate the implementation and hold nothing: it
// would still pass with the majority itself wrong.
func TestCrashToleranceIsCeilHalfMinusOne(t *testing.T) {
	for _, n := range []int{-1, 0, 1} {
		if got := CrashTolerance(n); got != 0 {
			t.Fatalf("CrashTolerance(%d) = %d, want 0 — a lone node survives no loss", n, got)
		}
	}
	for n := 2; n <= 64; n++ {
		want := (n+1)/2 - 1 // ⌈n/2⌉ − 1
		if got := CrashTolerance(n); got != want {
			t.Fatalf("CrashTolerance(%d) = %d, want ceil(n/2)-1 = %d", n, got, want)
		}
		// And the property that closed form exists to state: losing exactly the
		// tolerance still leaves a majority, losing one more does not.
		if n-CrashTolerance(n) < NovaQuorum(n) {
			t.Fatalf("at n=%d, losing %d leaves %d alive, below the majority of %d",
				n, CrashTolerance(n), n-CrashTolerance(n), NovaQuorum(n))
		}
		if n-CrashTolerance(n)-1 >= NovaQuorum(n) {
			t.Fatalf("at n=%d the tolerance %d understates what nova survives", n, CrashTolerance(n))
		}
	}
}

// TestNovaBetaIsOneOnlyForALoneNode holds the hysteresis rule: a lone node has
// no peer to confirm against, so any wait is a phantom-peer stall; from two
// upward one confirming round stops a transient majority flip igniting.
func TestNovaBetaIsOneOnlyForALoneNode(t *testing.T) {
	for _, n := range []int{-1, 0, 1} {
		if got := NovaBeta(n); got != 1 {
			t.Fatalf("NovaBeta(%d) = %d, want 1", n, got)
		}
	}
	for _, n := range []int{2, 3, 4, 21, 1000} {
		if got := NovaBeta(n); got != 2 {
			t.Fatalf("NovaBeta(%d) = %d, want 2", n, got)
		}
	}
}

// TestTheModeNamesAreStable pins the three regime names. They are what a log
// line and an operator's alarm read, and "unknown" in particular is the degraded
// answer — it must never render as one of the two live regimes.
func TestTheModeNamesAreStable(t *testing.T) {
	for _, row := range []struct {
		mode ConsensusMode
		name string
	}{
		{ModeSingleValidator, "single-validator"},
		{ModeQuorumFinality, "quorum-finality"},
		{ModeUnknown, "unknown"},
		{ConsensusMode(99), "unknown"},
	} {
		if got := row.mode.String(); got != row.name {
			t.Fatalf("mode %d is %q, want %q", row.mode, got, row.name)
		}
	}
}
