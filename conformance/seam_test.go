// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// seam_test.go — the seam the corpus weighs its cases through.
//
// Every verdict in the corpus is the live predicate's answer over a `stake` read
// as a StakeSource. If that seam answers differently from the set a case
// DESCRIBES, the corpus records a decision about a set nobody has — and it would
// record it silently, because the golden file is generated through the same seam
// it would be wrong in. The four numbers are therefore stated here directly,
// against a set whose two denominators are deliberately different.
//
// The keyless seat is the whole point. It holds stake and no key: it is in
// CarriedStake and in none of the other three, and the difference between those
// two numbers is exactly what a floor read over the wrong denominator would get
// wrong.
package conformance

import (
	"errors"
	"testing"

	"github.com/luxfi/consensus/engine/chain"
	"github.com/luxfi/ids"
)

// TestTheSeamAnswersOverTheSigners holds the three quorum projections to the
// SIGNERS and the fourth to the roll. A set of four seats where one is a
// spectator holding more stake than the rest together is the case where reading
// the wrong denominator strands the export rung with every signer agreeing.
func TestTheSeamAnswersOverTheSigners(t *testing.T) {
	set := stake{{weight: 10}, {weight: 20}, {weight: 30}, spectator(100)}

	if got := set.SignerStake(verdictEpoch); got != 60 {
		t.Fatalf("SignerStake = %d, want 60 — the spectator's stake is in no floor", got)
	}
	if got := set.SignerCount(verdictEpoch); got != 3 {
		t.Fatalf("SignerCount = %d, want 3 — the spectator is not a signer", got)
	}
	if got := set.CarriedStake(verdictEpoch); got != 160 {
		t.Fatalf("CarriedStake = %d, want 160 — the roll carries what the chain carries", got)
	}
	// The gap between the two denominators is the number the fourth projection
	// exists to make visible; if they were the same the seat would be pointless.
	if set.CarriedStake(verdictEpoch) == set.SignerStake(verdictEpoch) {
		t.Fatal("the fixture must have a keyless seat, or it states nothing")
	}
}

// TestWeightIsZeroForEverySeatThatCannotSign covers the two ways a vote earns no
// weight, which a tally must treat identically: a voter outside the set, and a
// voter inside it whose vote no verifier would accept.
func TestWeightIsZeroForEverySeatThatCannotSign(t *testing.T) {
	set := stake{{weight: 10}, {weight: 20}, spectator(100)}

	if got := set.Weight(seat(1), verdictEpoch); got != 10 {
		t.Fatalf("a keyed seat's weight is its stake, got %d", got)
	}
	if got := set.Weight(seat(3), verdictEpoch); got != 0 {
		t.Fatalf("a keyless seat contributes %d, want 0", got)
	}
	for _, row := range []struct {
		holds string
		id    ids.NodeID
	}{
		{"a seat past the end of the set", seat(4)},
		{"seat index zero, which no seat has", seat(0)},
		{"an id from another set entirely", ids.GenerateTestNodeID()},
	} {
		t.Run(row.holds, func(t *testing.T) {
			if got := set.Weight(row.id, verdictEpoch); got != 0 {
				t.Fatalf("an unknown voter contributed %d — a tally it can inflate is not a tally", got)
			}
		})
	}
}

// TestTrustCreditsOnlyTheSeatsThatHoldKeys holds the corpus's verifier to the
// same line. A case that credited a keyless seat's signature would state a
// quorum no implementation could ever assemble, because the signature it names
// is under a key that does not exist.
func TestTrustCreditsOnlyTheSeatsThatHoldKeys(t *testing.T) {
	set := stake{{weight: 1}, spectator(1)}
	v := trust{set: set}

	if !v.VerifyVote(seat(1), nil, nil, verdictEpoch) {
		t.Fatal("a keyed seat's vote must be credited, or no case can reach a quorum")
	}
	if v.VerifyVote(seat(2), nil, nil, verdictEpoch) {
		t.Fatal("a keyless seat has no signature to credit")
	}
	if v.VerifyVote(ids.GenerateTestNodeID(), nil, nil, verdictEpoch) {
		t.Fatal("a seat outside the set has no signature to credit")
	}
}

// TestEveryRefusalTheCorpusRecordsHasAName holds the mapping from the engine's
// errors onto the classes another language can compare against, and holds the
// guard behind it: a refusal the corpus cannot name is a hard failure rather
// than an unnamed row, because a row nobody else can reproduce is worse than no
// row at all.
func TestEveryRefusalTheCorpusRecordsHasAName(t *testing.T) {
	for _, row := range []struct {
		err  error
		name string
	}{
		{nil, ""},
		{chain.ErrQCBelowThreshold, "belowThreshold"},
		{chain.ErrQCStakeBelowMajority, "stakeBelowMajority"},
		{chain.ErrQCStakeBelowSupermajority, "stakeBelowSupermajority"},
	} {
		if got := refusal(row.err); got != row.name {
			t.Fatalf("refusal(%v) = %q, want %q", row.err, got, row.name)
		}
	}

	// The guard. An error class the corpus has no name for must stop the corpus
	// being written, not be recorded as something it is not.
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("an unnamed refusal was recorded instead of refused")
			}
		}()
		_ = refusal(errors.New("a refusal from somewhere the corpus does not know about"))
	}()
}
