// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quasar_count_floor_test.go — the EXPORT rung's distinct-signer floor, and the
// proof that adding it took nothing away.
//
// nova_count_floor_test.go holds the same ground one rung down. The two floors
// answer the same question at different volumes: how many INDEPENDENT parties
// agreed, which is the number a stake reading cannot report. Two thirds of the
// stake is one signature wherever two thirds of the stake is one validator, and
// a certificate one key can produce is not a Byzantine supermajority however
// much weight stands behind the key. Nova asks only "more than one party?" and
// saturates at three; Quasar asks for the supermajority itself, ⌊2n/3⌋+1.
//
// The floor is STRICTER than the stake predicate alone, so the second half of
// this file is about liveness. Every producer of an export certificate in this
// engine already gathered exactly this many signatures before it built one, and
// every consumer already demanded exactly this many before it read one. The
// clause added to the verifier is the same number a third time — so it refuses
// nothing the engine can produce, and refuses precisely the thing the engine
// never could: a certificate assembled by hand out of one whale's signature.
package chain

import (
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/constants"
	"github.com/luxfi/ids"
)

// TestQuasarCountFloor_IsTheSupermajorityInSeats recomputes the floor from its
// definition — the smallest k whose 3k strictly exceeds 2n — sharing no code
// with config.TwoThirdsCount. A closed form and the definition it closes are two
// statements, and this is where they are held to being one.
func TestQuasarCountFloor_IsTheSupermajorityInSeats(t *testing.T) {
	// The deployed sizes, stated outright so a formula change is visible as a
	// number and not only as a disagreement between two computations.
	for n, want := range map[int]int{1: 1, 2: 2, 3: 3, 4: 3, 5: 4, 11: 8, 21: 15, 41: 28, 100: 67} {
		if got := config.TwoThirdsCount(n); got != want {
			t.Errorf("TwoThirdsCount(%d) = %d, want %d", n, got, want)
		}
	}

	for n := 1; n <= 1000; n++ {
		k := 0
		for 3*k <= 2*n {
			k++
		}
		if got := config.TwoThirdsCount(n); got != k {
			t.Fatalf("n=%d: TwoThirdsCount=%d, the smallest k with 3k>2n is %d", n, got, k)
		}
		if k > n {
			t.Fatalf("n=%d: the floor %d is above the set — a rung nothing can satisfy", n, k)
		}
		// The floor is the stake floor read in seats: it is what the stake
		// predicate itself demands of n unit weights. If these ever part, a
		// uniform fleet would meet one half of the export rule and fail the other.
		if uint64(k) <= config.TwoThirdsStakeFloor(uint64(n)) {
			t.Fatalf("n=%d: %d seats of unit weight do not clear floor(2n/3)=%d",
				n, k, config.TwoThirdsStakeFloor(uint64(n)))
		}
	}
}

// TestQuasarCountFloor_SitsAboveNova — the two rungs are distinct authorizations,
// so on any set large enough for them to differ the export floor must be the
// higher one. Nova's saturates at three and Quasar's grows, so they can only
// coincide on the small sets where both are the whole committee.
func TestQuasarCountFloor_SitsAboveNova(t *testing.T) {
	for n := 1; n <= 1000; n++ {
		nova, quasar := NovaSignerFloor(n), config.TwoThirdsCount(n)
		if quasar < nova {
			t.Fatalf("n=%d: export floor %d is below the local-execution floor %d", n, quasar, nova)
		}
		if n >= 5 && quasar <= nova {
			t.Fatalf("n=%d: export floor %d does not sit above the local-execution floor %d",
				n, quasar, nova)
		}
	}
}

