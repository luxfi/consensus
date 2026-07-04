// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// voteguard — a READ-ONLY offline inspector for the durable consensus vote-guard file
// (engine/chain/vote_guard.go). It decodes one or more pods' vote-guard snapshots and reports,
// per height above the decided floor, the persisted view-change lock target — then, across the
// pods, detects a DURABLE SPLIT-LOCK (the block-1082880 incident: honest validators locked on
// DIFFERENT proposervm-wrapper ids of the SAME inner block, so no single target reaches α and no
// POL/cert ever forms).
//
// It exists so an operator can verify the stale-lock safety predicate on every live pod / PVC
// snapshot BEFORE any repair is applied:
//
//   - the decided floor (finalizedThrough) is identical across pods,
//   - the split is STRICTLY BELOW α (no α-of-K cert can exist at the split height — nothing
//     finalized is dropped by a repair),
//   - exactly the split heights need canonicalization; every other height is aligned.
//
// It MUTATES NOTHING (opens files O_RDONLY). The actual repair is the node's boot-time
// MigrateStaleLocks (engine/chain/lock_migration.go), which canonicalizes each stale outer id to
// its inner block id via the durable VM block store. This tool is the audit gate in front of it.
//
// The 314-byte v3 frame is FROZEN (magic|ver|count|finalizedThrough|records|crc). This decoder
// is a standalone, CGO-free mirror of decodeVoteGuard so it runs on any pod without the engine's
// build toolchain; main_test.go pins byte-parity against the real captured pod files.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"

	"github.com/luxfi/ids"
)

const (
	magic  = "LUXVGUARD" // 9 bytes
	recLen = 8 + 32 + 32 // height(u64) | reserved(32) | canonical(32)
	hdrV1  = 9 + 1 + 4   // magic | ver | count
	hdrV23 = 9 + 1 + 4 + 8
	verV1  = 1
	verV2  = 2
	verV3  = 3
)

// castagnoli is the exact CRC table the engine writes with (crc32.MakeTable(Castagnoli)).
var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// binding is one persisted per-height lock record.
type binding struct {
	height    uint64
	lockRound uint32
	hasLock   bool // v3 view-change lock round present (vs a legacy hard lock)
	target    ids.ID
}

// htKey groups pods by (height, lock target) across pod files.
type htKey struct {
	height uint64
	target ids.ID
}

// snapshot is one decoded vote-guard file.
type snapshot struct {
	label            string
	version          byte
	finalizedThrough uint64
	bindings         []binding
}

// decode is the fail-closed inverse of engine encodeVoteGuard. Any framing/CRC mismatch is an
// error (the snapshot is untrustworthy) — never a silent partial read.
func decode(label string, data []byte) (*snapshot, error) {
	if len(data) < hdrV1+4 {
		return nil, fmt.Errorf("too short (%d bytes)", len(data))
	}
	if string(data[:len(magic)]) != magic {
		return nil, errors.New("bad magic (not a vote-guard file)")
	}
	off := len(magic)
	ver := data[off]
	off++
	var hdrLen int
	switch ver {
	case verV1:
		hdrLen = hdrV1
	case verV2, verV3:
		hdrLen = hdrV23
	default:
		return nil, fmt.Errorf("unsupported version %d", ver)
	}
	if len(data) < hdrLen+4 {
		return nil, fmt.Errorf("too short for v%d header (%d bytes)", ver, len(data))
	}
	count := binary.BigEndian.Uint32(data[off : off+4])
	off += 4
	var floor uint64
	if ver == verV2 || ver == verV3 {
		floor = binary.BigEndian.Uint64(data[off : off+8])
		off += 8
	}
	if want := hdrLen + recLen*int(count) + 4; len(data) != want {
		return nil, fmt.Errorf("length %d != expected %d for count %d (v%d)", len(data), want, count, ver)
	}
	body := data[:len(data)-4]
	if got := binary.BigEndian.Uint32(data[len(data)-4:]); crc32.Checksum(body, castagnoli) != got {
		return nil, errors.New("crc mismatch (corrupt or truncated)")
	}
	s := &snapshot{label: label, version: ver, finalizedThrough: floor}
	for i := uint32(0); i < count; i++ {
		var b binding
		b.height = binary.BigEndian.Uint64(data[off : off+8])
		off += 8
		if ver == verV3 { // reserved[0:4] = (round+1) sentinel; 0 = no view-change lock
			if raw := binary.BigEndian.Uint32(data[off : off+4]); raw > 0 {
				b.lockRound, b.hasLock = raw-1, true
			}
		}
		off += 32
		b.target, _ = ids.ToID(data[off : off+32])
		off += 32
		s.bindings = append(s.bindings, b)
	}
	sort.Slice(s.bindings, func(i, j int) bool { return s.bindings[i].height < s.bindings[j].height })
	return s, nil
}

