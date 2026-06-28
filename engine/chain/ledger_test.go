// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// ledger_test.go — the pure (Ledger, Cert, DAG) → (Ledger', Plan) fold, exercised
// with a plain-map Ancestry and ZERO engine/VM. This is the proof that finality is
// a pure FOLD over values: no ChainConsensus, no Transitive, no *Block, no lock —
// just Finalize over a FinalityLedger and a Cert, reading a read-only Ancestry.
//
// If these pass, "accept = markFinalized(...)" is structurally unwriteable: the
// only thing that advances finality is replacing the whole ledger VALUE with the
// fold's output, and the fold never mutates its input (TestLedger_PureNoInputMutation).
package chain

import (
	"errors"
	"fmt"
	"testing"

	"github.com/luxfi/ids"
)

// mapDAG is a plain in-memory Ancestry — a parent/height table the fold reads. It
// proves Finalize needs NOTHING but the read-only ancestry view: no engine, no VM.
type mapDAG map[ids.ID]dagNode

type dagNode struct {
	parent ids.ID
	height uint64
}

func (d mapDAG) Parent(id ids.ID) (ids.ID, uint64, bool) {
	n, ok := d[id]
	if !ok {
		return ids.Empty, 0, false
	}
	return n.parent, n.height, true
}

func (d mapDAG) Children(id ids.ID) []ids.ID {
	var out []ids.ID
	for cid, n := range d {
		if n.parent == id {
			out = append(out, cid)
		}
	}
	return out
}

func (d mapDAG) add(id, parent ids.ID, height uint64) { d[id] = dagNode{parent: parent, height: height} }

// seedFold finalizes a genesis at height 0 through the public fold (not the
// seedLedger helper) so every test exercises Finalize end to end.
func seedFold(t *testing.T, g ids.ID, dag mapDAG) FinalityLedger {
	t.Helper()
	led, _, err := Finalize(FinalityLedger{}, Cert{Block: g, Parent: ids.Empty, Height: 0}, dag)
	if err != nil {
		t.Fatalf("seed genesis fold: %v", err)
	}
	return led
}

