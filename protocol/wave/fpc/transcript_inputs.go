// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// transcript_inputs.go — identity-layer hash-input encoding helpers
// for the FPC selector. See protocol/quasar/transcript_inputs.go for
// the LP-182 layer decomposition rationale.

package fpc

import "encoding/binary"

// u64BEFixed returns v as a fresh 8-byte big-endian slice.
func u64BEFixed(v uint64) []byte {
	var x [8]byte
	binary.BigEndian.PutUint64(x[:], v)
	return append([]byte(nil), x[:]...)
}

// u64BEFromBytes reads 8 big-endian bytes as a uint64.
func u64BEFromBytes(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}
