// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// weighted_merkle.go — the weighted validator-set commitment.
//
// A canonical, second-preimage-resistant Merkle commitment over the epoch
// validator set whose leaf binds EVERY accountability field: the validator
// identity, its canonical public key, its voting weight, the parameter set
// it signs under, and its key version. The resulting 48-byte root is the
// validator_set_root that ComputeRoundDigest (round_digest.go) binds into
// the consensus message, and that a Q-Block carries in ValidatorSetRoot.
//
// Why this exists separately from the block-hash Merkle in epoch.go /
// grouped_threshold.go:
//
//   - Those trees commit to BLOCK hashes for ordering; they pad an odd
//     level by DUPLICATING the last node. Duplication is the classic
//     CVE-2012-2459 second-preimage hazard: a different leaf multiset can
//     yield the same root. For a commitment whose leaves decide WHO signed
//     and WITH WHAT WEIGHT, that is a forgery surface (an attacker could
//     present an inclusion proof for a weight/key the committee never
//     committed). This tree refuses to duplicate.
//
//   - This tree is domain-separated: leaf hashing and internal-node hashing
//     use DISTINCT cSHAKE256 customization tags, so no internal-node digest
//     can ever be reinterpreted as a leaf digest (and vice versa). That is
//     the standard RFC 6962 / Certificate-Transparency defense against the
//     duplication and node/leaf-confusion second-preimage attacks.
//
//   - The leaf binds the (epoch, parameter_set_id, key_version) accountability
//     fields the block-hash trees have no notion of.
//
// One leaf encoding, one root function, one inclusion-proof verifier. This
// is the only weighted-validator-set commitment in the codebase.
package quasar

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

// weightedLeafCustomization is the SP 800-185 cSHAKE256 customization tag
// for a weighted-validator-set LEAF. Distinct from the node tag so a
// leaf digest can never collide with an internal-node digest — the
// node/leaf-confusion second-preimage defense. Wire-stable cryptographic
// constant: never rename.
const weightedLeafCustomization = "QUASAR-WVSET-LEAF-V1"

// weightedNodeCustomization is the SP 800-185 cSHAKE256 customization tag
// for a weighted-validator-set INTERNAL node. Distinct from the leaf tag.
// Wire-stable cryptographic constant: never rename.
const weightedNodeCustomization = "QUASAR-WVSET-NODE-V1"

// weightedLeafProtocolTag is the in-band redundant protocol tag bound as
// the first TupleHash part of every leaf. Defence-in-depth so a
// cross-customization-collision attacker also has to forge the leading part.
const weightedLeafProtocolTag = "Quasar/WVSet/Leaf"

// weightedNodeProtocolTag is the in-band redundant protocol tag bound as
// the first TupleHash part of every internal node.
const weightedNodeProtocolTag = "Quasar/WVSet/Node"

var (
	// ErrWVSetEmpty is returned when a build is attempted over an empty
	// validator set. A quorum certificate over zero validators is
	// meaningless; the caller must supply ≥ 1 leaf.
	ErrWVSetEmpty = errors.New("quasar: weighted validator set is empty")

	// ErrWVSetDuplicateID is returned when two leaves share a validator_id.
	// The validator set is a SET: a repeated identity is either a
	// duplicate-count attack or a caller bug. Refused at build, not papered
	// over by dedup.
	ErrWVSetDuplicateID = errors.New("quasar: duplicate validator_id in weighted validator set")

	// ErrWVSetZeroWeight is returned when a leaf carries zero voting weight.
	// A zero-weight signer contributes nothing to a quorum and is almost
	// always a misconfiguration; refusing it closes a silent
	// "phantom signer inflates the signer count but not the weight" surface.
	ErrWVSetZeroWeight = errors.New("quasar: validator has zero voting weight")

	// ErrWVSetEmptyPubKey is returned when a leaf carries an empty public
	// key. A leaf with no key cannot be the subject of a signature
	// verification and must not be committed.
	ErrWVSetEmptyPubKey = errors.New("quasar: validator has empty public key")

	// ErrWVSetProofShape is returned when an inclusion proof's structural
	// invariants do not hold (leaf index out of range, path length does
	// not match the tree, etc.).
	ErrWVSetProofShape = errors.New("quasar: weighted-set inclusion proof malformed")
)

