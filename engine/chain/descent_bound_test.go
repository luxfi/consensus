// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// descent_bound_test.go — Descent.From answers "give me what follows height H",
// and both of its arguments come off the wire.
//
// `height` is harmless: the walk stops at the first position this node cannot
// serve. `max` is not. It is turned into capacity before the walk begins —
//
//	run := Run{Base: height, Chain: make([]Certified, 0, max)}
//
// — so the reservation is a function of what the asker said, not of what this node
// holds. Every other peer-sized quantity in this package is bounded at the read:
// the cert codec caps vote_count against the remaining frame and sig_len against
// the buffer, the served-cert window is a constant, the Merkle step count caps at
// 64. This one is unbounded, and it is the newest of them.
package chain

import (
	"context"
	"math"
	"testing"

	"github.com/luxfi/ids"
)

// servingNode is a node holding finalized history at [1, count], each height with
// its cert and its block, ready to serve.
func servingNode(t *testing.T, count int) (*Runtime, []ids.ID) {
	t.Helper()
	vm := newEpochGateVM()
	rt := newReceiveGateRuntime(vm)

	ids_ := make([]ids.ID, 0, count)
	for h := 1; h <= count; h++ {
		blk := newEpochBlock(uint64(h), 1, ids.GenerateTestID(), "descent-"+string(rune('a'+h)))
		vm.register(blk)
		rt.Transitive.mu.Lock()
		if rt.Transitive.recoveredAt == nil {
			rt.Transitive.recoveredAt = map[uint64]ids.ID{}
		}
		rt.Transitive.recoveredAt[uint64(h)] = blk.id
		if rt.Transitive.certByDecision == nil {
			rt.Transitive.certByDecision = map[ids.ID][]byte{}
		}
		rt.Transitive.certByDecision[blk.id] = []byte{byte(h)}
		rt.Transitive.mu.Unlock()
		ids_ = append(ids_, blk.id)
	}
	return rt, ids_
}

// TestDescent_AskerCannotSizeTheAllocation. What a peer asks for bounds nothing
// about this node; only what this node finalized does. The reservation must follow
// the second, not the first.
func TestDescent_AskerCannotSizeTheAllocation(t *testing.T) {
	rt, _ := servingNode(t, 3)

	// CONTROL — an ordinary request is served, so a refusal below is about the
	// count and not about the node being unable to answer at all.
	if run, err := rt.From(context.Background(), 1, 2); err != nil || len(run.Chain) != 2 {
		t.Fatalf("control broke: a 2-block request over 3 finalized heights must serve 2; got %d, %v",
			len(run.Chain), err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("From PANICKED on a peer-supplied count: %v. `max` reaches "+
				"make([]Certified, 0, max) before the walk, so the reservation is whatever the "+
				"asker wrote in the request — this node holds 3 heights and was asked to reserve "+
				"for %d. One packet, one crash, no signature and no membership required. Every "+
				"other peer-sized quantity here is capped at the read.", r, math.MaxInt)
		}
	}()

	run, err := rt.From(context.Background(), 1, math.MaxInt)
	if err == nil && len(run.Chain) > 3 {
		t.Fatalf("served %d entries from a node holding 3 finalized heights", len(run.Chain))
	}
}

// TestDescent_RunIsContiguousAndAddressable: the type is meant to make a hole
// inexpressible, so the walk must stop at the first gap rather than skip it, and
// Next() must name the position the caller continues from.
func TestDescent_RunIsContiguousAndAddressable(t *testing.T) {
	rt, blocks := servingNode(t, 5)

	// Punch out height 3. The run from 1 must stop at 2 — never return 1,2,4,5.
	rt.Transitive.mu.Lock()
	delete(rt.Transitive.recoveredAt, 3)
	rt.Transitive.mu.Unlock()

	run, err := rt.From(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("a node holding heights 1-2 must serve them: %v", err)
	}
	if len(run.Chain) != 2 {
		t.Fatalf("run crossed a gap: %d entries from a history missing height 3", len(run.Chain))
	}
	if run.Base != 1 || run.Next() != 3 {
		t.Fatalf("run addresses (base %d, next %d), want (1, 3) — Next names the position the "+
			"caller asks for again", run.Base, run.Next())
	}
	if got := run.Chain[0].Block; string(got) == "" {
		t.Fatal("served an entry with no block bytes")
	}
	_ = blocks

	// An empty run must not advance the caller past a position it never received.
	empty := Run{Base: 9}
	if empty.Next() != 9 {
		t.Fatalf("an empty run advanced to %d; it must ask again at 9", empty.Next())
	}
}

// TestDescent_UnservablePositionIsACleanMiss: a height this node never finalized
// is answered with ErrNoDescent, never with a claim about the chain.
func TestDescent_UnservablePositionIsACleanMiss(t *testing.T) {
	rt, _ := servingNode(t, 2)

	for _, tc := range []struct {
		name   string
		height uint64
		max    int
	}{
		{"above our history", 900, 4},
		{"zero max", 1, 0},
		{"negative max", 1, -5},
	} {
		run, err := rt.From(context.Background(), tc.height, tc.max)
		if len(run.Chain) != 0 {
			t.Fatalf("%s: served %d entries", tc.name, len(run.Chain))
		}
		if run.Base != tc.height {
			t.Fatalf("%s: run base %d, want %d", tc.name, run.Base, tc.height)
		}
		if tc.max > 0 && err == nil {
			t.Fatalf("%s: an unservable position must report ErrNoDescent", tc.name)
		}
	}
}
