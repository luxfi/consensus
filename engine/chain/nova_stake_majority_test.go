// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// nova_stake_majority_test.go — Nova ignition over a set whose weights are not uniform.
//
// The shape these tests pin is a five-validator fleet of equal weight plus a sixth entry
// registered at minValidatorStake that never votes. Read as a head-count, that sixth entry
// moves ⌊n/2⌋+1 from 3 to 4: the fleet now needs four of its five to agree and tolerates
// one loss where it should tolerate two, and the same entry can cast one of those four
// votes. Registration is open at the minimum stake, so a head-count gate is for sale.
//
// Read as a majority of STAKE, neither is possible, and on a uniform fleet the two readings
// pick the identical quorum at every set size — so nothing moves where weights are equal.
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

// TestMinimumStakeEntryCannotRaiseNovaIgnition: six registered validators, five of them the
// fleet. THREE of the five sign — a majority of the stake (1.5e18 of 2.500000002e18) and the
// exact quorum the engine's own "survive 3 of 5" mandate promises. Read as a head-count this
// is 3 of 6, short of NovaQuorum(6)=4, and nothing accepts.
func TestMinimumStakeEntryCannotRaiseNovaIgnition(t *testing.T) {
	vs := newTestValidatorSet(6)
	// Index 5 carries the minimum stake and never votes.
	stake := newStakeMap(vs, fleetWeight, fleetWeight, fleetWeight, fleetWeight, fleetWeight, minWeight)

	blk := newTestBlock(1, ids.Empty, "minimum-stake-raises-the-bar")
	e, _ := driveSignedAcceptsAt(t, dyn6(), vs, stake, &recordingGossiper{}, blk, []int{1, 2, 3})

	mustFinalize(t, e, blk, 2*time.Second,
		"3 of the 5 fleet validators hold 60% of stake — a minimum-stake entry must not raise the gate")
}

// TestMinimumStakeEntryCannotCastTheDecidingVote is the other half, and it is a safety
// property rather than a liveness one: the minimum-stake entry must not COMPLETE a quorum
// either. Two fleet validators (1e18) plus that entry are three distinct signers — a
// head-count clearing the signer floor — but 1.000000002e18 is not a majority of
// 2.500000002e18, so nothing may accept.
func TestMinimumStakeEntryCannotCastTheDecidingVote(t *testing.T) {
	vs := newTestValidatorSet(6)
	stake := newStakeMap(vs, fleetWeight, fleetWeight, fleetWeight, fleetWeight, fleetWeight, minWeight)

	blk := newTestBlock(1, ids.Empty, "minimum-stake-buys-a-vote")
	e, _ := driveSignedAcceptsAt(t, dyn6(), vs, stake, &recordingGossiper{}, blk, []int{1, 2, 5})

	mustNotFinalize(t, e, blk, time.Second,
		"two fleet validators plus a minimum-stake entry hold 40% of stake — not a majority")
}

// TestNovaSignerFloorHoldsAgainstAStakeMajorityOfOne keeps the 1085013 guard honest under
// the stake reading: one validator holding 96% of the stake is a stake majority by itself,
// and must still never self-ignite. Two signers do not clear NovaSignerFloor either; three
// do, and 96+1+1 is a majority, so the block accepts there.
func TestNovaSignerFloorHoldsAgainstAStakeMajorityOfOne(t *testing.T) {
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
		mustNotFinalize(t, e, blk, time.Second, "two signers, below the Nova signer floor")
	})
	t.Run("three signers", func(t *testing.T) {
		e, blk := skewed(t, "heavy-trio", []int{0, 1, 2})
		mustFinalize(t, e, blk, 2*time.Second, "three signers holding 98% of stake")
	})
}

// TestNovaSignerFloorMatchesCountMajorityOnEqualStake is the no-op proof: for a uniform
// fleet the stake reading and the head-count reading pick the SAME quorum at every size,
// so nothing on any live Lux network moves. k·w > ⌊n·w/2⌋ ⟺ k > n/2.
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
