// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// witness_transcript.go — SP 800-185 transcript-input helpers for the
// VerkleWitness.SigningDigest() construction. Identical to the ones in
// round_digest.go / kmac256.go; vendored here so witness.go stays
// reviewable in isolation. LP-182 transcript layer.

package quasar

import (
	"encoding/binary"
)

func u64BEWitness(v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return b[:]
}

func encodeStringSP800185Witness(s []byte) []byte {
	out := leftEncodeSP800185Witness(uint64(len(s)) * 8)
	out = append(out, s...)
	return out
}

func leftEncodeSP800185Witness(x uint64) []byte {
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

func rightEncodeSP800185Witness(x uint64) []byte {
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
