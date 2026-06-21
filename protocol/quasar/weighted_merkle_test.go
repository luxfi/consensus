// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"testing"
)

// mkLeaf builds a deterministic test leaf with a distinct id/pubkey/weight.
func mkLeaf(idByte byte, weight uint64, param uint8, keyVer uint32) WeightedValidatorLeaf {
	var id [32]byte
	for i := range id {
		id[i] = idByte
	}
	pk := make([]byte, 16)
	for i := range pk {
		pk[i] = idByte ^ byte(i)
	}
	return WeightedValidatorLeaf{
		ValidatorID:    id,
		PublicKey:      pk,
		VotingWeight:   weight,
		ParameterSetID: param,
		KeyVersion:     keyVer,
	}
}

func TestWeightedMerkle_BuildAndRootDeterminism(t *testing.T) {
	leaves := []WeightedValidatorLeaf{
		mkLeaf(0x03, 10, 0x42, 1),
		mkLeaf(0x01, 20, 0x42, 1),
		mkLeaf(0x02, 30, 0x42, 1),
	}
	s1, err := BuildWeightedValidatorSet(7, leaves)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// Re-order input: root must be identical (canonical sort by id).
	reordered := []WeightedValidatorLeaf{leaves[2], leaves[0], leaves[1]}
	s2, err := BuildWeightedValidatorSet(7, reordered)
	if err != nil {
		t.Fatalf("build reordered: %v", err)
	}
	if s1.Root() != s2.Root() {
		t.Fatalf("root not invariant under input ordering: %x vs %x", s1.Root(), s2.Root())
	}
	var zero [48]byte
	if s1.Root() == zero {
		t.Fatal("root is zero for non-empty set")
	}
}

func TestWeightedMerkle_EpochBinding(t *testing.T) {
	leaves := []WeightedValidatorLeaf{mkLeaf(0x01, 10, 0x42, 1), mkLeaf(0x02, 10, 0x42, 1)}
	a, _ := BuildWeightedValidatorSet(7, leaves)
	b, _ := BuildWeightedValidatorSet(8, leaves)
	if a.Root() == b.Root() {
		t.Fatal("epoch is not bound into the root (different epochs collided)")
	}
}

func TestWeightedMerkle_WeightBinding(t *testing.T) {
	// The whole point of the task: bind WEIGHT. Flip a single weight, expect
	// a different root.
	base := []WeightedValidatorLeaf{mkLeaf(0x01, 10, 0x42, 1), mkLeaf(0x02, 20, 0x42, 1)}
	mut := []WeightedValidatorLeaf{mkLeaf(0x01, 10, 0x42, 1), mkLeaf(0x02, 21, 0x42, 1)}
	a, _ := BuildWeightedValidatorSet(7, base)
	b, _ := BuildWeightedValidatorSet(7, mut)
	if a.Root() == b.Root() {
		t.Fatal("voting weight is not bound into the leaf (weight flip collided)")
	}
}

func TestWeightedMerkle_ParamAndKeyVersionBinding(t *testing.T) {
	base := []WeightedValidatorLeaf{mkLeaf(0x01, 10, 0x42, 1)}
	param := []WeightedValidatorLeaf{mkLeaf(0x01, 10, 0x43, 1)}
	keyv := []WeightedValidatorLeaf{mkLeaf(0x01, 10, 0x42, 2)}
	a, _ := BuildWeightedValidatorSet(7, base)
	b, _ := BuildWeightedValidatorSet(7, param)
	c, _ := BuildWeightedValidatorSet(7, keyv)
	if a.Root() == b.Root() {
		t.Fatal("parameter_set_id not bound into leaf")
	}
	if a.Root() == c.Root() {
		t.Fatal("key_version not bound into leaf")
	}
}

func TestWeightedMerkle_RejectsBadInput(t *testing.T) {
	if _, err := BuildWeightedValidatorSet(7, nil); err != ErrWVSetEmpty {
		t.Fatalf("empty set err = %v, want ErrWVSetEmpty", err)
	}
	// Duplicate id.
	dup := []WeightedValidatorLeaf{mkLeaf(0x01, 10, 0x42, 1), mkLeaf(0x01, 20, 0x42, 1)}
	if _, err := BuildWeightedValidatorSet(7, dup); err == nil {
		t.Fatal("duplicate validator_id accepted, want ErrWVSetDuplicateID")
	}
	// Zero weight.
	zw := []WeightedValidatorLeaf{mkLeaf(0x01, 0, 0x42, 1)}
	if _, err := BuildWeightedValidatorSet(7, zw); err == nil {
		t.Fatal("zero weight accepted, want ErrWVSetZeroWeight")
	}
	// Empty pubkey.
	ep := []WeightedValidatorLeaf{{ValidatorID: [32]byte{1}, VotingWeight: 10, ParameterSetID: 0x42}}
	if _, err := BuildWeightedValidatorSet(7, ep); err == nil {
		t.Fatal("empty pubkey accepted, want ErrWVSetEmptyPubKey")
	}
}