// WeightedValidatorLeaf is the canonical leaf of the weighted
// validator-set commitment. Every field is an accountability axis bound
// into the leaf digest; flipping any byte yields a different leaf and
// therefore a different root.
//
// The (Epoch) under which the set is committed is NOT a leaf field — it is
// a tree-wide parameter passed to BuildWeightedValidatorSet and folded
// into every leaf digest, so the same (validator_id, pubkey, weight) in a
// different epoch produces a different commitment. This binds the set to
// the epoch without repeating the epoch in every leaf struct.
type WeightedValidatorLeaf struct {
	// ValidatorID is the validator's canonical 32-byte identifier. Leaves
	// are sorted by this field; duplicates are refused.
	ValidatorID [32]byte

	// PublicKey is the validator's canonical signature public key bytes
	// (e.g. ML-DSA-65 or SLH-DSA packed public key). Bound verbatim — the
	// verifier checks a signature under THIS key after proving inclusion.
	PublicKey []byte

	// VotingWeight is the validator's stake-weight for quorum accounting.
	// Must be non-zero.
	VotingWeight uint64

	// ParameterSetID is the wire byte naming the signature parameter set
	// this validator signs under (config.SigSchemeID / IdentitySchemeID
	// byte — e.g. 0x42 = ML-DSA-65). Bound so a key cannot be silently
	// reinterpreted under a different parameter set.
	ParameterSetID uint8

	// KeyVersion is the monotonically increasing key-rotation counter for
	// this validator. Bound so a retired key (lower version) cannot be
	// substituted for the current one under the same identity.
	KeyVersion uint32
}

// WeightedValidatorSet is the built commitment: the canonical sorted
// leaves plus their precomputed leaf digests and the root. Immutable after
// BuildWeightedValidatorSet returns; the prover queries it for inclusion
// proofs and the root is what gets bound into the round digest.
type WeightedValidatorSet struct {
	epoch      uint64
	leaves     []WeightedValidatorLeaf // sorted by ValidatorID, no duplicates
	leafHashes [][48]byte              // leafHashes[i] = hash of leaves[i]
	root       [48]byte
}

// Root returns the 48-byte weighted-validator-set root. This is the value
// bound into ComputeRoundDigest's validatorSetRoot and carried in
// QBlock.ValidatorSetRoot.
func (s *WeightedValidatorSet) Root() [48]byte { return s.root }

// Epoch returns the epoch this set was committed under.
func (s *WeightedValidatorSet) Epoch() uint64 { return s.epoch }

// Len returns the number of validators in the set.
func (s *WeightedValidatorSet) Len() int { return len(s.leaves) }

// Leaves returns a copy of the canonical sorted leaves. Defensive copy so
// callers cannot mutate the committed set.
func (s *WeightedValidatorSet) Leaves() []WeightedValidatorLeaf {
	out := make([]WeightedValidatorLeaf, len(s.leaves))
	copy(out, s.leaves)
	for i := range out {
		out[i].PublicKey = append([]byte(nil), s.leaves[i].PublicKey...)
	}
	return out
}

