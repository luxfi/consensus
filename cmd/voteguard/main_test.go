// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"encoding/binary"
	"hash/crc32"
	"os"
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

func TestDecode_RoundTrip(t *testing.T) {
	a := ids.GenerateTestID()
	b := ids.GenerateTestID()
	in := map[uint64]binding{
		1082879: {height: 1082879, target: a},                                 // hard lock (no round) — AT floor
		1082880: {height: 1082880, lockRound: 8450, hasLock: true, target: b}, // v3 view lock — above floor
	}
	s, err := decode("test", encodeV3(in, 1082879))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.finalizedThrough != 1082879 {
		t.Fatalf("floor: got %d want 1082879", s.finalizedThrough)
	}
	if len(s.bindings) != 2 {
		t.Fatalf("bindings: got %d want 2", len(s.bindings))
	}
	// sorted by height: [1082879 hard][1082880 round=8450]
	if s.bindings[0].height != 1082879 || s.bindings[0].hasLock {
		t.Fatalf("binding0: %+v", s.bindings[0])
	}
	if s.bindings[1].height != 1082880 || !s.bindings[1].hasLock || s.bindings[1].lockRound != 8450 || s.bindings[1].target != b {
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

// TestDecode_RealMainnetFiles pins byte-parity against the captured live pods (ground truth): the
// 1082879 decided floor and the 1082880 split-lock the incident is about. Skips when the evidence
// dir is absent (CI without the captured files).
func TestDecode_RealMainnetFiles(t *testing.T) {
	const dir = "/tmp/lux-1082880-evidence"
	files := map[string]string{ // pod → expected 1082880 lock target (cb58)
		dir + "/vote-guard.luxd-0": "2494WYuivjJ4V5scrrBDUopqnK6quAkgyfECkXDeG3Pgs6r5Wu",
		dir + "/vote-guard.luxd-1": "yezD2DSHgnbyqajqhYR19kvBbmK9x86MZubyNuU75eMHJ7HnA",
	}
	tested := 0
	for path, wantTarget := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // evidence not present on this host
		}
		s, err := decode(path, data)
		if err != nil {
			t.Fatalf("decode real file %s: %v", path, err)
		}
		if s.finalizedThrough != 1082879 {
			t.Fatalf("%s: floor got %d want 1082879", path, s.finalizedThrough)
		}
		var got ids.ID
		var found bool
		for _, b := range s.bindings {
			if b.height == 1082880 {
				got, found = b.target, true
				if b.lockRound != 8450 {
					t.Fatalf("%s: 1082880 lockRound got %d want 8450", path, b.lockRound)
				}
			}
		}
		if !found {
			t.Fatalf("%s: no 1082880 binding", path)
		}
		if got.String() != wantTarget {
			t.Fatalf("%s: 1082880 target got %s want %s", path, got, wantTarget)
		}
		tested++
	}
	if tested == 0 {
		t.Skip("no captured mainnet vote-guard files present; skipping real-file parity")
	}
}
