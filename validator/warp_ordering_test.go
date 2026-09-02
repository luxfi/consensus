// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// warp_ordering_test.go — the three functions a warp message is verified
// through, exercised through THIS package's names.
//
// They are re-exports, and a re-export is exactly the thing that drifts without
// saying so: a bump that changes an argument order, a name bound to the wrong
// function, a behaviour that quietly stops refusing something. Nothing in this
// repository calls them, so nothing else would notice.
//
// What a warp verifier does with them is one sentence: take the canonical set
// and the signers' bit indices off the wire, FILTER the set to those indices,
// SUM their weight against the threshold, AGGREGATE their keys, and check the
// aggregate signature against that aggregate key. The three steps share one
// ordering — the canonical order of the set — and a signature verifies only if
// every step read the same seats. So the properties tested here are the ones a
// forged warp message would need broken: that the indices select exactly the
// seats they name, that a weight cannot wrap past the threshold, and that the
// aggregate key is the aggregate of exactly the selected seats.
package validators

import (
	"errors"
	"math"
	"testing"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
)

// canonical builds a set of n proven validators in this package's canonical
// order, which is the order the three functions below index into.
func canonical(t *testing.T, n int, weight uint64) CanonicalValidatorSet {
	t.Helper()
	rs := make([]Registration, 0, n)
	for i := 0; i < n; i++ {
		rs = append(rs, registration(t, byte(0x80+i), byte(0x10+i), weight))
	}
	set, err := Register(rs)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return set
}