func TestWeightedMerkle_InclusionSoundness(t *testing.T) {
	// Every leaf at every set size must produce a verifying inclusion proof,
	// and a NON-member must be rejected. Exercise sizes 1..9 to cover both
	// even and odd levels (the promotion path).
	for n := 1; n <= 9; n++ {
		leaves := make([]WeightedValidatorLeaf, n)
		for i := 0; i < n; i++ {
			leaves[i] = mkLeaf(byte(i+1), uint64(10*(i+1)), 0x42, 1)
		}
		set, err := BuildWeightedValidatorSet(7, leaves)
		if err != nil {
			t.Fatalf("n=%d build: %v", n, err)
		}
		root := set.Root()
		sorted := set.Leaves()
		for idx := 0; idx < n; idx++ {
			proof, err := set.InclusionProof(idx)
			if err != nil {
				t.Fatalf("n=%d idx=%d proof: %v", n, idx, err)
			}
			if !VerifyWeightedInclusion(root, 7, sorted[idx], proof) {
				t.Fatalf("n=%d idx=%d: valid inclusion proof rejected", n, idx)
			}
			// Wrong epoch must fail.
			if VerifyWeightedInclusion(root, 8, sorted[idx], proof) {
				t.Fatalf("n=%d idx=%d: inclusion accepted under wrong epoch", n, idx)
			}
			// Non-member leaf (mutated weight) must fail under the same path.
			bad := sorted[idx]
			bad.VotingWeight++
			if VerifyWeightedInclusion(root, 7, bad, proof) {
				t.Fatalf("n=%d idx=%d: non-member (mutated weight) accepted", n, idx)
			}
		}
	}
}

func TestWeightedMerkle_TamperedPathRejected(t *testing.T) {
	leaves := make([]WeightedValidatorLeaf, 5)
	for i := range leaves {
		leaves[i] = mkLeaf(byte(i+1), 10, 0x42, 1)
	}
	set, _ := BuildWeightedValidatorSet(7, leaves)
	root := set.Root()
	sorted := set.Leaves()

	proof, _ := set.InclusionProof(2)
	if !VerifyWeightedInclusion(root, 7, sorted[2], proof) {
		t.Fatal("baseline proof must verify")
	}

	// Flip a sibling byte → reject.
	if len(proof.Steps) > 0 {
		tampered := *proof
		tampered.Steps = append([]WeightedProofStep(nil), proof.Steps...)
		for i := range tampered.Steps {
			if !tampered.Steps[i].Promoted {
				tampered.Steps[i].Sibling[0] ^= 0xFF
				break
			}
		}
		if VerifyWeightedInclusion(root, 7, sorted[2], &tampered) {
			t.Fatal("tampered sibling accepted")
		}
	}

	// Flip an orientation bit → reject (verifier re-derives orientation).
	flip := *proof
	flip.Steps = append([]WeightedProofStep(nil), proof.Steps...)
	for i := range flip.Steps {
		if !flip.Steps[i].Promoted {
			flip.Steps[i].SiblingIsRight = !flip.Steps[i].SiblingIsRight
			break
		}
	}
	if VerifyWeightedInclusion(root, 7, sorted[2], &flip) {
		t.Fatal("flipped orientation accepted")
	}

	// Lie about LeafIndex/LeafCount → reject (shape mismatch).
	wrongShape := *proof
	wrongShape.LeafCount = 99
	if VerifyWeightedInclusion(root, 7, sorted[2], &wrongShape) {
		t.Fatal("wrong LeafCount accepted")
	}

	// Out-of-range index in InclusionProof → error.
	if _, err := set.InclusionProof(5); err == nil {
		t.Fatal("out-of-range inclusion index accepted")
	}
}