// TestQuasarCountFloor_SizerMatchesVerifier closes the gossip path.
//
// A certificate arriving from a peer meets the runtime's own tier floor first
// (Runtime.HandleIncomingCert reads ChainConsensus.Alpha() for Quasar) and the
// cert predicate second. Those are two numbers in two files, and the risk is two
// DEFINITIONS of the count rather than two readings of one.
//
// What is held here is exactly that: α == TwoThirdsCount(p.K). The door is sized by
// the function the verifier enforces, so neither is a second rule. It is one rule
// read at two set sizes — the door at the chain's K, the predicate at the live n —
// and DoorAndVerifierAreOneRuleAtTwoSizes below says what follows when they differ.
func TestQuasarCountFloor_SizerMatchesVerifier(t *testing.T) {
	for _, networkID := range []uint32{constants.MainnetID, constants.TestnetID, 1337} {
		for n := 1; n <= 1000; n++ {
			p := config.FeasibleParams(networkID, n)
			if want := config.TwoThirdsCount(p.K); p.AlphaConfidence != want {
				t.Fatalf("network=%d n=%d: α=%d, the export floor for K=%d is %d",
					networkID, n, p.AlphaConfidence, p.K, want)
			}
		}
	}
}

// TestQuasarCountFloor_DoorAndVerifierAreOneRuleAtTwoSizes reaches the case the
// sizer test does not: K ≠ n.
//
// The door is fixed when the chain is built, from its K. The predicate is
// recomputed per certificate from the set live at that cert's epoch. FeasibleParams
// sets K = n, so the two are one number whenever the params were sized to the set
// that is running, and they part only when the set has moved since.
//
// GROWN (K < n). TwoThirdsCount does not decrease, so the door is the looser of the
// two and a certificate it admits is refused by the predicate afterwards, if at all.
// This is the direction that matters to the clause this file adds: it cannot halt an
// export the door let through, because the door is never the higher bar here.
//
// SHRUNK (K > n). The door is the stricter number and refuses first. That is the
// runtime's gossip-admission bound, which reads the same before and after this
// clause — the verifier's floor is not what refuses there. Asserted as the observed
// shape, so a later change that inverts it has to say so.
func TestQuasarCountFloor_DoorAndVerifierAreOneRuleAtTwoSizes(t *testing.T) {
	for _, networkID := range []uint32{constants.MainnetID, constants.TestnetID, 1337} {
		for _, configured := range []int{1, 2, 3, 4, 5, 7, 11, 21, 41, 100} {
			p := config.FeasibleParams(networkID, configured)
			door := p.AlphaConfidence
			for live := 1; live <= 200; live++ {
				verifier := config.TwoThirdsCount(live)
				switch {
				case p.K == live && door != verifier:
					t.Fatalf("network=%d K=%d live=%d: one rule at one size gave two numbers, door=%d verifier=%d",
						networkID, p.K, live, door, verifier)
				case p.K < live && door > verifier:
					t.Fatalf("network=%d K=%d live=%d: the set grew and the door became the higher bar "+
						"(door=%d verifier=%d) — the count is no longer monotone and the door can now "+
						"withhold an export the predicate would accept",
						networkID, p.K, live, door, verifier)
				case p.K > live && door < verifier:
					t.Fatalf("network=%d K=%d live=%d: the set shrank and the door became the looser bar "+
						"(door=%d verifier=%d), which is not the observed shape",
						networkID, p.K, live, door, verifier)
				}
			}
		}
	}
}

// TestQuasarCountFloor_ProducerNeverBuildsARefusedCert is the LIVENESS proof.
//
// QuasarAttestor is the engine's only producer of export certificates. It emits
// on the first attestation that reaches the export count, so the question "does
// the new clause halt a real export?" is exactly "is the count it emits at ever
// below the count the verifier now demands?" — and it is the same call, so it
// cannot be. This test holds that to the running attestor rather than to the
// reading: it drives a real set to its first emission and verifies the artifact
// through the full predicate, including the clause this file is about.
func TestQuasarCountFloor_ProducerNeverBuildsARefusedCert(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 5, 7, 11, 21, 41} {
		vs := newTestValidatorSet(n)
		q := NewQuasarAttestor(vs, vs)
		pos := attestPos(ids.GenerateTestID(), 1, ids.GenerateTestID())

		cert, emitAt := attestAll(t, q, vs, pos, 0, n)
		if cert == nil {
			t.Fatalf("n=%d: the whole set attesting produced no export certificate", n)
		}
		floor := config.TwoThirdsCount(n)
		// emitAt is the INDEX of the attester that tipped it, so the count is one more.
		if emitAt+1 != floor {
			t.Errorf("n=%d: emitted at %d attestations, the export floor is %d — a producer "+
				"that emits early builds certificates the verifier refuses, and one that "+
				"emits late withholds finality the set has already reached",
				n, emitAt+1, floor)
		}
		if cert.VoterCount() != floor {
			t.Errorf("n=%d: the emitted certificate carries %d voters, the floor is %d",
				n, cert.VoterCount(), floor)
		}
		if err := cert.VerifyWeighted(vs, vs, pos.Height); err != nil {
			t.Errorf("n=%d: the engine's own export artifact does not clear the export rule: %v",
				n, err)
		}
	}
}