// alphaFor is the BFT accept quorum α = ⌊2n/3⌋+1 for n pods — the same threshold the engine's
// bftAlpha uses. A split strictly below α proves no α-of-K cert exists at that height.
func alphaFor(n int) int {
	if n <= 0 {
		return 0
	}
	return (2*n)/3 + 1
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: voteguard <vote-guard-file>...\n"+
			"  read-only: decodes each pod's vote-guard snapshot, prints per-height locks above the\n"+
			"  decided floor, and detects a cross-pod durable split-lock + its α-of-K cert safety gate.\n")
		os.Exit(2)
	}
	paths := os.Args[1:]
	var snaps []*snapshot
	for _, p := range paths {
		data, err := os.ReadFile(p) // O_RDONLY — mutates nothing
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", p, err)
			os.Exit(1)
		}
		s, err := decode(filepath.Base(p), data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "decode %s: %v\n", p, err)
			os.Exit(1)
		}
		snaps = append(snaps, s)
	}
	report(snaps)
}

func report(snaps []*snapshot) {
	fmt.Printf("== vote-guard inspection (%d pod file(s), READ-ONLY) ==\n\n", len(snaps))

	// Per-pod: floor + every lock above it (the window the migration may touch).
	floorSet := map[uint64]struct{}{}
	for _, s := range snaps {
		floorSet[s.finalizedThrough] = struct{}{}
		fmt.Printf("%-22s v%d  decidedFloor(finalizedThrough)=%d\n", s.label, s.version, s.finalizedThrough)
		for _, b := range s.bindings {
			marker := "  "
			if b.height > s.finalizedThrough {
				marker = "^ " // above the floor — a migration candidate
			}
			lock := "hard-lock(legacy)"
			if b.hasLock {
				lock = fmt.Sprintf("round=%d", b.lockRound)
			}
			fmt.Printf("   %sh=%-9d %-17s target=%s\n", marker, b.height, lock, b.target)
		}
		fmt.Println()
	}

	// Cross-pod split-lock detection (needs ≥2 pods).
	if len(snaps) < 2 {
		fmt.Println("(single file: cross-pod split detection needs ≥2 pod files)")
		return
	}
	if len(floorSet) != 1 {
		fmt.Printf("!! STOP: pods disagree on the decided floor %v — resolve before any repair.\n", keysOf(floorSet))
	}
	floor := snaps[0].finalizedThrough

	// Group, per height above the floor, which pods hold which target.
	groups := map[htKey][]string{}
	heights := map[uint64]struct{}{}
	for _, s := range snaps {
		for _, b := range s.bindings {
			if b.height <= floor {
				continue
			}
			groups[htKey{b.height, b.target}] = append(groups[htKey{b.height, b.target}], s.label)
			heights[b.height] = struct{}{}
		}
	}

	alpha := alphaFor(len(snaps))
	fmt.Printf("== cross-pod locks above floor %d (n=%d pods, α=⌊2n/3⌋+1=%d) ==\n\n", floor, len(snaps), alpha)
	anyUnsafe := false
	anySplit := false
	for _, h := range sortedKeys(heights) {
		// Collect the distinct targets at this height and their pod sets.
		var targets []ids.ID
		for k := range groups {
			if k.height == h {
				targets = append(targets, k.target)
			}
		}
		sort.Slice(targets, func(i, j int) bool { return targets[i].Compare(targets[j]) < 0 })
		max := 0
		fmt.Printf("h=%d:\n", h)
		for _, tg := range targets {
			pods := groups[htKey{h, tg}]
			if len(pods) > max {
				max = len(pods)
			}
			fmt.Printf("   %s  ×%d  %v\n", tg, len(pods), pods)
		}
		switch {
		case len(targets) == 1:
			fmt.Printf("   → ALIGNED (all pods agree; no repair needed)\n")
		case max >= alpha:
			anyUnsafe = true
			fmt.Printf("   → !! UNSAFE: a target has ≥α=%d pods — an α-of-K cert MAY exist here. DO NOT auto-repair; investigate.\n", alpha)
		default:
			anySplit = true
			fmt.Printf("   → SPLIT %s, max group %d < α=%d ⇒ no α-of-K cert can exist ⇒ SAFE to canonicalize (targets are wrappers to verify are one inner block).\n",
				splitShape(targets, groups, h), max, alpha)
		}
		fmt.Println()
	}

	fmt.Println("== safety predicate ==")
	switch {
	case anyUnsafe:
		fmt.Println("VERDICT: STOP — at least one height has ≥α pods on one target (possible cert). Manual review required.")
	case anySplit:
		fmt.Printf("VERDICT: SAFE-TO-REPAIR — split(s) strictly below α, floor uniform at %d. Confirm each split's targets are proposervm wrappers of ONE inner block, then run boot-time MigrateStaleLocks.\n", floor)
	default:
		fmt.Println("VERDICT: NO-OP — no split-lock above the floor; nothing to repair.")
	}
}

func splitShape(targets []ids.ID, groups map[htKey][]string, h uint64) string {
	counts := make([]int, 0, len(targets))
	for _, tg := range targets {
		counts = append(counts, len(groups[htKey{h, tg}]))
	}
	sort.Sort(sort.Reverse(sort.IntSlice(counts)))
	out := ""
	for i, c := range counts {
		if i > 0 {
			out += "/"
		}
		out += fmt.Sprintf("%d", c)
	}
	return out
}

func keysOf(m map[uint64]struct{}) []uint64 { return sortedKeys(m) }

func sortedKeys(m map[uint64]struct{}) []uint64 {
	out := make([]uint64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