// computeWeightedLeafHash returns the 48-byte digest of one leaf under the
// epoch and the total leaf count. The encoding is canonical and length-framed
// via TupleHash256, so no boundary-shifting attack can move bytes between
// fields.
//
// leafCount is the total number of leaves in the committed set. It is a
// tree-wide parameter folded into EVERY leaf digest — exactly as epoch is —
// so the same (validator_id, pubkey, weight) under a different claimed count
// produces a different leaf digest and therefore a different root. This is
// what makes the inclusion proof's (LeafIndex, LeafCount) canonical: the
// weighted proof SHAPE alone is many-to-one in the count (the collision
// classes {3,4}, {5,6,7,8}, … all share a shape), so without binding the
// count cryptographically into the root, an attacker could relabel LeafCount
// within its shape-class, keep the same siblings, and recompute an IDENTICAL
// root — a second byte-distinct cert over the same signer set (QUASAR-C5
// non-malleability break, see quorum_cert.go). Binding the count here makes
// the recomputed leaf digest depend on it, so any relabel changes the root
// and is rejected: exactly ONE (LeafIndex, LeafCount) verifies per real tree.
//
// Layout (TupleHash256, customization "QUASAR-WVSET-LEAF-V1"):
//
//	parts[0] = "Quasar/WVSet/Leaf"  (in-band protocol tag)
//	parts[1] = epoch                (8 BE)
//	parts[2] = leaf_count           (4 BE)
//	parts[3] = validator_id         ([32]byte)
//	parts[4] = canonical_pubkey     (variable, length-framed)
//	parts[5] = voting_weight        (8 BE)
//	parts[6] = parameter_set_id     (1)
//	parts[7] = key_version          (4 BE)
func computeWeightedLeafHash(epoch uint64, leafCount uint32, leaf WeightedValidatorLeaf) [48]byte {
	var u64 [8]byte
	var u32 [4]byte

	binary.BigEndian.PutUint64(u64[:], epoch)
	epochBytes := append([]byte(nil), u64[:]...)

	binary.BigEndian.PutUint32(u32[:], leafCount)
	countBytes := append([]byte(nil), u32[:]...)

	binary.BigEndian.PutUint64(u64[:], leaf.VotingWeight)
	weightBytes := append([]byte(nil), u64[:]...)

	binary.BigEndian.PutUint32(u32[:], leaf.KeyVersion)
	keyVerBytes := append([]byte(nil), u32[:]...)

	parts := [][]byte{
		[]byte(weightedLeafProtocolTag),
		epochBytes,
		countBytes,
		leaf.ValidatorID[:],
		leaf.PublicKey,
		weightBytes,
		{leaf.ParameterSetID},
		keyVerBytes,
	}
	out := tupleHash256RoundDigest(parts, 48, weightedLeafCustomization)
	var h [48]byte
	copy(h[:], out)
	return h
}

// computeWeightedNodeHash returns the 48-byte digest of an internal node
// combining a left and right child. Distinct customization from the leaf
// hash guarantees node/leaf domain separation.
//
// Layout (TupleHash256, customization "QUASAR-WVSET-NODE-V1"):
//
//	parts[0] = "Quasar/WVSet/Node"  (in-band protocol tag)
//	parts[1] = left  ([48]byte)
//	parts[2] = right ([48]byte)
func computeWeightedNodeHash(left, right [48]byte) [48]byte {
	parts := [][]byte{
		[]byte(weightedNodeProtocolTag),
		left[:],
		right[:],
	}
	out := tupleHash256RoundDigest(parts, 48, weightedNodeCustomization)
	var h [48]byte
	copy(h[:], out)
	return h
}

// BuildWeightedValidatorSet builds the commitment over the epoch validator
// set. Leaves are canonicalised by sorting on ValidatorID; a duplicate
// validator_id, a zero weight, or an empty public key is a hard error
// (fail-closed — a malformed set must never silently produce a root).
//
// The function takes ownership of nothing: it defensively copies the leaf
// public keys so a later mutation of the caller's slice cannot move the
// committed set underneath an already-produced root.
func BuildWeightedValidatorSet(epoch uint64, leaves []WeightedValidatorLeaf) (*WeightedValidatorSet, error) {
	if len(leaves) == 0 {
		return nil, ErrWVSetEmpty
	}

	// Defensive copy + canonical sort by validator_id.
	sorted := make([]WeightedValidatorLeaf, len(leaves))
	copy(sorted, leaves)
	for i := range sorted {
		if len(sorted[i].PublicKey) == 0 {
			return nil, fmt.Errorf("%w: index %d", ErrWVSetEmptyPubKey, i)
		}
		if sorted[i].VotingWeight == 0 {
			return nil, fmt.Errorf("%w: index %d", ErrWVSetZeroWeight, i)
		}
		sorted[i].PublicKey = append([]byte(nil), sorted[i].PublicKey...)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return bytesLess(sorted[i].ValidatorID[:], sorted[j].ValidatorID[:])
	})

	// Reject duplicate validator_id after sorting (adjacency check).
	for i := 1; i < len(sorted); i++ {
		if sorted[i].ValidatorID == sorted[i-1].ValidatorID {
			return nil, fmt.Errorf("%w: %x", ErrWVSetDuplicateID, sorted[i].ValidatorID[:])
		}
	}

	leafHashes := make([][48]byte, len(sorted))
	count := uint32(len(sorted))
	for i := range sorted {
		leafHashes[i] = computeWeightedLeafHash(epoch, count, sorted[i])
	}

	root := weightedMerkleRoot(leafHashes)

	return &WeightedValidatorSet{
		epoch:      epoch,
		leaves:     sorted,
		leafHashes: leafHashes,
		root:       root,
	}, nil
}

