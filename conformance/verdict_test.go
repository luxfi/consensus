// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// verdict_test.go — the second guard on the decision.
//
// The golden file alone would let a changed threshold be re-blessed with
// -update and disappear. So the boundaries are also stated OUTRIGHT here,
// transcribed from the spec and recomputed in arbitrary precision, sharing no
// code with config.HalfStakeFloor, config.TwoThirdsStakeFloor,
// config.TwoThirdsCount or chain.NovaSignerFloor:
//
//	Nova   accepts iff signers >= min(floor(n/2)+1, 3) and 2*voted > total
//	Quasar accepts iff signers >= floor(2*n/3)+1       and 3*voted > 2*total
//
// Both stake halves are the strict forms. "voted > floor(total/2)" and
// "2*voted > total" are the same predicate over the integers, as are
// "voted > floor(2*total/3)" and "3*voted > 2*total" — but the multiplied forms
// cannot share an off-by-one with the production floors, which is the point of
// writing them this way. The two count floors are written out for the same
// reason, and they are deliberately different shapes: Nova's saturates at three,
// Quasar's is the supermajority itself and grows with the set.
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

		var floor int
		var want bool
		switch c.Rung {
		case "nova":
			// The Nova signer floor, transcribed: the majority of the minimal
			// BFT committee of four, capped by the majority of the live set so
			// a genuinely small chain is not made unsatisfiable.
			floor = n/2 + 1
			if floor > 3 {
				floor = 3
			}
			// 2*voted > total
			twice := new(big.Int).Lsh(voted, 1)
			want = signers >= floor && twice.Cmp(total) > 0
		case "quasar":
			// The export signer floor, transcribed as the smallest integer
			// strictly greater than two thirds of n. Written as a search rather
			// than as floor(2n/3)+1 so it cannot share an off-by-one with the
			// production closed form either: it is the definition, not the
			// formula.
			for floor = 0; 3*floor <= 2*n; floor++ {
			}
			// 3*voted > 2*total
			thrice := new(big.Int).Mul(voted, big.NewInt(3))
			twice := new(big.Int).Lsh(total, 1)
			want = signers >= floor && thrice.Cmp(twice) > 0
		default:
			t.Fatalf("%s: a certificate attests nova or quasar, not %q", c.Name, c.Rung)
		}

		if c.Accept != want {
			t.Errorf("%s: corpus says accept=%v, the transcribed rule says %v "+
				"(rung=%s n=%d signers=%d floor=%d voted=%s total=%s)",
				c.Name, c.Accept, want, c.Rung, n, signers, floor, c.Voted, c.Total)
		}
		// The floor the corpus RECORDS must be the floor the rule states — a row
		// that decided correctly while advertising the other rung's floor would
		// tell a foreign runner the wrong number about the clause it just failed.
		if c.SignerFloor != floor {
			t.Errorf("%s: corpus records signerFloor=%d, the transcribed %s floor is %d",
				c.Name, c.SignerFloor, c.Rung, floor)
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

// TestStakeCannotBuyExportEither is the same safety property one rung up, and it
// is the one R4 added: a holder of a ⅔ STAKE supermajority must not be able to
// mint an EXPORT certificate alone. Export finality is a claim that a Byzantine
// supermajority of independent parties agreed; a certificate one key can produce
// is not that claim however much stake stands behind the key.
//
// The refusal must land on the COUNT. A stake refusal here would mean the lone
// signer never held ⅔ in the first place and the case proved nothing about the
// floor.
func TestStakeCannotBuyExportEither(t *testing.T) {
	c := findFinality(t, "quasar_whale_alone")

	if c.Rung != "quasar" {
		t.Fatalf("the export whale case attests %q", c.Rung)
	}
	if len(c.Signers) != 1 {
		t.Fatalf("the lone-signer case carries %d signers", len(c.Signers))
	}
	// The lone signer really does clear the ⅔ STAKE floor — otherwise the case
	// would be refused by the stake clause and would prove nothing about the count.
	thrice := new(big.Int).Mul(bigOf(t, c.Voted), big.NewInt(3))
	twice := new(big.Int).Lsh(bigOf(t, c.Total), 1)
	if thrice.Cmp(twice) <= 0 {
		t.Fatalf("the lone signer holds %s of %s — not a ⅔ stake supermajority, so this "+
			"case does not test the export signer floor", c.Voted, c.Total)
	}
	if c.Accept {
		t.Error("a lone holder of a ⅔ stake supermajority minted an export certificate: " +
			"the export signer floor did not run")
	}
	if c.Refusal != "belowThreshold" {
		t.Errorf("refused with %q, want belowThreshold — a stake refusal here would mean "+
			"the count floor was never reached", c.Refusal)
	}

	// The same stake, once the floor is met, carries. This is what pins the
	// refusal above to the COUNT and not to some second stake rule.
	with := findFinality(t, "quasar_whale_with_three")
	if !with.Accept {
		t.Error("the same holder with the export floor met did not carry; the floor is not " +
			"the binding clause")
	}
	if len(with.Signers) != c.SignerFloor {
		t.Errorf("the carrying case has %d signers and the floor is %d — the accepting side "+
			"must sit exactly ON the floor or the edge is not sharp",
			len(with.Signers), c.SignerFloor)
	}
}

// TestBothRungsCarryASignerFloor — neither rung may be decided by stake alone.
// A rung whose count floor is dropped stops asking how many parties agreed and
// starts asking only how much stake did, which is one signature wherever stake
// is concentrated. Both floors are therefore stated positively here, so removing
// either fails at the source rather than at a foreign node six months later.
func TestBothRungsCarryASignerFloor(t *testing.T) {
	for _, rung := range []string{"nova", "quasar"} {
		if !hasRefusal(t, rung, "belowThreshold") {
			t.Errorf("%s has no case refused on its count floor: a rung with no count "+
				"refusal in the corpus is a rung whose floor nothing holds", rung)
		}
	}
}

// hasRefusal reports whether a rung has at least one case refused on a clause.
func hasRefusal(t *testing.T, rung, class string) bool {
	t.Helper()
	for _, c := range Build().Verdict.Finality {
		if c.Rung == rung && c.Refusal == class {
			return true
		}
	}
	return false
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

	if got, want := len(v.Finality), 11; got != want {
		t.Errorf("%d finality cases, want %d — adding one is deliberate, losing one is not", got, want)
	}
	if got, want := len(v.Admission), 4; got != want {
		t.Errorf("%d admission cases, want %d", got, want)
	}
	if _, err := strconv.ParseUint(v.Epoch, 10, 64); err != nil {
		t.Errorf("epoch %q is not a height: %v", v.Epoch, err)
	}

	// A case is named by (name, door) at the admission section and by name at
	// the finality one, and a runner looks a case up by that key. Two rows
	// sharing a key are two different questions with one answer: the runner
	// finds whichever comes first and reports PASS having silently skipped the
	// other. Both doors legitimately answer for the same weight vector — the
	// zero-weight rows are deliberately named alike — so the admission key is
	// the PAIR, and it is the pair that has to be unique.
	seenAdmission := map[[2]string]bool{}
	for _, c := range v.Admission {
		key := [2]string{c.Name, c.Door}
		if seenAdmission[key] {
			t.Errorf("two admission cases named %q at the %s door: a runner keyed on the "+
				"pair answers one of them and skips the other", c.Name, c.Door)
		}
		seenAdmission[key] = true
		if c.Door == "" {
			t.Errorf("admission case %q names no door: the two doors do not enforce the "+
				"same clauses, so which one decided is part of the verdict", c.Name)
		}
	}
	seenFinality := map[string]bool{}
	for _, c := range v.Finality {
		if seenFinality[c.Name] {
			t.Errorf("two finality cases named %q: findFinality answers the first and the "+
				"second is never weighed", c.Name)
		}
		seenFinality[c.Name] = true
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
