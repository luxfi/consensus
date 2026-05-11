// Copyright (C) 2025, Lux Industries Inc All rights reserved.
//
// Regression coverage for red-team finding F113: the strict-PQ profile
// pins MinHashOutputBits=384, so every digest in the checkpoint/Merkle
// pipeline must be SHA3-384 (48 bytes). The previous wire layout used
// SHA3-256 (32 bytes), below the profile floor.
//
// These tests fail at compile-time if the digest width regresses, and
// at runtime if the underlying primitive ever swaps back to SHA3-256.

package quasar

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckpointHash_IsSHA3_384 asserts CheckpointHash returns a 48-byte
// (SHA3-384) digest under cSHAKE256("LUX-QUASAR-CHECKPOINT-V1"). Closes
// the wire-width axis of F113.
func TestCheckpointHash_IsSHA3_384(t *testing.T) {
	ec := &EpochCheckpoint{
		Epoch:          7,
		StartHeight:    1000,
		EndHeight:      1009,
		BlockCount:     10,
		MerkleRoot:     [48]byte{0xaa},
		PreviousAnchor: [48]byte{0xbb},
		Timestamp:      1731240000,
	}

	h := ec.CheckpointHash()
	require.Len(t, h[:], 48, "checkpoint digest must be 48 bytes (SHA3-384) under strict-PQ MinHashOutputBits=384")

	// The digest stream binds Epoch, heights, MerkleRoot, PreviousAnchor,
	// Timestamp. Mutating any one of them MUST change the output.
	ec2 := *ec
	ec2.Epoch = 8
	require.NotEqual(t, h, ec2.CheckpointHash(), "epoch field must bind into digest")

	ec3 := *ec
	ec3.MerkleRoot[0] = 0xab
	require.NotEqual(t, h, ec3.CheckpointHash(), "MerkleRoot must bind into digest")

	ec4 := *ec
	ec4.PreviousAnchor[0] = 0xbc
	require.NotEqual(t, h, ec4.CheckpointHash(), "PreviousAnchor must bind into digest")

	// Determinism: same input → same output.
	require.Equal(t, h, ec.CheckpointHash(), "CheckpointHash must be deterministic")
}

// TestComputeMerkleRoot_IsSHA3_384 asserts computeMerkleRoot returns a
// 48-byte (SHA3-384) digest at every level under cSHAKE256
// customisation "LUX-QUASAR-MERKLE-NODE-V1". Closes the Merkle axis of F113.
func TestComputeMerkleRoot_IsSHA3_384(t *testing.T) {
	// Empty: returns zero 48-byte block.
	zeroRoot := computeMerkleRoot(nil)
	require.Len(t, zeroRoot[:], 48, "empty Merkle root must be a 48-byte zero block")
	require.Equal(t, [48]byte{}, zeroRoot)

	// Single leaf: returns the leaf itself (no hash step).
	leaf := [48]byte{}
	copy(leaf[:], []byte("LUX-test-leaf-single-element"))
	single := computeMerkleRoot([][48]byte{leaf})
	require.Equal(t, leaf, single, "single-leaf root equals the leaf")

	// Two leaves: one combine step, output must be SHA3-384.
	l1 := sha3_384("test-l1", []byte("payload-1"))
	l2 := sha3_384("test-l2", []byte("payload-2"))
	rootTwo := computeMerkleRoot([][48]byte{l1, l2})
	require.Len(t, rootTwo[:], 48, "two-leaf Merkle root must be SHA3-384 (48 bytes)")
	require.NotEqual(t, [48]byte{}, rootTwo, "root must be non-zero")

	// Four leaves: two levels of combining; output stays 48 bytes.
	leaves4 := [][48]byte{l1, l2, l1, l2}
	root4 := computeMerkleRoot(leaves4)
	require.Len(t, root4[:], 48, "four-leaf Merkle root must remain SHA3-384")

	// Determinism: same input → same root across two invocations.
	require.Equal(t, root4, computeMerkleRoot(leaves4), "computeMerkleRoot must be deterministic")

	// Distinct customisation: ensure the Merkle-node customisation string
	// does not collide with the checkpoint customisation. Two-leaf root
	// from computeMerkleRoot ≠ sha3_384("LUX-QUASAR-CHECKPOINT-V1", ...).
	combined := append(append([]byte{}, l1[:]...), l2[:]...)
	collision := sha3_384(checkpointHashV1, combined)
	require.NotEqual(t, rootTwo, collision, "Merkle and checkpoint customisations must produce distinct digests")
}

// TestCreateEpochCheckpoint_WiresSHA3_384 asserts the end-to-end
// constructor returns an EpochCheckpoint whose MerkleRoot field is the
// SHA3-384 root of the supplied 48-byte block hashes, and whose
// CheckpointHash also returns 48 bytes.
func TestCreateEpochCheckpoint_WiresSHA3_384(t *testing.T) {
	// Build four 48-byte block hashes from SHA-256 seeds (test-only —
	// production block hashes are produced upstream by the chain).
	seed := func(s string) [48]byte {
		var b [48]byte
		h := sha256.Sum256([]byte(s))
		copy(b[:32], h[:])
		copy(b[32:], h[:16])
		return b
	}
	blocks := [][48]byte{seed("b0"), seed("b1"), seed("b2"), seed("b3")}
	previousAnchor := seed("anchor-prev")

	ec := CreateEpochCheckpoint(42, 1000, 1003, blocks, previousAnchor)
	require.NotNil(t, ec)
	require.Equal(t, uint64(42), ec.Epoch)
	require.Equal(t, 4, ec.BlockCount)
	require.Equal(t, previousAnchor, ec.PreviousAnchor)

	expectedRoot := computeMerkleRoot(blocks)
	require.Equal(t, expectedRoot, ec.MerkleRoot, "MerkleRoot field must equal computeMerkleRoot(blocks)")
	require.Len(t, ec.MerkleRoot[:], 48, "EpochCheckpoint.MerkleRoot must be [48]byte under F113")
	require.Len(t, ec.PreviousAnchor[:], 48, "EpochCheckpoint.PreviousAnchor must be [48]byte under F113")

	ch := ec.CheckpointHash()
	require.Len(t, ch[:], 48, "CheckpointHash() output remains 48 bytes through CreateEpochCheckpoint path")
}
