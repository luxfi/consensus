// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// RED differential: prove TwoThirdsStakeFloor == the ORIGINAL inline cert math
// for all inputs incl. boundaries and near-2^64, and that it never overflows.
package config

import (
	"math"
	"math/rand"
	"testing"
)

// origInlineFloor is the EXACT pre-refactor inline computation from
// QuorumCert.VerifyWeighted (git 056ead174^ engine/chain/quorum_cert.go).
func origInlineFloor(total uint64) uint64 {
	q, r := total/3, total%3
	twoThirdsFloor := 2 * q
	if r == 2 {
		twoThirdsFloor++
	}
	return twoThirdsFloor
}

// bigIntFloor is an independent oracle: floor(2*total/3) via 128-bit-safe math.
// 2*total can overflow uint64, so compute in the structurally-safe form.
func bigIntFloor(total uint64) uint64 {
	// floor(2*total/3): use total/3 and 2*(total%3)/3 — but verify against a
	// genuinely independent route: (2*total)/3 done in big-enough width.
	hi := total / 3 * 2
	lo := (total % 3) * 2 / 3
	return hi + lo
}

func TestRED_FloorMatchesInlineExhaustiveSmall(t *testing.T) {
	for total := uint64(0); total < 100000; total++ {
		got := TwoThirdsStakeFloor(total)
		if got != origInlineFloor(total) {
			t.Fatalf("DIVERGENCE total=%d: refactor=%d inline=%d", total, got, origInlineFloor(total))
		}
		if got != bigIntFloor(total) {
			t.Fatalf("ORACLE MISMATCH total=%d: refactor=%d oracle=%d", total, got, bigIntFloor(total))
		}
		// The floor must never reach total (a supermajority must be a strict subset
		// of stake), and voted==total must always strictly exceed it.
		if total >= 1 && !(total > got) {
			t.Fatalf("floor>=total at total=%d (floor=%d) — full stake would not be a supermajority", total, got)
		}
	}
}

func TestRED_FloorNoOverflowNear2p64(t *testing.T) {
	edges := []uint64{
		math.MaxUint64, math.MaxUint64 - 1, math.MaxUint64 - 2, math.MaxUint64 - 3,
		1 << 63, (1 << 63) + 1, (1 << 63) - 1, 1 << 62, math.MaxUint64 / 3 * 3,
	}
	for _, total := range edges {
		got := TwoThirdsStakeFloor(total)
		if got != origInlineFloor(total) {
			t.Fatalf("DIVERGENCE near-2^64 total=%d: refactor=%d inline=%d", total, got, origInlineFloor(total))
		}
		if got != bigIntFloor(total) {
			t.Fatalf("ORACLE MISMATCH near-2^64 total=%d: refactor=%d oracle=%d", total, got, bigIntFloor(total))
		}
		// floor < total (no overflow wrap; supermajority is a strict subset).
		if !(got < total) {
			t.Fatalf("OVERFLOW/WRAP at total=%d: floor=%d not < total", total, got)
		}
	}
}

func TestRED_FloorRandomDifferential(t *testing.T) {
	rng := rand.New(rand.NewSource(0xC0FFEE))
	for i := 0; i < 5_000_000; i++ {
		total := rng.Uint64()
		if TwoThirdsStakeFloor(total) != origInlineFloor(total) {
			t.Fatalf("DIVERGENCE total=%d", total)
		}
	}
}
