// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// tally_overflow_test.go — the tally has to be the sum it claims to be.
//
// Both rungs decide by comparing a summed stake against a floor read off the
// total. Go's + is modular, so an unchecked loop does not fail when the weights
// a StakeSource reports run past 2^64 — it returns a DIFFERENT number and the
// comparison proceeds as if nothing happened. That is the fail-open direction:
// the wrapped value can sit above the floor while the stake it claims to
// represent does not exist, and the cert is accepted on arithmetic rather than
// on votes.
//
// Each case here states the wrap outright — the value the old loop produced,
// and the floor it cleared — and then asserts the predicate refuses. Stating
// the wrap is what makes these tests bite: without it they would pass against
// an implementation that simply refused everything.
package chain

import (
	"errors"
	"math"
	"testing"

	"github.com/luxfi/consensus/config"
	validators "github.com/luxfi/consensus/validator"
	"github.com/luxfi/ids"
)

// wrappingStake reports per-voter weights that sum past 2^64 against a total
// that does not. No admitted set can hold these numbers — Register and
// FlattenValidatorSet both refuse a set whose weights overflow — so reaching
// this is evidence about the SOURCE, not about the votes.
func wrappingStake(vs *testValidatorSet, n int, weights map[int]uint64) *stakeMap {
	s := &stakeMap{w: make(map[ids.NodeID]uint64, n), total: math.MaxUint64}
	for i := 0; i < n; i++ {
		s.w[vs.nodeID(i)] = weights[i]
	}
	return s
}

// TestNovaTallyRefusesAWrappingStakeSource — three voters at 2^63 sum to 2^64 +
// 2^63, which wraps to 2^63 and clears floor(total/2) = 2^63 − 1 by exactly one.
// The signer floor is met, so nothing else stands in the way: before the tally
// was checked this cert was ACCEPTED at the local-execution rung.
func TestNovaTallyRefusesAWrappingStakeSource(t *testing.T) {
	vs := newTestValidatorSet(5)
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()}
	const epoch = uint64(1) // == pos.Height; the stake map is height-independent

	const half = uint64(1) << 63
	src := wrappingStake(vs, 5, map[int]uint64{0: half, 1: half, 2: half})

	votes := make([]SignedVote, 0, 3)
	for _, i := range []int{0, 1, 2} {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	cert := certDeclaring(t, pos, Nova, 5, votes)

	// The count clause must be satisfied, or the case would be refused before
	// the tally is ever read and would prove nothing about it.
	if floor := NovaSignerFloor(src.SignerCount(epoch)); cert.VoterCount() < floor {
		t.Fatalf("this set does not reach the signer floor (%d of %d), so the tally is never read",
			cert.VoterCount(), floor)
	}

	// The wrap, stated outright: what the unchecked loop computed, and the floor
	// it cleared. If either stops holding, this case no longer demonstrates the
	// fail-open and the assertion below would pass for the wrong reason.
	var wrapped uint64
	for i := range cert.Votes {
		wrapped += src.Weight(cert.Votes[i].NodeID, epoch)
	}
	if wrapped != half {
		t.Fatalf("the tally no longer wraps to 2^63 (got %d) — reread this case", wrapped)
	}
	if got := config.HalfStakeFloor(src.SignerStake(epoch)); wrapped <= got {
		t.Fatalf("the wrapped tally %d no longer clears floor(total/2)=%d, so there is no "+
			"fail-open left to demonstrate", wrapped, got)
	}

	// And the predicate refuses it, on the tally rather than on the floor.
	err := cert.VerifyWeighted(vs, src, epoch)
	if !errors.Is(err, validators.ErrWeightOverflow) {
		t.Fatalf("a wrapping tally cleared the Nova majority: %v", err)
	}
}

// TestQuasarTallyRefusesAWrappingStakeSource — the same hole at the EXPORT rung,
// which is the one a bridge admits on. Two voters at 2^63 + 7·10^18 sum past
// 2^64 and wrap to 1.4·10^19, above floor(2·total/3). Quasar carries no signer
// floor, so the tally is the only thing between this cert and export-grade
// finality.
func TestQuasarTallyRefusesAWrappingStakeSource(t *testing.T) {
	vs := newTestValidatorSet(5)
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()}
	const epoch = uint64(1)

	// Two seats hold a weight that wraps when summed, and two hold a unit each so
	// the certificate reaches the export rung's DISTINCT-SIGNER floor of four. The
	// count clause has to be satisfied or the tally is never read, and a row about
	// the tally that is answered by the count proves nothing about the tally.
	const w = uint64(1)<<63 + 7_000_000_000_000_000_000
	src := wrappingStake(vs, 5, map[int]uint64{0: w, 1: w, 2: 1, 3: 1})

	votes := make([]SignedVote, 0, 4)
	for _, i := range []int{0, 1, 2, 3} {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	// Declaring the floor the five-signer set derives, so the row reaches the tally
	// rather than being answered by a threshold the certificate named for itself.
	cert := certDeclaring(t, pos, Quasar, 5, votes)
	if floor := SignerFloor(Quasar, src.SignerCount(epoch)); cert.VoterCount() < floor {
		t.Fatalf("this set does not reach the export signer floor (%d of %d), so the tally is never read",
			cert.VoterCount(), floor)
	}

	var wrapped uint64
	for i := range cert.Votes {
		wrapped += src.Weight(cert.Votes[i].NodeID, epoch)
	}
	if got := config.TwoThirdsStakeFloor(src.SignerStake(epoch)); wrapped <= got {
		t.Fatalf("the wrapped tally %d no longer clears floor(2*total/3)=%d, so there is no "+
			"fail-open left to demonstrate", wrapped, got)
	}

	err := cert.VerifyWeighted(vs, src, epoch)
	if !errors.Is(err, validators.ErrWeightOverflow) {
		t.Fatalf("a wrapping tally cleared the export supermajority: %v", err)
	}
}

// TestTallyStillSumsASetThatFits — the guard must not have made the honest path
// stricter. A set whose voters sum to exactly the widest representable total is
// still tallied and still decided on its merits.
func TestTallyStillSumsASetThatFits(t *testing.T) {
	vs := newTestValidatorSet(minBFTCommittee)
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()}
	const epoch = uint64(1)

	const half = uint64(1) << 63
	// Every seat carries stake and every seat signs. A weightless seat would be
	// refused by both admission doors (validators.ErrZeroWeight), and a set below
	// minBFTCommittee signers is refused by the export rung's committee clause —
	// neither would reach the tally this test is about.
	src := &stakeMap{
		w: map[ids.NodeID]uint64{
			vs.nodeID(0): half,
			vs.nodeID(1): half - 3,
			vs.nodeID(2): 1,
			vs.nodeID(3): 1,
		},
		total: math.MaxUint64, // == half + (half-3) + 1 + 1, the widest total that fits
	}

	votes := make([]SignedVote, 0, minBFTCommittee)
	for _, i := range []int{0, 1, 2, 3} {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	cert := certDeclaring(t, pos, Quasar, minBFTCommittee, votes)
	if err := cert.VerifyWeighted(vs, src, epoch); err != nil {
		t.Fatalf("a set that fits was refused: %v", err)
	}
}
