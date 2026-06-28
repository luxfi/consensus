// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"bytes"
	"testing"

	"github.com/luxfi/ids"
)

// mkValidators builds a deterministic validator set of `count` nodes; node i has
// weight base+i so weights are distinct and positive.
func mkValidators(count int, base uint64) []SortitionValidator {
	vs := make([]SortitionValidator, count)
	for i := 0; i < count; i++ {
		var nid ids.NodeID
		nid[0] = byte(i)
		nid[1] = byte(i >> 8)
		nid[19] = 0xAB
		vs[i] = SortitionValidator{NodeID: nid, Weight: base + uint64(i)}
	}
	return vs
}

func TestSortition_Deterministic(t *testing.T) {
	var prev ids.ID
	copy(prev[:], []byte("prev-finalized-block-id-32bytes!!"))
	var ss [48]byte
	copy(ss[:], []byte("signer-set-id-48-bytes-aaaaaaaaaaaaaaaaaaaaaaaa!!"))
	vals := mkValidators(200, 1000)

	seed1 := SortitionSeed(prev, ss, 4242, 9, 0x0C0DE002)
	seed2 := SortitionSeed(prev, ss, 4242, 9, 0x0C0DE002)
	if !bytes.Equal(seed1, seed2) {
		t.Fatal("seed not deterministic")
	}

	p1, err := DeriveCommitteePlan(PulsarHybridPQv1, seed1, vals)
	if err != nil {
		t.Fatalf("DeriveCommitteePlan: %v", err)
	}
	p2, err := DeriveCommitteePlan(PulsarHybridPQv1, seed2, vals)
	if err != nil {
		t.Fatalf("DeriveCommitteePlan: %v", err)
	}
	if !bytes.Equal(p1.PlanHash, p2.PlanHash) || !bytes.Equal(p1.KeyEraRoot, p2.KeyEraRoot) {
		t.Fatal("plan hash / key-era root not deterministic")
	}
	if len(p1.Committees) != int(PulsarHybridPQv1.M) {
		t.Fatalf("want %d committees, got %d", PulsarHybridPQv1.M, len(p1.Committees))
	}
	for j := range p1.Committees {
		if p1.Committees[j].ID != p2.Committees[j].ID {
			t.Fatalf("committee %d id not deterministic", j)
		}
		if len(p1.Committees[j].Members) != int(PulsarHybridPQv1.N) {
			t.Fatalf("committee %d: want n=%d members, got %d", j, PulsarHybridPQv1.N, len(p1.Committees[j].Members))
		}
	}
}

func TestSortition_UnbiasableInputsChangePlan(t *testing.T) {
	var prev ids.ID
	copy(prev[:], []byte("prev-finalized-block-id-32bytes!!"))
	var ss [48]byte
	copy(ss[:], []byte("signer-set-id-48-bytes-aaaaaaaaaaaaaaaaaaaaaaaa!!"))
	vals := mkValidators(200, 1000)

	base := SortitionSeed(prev, ss, 4242, 9, 0x0C0DE002)
	basePlan, err := DeriveCommitteePlan(PulsarHybridPQv1, base, vals)
	if err != nil {
		t.Fatalf("DeriveCommitteePlan(base): %v", err)
	}
	// Changing ANY unbiasable input must change the seed AND the derived plan
	// hash — the plan is a pure function of the seed, so a different seed yields
	// a different committee plan and thus a different committeePlanHash bound
	// into M. An adversary that could change one input without changing the plan
	// could pre-stage committees; this asserts that is impossible.
	var prev2 ids.ID
	copy(prev2[:], []byte("DIFFERENT-finalized-block-32byte"))
	cases := map[string][]byte{
		"prevBlock": SortitionSeed(prev2, ss, 4242, 9, 0x0C0DE002),
		"pChainHt":  SortitionSeed(prev, ss, 4243, 9, 0x0C0DE002),
		"epoch":     SortitionSeed(prev, ss, 4242, 10, 0x0C0DE002),
		"policyID":  SortitionSeed(prev, ss, 4242, 9, 0x0C0DE003),
	}
	for name, s := range cases {
		if bytes.Equal(base, s) {
			t.Fatalf("changing %s did not change the sortition seed", name)
		}
		plan, err := DeriveCommitteePlan(PulsarHybridPQv1, s, vals)
		if err != nil {
			t.Fatalf("DeriveCommitteePlan(%s): %v", name, err)
		}
		if bytes.Equal(basePlan.PlanHash, plan.PlanHash) {
			t.Fatalf("changing %s did not change the committeePlanHash", name)
		}
	}
}

