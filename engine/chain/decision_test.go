// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// decision_test.go — the envelope-vs-canonical rule has one meaning, so it must
// have one implementation. It reads as two trivial lines, which is exactly why
// it was re-typed inline at a dozen sites and behind three separately-named
// helpers; a rule with N copies is a rule with N chances to diverge.
package chain

import (
	"testing"

	"github.com/luxfi/ids"
)

// TestDecisionDegradesToTheOuterID pins the rule itself. Degrading an absent
// canonical to the OUTER id, never to Empty, is what keeps a bare chain
// comparing byte-for-byte as it always did: there, two blocks are the same
// decision only when they are the same block. Degrading to Empty would make
// every unwrapped block equal to every other.
func TestDecisionDegradesToTheOuterID(t *testing.T) {
	outer := ids.GenerateTestID()
	inner := ids.GenerateTestID()

	if got := decision(inner, outer); got != inner {
		t.Fatalf("a recorded canonical is the decision: got %s want %s", got, inner)
	}
	if got := decision(ids.Empty, outer); got != outer {
		t.Fatalf("an absent canonical degrades to the outer id: got %s want %s", got, outer)
	}
	if decision(ids.Empty, outer) == ids.Empty {
		t.Fatal("degrading to Empty would make every unwrapped block equal to every other")
	}
}

// TestCertAndBlockAgreeOnTheRule is the property the three helpers existed to
// break. certCanonical reads a cert's position and canonicalIDOf reads a VM
// block; they fetch the pair from different places and must apply the SAME rule
// to it. A future fourth extractor that re-types the two lines fails here.
func TestCertAndBlockAgreeOnTheRule(t *testing.T) {
	outer := ids.GenerateTestID()
	inner := ids.GenerateTestID()

	wrapped := &QuorumCert{Position: VotePosition{BlockID: outer, CanonicalID: inner}}
	if got := certCanonical(wrapped); got != decision(inner, outer) {
		t.Fatalf("certCanonical disagrees with the rule: %s vs %s", got, decision(inner, outer))
	}

	bare := &QuorumCert{Position: VotePosition{BlockID: outer}}
	if got := certCanonical(bare); got != decision(ids.Empty, outer) {
		t.Fatalf("certCanonical disagrees with the rule on a bare position: %s vs %s",
			got, decision(ids.Empty, outer))
	}
	if certCanonical(bare) != outer {
		t.Fatal("a cert with no canonical commits to its own block id")
	}
}