// TestWeightedMerkle_LeafCountCanonical is the RED-1 regression: the weighted
// proof SHAPE is many-to-one in the leaf count (collision classes {3,4},
// {5,6,7,8}, {9..16}, {17..32}, … all share a shape per leaf index), so the
// step-count/orientation cross-check alone cannot distinguish an alternate
// LeafCount within a class. Before binding the count into the leaf digest, an
// attacker could relabel LeafCount within its shape-class, keep the same
// siblings, and recompute an IDENTICAL root — a second byte-distinct cert over
// the same signer set (a QUASAR-C5 non-malleability break and a dedup/
// equivocation hazard). This test asserts that across every (n, idx) up to 32,
// ONLY the canonical (idx, n) verifies and EVERY alternate count is rejected,
// including same-shape-class counts that defeat the structural guards.
func TestWeightedMerkle_LeafCountCanonical(t *testing.T) {
	const maxN = 32

	// shapeKey serialises weightedProofShape so we can detect when an alternate
	// count lands in the SAME shape class as the honest one (the dangerous
	// case the structural cross-check cannot catch).
	shapeKey := func(idx, n int) string {
		var b []byte
		for _, s := range weightedProofShape(idx, n) {
			switch {
			case s.promoted:
				b = append(b, 'P')
			case s.siblingIsRight:
				b = append(b, 'R')
			default:
				b = append(b, 'L')
			}
		}
		return string(b)
	}

	sawSameClassForgeryAttempt := false

	for n := 1; n <= maxN; n++ {
		leaves := make([]WeightedValidatorLeaf, n)
		for i := 0; i < n; i++ {
			leaves[i] = mkLeaf(byte(i+1), uint64(10*(i+1)), 0x42, 1)
		}
		set, err := BuildWeightedValidatorSet(7, leaves)
		if err != nil {
			t.Fatalf("n=%d build: %v", n, err)
		}
		root := set.Root()
		sorted := set.Leaves()

		for idx := 0; idx < n; idx++ {
			proof, err := set.InclusionProof(idx)
			if err != nil {
				t.Fatalf("n=%d idx=%d proof: %v", n, idx, err)
			}
			// Canonical proof must verify.
			if !VerifyWeightedInclusion(root, 7, sorted[idx], proof) {
				t.Fatalf("n=%d idx=%d: canonical proof rejected", n, idx)
			}
			honestShape := shapeKey(idx, n)

			// Forge by relabeling LeafCount to every other valid count, keeping
			// the honest leaf and the honest Steps (siblings + flags). EVERY
			// alternate count must be rejected.
			for np := 1; np <= maxN; np++ {
				if np == n || idx >= np {
					continue
				}
				forged := *proof
				forged.LeafCount = uint32(np)
				if shapeKey(idx, np) == honestShape {
					// Same-class relabel: step count + orientation flags STILL
					// match expSteps, so only the count-binding can reject it.
					sawSameClassForgeryAttempt = true
				}
				if VerifyWeightedInclusion(root, 7, sorted[idx], &forged) {
					t.Fatalf("n=%d idx=%d: forged LeafCount=%d (shape %q vs honest %q) ACCEPTED — malleability open",
						n, idx, np, shapeKey(idx, np), honestShape)
				}
			}
		}
	}

	// Guard the test itself: confirm we actually exercised the dangerous
	// same-shape-class relabel (e.g. (idx=0,n=3) vs (idx=0,n=4) both "RR"),
	// not just trivially-different shapes the length check already caught.
	if !sawSameClassForgeryAttempt {
		t.Fatal("test did not exercise a same-shape-class LeafCount relabel; " +
			"the regression would pass vacuously")
	}
}

// TestWeightedMerkle_NoDuplicationSecondPreimage guards the CVE-2012-2459
// class: with an odd number of leaves, a tree that DUPLICATES the last leaf
// makes the (n) and (n with last leaf repeated) multisets share a root.
// This tree PROMOTES instead, so appending a duplicate of the last leaf is
// rejected at build (duplicate id), and even constructing the doubled leaf
// hash list directly yields a different root than the n-leaf set.
func TestWeightedMerkle_NoDuplicationSecondPreimage(t *testing.T) {
	leaves := []WeightedValidatorLeaf{
		mkLeaf(0x01, 10, 0x42, 1),
		mkLeaf(0x02, 10, 0x42, 1),
		mkLeaf(0x03, 10, 0x42, 1),
	}
	set, _ := BuildWeightedValidatorSet(7, leaves)

	// Hand-roll the "duplicate last leaf" root the vulnerable construction
	// would produce, and confirm it does NOT equal our promotion root. The
	// vulnerable construction has FOUR leaf slots, so it binds leaf_count=4;
	// the honest set binds leaf_count=3. The roots therefore differ both by
	// tree structure (promotion vs duplication) AND by the bound count.
	dup := make([][48]byte, 0, 4)
	for _, l := range leaves {
		dup = append(dup, computeWeightedLeafHash(7, 4, l))
	}
	dup = append(dup, dup[len(dup)-1]) // 4 leaves, last duplicated
	dupRoot := weightedMerkleRoot(dup)
	if dupRoot == set.Root() {
		t.Fatal("promotion root collides with last-leaf-duplication root (CVE-2012-2459 surface)")
	}
}