func TestSortition_CommitteesDistinct(t *testing.T) {
	var prev ids.ID
	copy(prev[:], []byte("prev-finalized-block-id-32bytes!!"))
	var ss [48]byte
	copy(ss[:], []byte("signer-set-id-48-bytes-aaaaaaaaaaaaaaaaaaaaaaaa!!"))
	// Large, well-spread validator set ⇒ the 12 committees are distinct w.h.p.
	vals := mkValidators(500, 1_000_000)
	seed := SortitionSeed(prev, ss, 1, 1, 1)
	plan, err := DeriveCommitteePlan(PulsarHybridPQv1, seed, vals)
	if err != nil {
		t.Fatalf("DeriveCommitteePlan: %v", err)
	}
	seen := map[ids.ID]bool{}
	for _, c := range plan.Committees {
		if seen[c.ID] {
			t.Fatalf("committee id %s repeated — committees must be independent", c.ID)
		}
		seen[c.ID] = true
		// Members within a committee must be distinct (without replacement).
		ms := map[ids.NodeID]bool{}
		for _, m := range c.Members {
			if ms[m] {
				t.Fatal("duplicate member within a committee (replacement leaked)")
			}
			ms[m] = true
		}
		// Plan membership lookup works.
		if idx, ok := plan.InPlan(c.ID); !ok || idx != c.Index {
			t.Fatalf("InPlan(%s) = (%d,%v), want (%d,true)", c.ID, idx, ok, c.Index)
		}
	}
}

// TestSortition_StakeWeighted checks the selection is stake-PROPORTIONAL: across
// many committees a heavy validator is picked far more often than a light one.
func TestSortition_StakeWeighted(t *testing.T) {
	// Two heavy nodes (weight 1e6) and many light nodes (weight 1).
	const light = 300
	vals := make([]SortitionValidator, 0, light+2)
	var heavy1, heavy2 ids.NodeID
	heavy1[0], heavy1[19] = 0xF1, 0xF1
	heavy2[0], heavy2[19] = 0xF2, 0xF2
	vals = append(vals, SortitionValidator{NodeID: heavy1, Weight: 1_000_000})
	vals = append(vals, SortitionValidator{NodeID: heavy2, Weight: 1_000_000})
	for i := 0; i < light; i++ {
		var nid ids.NodeID
		nid[0] = byte(i)
		nid[1] = 0x10
		nid[19] = 0x01
		vals = append(vals, SortitionValidator{NodeID: nid, Weight: 1})
	}
	// Sample many independent committees by varying the epoch.
	heavyHits, total := 0, 0
	var prev ids.ID
	var ss [48]byte
	params := SortitionParams{N: 4, T: 3, M: 8, R: 5, SelectionAlgorithm: SelectionStakeWeightedSHAKE256, FMaxNum: 1, FMaxDen: 3}
	for epoch := uint64(0); epoch < 50; epoch++ {
		seed := SortitionSeed(prev, ss, 0, epoch, 0)
		plan, err := DeriveCommitteePlan(params, seed, vals)
		if err != nil {
			t.Fatalf("DeriveCommitteePlan: %v", err)
		}
		for _, c := range plan.Committees {
			for _, m := range c.Members {
				total++
				if m == heavy1 || m == heavy2 {
					heavyHits++
				}
			}
		}
	}
	// The two heavy nodes hold 2e6 of ~2e6+300 total weight ⇒ ~>99.98% mass.
	// They must dominate the membership (allow slack for without-replacement).
	if heavyHits*100 < total*50 {
		t.Fatalf("stake weighting too weak: heavy nodes won %d/%d slots (<50%%)", heavyHits, total)
	}
}

func TestSortition_FailClosed(t *testing.T) {
	var prev ids.ID
	var ss [48]byte
	seed := SortitionSeed(prev, ss, 0, 0, 0)

	// Too few validators for n=8.
	if _, err := DeriveCommitteePlan(PulsarHybridPQv1, seed, mkValidators(5, 10)); err == nil {
		t.Fatal("expected error for validator set smaller than committee size")
	}
	// Zero total weight.
	zero := []SortitionValidator{{NodeID: ids.NodeID{1}, Weight: 0}, {NodeID: ids.NodeID{2}, Weight: 0}}
	if _, err := DeriveCommitteePlan(PulsarHybridPQv1, seed, zero); err == nil {
		t.Fatal("expected error for zero total weight")
	}
	// Bad params: t > n.
	bad := SortitionParams{N: 4, T: 5, M: 8, R: 5, SelectionAlgorithm: SelectionStakeWeightedSHAKE256, FMaxNum: 1, FMaxDen: 3}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected error for t > n")
	}
	// Bad params: r > m.
	bad = SortitionParams{N: 8, T: 7, M: 8, R: 9, SelectionAlgorithm: SelectionStakeWeightedSHAKE256, FMaxNum: 1, FMaxDen: 3}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected error for r > m")
	}
	// Bad params: f_max ≥ 1.
	bad = SortitionParams{N: 8, T: 7, M: 12, R: 8, SelectionAlgorithm: SelectionStakeWeightedSHAKE256, FMaxNum: 3, FMaxDen: 3}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected error for f_max >= 1")
	}
	// Unknown selection algorithm.
	bad = SortitionParams{N: 8, T: 7, M: 12, R: 8, SelectionAlgorithm: 99, FMaxNum: 1, FMaxDen: 3}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected error for unknown selection algorithm")
	}
}
