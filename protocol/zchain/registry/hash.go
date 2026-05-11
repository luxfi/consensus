// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package registry

import (
	"encoding/binary"

	"golang.org/x/crypto/sha3"
)

// hash.go — SP 800-185 TupleHash256 + SHAKE256 helpers used by every
// registry transcript. Mirrors the primitives in protocol/zchain/hash.go
// and protocol/auth/hash.go; vendored here so the registry package has
// no upward dependency on its parent (consensus dependency rule: every
// package below proof/VM owns its hash kernel directly).
//
// Output width here is 48 bytes (384 bits) — the canonical PQ-aligned
// digest width used across zchain, auth, and quasar.

// tupleHash256 computes TupleHash256(parts, outLen, customization) per
// NIST SP 800-185 §5. Caller owns the encoding of each part.
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

// tupleHash48 is the 48-byte (384-bit) wrapper. Every registry digest
// uses it; the output width is the canonical Z-Chain transcript width.
func tupleHash48(parts [][]byte, customization string) [48]byte {
	out := tupleHash256(parts, 48, customization)
	var d [48]byte
	copy(d[:], out)
	return d
}

// shake256_384 returns SHAKE256(input) squeezed to 48 bytes. Used by
// DeriveAccountID to bind a deterministic 48-byte AccountID to (profile,
// chain, scheme, compact_pubkey). Distinct from TupleHash because
// AccountID derivation is performance-sensitive — a single cSHAKE256
// absorb is the cheapest possible PQ-aligned hash invocation.
func shake256_384(input []byte, customization string) [48]byte {
	h := sha3.NewCShake256(nil, []byte(customization))
	_, _ = h.Write(input)
	var out [48]byte
	_, _ = h.Read(out[:])
	return out
}

// leftEncodeSP800185 returns the SP 800-185 §2.3 left_encode(x) byte
// string. Operates on the BIT length; every caller multiplies by 8.
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
// string. Used at the tail of TupleHash to bind output length into the
// absorbed stream.
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

// u16BE returns v as a fresh 2-byte big-endian slice.
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
