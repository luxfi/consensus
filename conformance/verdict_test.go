// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// verdict_test.go — the second guard on the decision.
//
// The golden file alone would let a changed threshold be re-blessed with
// -update and disappear. So the boundaries are also stated OUTRIGHT here,
// transcribed from the spec and recomputed in arbitrary precision, sharing no
// code with config.HalfStakeFloor, config.TwoThirdsStakeFloor or
// chain.NovaSignerFloor:
//
//	Nova   accepts iff signers >= min(floor(n/2)+1, 3) and 2*voted > total
//	Quasar accepts iff                                     3*voted > 2*total
//
// Both are the strict forms. "voted > floor(total/2)" and "2*voted > total" are
// the same predicate over the integers, as are "voted > floor(2*total/3)" and
// "3*voted > 2*total" — but the multiplied forms cannot share an off-by-one with
// the production floors, which is the point of writing them this way.
package conformance

import (
	"math/big"
	"strconv"
	"testing"
)

// bigOf parses a decimal string the corpus recorded.
func bigOf(t *testing.T, s string) *big.Int {
	t.Helper()
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("not a decimal: %q", s)
	}
	return v
}

// TestVerdictsStatedOutright recomputes every frozen finality verdict from the
// transcribed rule. A rung whose floor moved fails here even if the golden was
// re-blessed in the same commit.
func TestVerdictsStatedOutright(t *testing.T) {
	for _, c := range Build().Verdict.Finality {
		n := len(c.Set)
		total := bigOf(t, c.Total)
		voted := bigOf(t, c.Voted)
		signers := len(c.Signers)

		// The signer floor, transcribed: the majority of the minimal BFT
		// committee of four, capped by the majority of the live set so a
		// genuinely small chain is not made unsatisfiable.
		floor := n/2 + 1
		if floor > 3 {
			floor = 3
		}

		var want bool
		switch c.Rung {
		case "nova":
			// 2*voted > total
			twice := new(big.Int).Lsh(voted, 1)
			want = signers >= floor && twice.Cmp(total) > 0
		case "quasar":
			// 3*voted > 2*total
			thrice := new(big.Int).Mul(voted, big.NewInt(3))
			twice := new(big.Int).Lsh(total, 1)
			want = thrice.Cmp(twice) > 0
		default:
			t.Fatalf("%s: a certificate attests nova or quasar, not %q", c.Name, c.Rung)
		}

		if c.Accept != want {
			t.Errorf("%s: corpus says accept=%v, the transcribed rule says %v "+
				"(rung=%s n=%d signers=%d floor=%d voted=%s total=%s)",
				c.Name, c.Accept, want, c.Rung, n, signers, floor, c.Voted, c.Total)
		}
		if c.Accept != (c.Refusal == "") {
			t.Errorf("%s: accept=%v but refusal=%q — a decision and its reason disagree",
				c.Name, c.Accept, c.Refusal)
		}
	}
}

// TestVerdictTallyIsTheSignersSum — the recorded tally must be the sum of the
// weights of the recorded signers. A corpus that wrote a tally it did not
// compute would let every other assertion here pass while describing a set
// nobody weighed.
func TestVerdictTallyIsTheSignersSum(t *testing.T) {
	for _, c := range Build().Verdict.Finality {
		weight := make(map[string]string, len(c.Set))
		for _, s := range c.Set {
			weight[s.NodeID] = s.Weight
		}

		sum := new(big.Int)
		seen := make(map[string]struct{}, len(c.Signers))
		prev := ""
		for i, id := range c.Signers {
			w, inSet := weight[id]
			if !inSet {
				t.Errorf("%s: signer %s is not in the set it is weighed against", c.Name, id)
				continue
			}
			if _, dup := seen[id]; dup {
				t.Errorf("%s: signer %s appears twice — one voter counted twice", c.Name, id)
			}
			seen[id] = struct{}{}
			if i > 0 && id <= prev {
				t.Errorf("%s: signer %d (%s) does not follow %s; the wire is ascending by node id",
					c.Name, i, id, prev)
			}
			prev = id
			sum.Add(sum, bigOf(t, w))
		}

		if got := bigOf(t, c.Voted); got.Cmp(sum) != 0 {
			t.Errorf("%s: recorded tally %s is not the sum of its signers' weights %s",
				c.Name, c.Voted, sum)
		}
		if uint32(len(c.Signers)) != c.Threshold {
			t.Errorf("%s: threshold %d is not the vote count %d — the count clause is "+
				"supposed to be satisfied so the weighted half is what decides",
				c.Name, c.Threshold, len(c.Signers))
		}
	}
}

// TestStakeCannotBuyPastTheSignerFloor is the safety property the Nova rung's
// count floor exists for, stated in words: a single holder of a stake majority
// must not self-ignite. The whale case must refuse, and it must refuse on the
// COUNT clause — refusing on stake would mean the floor never ran.
func TestStakeCannotBuyPastTheSignerFloor(t *testing.T) {
	c := findFinality(t, "nova_whale_alone")

	if len(c.Signers) != 1 {
		t.Fatalf("the lone-signer case carries %d signers", len(c.Signers))
	}
	// The lone signer really does hold a majority of stake — otherwise the case
	// would be refused by the stake clause and prove nothing about the floor.
	twice := new(big.Int).Lsh(bigOf(t, c.Voted), 1)
	if twice.Cmp(bigOf(t, c.Total)) <= 0 {
		t.Fatalf("the lone signer holds %s of %s — not a stake majority, so this case "+
			"does not test the signer floor", c.Voted, c.Total)
	}
	if c.Accept {
		t.Error("a lone holder of a stake majority self-ignited: the signer floor did not run")
	}
	if c.Refusal != "belowThreshold" {
		t.Errorf("refused with %q, want belowThreshold — a stake refusal here would mean "+
			"the count floor was never reached", c.Refusal)
	}

	// The same stake, once the floor is met, carries. This is what pins the
	// refusal above to the COUNT and not to some second stake rule.
	with := findFinality(t, "nova_whale_with_two")
	if !with.Accept {
		t.Error("the same holder with the floor met did not carry; the floor is not the binding clause")
	}
}

