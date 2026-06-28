// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import "testing"

// TestValidateMithrilSigningCommittee pins the consensus-side Pulsar dealerless-
// RSS committee bound 2 ≤ T ≤ N ≤ 6, delegated to luxfi/dkg/rss.
func TestValidateMithrilSigningCommittee(t *testing.T) {
	valid := [][2]int{
		{2, 2}, {2, 3}, {3, 3}, {2, 4}, {3, 4}, {4, 4},
		{2, 5}, {3, 5}, {4, 5}, {5, 5},
		{2, 6}, {3, 6}, {4, 6}, {5, 6}, {6, 6},
	}
	for _, tn := range valid {
		if err := ValidateMithrilSigningCommittee(tn[0], tn[1]); err != nil {
			t.Fatalf("rejected viable Pulsar committee (T=%d,N=%d): %v", tn[0], tn[1], err)
		}
	}
	bad := [][2]int{{1, 2}, {3, 2}, {2, 7}, {7, 7}, {4, 100}}
	for _, tn := range bad {
		if err := ValidateMithrilSigningCommittee(tn[0], tn[1]); err == nil {
			t.Fatalf("admitted non-viable Pulsar committee (T=%d,N=%d)", tn[0], tn[1])
		}
	}
}

func TestRecommendedMithrilCommitteeIsViable(t *testing.T) {
	tt, n := RecommendedMithrilCommittee()
	if tt != 4 || n != 6 {
		t.Fatalf("RecommendedMithrilCommittee()=(%d,%d), want (4,6)", tt, n)
	}
	if err := ValidateMithrilSigningCommittee(tt, n); err != nil {
		t.Fatalf("recommended committee is not viable: %v", err)
	}
}

func TestClampMithrilCommitteeSize(t *testing.T) {
	cases := []struct{ in, want int }{
		{100, MithrilMaxCommittee}, {7, MithrilMaxCommittee}, {6, 6},
		{3, 3}, {2, 3}, {0, 3},
	}
	for _, c := range cases {
		if got := ClampMithrilCommitteeSize(c.in); got != c.want {
			t.Errorf("ClampMithrilCommitteeSize(%d)=%d, want %d", c.in, got, c.want)
		}
	}
}

// TestGroupedEpochManagerCapsAtMithrilBound proves the sortition cap: an
// over-large group request is clamped to the Mithril viability bound so every
// signing group can host a dealerless-RSS Pulsar key.
func TestGroupedEpochManagerCapsAtMithrilBound(t *testing.T) {
	gem := NewGroupedEpochManager(10, 7, 3)
	if gem.groupSize > MithrilMaxCommittee {
		t.Fatalf("group size %d exceeds Mithril bound %d", gem.groupSize, MithrilMaxCommittee)
	}
	if gem.groupSize != MithrilMaxCommittee {
		t.Fatalf("group size %d, want clamp to %d", gem.groupSize, MithrilMaxCommittee)
	}
	// Default (3) must be unchanged — the cap only constrains over-large configs.
	gem3 := NewGroupedEpochManager(3, 2, 3)
	if gem3.groupSize != 3 {
		t.Fatalf("default group size changed to %d, want 3", gem3.groupSize)
	}
}
