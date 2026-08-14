// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/luxfi/ids"
)

// encodeV3 mirrors engine/chain/vote_guard.go encodeVoteGuard so the round-trip test pins the
// decoder to the exact frozen frame the engine writes (magic|ver|count|floor|records|crc).
func encodeV3(bindings map[uint64]binding, floor uint64) []byte {
	buf := append([]byte{}, []byte(magic)...)
	buf = append(buf, verV3)
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], uint32(len(bindings)))
	buf = append(buf, u32[:]...)
	var u64 [8]byte
	binary.BigEndian.PutUint64(u64[:], floor)
	buf = append(buf, u64[:]...)
	for _, b := range bindings {
		binary.BigEndian.PutUint64(u64[:], b.height)
		buf = append(buf, u64[:]...)
		var reserved [32]byte
		if b.hasLock {
			binary.BigEndian.PutUint32(reserved[:4], b.lockRound+1)
		}
		buf = append(buf, reserved[:]...)
		buf = append(buf, b.target[:]...)
	}
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.Checksum(buf, castagnoli))
	return append(buf, crc[:]...)
}

// Both record shapes in one frame: a hard lock sitting exactly AT the floor (no round) and a
// v3 view lock above it. The floor is inclusive, so a binding at the floor must survive the
// round trip rather than be trimmed with the decided past.
func TestDecode_RoundTrip(t *testing.T) {
	const floor = 4_000_000
	const round = 12
	a := ids.GenerateTestID()
	b := ids.GenerateTestID()
	in := map[uint64]binding{
		floor:     {height: floor, target: a},
		floor + 1: {height: floor + 1, lockRound: round, hasLock: true, target: b},
	}
	s, err := decode("test", encodeV3(in, floor))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.finalizedThrough != floor {
		t.Fatalf("floor: got %d want %d", s.finalizedThrough, floor)
	}
	if len(s.bindings) != 2 {
		t.Fatalf("bindings: got %d want 2", len(s.bindings))
	}
	// bindings come back sorted by height: [floor, hard] then [floor+1, round]
	if s.bindings[0].height != floor || s.bindings[0].hasLock {
		t.Fatalf("binding0: %+v", s.bindings[0])
	}
	if s.bindings[1].height != floor+1 || !s.bindings[1].hasLock || s.bindings[1].lockRound != round || s.bindings[1].target != b {
		t.Fatalf("binding1: %+v", s.bindings[1])
	}
}

func TestDecode_RejectsCorruptCRC(t *testing.T) {
	data := encodeV3(map[uint64]binding{1: {height: 1, target: ids.GenerateTestID()}}, 0)
	data[len(data)-1] ^= 0xFF // flip a CRC byte
	if _, err := decode("x", data); err == nil {
		t.Fatal("expected crc mismatch error, got nil (silent corruption)")
	}
}

func TestDecode_RejectsBadMagic(t *testing.T) {
	if _, err := decode("x", append([]byte("NOTGUARD0"), make([]byte, 40)...)); err == nil {
		t.Fatal("expected bad-magic error")
	}
}

func TestAlphaFor(t *testing.T) {
	// α = ⌊2n/3⌋+1 — must match engine bftAlpha (K4→3, K5→4, K11→8, K21→15).
	for _, c := range []struct{ n, a int }{{1, 1}, {3, 3}, {4, 3}, {5, 4}, {11, 8}, {21, 15}} {
		if got := alphaFor(c.n); got != c.a {
			t.Errorf("alphaFor(%d)=%d want %d", c.n, got, c.a)
		}
	}
}
