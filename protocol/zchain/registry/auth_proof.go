// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package registry

import (
	"errors"
	"fmt"
)

// auth_proof.go — execution hot-path verification for transactions whose
// signature was already proven in a Z-Chain tx_auth batch.
//
// The big win HIP-0078 §"Batched auth proof" names: ML-DSA-65 signatures
// are ~3309 bytes; verifying one on every transaction is wasteful when
// the rollup has already verified them in bulk. VerifyAuthPassed replaces
// the per-tx ML-DSA verify with a 48-byte Merkle inclusion check against
// the rollup's accepted TxAuthRoot.
//
// Soundness: the inclusion proof is sound iff the rollup's commitment
// is sound. The Z-Chain STARK proves that every (account_id, tx_digest)
// pair below TxAuthRoot was verified under a valid ML-DSA signature
// against the corresponding registry record. So a successful inclusion
// check is equivalent to "the chain accepted this signature."
//
// Caller is responsible for:
//
//   1. Resolving the current TxAuthRoot from the chain's EpochCommitment.
//   2. Constructing the inclusion proof at batch-acceptance time and
//      handing it to the executor with the transaction.
//   3. Refusing any tx that does not carry an inclusion proof (the
//      executor can fall back to direct ML-DSA verify if the chain's
//      policy permits, but a strict-PQ chain SHOULD refuse).

// MerkleProof is the inclusion witness for a leaf in a 48-byte-wide
// binary Merkle tree. The verifier walks Siblings from leaf to root,
// combining at each level according to LeafIndex's bits.
//
// Hash family is fixed at cSHAKE256 / TupleHash256 with customization
// "LUX_MERKLE_NODE_V1" — distinct from any other digest customization
// in this package so a leaf digest cannot collide with a node digest.
//
// Empty Siblings means "leaf is the root" (single-element tree).
type MerkleProof struct {
	// LeafIndex is the leaf's position in the tree (0-indexed, left
	// to right). Bit i selects the combine direction at level i:
	// 0 = sibling-on-right, 1 = sibling-on-left.
	LeafIndex uint64

	// LeafHash is the canonical 48-byte leaf digest. Caller computes
	// it the same way the rollup did when committing the batch.
	LeafHash [48]byte

	// Siblings is the path from leaf to root. Length determines the
	// tree depth; an empty slice means LeafHash IS the root.
	Siblings [][48]byte
}

// merkleNodeCustomization is the cSHAKE256 customization for internal
// Merkle nodes. Distinct from "LUX_MERKLE_LEAF_V1" (the leaf customization
// the rollup uses to encode (account_id || tx_digest)) so a leaf digest
// cannot collide with an internal-node digest.
const merkleNodeCustomization = "LUX_MERKLE_NODE_V1"

// merkleLeafCustomization is the cSHAKE256 customization the rollup
// uses when committing each (account_id, tx_digest) tuple as a leaf.
// Exported (not unexported) so the rollup and the verifier share one
// string literal at one place in the codebase.
const merkleLeafCustomization = "LUX_MERKLE_LEAF_V1"

// LeafDigest returns the canonical 48-byte leaf digest for a
// (account_id, tx_digest) pair. The rollup computes it the same way
// when building TxAuthRoot; VerifyAuthPassed recomputes it before
// the inclusion walk.
//
// Bound via TupleHash256 so account_id || tx_digest is unambiguously
// framed. A flipped byte in either input changes the digest.
func LeafDigest(accountID [48]byte, txDigest [48]byte) [48]byte {
	parts := [][]byte{
		[]byte("Lux/MerkleLeaf/AuthBatch/v1"),
		accountID[:],
		txDigest[:],
	}
	return tupleHash48(parts, merkleLeafCustomization)
}

// nodeDigest combines two child digests into a parent.
func nodeDigest(left, right [48]byte) [48]byte {
	parts := [][]byte{
		[]byte("Lux/MerkleNode/v1"),
		left[:],
		right[:],
	}
	return tupleHash48(parts, merkleNodeCustomization)
}