// TestExportSitsAboveLocalExecution — on one set the export rung must demand
// strictly more than the local-execution rung. If the two edges ever coincide,
// Nova has stopped being a bare majority or Quasar has stopped being two thirds,
// and the ladder has collapsed into one rung.
func TestExportSitsAboveLocalExecution(t *testing.T) {
	nova := findFinality(t, "nova_majority")
	quasar := findFinality(t, "quasar_supermajority")

	if nova.Total != quasar.Total {
		t.Fatalf("the two edges are read off different sets (%s vs %s)", nova.Total, quasar.Total)
	}
	if !nova.Accept || !quasar.Accept {
		t.Fatal("both edge cases are supposed to be the accepting side of their rung")
	}
	if len(nova.Signers) >= len(quasar.Signers) {
		t.Errorf("local execution ignites at %d signers and export at %d: export must cost more",
			len(nova.Signers), len(quasar.Signers))
	}

	// And each edge is sharp: one signer fewer refuses.
	below := findFinality(t, "nova_below_majority")
	if below.Accept || len(below.Signers)+1 != len(nova.Signers) {
		t.Errorf("the majority edge is not sharp: %d signers accept=%v sits below %d",
			len(below.Signers), below.Accept, len(nova.Signers))
	}
	belowQ := findFinality(t, "quasar_below_supermajority")
	if belowQ.Accept || len(belowQ.Signers)+1 != len(quasar.Signers) {
		t.Errorf("the supermajority edge is not sharp: %d signers accept=%v sits below %d",
			len(belowQ.Signers), belowQ.Accept, len(quasar.Signers))
	}
}

// TestOnlyNovaCarriesASignerFloor — the export rung's refusals must never be
// count refusals, because Quasar has no signer floor of its own. This is the
// asymmetry R4 is about; freezing it here means the day a floor is added to the
// export rung, this test names it rather than a foreign node six months later.
func TestOnlyNovaCarriesASignerFloor(t *testing.T) {
	for _, c := range Build().Verdict.Finality {
		if c.Rung == "quasar" && c.Refusal == "belowThreshold" {
			t.Errorf("%s: the export rung refused on the count floor, which it does not have",
				c.Name)
		}
	}
}

// TestAdmissionWeightRulesStatedOutright recomputes the two weight clauses of
// the door in arbitrary precision: no seat may carry zero stake, and the total
// must be representable.
func TestAdmissionWeightRulesStatedOutright(t *testing.T) {
	ceiling := new(big.Int).Lsh(big.NewInt(1), 64) // 2^64, one past the widest total

	for _, c := range Build().Verdict.Admission {
		sum := new(big.Int)
		zero := false
		for _, w := range c.Weights {
			v := bigOf(t, w)
			if v.Sign() == 0 {
				zero = true
			}
			sum.Add(sum, v)
		}

		switch {
		case zero:
			if c.Admitted || c.Refusal != "zeroWeight" {
				t.Errorf("%s: a seat carries no stake; want refused zeroWeight, got admitted=%v %q",
					c.Name, c.Admitted, c.Refusal)
			}
		case sum.Cmp(ceiling) >= 0:
			if c.Admitted || c.Refusal != "weightOverflow" {
				t.Errorf("%s: the total %s does not fit in a uint64; want refused weightOverflow, "+
					"got admitted=%v %q", c.Name, sum, c.Admitted, c.Refusal)
			}
		default:
			if !c.Admitted {
				t.Errorf("%s: every weight is positive and the total %s fits; want admitted, got %q",
					c.Name, sum, c.Refusal)
			}
		}
	}
}

// TestVerdictSectionIsPopulated — a section that quietly lost its cases would
// let every assertion above pass over an empty list. The counts are stated so
// that adding or dropping a case is a deliberate edit to this file.
func TestVerdictSectionIsPopulated(t *testing.T) {
	v := Build().Verdict

	if got, want := len(v.Finality), 8; got != want {
		t.Errorf("%d finality cases, want %d — adding one is deliberate, losing one is not", got, want)
	}
	if got, want := len(v.Admission), 4; got != want {
		t.Errorf("%d admission cases, want %d", got, want)
	}
	if _, err := strconv.ParseUint(v.Epoch, 10, 64); err != nil {
		t.Errorf("epoch %q is not a height: %v", v.Epoch, err)
	}

	// Both sides of every rung are represented. A section of refusals alone
	// would pass an implementation that refuses everything.
	seen := map[string]map[bool]bool{}
	for _, c := range v.Finality {
		if seen[c.Rung] == nil {
			seen[c.Rung] = map[bool]bool{}
		}
		seen[c.Rung][c.Accept] = true
	}
	for _, rung := range []string{"nova", "quasar"} {
		if !seen[rung][true] || !seen[rung][false] {
			t.Errorf("%s has no accepting case or no refusing one; a runner that always "+
				"answers the same way would pass", rung)
		}
	}
}

// findFinality returns the named case, or fails.
func findFinality(t *testing.T, name string) FinalityCase {
	t.Helper()
	for _, c := range Build().Verdict.Finality {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no finality case named %q", name)
	return FinalityCase{}
}
