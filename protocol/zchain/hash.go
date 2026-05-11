// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zchain

import (
	"encoding/binary"

	"golang.org/x/crypto/sha3"
)

// hash.go — SP 800-185 TupleHash256 with a configurable output length.
// Mirrors the FIPS-aligned primitive in github.com/luxfi/pulsar; vendored
// here so consensus stays below pulsar in the module-dependency graph.
//
// Why TupleHash and not plain SHAKE: TupleHash provides unambiguous length
// framing per part, so (parts ["ab", "cd"]) and (parts ["a", "bcd"]) hash
// to distinct values. That injectivity is the property that makes flipping
// any single field's bytes change the digest, even if a neighbouring field
// could absorb the flipped bytes.
//
// Output width here is 48 bytes (384 bits) — the HIP-0078 canonical width
// for Z-Chain transcripts / state roots. cSHAKE256 is an XOF so the width
// is a runtime parameter; we read 48 bytes off the squeeze.

// tupleHash256 computes TupleHash256(parts, outLen, customization) per
// NIST SP 800-185 §5.
//
// Caller owns the encoding of each part. Parts MAY be empty byte slices;
// TupleHash treats them as zero-length strings (which still get a length
// prefix). The customization tag is the only domain-separator: bumping it
// invalidates every prior digest computed with this layout.
func tupleHash256(parts [][]byte, outLen int, customization string) []byte {
	var x []byte
	for _, p := range parts {
		x = append(x, encodeStringSP800185(p)...)
	}
	x = append(x, rightEncodeSP800185(uint64(outLen)*8)...)

	h := sha3.NewCShake256([]byte("TupleHash"), []byte(customization))
	_, _ = h.Write(x)
	out := make([]byte, outLen)
	_, _ = h.Read(out)
	return out
}

// tupleHash48 is the 48-byte (384-bit) wrapper used everywhere in this
// package. Production callers MUST go through this helper rather than
// calling tupleHash256 with a length parameter, so the output width is
// fixed at one place and reviewable in one diff.
func tupleHash48(parts [][]byte, customization string) [48]byte {
	out := tupleHash256(parts, 48, customization)
	var d [48]byte
	copy(d[:], out)
	return d
}

// leftEncodeSP800185 returns the SP 800-185 §2.3 left_encode(x) byte
// string. Operates on the BIT length, not the byte length — every
// caller multiplies by 8 before passing in.
func leftEncodeSP800185(x uint64) []byte {
	if x == 0 {
		return []byte{0x01, 0x00}
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], x)
	i := 0
	for i < 7 && buf[i] == 0 {
		i++
	}
	out := make([]byte, 0, 9-i)
	out = append(out, byte(8-i))
	out = append(out, buf[i:]...)
	return out
}

// rightEncodeSP800185 returns the SP 800-185 §2.3 right_encode(x) byte
// string. Used at the tail of TupleHash to bind the requested output
// length into the absorbed stream.
func rightEncodeSP800185(x uint64) []byte {
	if x == 0 {
		return []byte{0x00, 0x01}
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], x)
	i := 0
	for i < 7 && buf[i] == 0 {
		i++
	}
	out := make([]byte, 0, 9-i)
	out = append(out, buf[i:]...)
	out = append(out, byte(8-i))
	return out
}

// encodeStringSP800185 returns left_encode(bit_len(s)) || s per
// SP 800-185 §2.3.
func encodeStringSP800185(s []byte) []byte {
	out := leftEncodeSP800185(uint64(len(s)) * 8)
	out = append(out, s...)
	return out
}

// ---------------------------------------------------------------------------
// Big-endian writers used by the canonical envelope codec.
// ---------------------------------------------------------------------------

// appendU16 appends v as 2 big-endian bytes to b.
func appendU16(b []byte, v uint16) []byte {
	var x [2]byte
	binary.BigEndian.PutUint16(x[:], v)
	return append(b, x[:]...)
}

// appendU32 appends v as 4 big-endian bytes to b.
func appendU32(b []byte, v uint32) []byte {
	var x [4]byte
	binary.BigEndian.PutUint32(x[:], v)
	return append(b, x[:]...)
}

// appendU64 appends v as 8 big-endian bytes to b.
func appendU64(b []byte, v uint64) []byte {
	var x [8]byte
	binary.BigEndian.PutUint64(x[:], v)
	return append(b, x[:]...)
}

// u16BE returns v as a fresh 2-byte big-endian slice (suitable for use
// as a TupleHash part).
func u16BE(v uint16) []byte {
	var x [2]byte
	binary.BigEndian.PutUint16(x[:], v)
	return append([]byte(nil), x[:]...)
}

// u32BE returns v as a fresh 4-byte big-endian slice.
func u32BE(v uint32) []byte {
	var x [4]byte
	binary.BigEndian.PutUint32(x[:], v)
	return append([]byte(nil), x[:]...)
}

// u64BE returns v as a fresh 8-byte big-endian slice.
func u64BE(v uint64) []byte {
	var x [8]byte
	binary.BigEndian.PutUint64(x[:], v)
	return append([]byte(nil), x[:]...)
}

// boolByte returns 1 for true, 0 for false.
func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}