// TestLedger_FirstFinalizeSeeds: the empty ledger folds the first cert into a seed
// — tip/height set, the block is the sole Accept, nothing pruned.
func TestLedger_FirstFinalizeSeeds(t *testing.T) {
	g := ids.GenerateTestID()
	led, plan, err := Finalize(FinalityLedger{}, Cert{Block: g, Parent: ids.Empty, Height: 0}, mapDAG{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if led.Tip() != g {
		t.Fatalf("seed tip=%s want %s", led.Tip(), g)
	}
	if h, set := led.Height(); !set || h != 0 {
		t.Fatalf("seed height=(%d,%v) want (0,true)", h, set)
	}
	if id, ok := led.At(0); !ok || id != g {
		t.Fatalf("seed byHeight[0]=(%s,%v) want (%s,true)", id, ok, g)
	}
	if len(plan.Accept) != 1 || plan.Accept[0] != g || len(plan.Reject) != 0 {
		t.Fatalf("seed plan=%+v want Accept=[%s] Reject=[]", plan, g)
	}
}

// TestLedger_SiblingReorg is the heart of the decomplect: P→{A,B}, B→B2. A cert for
// A finalizes A and PRUNES the losing sibling subtree {B,B2}. Pure: zero engine.
func TestLedger_SiblingReorg(t *testing.T) {
	p := ids.GenerateTestID()
	a := ids.GenerateTestID()
	b := ids.GenerateTestID()
	b2 := ids.GenerateTestID()
	dag := mapDAG{}
	dag.add(a, p, 1)
	dag.add(b, p, 1)
	dag.add(b2, b, 2)

	led := seedFold(t, p, dag) // P finalized at height 0
	led, plan, err := Finalize(led, Cert{Block: a, Parent: p, Height: 1}, dag)
	if err != nil {
		t.Fatalf("finalize A: %v", err)
	}
	if led.Tip() != a {
		t.Fatalf("tip=%s want A=%s", led.Tip(), a)
	}
	if len(plan.Accept) != 1 || plan.Accept[0] != a {
		t.Fatalf("Accept=%v want [A]", plan.Accept)
	}
	if len(plan.Reject) != 2 || !idIn(plan.Reject, b) || !idIn(plan.Reject, b2) {
		t.Fatalf("Reject=%v want {B,B2}", plan.Reject)
	}
}

// TestLedger_MultiStepDescendant: a catch-up cert for a DESCENDANT (A3) several
// heights above the tip finalizes the whole contiguous path {A1,A2,A3} ascending in
// ONE fold, and prunes the competing sibling B1.
func TestLedger_MultiStepDescendant(t *testing.T) {
	g := ids.GenerateTestID()
	a1 := ids.GenerateTestID()
	a2 := ids.GenerateTestID()
	a3 := ids.GenerateTestID()
	b1 := ids.GenerateTestID()
	dag := mapDAG{}
	dag.add(a1, g, 1)
	dag.add(a2, a1, 2)
	dag.add(a3, a2, 3)
	dag.add(b1, g, 1) // sibling of a1 under genesis

	led := seedFold(t, g, dag)
	led, plan, err := Finalize(led, Cert{Block: a3, Parent: a2, Height: 3}, dag)
	if err != nil {
		t.Fatalf("finalize A3: %v", err)
	}
	if led.Tip() != a3 {
		t.Fatalf("tip=%s want A3=%s", led.Tip(), a3)
	}
	if len(plan.Accept) != 3 || plan.Accept[0] != a1 || plan.Accept[1] != a2 || plan.Accept[2] != a3 {
		t.Fatalf("Accept=%v want [A1,A2,A3] ascending", plan.Accept)
	}
	if len(plan.Reject) != 1 || plan.Reject[0] != b1 {
		t.Fatalf("Reject=%v want [B1]", plan.Reject)
	}
	// every accepted height is now recorded.
	for h, want := range map[uint64]ids.ID{1: a1, 2: a2, 3: a3} {
		if id, ok := led.At(h); !ok || id != want {
			t.Fatalf("byHeight[%d]=(%s,%v) want %s", h, id, ok, want)
		}
	}
}

// TestLedger_Idempotent: re-folding the already-finalized head is a nil-error no-op
// with an EMPTY plan, and returns the ledger UNCHANGED.
func TestLedger_Idempotent(t *testing.T) {
	g := ids.GenerateTestID()
	a := ids.GenerateTestID()
	dag := mapDAG{}
	dag.add(a, g, 1)

	led := seedFold(t, g, dag)
	led, _, err := Finalize(led, Cert{Block: a, Parent: g, Height: 1}, dag)
	if err != nil {
		t.Fatalf("finalize A: %v", err)
	}
	again, plan, err := Finalize(led, Cert{Block: a, Parent: g, Height: 1}, dag)
	if err != nil {
		t.Fatalf("idempotent re-fold must be nil error, got %v", err)
	}
	if len(plan.Accept) != 0 || len(plan.Reject) != 0 {
		t.Fatalf("idempotent plan must be empty, got %+v", plan)
	}
	if again.Tip() != led.Tip() {
		t.Fatalf("idempotent fold moved the tip: %s -> %s", led.Tip(), again.Tip())
	}
}

// TestLedger_Equivocation: a DIFFERENT block at an already-finalized height is
// equivocation evidence — ErrHeightAlreadyFinalized, ledger unchanged.
func TestLedger_Equivocation(t *testing.T) {
	g := ids.GenerateTestID()
	a := ids.GenerateTestID()
	c := ids.GenerateTestID()
	dag := mapDAG{}
	dag.add(a, g, 1)
	dag.add(c, g, 1)

	led := seedFold(t, g, dag)
	led, _, err := Finalize(led, Cert{Block: a, Parent: g, Height: 1}, dag)
	if err != nil {
		t.Fatalf("finalize A: %v", err)
	}
	out, _, err := Finalize(led, Cert{Block: c, Parent: g, Height: 1}, dag)
	if !errors.Is(err, ErrHeightAlreadyFinalized) {
		t.Fatalf("equivocation must be refused, got %v", err)
	}
	if out.Tip() != led.Tip() {
		t.Fatalf("a refused fold must not move the tip")
	}
}

// TestLedger_ConflictWithPrunedBranch: after A wins height 1, a cert for B2 — which
// descends from the LOSING sibling B1 at the already-finalized height 1 — is refused
// as ErrConflictsWithFinalizedBranch (it would branch finalized history).
func TestLedger_ConflictWithPrunedBranch(t *testing.T) {
	g := ids.GenerateTestID()
	a := ids.GenerateTestID()
	b1 := ids.GenerateTestID()
	b2 := ids.GenerateTestID()
	dag := mapDAG{}
	dag.add(a, g, 1)
	dag.add(b1, g, 1)
	dag.add(b2, b1, 2)

	led := seedFold(t, g, dag)
	led, _, err := Finalize(led, Cert{Block: a, Parent: g, Height: 1}, dag)
	if err != nil {
		t.Fatalf("finalize A: %v", err)
	}
	if _, _, err := Finalize(led, Cert{Block: b2, Parent: b1, Height: 2}, dag); !errors.Is(err, ErrConflictsWithFinalizedBranch) {
		t.Fatalf("a losing-branch descendant must conflict, got %v", err)
	}
}

// TestLedger_HeightGap: a cert whose block sits a height ABOVE the tip's successor,
// with the tip as DIRECT parent (a skipped height), breaks contiguity →
// ErrNonMonotonicFinalizedHeight.
func TestLedger_HeightGap(t *testing.T) {
	g := ids.GenerateTestID()
	x := ids.GenerateTestID()
	dag := mapDAG{}
	dag.add(x, g, 2) // height 2 with parent g(0) — skips height 1

	led := seedFold(t, g, dag)
	if _, _, err := Finalize(led, Cert{Block: x, Parent: g, Height: 2}, dag); !errors.Is(err, ErrNonMonotonicFinalizedHeight) {
		t.Fatalf("a height gap must be refused, got %v", err)
	}
}

// TestLedger_StaleHeightBelowFrontier hits the direct cert.Height<=led.height branch
// using a sparse ledger (only height 2 recorded): a cert at height 1 (below the
// frontier, not in the index) is non-monotonic, never a silent re-finalize.
func TestLedger_StaleHeightBelowFrontier(t *testing.T) {
	a := ids.GenerateTestID()
	x := ids.GenerateTestID()
	// hand-built sparse ledger: head at height 2, height 1 NOT indexed.
	led := FinalityLedger{tip: a, height: 2, set: true, byHeight: map[uint64]ids.ID{2: a}}
	if _, _, err := Finalize(led, Cert{Block: x, Parent: ids.Empty, Height: 1}, mapDAG{}); !errors.Is(err, ErrNonMonotonicFinalizedHeight) {
		t.Fatalf("a stale below-frontier height must be refused, got %v", err)
	}
}

// TestLedger_AncestorNotTracked: a cert for a descendant whose intermediate ancestor
// is NOT in the DAG is a behind-node DEFER (ErrAncestorNotTracked), never a finalize
// on an unproven path.
func TestLedger_AncestorNotTracked(t *testing.T) {
	g := ids.GenerateTestID()
	missing := ids.GenerateTestID() // height 1, NOT added to the dag
	x := ids.GenerateTestID()
	dag := mapDAG{}
	dag.add(x, missing, 2) // x's parent `missing` is untracked

	led := seedFold(t, g, dag)
	if _, _, err := Finalize(led, Cert{Block: x, Parent: missing, Height: 2}, dag); !errors.Is(err, ErrAncestorNotTracked) {
		t.Fatalf("an untracked ancestor must DEFER, got %v", err)
	}
}

// TestLedger_PureNoInputMutation is the immutability proof: a successful fold must
// NOT mutate the INPUT ledger value (its tip/height/byHeight). Finality is a value
// you replace, not a place you poke.
func TestLedger_PureNoInputMutation(t *testing.T) {
	g := ids.GenerateTestID()
	a1 := ids.GenerateTestID()
	dag := mapDAG{}
	dag.add(a1, g, 1)

	led0 := seedFold(t, g, dag) // {tip:g, height:0, byHeight:{0:g}}
	wantTip := led0.Tip()
	wantH, wantSet := led0.Height()
	wantLen := len(led0.byHeight)

	led1, _, err := Finalize(led0, Cert{Block: a1, Parent: g, Height: 1}, dag)
	if err != nil {
		t.Fatalf("finalize A1: %v", err)
	}
	// the NEW value advanced...
	if led1.Tip() != a1 {
		t.Fatalf("new tip=%s want A1=%s", led1.Tip(), a1)
	}
	// ...the INPUT value is byte-for-byte unchanged — no place was poked.
	if led0.Tip() != wantTip {
		t.Fatalf("input tip mutated: %s want %s", led0.Tip(), wantTip)
	}
	if h, set := led0.Height(); h != wantH || set != wantSet {
		t.Fatalf("input height mutated: (%d,%v) want (%d,%v)", h, set, wantH, wantSet)
	}
	if len(led0.byHeight) != wantLen {
		t.Fatalf("input byHeight mutated: len %d want %d", len(led0.byHeight), wantLen)
	}
	if _, ok := led0.byHeight[1]; ok {
		t.Fatal("input byHeight gained height 1 — the fold mutated its input map (NOT a pure value)")
	}
}

// TestLedger_WindowPrune: finalizing far past the equivocation window drops the index
// for ancient heights (so clone() stays O(window) — the HIGH-1 fix) while keeping the
// recent window. A pruned-out old height is still safely UN-finalizable: the monotonic
// guard refuses it (cert.Height <= led.height) without the index, so dropping the entry
// never opens a double-finalize.
func TestLedger_WindowPrune(t *testing.T) {
	dag := mapDAG{}
	g := ids.GenerateTestID()
	dag.add(g, ids.Empty, 0)
	led := seedFold(t, g, dag)

	const n = equivocationWindow + 50 // finalize well past the window
	parent := g
	for h := uint64(1); h <= n; h++ {
		id := ids.GenerateTestID()
		dag.add(id, parent, h)
		var err error
		led, _, err = Finalize(led, Cert{Block: id, Parent: parent, Height: h}, dag)
		if err != nil {
			t.Fatalf("finalize height %d: %v", h, err)
		}
		parent = id
	}

	// Recent heights (>= tip-window) retained; ancient heights (< tip-window) pruned.
	if _, ok := led.At(n); !ok {
		t.Fatalf("tip height %d must be retained", n)
	}
	if _, ok := led.At(n - equivocationWindow); !ok {
		t.Fatalf("lowest in-window height %d must be retained", n-equivocationWindow)
	}
	if _, ok := led.At(n - equivocationWindow - 1); ok {
		t.Fatalf("height %d below the window must be pruned", n-equivocationWindow-1)
	}
	if _, ok := led.At(0); ok {
		t.Fatal("genesis height 0 must be pruned below the window")
	}
	// byHeight is bounded — this is what keeps clone() O(window).
	if len(led.byHeight) > equivocationWindow+1 {
		t.Fatalf("byHeight=%d exceeds the window bound %d", len(led.byHeight), equivocationWindow+1)
	}
	// SAFETY: a stale cert at a pruned height is still refused (monotonic guard), never
	// silently double-finalized just because its index entry was dropped.
	stale := ids.GenerateTestID()
	if _, _, err := Finalize(led, Cert{Block: stale, Parent: g, Height: 0}, dag); !errors.Is(err, ErrNonMonotonicFinalizedHeight) {
		t.Fatalf("stale cert at pruned height: err=%v want ErrNonMonotonicFinalizedHeight", err)
	}
}

// BenchmarkFinalize_FlatCost is the HIGH-1 proof: per-finalize cost is FLAT regardless
// of chain height, because the window-bounded byHeight keeps clone() O(window). Before
// the fix clone copied the whole unbounded index — ~87ms/op and 100MB/op at 10^6 heights
// (measured). A near-constant ns/op across the four tip heights below is the fix.
func BenchmarkFinalize_FlatCost(b *testing.B) {
	for _, start := range []uint64{1 << 6, 1 << 12, 1 << 18, 1 << 20} {
		b.Run(fmt.Sprintf("height_%d", start), func(b *testing.B) {
			led := seedLedger(ids.GenerateTestID(), start)
			base := uint64(0)
			if start >= equivocationWindow {
				base = start - equivocationWindow + 1
			}
			for h := base; h <= start; h++ { // fill a full window so clone copies window-many
				led.byHeight[h] = ids.GenerateTestID()
			}
			parent := led.tip
			next := ids.GenerateTestID()
			dag := mapDAG{}
			dag.add(parent, ids.Empty, start)
			dag.add(next, parent, start+1)
			cert := Cert{Block: next, Parent: parent, Height: start + 1}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _, _ = Finalize(led, cert, dag)
			}
		})
	}
}