// TestFilterSelectsExactlyTheSeatsNamed holds the step a forged message would
// attack first: the indices must select the seats they name and no others, in
// the canonical order, so the key the aggregate is checked against belongs to
// the validators the message claims signed it.
func TestFilterSelectsExactlyTheSeatsNamed(t *testing.T) {
	vdrs := canonical(t, 4, 10).Validators

	bits := set.NewBits()
	bits.Add(0)
	bits.Add(2)

	got, err := FilterValidators(bits, vdrs)
	if err != nil {
		t.Fatalf("FilterValidators: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("filtered to %d validators, want 2", len(got))
	}
	if got[0] != vdrs[0] || got[1] != vdrs[2] {
		t.Fatal("the filter did not return the seats the indices name, in canonical order")
	}

	// No indices selects no seats — an empty signer set is not the whole set.
	empty, err := FilterValidators(set.NewBits(), vdrs)
	if err != nil {
		t.Fatalf("FilterValidators over no indices: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("no indices selected %d validators", len(empty))
	}
}

// TestFilterRefusesAnIndexTheSetDoesNotHave is the fail-closed half. A message
// naming a seat past the end of the set is a message about a validator that does
// not exist, and admitting it would mean verifying a signature against a shorter
// set than the one the indices were built for.
func TestFilterRefusesAnIndexTheSetDoesNotHave(t *testing.T) {
	vdrs := canonical(t, 3, 10).Validators

	bits := set.NewBits()
	bits.Add(3) // the fourth seat of a three-seat set

	got, err := FilterValidators(bits, vdrs)
	if !errors.Is(err, ErrUnknownValidator) {
		t.Fatalf("want the unknown-validator refusal, got %v", err)
	}
	if got != nil {
		t.Fatal("a refused filter returned a validator list")
	}
}

// TestSumWeightRefusesToWrap is the threshold's arithmetic. A wrapped sum does
// not fail, it returns a different number, and the number a warp threshold is
// compared against must be the sum it claims to be.
func TestSumWeightRefusesToWrap(t *testing.T) {
	got, err := SumWeight(canonical(t, 3, 10).Validators)
	if err != nil {
		t.Fatalf("SumWeight: %v", err)
	}
	if got != 30 {
		t.Fatalf("SumWeight = %d, want 30", got)
	}
	if empty, err := SumWeight(nil); err != nil || empty != 0 {
		t.Fatalf("SumWeight over no validators = (%d, %v), want (0, nil)", empty, err)
	}

	// Built by hand rather than through Register, which refuses the same total
	// at admission. SumWeight is handed a list that came off the wire through
	// the filter, so it does its own arithmetic and must refuse independently:
	// two weights that wrap to two would otherwise clear any threshold below it.
	huge := []*CanonicalValidator{
		{Weight: math.MaxUint64 - 1},
		{Weight: 3},
	}
	if _, err := SumWeight(huge); !errors.Is(err, ErrWeightOverflow) {
		t.Fatalf("a total past MaxUint64 must be refused, not wrapped: %v", err)
	}
}

// TestTheAggregateKeyIsTheKeyTheSignatureVerifiesUnder is the step the other two
// exist to serve, checked the only way that means anything: sign the same
// message with each selected validator's secret, aggregate the signatures, and
// verify the aggregate against the key AggregatePublicKeys derives. A seat left
// out of either aggregate makes it fail, which is what binds the filtered set to
// the signature.
func TestTheAggregateKeyIsTheKeyTheSignatureVerifiesUnder(t *testing.T) {
	// Build the set and keep the secrets, in the SAME canonical order the
	// filter indexes into, by looking each validator's key up by its bytes.
	const n = 4
	secrets := make(map[string]*bls.SecretKey, n)
	rs := make([]Registration, 0, n)
	for i := 0; i < n; i++ {
		sk := secret(t, byte(0x90+i))
		secrets[string(bls.PublicKeyToCompressedBytes(sk.PublicKey()))] = sk
		rs = append(rs, registration(t, byte(0x90+i), byte(0x20+i), 10))
	}
	set0, err := Register(rs)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	bits := set.NewBits()
	bits.Add(1)
	bits.Add(3)
	signers, err := FilterValidators(bits, set0.Validators)
	if err != nil {
		t.Fatalf("FilterValidators: %v", err)
	}

	message := []byte("a warp message the selected seats agree on")
	sigs := make([]*bls.Signature, 0, len(signers))
	for _, v := range signers {
		sk, ok := secrets[string(v.PublicKeyBytes)]
		if !ok {
			t.Fatal("a filtered validator has no secret — the canonical order was not preserved")
		}
		sig, err := sk.Sign(message)
		if err != nil {
			t.Fatalf("Sign: %v", err)
		}
		sigs = append(sigs, sig)
	}
	agg, err := bls.AggregateSignatures(sigs)
	if err != nil {
		t.Fatalf("AggregateSignatures: %v", err)
	}

	key, err := AggregatePublicKeys(signers)
	if err != nil {
		t.Fatalf("AggregatePublicKeys: %v", err)
	}
	if !bls.Verify(key, agg, message) {
		t.Fatal("the aggregate signature does not verify under the aggregate of the seats that made it")
	}

	// The binding, stated the other way: the aggregate of a DIFFERENT selection
	// does not verify it. Without this the row above would pass for a function
	// that returned any key at all.
	otherBits := set.NewBits()
	otherBits.Add(0)
	otherBits.Add(2)
	others, err := FilterValidators(otherBits, set0.Validators)
	if err != nil {
		t.Fatalf("FilterValidators: %v", err)
	}
	otherKey, err := AggregatePublicKeys(others)
	if err != nil {
		t.Fatalf("AggregatePublicKeys: %v", err)
	}
	if bls.Verify(otherKey, agg, message) {
		t.Fatal("a signature verified under the aggregate of seats that did not sign it")
	}
}

// TestNewManagerReturnsALiveManager holds the constructor to what a caller needs
// from it: a manager that is empty to start with and remembers what it is given,
// per network. A nil interface, or one bound to a shared instance, would both
// pass a test that only checked for non-nil.
func TestNewManagerReturnsALiveManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nothing")
	}

	netID, other := ids.GenerateTestID(), ids.GenerateTestID()
	node := ids.GenerateTestNodeID()
	if got := m.Count(netID); got != 0 {
		t.Fatalf("a fresh manager holds %d validators, want 0", got)
	}

	sk := secret(t, 0xA0)
	if err := m.AddStaker(netID, node, bls.PublicKeyToCompressedBytes(sk.PublicKey()), ids.GenerateTestID(), 42); err != nil {
		t.Fatalf("AddStaker: %v", err)
	}
	if got := m.Count(netID); got != 1 {
		t.Fatalf("the manager holds %d validators, want 1", got)
	}
	if got := m.GetWeight(netID, node); got != 42 {
		t.Fatalf("weight = %d, want 42", got)
	}
	// The network is part of the key: a staker on one network is not on another.
	if got := m.Count(other); got != 0 {
		t.Fatalf("another network holds %d validators, want 0", got)
	}

	// And a second manager is a second manager.
	if got := NewManager().Count(netID); got != 0 {
		t.Fatalf("a new manager already holds %d validators", got)
	}
}
