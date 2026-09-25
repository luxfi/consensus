// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// nova_stake_majority_test.go — acceptance over a set whose weights are not uniform.
//
// A block is accepted only on the ⅔ certificate: its signers hold more than two thirds of
// the signing stake AND number at least TwoThirdsCount(n) of the n signers. Both halves
// bind here. The shape pinned is a five-validator fleet of equal weight plus a sixth entry
// registered at minValidatorStake that never votes: a stake majority of the fleet accepts
// nothing, and the seat half of the certificate counts the sixth entry — four of the five
// fleet validators hold 80% of the stake and still fall one seat short of five of six.
//
// The last test pins the Nova rung's own floor definitions (NovaQuorum, NovaSignerFloor),
// which no longer decide acceptance but still define that rung's certificate.
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/constants"
	"github.com/luxfi/ids"
)

const (
	fleetWeight = 500_000_000_000_000_000 // 5e17 nLUX, the weight each fleet validator carries
	minWeight   = 2_000_000_000           // 2e9 nLUX = 2 LUX = minValidatorStake
)

// dyn6 sizes the committee to SIX registered validators, which is what effectiveCommittee
// produces for a six-entry set (bftCommittee clamps an oversized preset K down to the
// resolved count).
func dyn6() config.Parameters { return config.FeasibleParams(constants.LocalID, 6) }

// TestSixEntrySetAcceptsOnlyAtTwoThirdsOfStakeAndSeats: six registered validators, five of
// them the fleet, the sixth at the minimum stake and silent. Three of the five — a stake
// majority, 1.5e18 of 2.500000002e18 — accept nothing. Four of the five hold 80% of the
// stake, past ⅔, but are four of six seats, short of TwoThirdsCount(6)=5, and accept
// nothing either. All five fleet validators clear both halves, and the block is accepted.
func TestSixEntrySetAcceptsOnlyAtTwoThirdsOfStakeAndSeats(t *testing.T) {
	// Index 5 carries the minimum stake and never votes.
	run := func(t *testing.T, label string, voters []int) (*Transitive, *verifyOnceBlock) {
		t.Helper()
		vs := newTestValidatorSet(6)
		stake := newStakeMap(vs, fleetWeight, fleetWeight, fleetWeight, fleetWeight, fleetWeight, minWeight)
		blk := newTestBlock(1, ids.Empty, label)
		e, _ := driveSignedAcceptsAt(t, dyn6(), vs, stake, &recordingGossiper{}, blk, voters)
		return e, blk
	}
	t.Run("three of the fleet", func(t *testing.T) {
		e, blk := run(t, "fleet-three", []int{1, 2, 3})
		mustNotFinalize(t, e, blk, time.Second, "3 of the 5 fleet validators hold 60% of stake — not ⅔")
	})
	t.Run("four of the fleet", func(t *testing.T) {
		e, blk := run(t, "fleet-four", []int{1, 2, 3, 4})
		mustNotFinalize(t, e, blk, time.Second, "4 of 6 seats hold 80% of stake — one seat short of TwoThirdsCount(6)=5")
	})
	t.Run("the whole fleet", func(t *testing.T) {
		e, blk := run(t, "fleet-five", []int{0, 1, 2, 3, 4})
		mustFinalize(t, e, blk, 2*time.Second, "5 of 6 seats holding all but the minimum stake")
		mustQuasar(t, e, blk, 2*time.Second, "5 of 6 seats holding all but the minimum stake (export)")
	})
}

// TestMinimumStakeEntryCannotCastTheDecidingVote is the safety half: the minimum-stake
// entry must not COMPLETE a quorum. Two fleet validators (1e18) plus that entry are three
// distinct signers, but 1.000000002e18 is not two thirds of 2.500000002e18, so nothing may
// accept.
func TestMinimumStakeEntryCannotCastTheDecidingVote(t *testing.T) {
	vs := newTestValidatorSet(6)
	stake := newStakeMap(vs, fleetWeight, fleetWeight, fleetWeight, fleetWeight, fleetWeight, minWeight)

	blk := newTestBlock(1, ids.Empty, "minimum-stake-buys-a-vote")
	e, _ := driveSignedAcceptsAt(t, dyn6(), vs, stake, &recordingGossiper{}, blk, []int{1, 2, 5})

	mustNotFinalize(t, e, blk, time.Second,
		"two fleet validators plus a minimum-stake entry hold 40% of stake — not ⅔")
}

// TestSeatFloorHoldsAgainstAStakeSupermajorityOfOne keeps the 1085013 guard honest under
// the stake reading: one validator holding 96% of the stake is past ⅔ of it by itself, and
// must still never accept alone. The certificate also counts seats, TwoThirdsCount(5)=4:
// two and three signers holding 97% and 98% accept nothing; four do.
func TestSeatFloorHoldsAgainstAStakeSupermajorityOfOne(t *testing.T) {
	// Each case gets its own validator set and engine; sharing one across engines in a
	// single test crosses signer state between them.
	skewed := func(t *testing.T, label string, voters []int) (*Transitive, *verifyOnceBlock) {
		t.Helper()
		vs := newTestValidatorSet(5)
		blk := newTestBlock(1, ids.Empty, label)
		e, _ := driveSignedAccepts(t, vs, newStakeMap(vs, 96, 1, 1, 1, 1), &recordingGossiper{}, blk, voters)
		return e, blk
	}

	t.Run("lone heavy holder", func(t *testing.T) {
		e, blk := skewed(t, "lone-heavy", []int{0})
		mustNotFinalize(t, e, blk, time.Second, "a single validator holding 96% of stake self-igniting")
	})
	t.Run("two signers", func(t *testing.T) {
		e, blk := skewed(t, "heavy-pair", []int{0, 1})
		mustNotFinalize(t, e, blk, time.Second, "two signers, below the seat floor")
	})
	t.Run("three signers", func(t *testing.T) {
		e, blk := skewed(t, "heavy-trio", []int{0, 1, 2})
		mustNotFinalize(t, e, blk, time.Second, "three signers holding 98% of stake, below the seat floor of four")
	})
	t.Run("four signers", func(t *testing.T) {
		e, blk := skewed(t, "heavy-quad", []int{0, 1, 2, 3})
		mustFinalize(t, e, blk, 2*time.Second, "four signers holding 99% of stake")
	})
}

// TestNovaSignerFloorMatchesCountMajorityOnEqualStake pins the Nova rung's definitions:
// for a uniform fleet its stake reading and head-count reading pick the SAME majority at
// every size, k·w > ⌊n·w/2⌋ ⟺ k > n/2, and its signer floor never exceeds it.
func TestNovaSignerFloorMatchesCountMajorityOnEqualStake(t *testing.T) {
	for n := 1; n <= 32; n++ {
		total := uint64(n) * fleetWeight
		half := config.HalfStakeFloor(total)
		want := NovaQuorum(n)
		got := 0
		for k := 0; k <= n; k++ {
			if uint64(k)*fleetWeight > half {
				got = k
				break
			}
		}
		if got != want {
			t.Fatalf("n=%d: stake majority needs %d signers, head-count majority needs %d", n, got, want)
		}
		if floor := NovaSignerFloor(n); floor > want {
			t.Fatalf("n=%d: signer floor %d exceeds the count majority %d — it must only ever be lower", n, floor, want)
		}
	}
}
