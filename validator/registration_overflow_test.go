// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// registration_overflow_test.go — the two clauses that are about ARITHMETIC
// rather than about identity, plus the nil entry.
//
// The total weight is what both stake floors are taken of, and Go's + is
// modular: a sum past 2^64 does not fail, it returns a different number and
// every floor read against it is a floor read against a lie. Two voters at 2^63
// wrap to zero, and a "majority of zero" is any vote at all. So both doors check
// the add before they take it, and both must — the two doors admit the same
// sets, and a set one accepts and the other wraps on is a set two nodes disagree
// about the existence of.
package validators

import (
	"errors"
	"math"
	"testing"

	"github.com/luxfi/ids"
)

// TestRegisterRefusesATotalThatWraps holds the admission door's arithmetic. The
// two weights are individually representable and their sum is not; the door must
// name the overflow rather than admit a set whose total is smaller than one of
// its members.
func TestRegisterRefusesATotalThatWraps(t *testing.T) {
	big := registration(t, 41, 0x01, math.MaxUint64-1)
	rest := registration(t, 42, 0x02, 2)

	set, err := Register([]Registration{big, rest})
	if !errors.Is(err, ErrWeightOverflow) {
		t.Fatalf("want the overflow refusal, got %v", err)
	}
	if len(set.Validators) != 0 || set.TotalWeight != 0 {
		t.Fatal("a refused registration returned a set — a caller must not read one out of a no")
	}

	// The same two weights one unit apart do fit, so the refusal above is the
	// boundary and not a blanket rejection of large stake.
	fits := registration(t, 42, 0x02, 1)
	ok, err := Register([]Registration{big, fits})
	if err != nil {
		t.Fatalf("a total of exactly MaxUint64 is representable: %v", err)
	}
	if ok.TotalWeight != math.MaxUint64 {
		t.Fatalf("total weight = %d, want MaxUint64", ok.TotalWeight)
	}
}

// TestRegisterRefusesAKeyedSeatWithNoStake is the phantom signer at the
// admission door — the seat that raises the count of distinct signers a floor is
// read against and raises the weight by nothing. Flatten refuses it with the
// same error; that agreement is what makes the two doors one rule.
func TestRegisterRefusesAKeyedSeatWithNoStake(t *testing.T) {
	good := registration(t, 51, 0x01, 10)
	phantom := registration(t, 52, 0x02, 0)

	set, err := Register([]Registration{good, phantom})
	if !errors.Is(err, ErrZeroWeight) {
		t.Fatalf("want the zero-weight refusal, got %v", err)
	}
	if len(set.Validators) != 0 {
		t.Fatal("the set is admitted whole or not at all")
	}
}

// TestFlattenRefusesATotalThatWraps is the same arithmetic on the other door,
// and it is checked BEFORE the key is looked at — a set whose total does not
// exist is refused whether or not this node could have parsed its keys.
func TestFlattenRefusesATotalThatWraps(t *testing.T) {
	a, first := output(0x01, secret(t, 61), math.MaxUint64-1)
	b, second := output(0x02, secret(t, 62), 2)

	set, err := FlattenValidatorSet(map[ids.NodeID]*GetValidatorOutput{a: first, b: second})
	if !errors.Is(err, ErrWeightOverflow) {
		t.Fatalf("want the overflow refusal, got %v", err)
	}
	if len(set.Validators) != 0 || set.TotalWeight != 0 {
		t.Fatal("a refused flatten returned a set")
	}
}

// TestFlattenSkipsAnAbsentEntry covers the map that names a node and holds
// nothing for it. It is neither a member nor a defect — it contributes no
// weight, no key and no seat, and the set around it is still the set.
func TestFlattenSkipsAnAbsentEntry(t *testing.T) {
	present, entry := output(0x01, secret(t, 71), 10)
	absent := nodeID(0x02)

	set, err := FlattenValidatorSet(map[ids.NodeID]*GetValidatorOutput{
		present: entry,
		absent:  nil,
	})
	if err != nil {
		t.Fatalf("an absent entry is not a refusal: %v", err)
	}
	if len(set.Validators) != 1 {
		t.Fatalf("the set has %d validators, want the one that is actually there", len(set.Validators))
	}
	if set.TotalWeight != 10 {
		t.Fatalf("total weight = %d, want 10 — an absent entry weighs nothing", set.TotalWeight)
	}
}