// weightedMerkleRoot folds the leaf digests into a single 48-byte root.
//
// Odd levels PROMOTE the unpaired node unchanged to the next level — they
// do NOT duplicate it. Promotion is sound here precisely because leaf and
// internal-node hashes are domain-separated and the per-level node count
// is determined by the (fixed) leaf count: there is no second multiset of
// leaves that yields the same root, because no leaf digest can ever be
// confused with an internal-node digest, and the structure is a function
// of n alone.
//
// A single-leaf set roots at that leaf's digest (the standard convention);
// the leaf-vs-node domain separation means even this degenerate root
// cannot be confused with an internal node.
func weightedMerkleRoot(leafHashes [][48]byte) [48]byte {
	if len(leafHashes) == 0 {
		return [48]byte{}
	}
	level := make([][48]byte, len(leafHashes))
	copy(level, leafHashes)

	for len(level) > 1 {
		next := make([][48]byte, 0, (len(level)+1)/2)
		i := 0
		for ; i+1 < len(level); i += 2 {
			next = append(next, computeWeightedNodeHash(level[i], level[i+1]))
		}
		if i < len(level) {
			// Odd tail: promote unchanged. No duplication.
			next = append(next, level[i])
		}
		level = next
	}
	return level[0]
}

// WeightedInclusionProof is a Merkle authentication path proving a single
// leaf is committed under a WeightedValidatorSet root. The proof carries
// the sibling digests bottom-up plus a left/right orientation bit per
// level. Promotion levels (where the leaf's subtree was the odd tail) carry
// NO sibling — recorded as a skip so verification mirrors construction
// exactly.
type WeightedInclusionProof struct {
	// LeafIndex is the position of the proven leaf in the canonical sorted
	// order. Bound so a verifier can re-derive the orientation independently
	// of the Siblings orientation bits (defence-in-depth cross-check).
	LeafIndex uint32

	// LeafCount is the total number of leaves in the committed set. Bound so
	// the verifier reconstructs the exact tree shape (which levels promote).
	LeafCount uint32

	// Steps is the bottom-up authentication path.
	Steps []WeightedProofStep
}

// WeightedProofStep is one level of an inclusion proof.
type WeightedProofStep struct {
	// Promoted is true at a level where the running node was the odd tail
	// and was promoted unchanged (no sibling combined). Sibling/IsRight are
	// ignored when Promoted is true.
	Promoted bool

	// Sibling is the 48-byte sibling digest combined at this level.
	Sibling [48]byte

	// SiblingIsRight reports whether Sibling is the RIGHT child (i.e. the
	// running node is the LEFT child) at this level.
	SiblingIsRight bool
}

// InclusionProof returns the authentication path for the leaf at the given
// canonical-sorted index. Returns ErrWVSetProofShape if the index is out of
// range.
func (s *WeightedValidatorSet) InclusionProof(leafIndex int) (*WeightedInclusionProof, error) {
	n := len(s.leafHashes)
	if leafIndex < 0 || leafIndex >= n {
		return nil, fmt.Errorf("%w: index %d not in [0,%d)", ErrWVSetProofShape, leafIndex, n)
	}

	steps := make([]WeightedProofStep, 0)
	level := make([][48]byte, n)
	copy(level, s.leafHashes)
	idx := leafIndex

	for len(level) > 1 {
		next := make([][48]byte, 0, (len(level)+1)/2)
		i := 0
		for ; i+1 < len(level); i += 2 {
			next = append(next, computeWeightedNodeHash(level[i], level[i+1]))
		}
		hasOddTail := i < len(level)
		if hasOddTail {
			next = append(next, level[i])
		}

		if idx == len(level)-1 && hasOddTail {
			// Our node is the promoted odd tail: no sibling this level.
			steps = append(steps, WeightedProofStep{Promoted: true})
			idx = len(next) - 1
		} else {
			// Our node was combined with its pair sibling.
			if idx%2 == 0 {
				// We are the left child; sibling is the right child.
				steps = append(steps, WeightedProofStep{
					Sibling:        level[idx+1],
					SiblingIsRight: true,
				})
			} else {
				// We are the right child; sibling is the left child.
				steps = append(steps, WeightedProofStep{
					Sibling:        level[idx-1],
					SiblingIsRight: false,
				})
			}
			idx = idx / 2
		}
		level = next
	}

	return &WeightedInclusionProof{
		LeafIndex: uint32(leafIndex),
		LeafCount: uint32(n),
		Steps:     steps,
	}, nil
}

