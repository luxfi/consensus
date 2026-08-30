// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/ids"
)

// descentAt builds a node that finalized [from, from+n) and can serve every one
// of those heights: a height index, a cert per block, and the blocks themselves.
func descentAt(from uint64, n int) (*Runtime, []ids.ID) {
	vm := &mockVM{blocks: map[ids.ID]*mockBlock{}}
	e := &Transitive{
		certByDecision: map[ids.ID][]byte{},
		recoveredAt:    map[uint64]ids.ID{},
	}
	ids_ := make([]ids.ID, 0, n)
	for i := 0; i < n; i++ {
		h := from + uint64(i)
		id := ids.GenerateTestID()
		vm.blocks[id] = &mockBlock{id: id, height: h, bytes: []byte{byte(h)}}
		e.certByDecision[id] = []byte{'c', byte(h)}
		e.recoveredAt[h] = id
		ids_ = append(ids_, id)
	}
	return &Runtime{Transitive: e, config: NetworkConfig{VM: vm}}, ids_
}

// TestDescentIsContiguous is the property the type exists to guarantee: entry i
// is the block at Base+i, with its own cert. A caller can therefore trust
// position without re-deriving it, which is what a behind node could not do when
// recovery was addressed by block id.
func TestDescentIsContiguous(t *testing.T) {
	rt, want := descentAt(100, 5)

	run, err := rt.From(context.Background(), 100, 5)
	if err != nil {
		t.Fatalf("From: %v", err)
	}
	if run.Base != 100 || len(run.Chain) != 5 {
		t.Fatalf("run = base %d len %d, want base 100 len 5", run.Base, len(run.Chain))
	}
	for i, c := range run.Chain {
		h := byte(100 + i)
		if len(c.Block) != 1 || c.Block[0] != h {
			t.Fatalf("entry %d is block %v, want the block at height %d", i, c.Block, h)
		}
		if len(c.Cert) != 2 || c.Cert[1] != h {
			t.Fatalf("entry %d carries cert %v, want the cert for height %d", i, c.Cert, h)
		}
	}
	if run.Next() != 105 {
		t.Fatalf("Next = %d, want 105", run.Next())
	}
	_ = want
}

// TestDescentStopsAtTheFirstHoleRatherThanSkippingIt is the whole reason Run has
// no per-entry height. A node that finalized 100..102 and 104 must serve three
// entries and stop, not four with 104 sitting where 103 belongs -- a caller
// counting what it received would read that as progress and skip a height
// forever, which is precisely the failure this file exists to end.
func TestDescentStopsAtTheFirstHoleRatherThanSkippingIt(t *testing.T) {
	rt, _ := descentAt(100, 3)
	// 104 is finalized; 103 is not.
	orphan := ids.GenerateTestID()
	rt.config.VM.(*mockVM).blocks[orphan] = &mockBlock{id: orphan, height: 104, bytes: []byte{104}}
	rt.Transitive.certByDecision[orphan] = []byte{'c', 104}
	rt.Transitive.recoveredAt[104] = orphan

	run, err := rt.From(context.Background(), 100, 10)
	if err != nil {
		t.Fatalf("From: %v", err)
	}
	if len(run.Chain) != 3 {
		t.Fatalf("served %d entries, want 3 — the run must END at the hole, not span it", len(run.Chain))
	}
	if run.Next() != 103 {
		t.Fatalf("Next = %d, want 103 — the caller must resume AT the hole", run.Next())
	}
}

// TestDescentWithoutTheCertServesNothingAtThatHeight: a block whose cert this
// node cannot produce is not servable, because the pair is what the peer needs.
// Serving the block alone would hand over something unacceptable and look like
// progress.
func TestDescentWithoutTheCertServesNothingAtThatHeight(t *testing.T) {
	rt, idsAt := descentAt(100, 3)
	delete(rt.Transitive.certByDecision, idsAt[1]) // height 101 loses its cert

	run, err := rt.From(context.Background(), 100, 10)
	if err != nil {
		t.Fatalf("From: %v", err)
	}
	if len(run.Chain) != 1 {
		t.Fatalf("served %d entries, want 1 — a block without its cert is not a Certified", len(run.Chain))
	}
}

// TestDescentAtAnUnknownPositionIsACleanMiss: the caller must be able to tell
// "I cannot serve that" from "that height does not exist", so it asks another
// peer instead of concluding it is caught up.
func TestDescentAtAnUnknownPositionIsACleanMiss(t *testing.T) {
	rt, _ := descentAt(100, 3)

	_, err := rt.From(context.Background(), 500, 10)
	if !errors.Is(err, ErrNoDescent) {
		t.Fatalf("err = %v, want ErrNoDescent", err)
	}
}