// VerifyAuthPassed returns nil iff (accountID, txDigest) was included
// in an accepted Z-Chain tx_auth batch whose root equals txAuthRoot.
// The Merkle inclusion walk is the entire hot-path cost — no ML-DSA
// verify happens here because the Z-Chain STARK already proved every
// signature in the batch.
//
// Checks (fail-closed, in order):
//
//   1. txAuthRoot is non-zero.
//   2. proof.LeafHash equals LeafDigest(accountID, txDigest).
//      Closes the "wrong leaf with a valid inclusion path" attack.
//   3. proof.LeafIndex fits within the proof depth (a 2^d-leaf tree
//      admits LeafIndex < 2^d). Refuses an out-of-range index.
//   4. Walk Siblings from leaf to root combining via nodeDigest; the
//      final hash MUST equal txAuthRoot.
//
// Returns ErrAuthProofStaleEpoch when the caller-supplied txAuthRoot
// is the all-zero sentinel — by convention the registry uses zero to
// mean "no batch accepted yet at this epoch" so a stale-epoch lookup
// surfaces an immediate refusal rather than a silently-passing all-zero
// inclusion check.
func VerifyAuthPassed(
	txAuthRoot [48]byte,
	accountID [48]byte,
	txDigest [48]byte,
	inclusionProof MerkleProof,
) error {
	if isZero48(txAuthRoot) {
		return ErrAuthProofStaleEpoch
	}
	expectedLeaf := LeafDigest(accountID, txDigest)
	if inclusionProof.LeafHash != expectedLeaf {
		return fmt.Errorf("%w: leaf digest does not match (account_id, tx_digest)", ErrAuthProofLeafMismatch)
	}
	depth := len(inclusionProof.Siblings)
	if depth >= 64 {
		// A 2^64-leaf tree is implausible; refuse before integer overflow.
		return ErrAuthProofDepthExceeded
	}
	// LeafIndex must fit within the proof depth.
	if depth < 64 {
		maxIndex := uint64(1) << depth
		if inclusionProof.LeafIndex >= maxIndex {
			return fmt.Errorf("%w: leaf_index=%d depth=%d",
				ErrAuthProofIndexOutOfRange, inclusionProof.LeafIndex, depth)
		}
	}
	// Walk leaf -> root.
	cur := inclusionProof.LeafHash
	for level, sibling := range inclusionProof.Siblings {
		bit := (inclusionProof.LeafIndex >> uint(level)) & 1
		if bit == 0 {
			// Leaf path bit = 0 → current is left child, sibling is right.
			cur = nodeDigest(cur, sibling)
		} else {
			// Leaf path bit = 1 → current is right child, sibling is left.
			cur = nodeDigest(sibling, cur)
		}
	}
	if cur != txAuthRoot {
		return ErrAuthProofWrongRoot
	}
	return nil
}

// =============================================================================
// Typed errors
// =============================================================================

var (
	// ErrAuthProofStaleEpoch — txAuthRoot was the all-zero sentinel,
	// meaning no batch has been accepted yet at this epoch.
	ErrAuthProofStaleEpoch = errors.New("registry: tx_auth_root is zero (stale or pre-batch epoch)")

	// ErrAuthProofLeafMismatch — proof.LeafHash did not match
	// LeafDigest(account_id, tx_digest). Wrong leaf for the claimed
	// (account, tx).
	ErrAuthProofLeafMismatch = errors.New("registry: inclusion proof leaf digest mismatch")

	// ErrAuthProofIndexOutOfRange — proof.LeafIndex was >= 2^depth.
	ErrAuthProofIndexOutOfRange = errors.New("registry: inclusion proof leaf index out of range")

	// ErrAuthProofDepthExceeded — proof depth >= 64; implausibly large
	// tree. Refused before integer-overflow risk.
	ErrAuthProofDepthExceeded = errors.New("registry: inclusion proof depth >= 64")

	// ErrAuthProofWrongRoot — the walked root did not equal txAuthRoot.
	// Wrong inclusion path for the claimed (account, tx) pair.
	ErrAuthProofWrongRoot = errors.New("registry: inclusion proof root does not match tx_auth_root")
)