// VerifyWeightedInclusion recomputes the root from a leaf and an inclusion
// proof and reports whether it equals the expected root. Fail-closed: any
// structural mismatch (path length inconsistent with LeafCount, index out
// of range) returns false, never a partial accept.
//
// The verifier independently re-derives the per-level orientation from
// LeafIndex/LeafCount and cross-checks it against the proof's orientation
// bits, so a proof that lies about left/right is rejected even if its
// recomputed root happened to match.
func VerifyWeightedInclusion(root [48]byte, epoch uint64, leaf WeightedValidatorLeaf, proof *WeightedInclusionProof) bool {
	if proof == nil {
		return false
	}
	n := int(proof.LeafCount)
	idx := int(proof.LeafIndex)
	if n <= 0 || idx < 0 || idx >= n {
		return false
	}

	// Re-derive the expected step shape from (idx, n) and confirm the
	// proof's step count and promotion/orientation flags match exactly.
	expSteps := weightedProofShape(idx, n)
	if len(expSteps) != len(proof.Steps) {
		return false
	}

	// Fold the CLAIMED leaf count into the leaf digest, exactly as the
	// builder folds the real count. The weighted proof shape is many-to-one
	// in the count (collision classes {3,4}, {5,6,7,8}, … share a shape), so
	// the step-count/orientation cross-check above cannot distinguish an
	// alternate count within the same class. Binding the count into the leaf
	// digest here makes the recomputed root depend on it: a cert that relabels
	// LeafCount within its shape-class yields a DIFFERENT digest and fails
	// against the real root, so exactly ONE (LeafIndex, LeafCount) verifies
	// per committed tree (QUASAR-C5 non-malleability).
	running := computeWeightedLeafHash(epoch, uint32(n), leaf)
	for level, exp := range expSteps {
		got := proof.Steps[level]
		if got.Promoted != exp.promoted {
			return false
		}
		if exp.promoted {
			// Promotion: running node carries up unchanged.
			continue
		}
		if got.SiblingIsRight != exp.siblingIsRight {
			return false
		}
		if exp.siblingIsRight {
			running = computeWeightedNodeHash(running, got.Sibling)
		} else {
			running = computeWeightedNodeHash(got.Sibling, running)
		}
	}

	return running == root
}

// weightedProofShapeStep is the verifier-derived expectation for one level.
type weightedProofShapeStep struct {
	promoted       bool
	siblingIsRight bool
}

// weightedProofShape returns the canonical per-level orientation/promotion
// sequence for a leaf at index idx in a tree of n leaves. This is the
// single source of truth for tree geometry, shared between construction
// (InclusionProof) intent and verification (VerifyWeightedInclusion) so the
// two can never disagree about which levels promote.
func weightedProofShape(idx, n int) []weightedProofShapeStep {
	steps := make([]weightedProofShapeStep, 0)
	levelLen := n
	for levelLen > 1 {
		pairedLen := levelLen - (levelLen % 2) // number of nodes that pair up
		hasOddTail := levelLen%2 == 1
		if idx == levelLen-1 && hasOddTail {
			steps = append(steps, weightedProofShapeStep{promoted: true})
			// Promoted node becomes the last node of the next level.
			idx = (pairedLen / 2)
		} else {
			steps = append(steps, weightedProofShapeStep{
				siblingIsRight: idx%2 == 0,
			})
			idx = idx / 2
		}
		nextLen := pairedLen / 2
		if hasOddTail {
			nextLen++
		}
		levelLen = nextLen
	}
	return steps
}

// bytesLess is a constant-shape lexicographic comparison of equal-length
// byte slices (validator IDs are [32]byte). Not constant-time — validator
// IDs are public, so a data-dependent branch here leaks nothing secret.
func bytesLess(a, b []byte) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}
