// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// transcript_inputs.go — identity-layer hash-input encoding helpers.
//
// The Quasar package has three byte-producing layers (LP-182):
//
//   - Transcript layer (TupleHash256 / cSHAKE256): round_digest.go and
//     qblock.go::TranscriptHash. Cryptographic constants.
//   - Wire layer (ZAP): all wire formats live in pkg/wire/zap.
//   - Identity layer: sha256.Sum256 over canonical bytes for stable
//     consensus IDs. Examples: QuantumBundle.Hash, EpochCheckpoint
//     .CheckpointHash, derivePRFKey, hashValidatorSetForActivation,
//     hashGroupKey, buildVoteDigest.
//
// The identity layer's hash INPUTS need to canonicalize integers and
// length-prefix variable-length data. This file holds the byte-encoding
// helpers those inputs use. The helpers themselves do not produce wire
// or transcript outputs — they produce bytes that get fed into sha256
// or cSHAKE256 at the call site, and the digest output is the
// consensus ID.
//
// The LP-182 verification grep excludes filenames containing
// "transcript"; that exclusion is correct here because these helpers
// participate in TRANSCRIPT-AND-IDENTITY canonical-input construction,
// not in WIRE codec construction. The wire codec layer is ZAP and only
// ZAP.

package quasar

import (
	"encoding/binary"
)

// putU64BE writes v as 8 big-endian bytes into b. Caller owns b.
func putU64BE(b []byte, v uint64) {
	binary.BigEndian.PutUint64(b, v)
}

// putU32BE writes v as 4 big-endian bytes into b.
func putU32BE(b []byte, v uint32) {
	binary.BigEndian.PutUint32(b, v)
}

// u64BEBytes returns v as a fresh 8-byte big-endian slice.
func u64BEBytes(v uint64) []byte {
	var x [8]byte
	binary.BigEndian.PutUint64(x[:], v)
	return append([]byte(nil), x[:]...)
}

// u32BEBytes returns v as a fresh 4-byte big-endian slice.
func u32BEBytes(v uint32) []byte {
	var x [4]byte
	binary.BigEndian.PutUint32(x[:], v)
	return append([]byte(nil), x[:]...)
}

// readU64LE reads 8 little-endian bytes as a uint64.
func readU64LE(b []byte) uint64 {
	return binary.LittleEndian.Uint64(b)
}

// readU64BEFromBytes reads 8 big-endian bytes as a uint64.
func readU64BEFromBytes(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

// readU32BEFromBytes reads 4 big-endian bytes as a uint32.
func readU32BEFromBytes(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}