// TestQuasarCountFloor_StakeCannotBuyExport is the clause stated as the attack it
// refuses. One validator holds a hundred of a hundred and four — more than two
// thirds several times over — and signs alone. Stake alone would export it.
func TestQuasarCountFloor_StakeCannotBuyExport(t *testing.T) {
	vs := newTestValidatorSet(5)
	whale := newStakeMap(vs, 100, 1, 1, 1, 1)
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()}
	const epoch = uint64(1)

	mkCert := func(idx ...int) *QuorumCert {
		votes := make([]SignedVote, 0, len(idx))
		for _, i := range idx {
			votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
		}
		c, err := AssembleQuorumCert(pos, Quasar, uint32(len(idx)), votes)
		if err != nil {
			t.Fatalf("assemble %v: %v", idx, err)
		}
		return c
	}

	// The premise: the lone signer really does hold the stake supermajority, so a
	// refusal below can only be the count.
	if voted, total := uint64(100), whale.TotalStake(epoch); voted <= config.TwoThirdsStakeFloor(total) {
		t.Fatalf("the whale holds %d of %d — not a ⅔ supermajority, so this test proves nothing",
			voted, total)
	}

	err := mkCert(0).VerifyWeighted(vs, whale, epoch)
	if !errors.Is(err, ErrQCBelowThreshold) {
		t.Fatalf("a lone holder of ⅔ of the stake minted an export certificate (%v)", err)
	}
	// Three signers is still one short of ⌊2·5/3⌋+1 = 4, and the stake is untouched.
	if err := mkCert(0, 1, 2).VerifyWeighted(vs, whale, epoch); !errors.Is(err, ErrQCBelowThreshold) {
		t.Fatalf("three of five is below the export floor of four, got %v", err)
	}
	// At the floor the same stake carries — the count was the binding clause.
	if err := mkCert(0, 1, 2, 3).VerifyWeighted(vs, whale, epoch); err != nil {
		t.Fatalf("four of five holding 103 of 104 must export: %v", err)
	}
	// And the count alone does not export either: four LIGHT signers meet the floor
	// and hold 4 of 104. Neither half is sufficient, which is the whole design.
	if err := mkCert(1, 2, 3, 4).VerifyWeighted(vs, whale, epoch); !errors.Is(err, ErrQCStakeBelowSupermajority) {
		t.Fatalf("four minimum-stake signers must be refused on stake, got %v", err)
	}
}

// TestQuasarCountFloor_UnresolvedSetFailsClosed — a stake source that reports no
// validators while reporting stake has no set to read two thirds of. The count
// floor of an unknown set is not a number, and TwoThirdsCount(0)=1 would hand a
// lone signer a floor of one, so the case is refused rather than computed.
func TestQuasarCountFloor_UnresolvedSetFailsClosed(t *testing.T) {
	vs := newTestValidatorSet(1)
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()}
	const epoch = uint64(1)

	votes := []SignedVote{{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)}}
	cert, err := AssembleQuorumCert(pos, Quasar, 1, votes)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	// Stake is present and the lone voter holds all of it; only the COUNT of the
	// set is unknown. The stake half passes, so this reaches the clause under test.
	unresolved := &stakeMap{w: map[ids.NodeID]uint64{vs.nodeID(0): 10}, total: 10}

	if err := cert.VerifyWeighted(vs, unresolvedCount{unresolved}, epoch); !errors.Is(err, ErrQCBelowThreshold) {
		t.Fatalf("an export certificate over an unresolved set must fail closed, got %v", err)
	}
}

// unresolvedCount reports a real total and no validators — the disagreement a
// count floor has to fail closed on.
type unresolvedCount struct{ *stakeMap }

func (unresolvedCount) ValidatorCount(uint64) int { return 0 }
