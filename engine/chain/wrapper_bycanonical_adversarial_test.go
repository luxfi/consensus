// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// wrapper_bycanonical_adversarial_test.go — RED-TEAM probes of the PRODUCTION
// blocksAncestry.WrapperByCanonical scan (topological.go), which the pure-fold
// harness stubs out. Targets: the height+canonicalRep filter is exact (no cross-height
// or cross-execution collapse), and the "prefer accepted" tiebreak lands on the
// committed wrapper.
package chain

import "testing"
import "github.com/luxfi/ids"

func mkBlock(id, parent ids.ID, height uint64, canonical, parentCanonical ids.ID, accepted bool) *Block {
	return &Block{
		id:                id,
		parentID:          parent,
		height:            height,
		canonicalID:       canonical,
		parentCanonicalID: parentCanonical,
		accepted:          accepted,
	}
}

// #1/#4 — the scan matches ONLY on (height, canonicalRep). A block with a DIFFERENT
// canonical at the same height, or the SAME canonical at a different height, is never
// returned. This is the binding boundary in the real scan.
func TestAdvProd_WrapperByCanonical_ExactHeightAndCanonical(t *testing.T) {
	tip := ids.GenerateTestID()
	inner6 := ids.GenerateTestID()
	inner6b := ids.GenerateTestID() // different execution
	inner7 := ids.GenerateTestID()

	wrap6 := ids.GenerateTestID()
	wrap6diff := ids.GenerateTestID()
	wrap7same := ids.GenerateTestID() // same canonical value as target but WRONG height

	blocks := map[ids.ID]*Block{
		wrap6:     mkBlock(wrap6, tip, 6, inner6, tip, false),
		wrap6diff: mkBlock(wrap6diff, tip, 6, inner6b, tip, false),
		// Deliberately reuse inner6 as a canonical at height 7 to test the height filter.
		wrap7same: mkBlock(wrap7same, wrap6, 7, inner6, inner6, false),
	}
	a := blocksAncestry{blocks: blocks}

	// Exact match at height 6 returns wrap6 (the only inner6 wrapper at h6).
	if got, ok := a.WrapperByCanonical(inner6, 6); !ok || got != wrap6 {
		t.Fatalf("expected wrap6 for (inner6,6), got %v ok=%v", got, ok)
	}
	// A different canonical at h6 (inner7, no wrapper) misses.
	if got, ok := a.WrapperByCanonical(inner7, 6); ok {
		t.Fatalf("no wrapper of inner7 at h6 exists; must miss, got %v", got)
	}
	// SAME canonical value (inner6) but at height 7: must NOT return wrap6 (height filter);
	// it returns wrap7same only because wrap7same is literally at h7 with canonical inner6.
	if got, ok := a.WrapperByCanonical(inner6, 7); !ok || got != wrap7same {
		t.Fatalf("height filter: (inner6,7) must return wrap7same, not the h6 wrapper, got %v ok=%v", got, ok)
	}
	// Cross-height leak check: (inner6,6) must never leak wrap7same.
	if got, _ := a.WrapperByCanonical(inner6, 6); got == wrap7same {
		t.Fatalf("cross-height leak: returned a height-7 wrapper for a height-6 query")
	}
}

// #4 — "prefer accepted" tiebreak: when several aliases of one inner block are tracked,
// the accepted one (the wrapper the VM committed) is returned deterministically.
func TestAdvProd_WrapperByCanonical_PrefersAccepted(t *testing.T) {
	tip := ids.GenerateTestID()
	inner6 := ids.GenerateTestID()

	accepted6 := ids.GenerateTestID()
	pending6a := ids.GenerateTestID()
	pending6b := ids.GenerateTestID()

	blocks := map[ids.ID]*Block{
		pending6a: mkBlock(pending6a, tip, 6, inner6, tip, false),
		accepted6: mkBlock(accepted6, tip, 6, inner6, tip, true),
		pending6b: mkBlock(pending6b, tip, 6, inner6, tip, false),
	}
	a := blocksAncestry{blocks: blocks}
	// Repeat to defeat map-iteration randomness: accepted must ALWAYS win.
	for i := 0; i < 64; i++ {
		got, ok := a.WrapperByCanonical(inner6, 6)
		if !ok || got != accepted6 {
			t.Fatalf("iter %d: accepted wrapper must always be preferred, got %v ok=%v", i, got, ok)
		}
	}
}

// #1 — bare-block degrade: canonicalID Empty ⇒ canonicalRep falls back to the outer id,
// so a bare chain's WrapperByCanonical is keyed on the outer id (no two envelopes share
// it ⇒ collapse is inert), exactly as claimed.
func TestAdvProd_WrapperByCanonical_BareDegrade(t *testing.T) {
	tip := ids.GenerateTestID()
	bare := ids.GenerateTestID()
	blocks := map[ids.ID]*Block{
		bare: mkBlock(bare, tip, 6, ids.Empty, ids.Empty, false), // canonicalID Empty ⇒ rep == id
	}
	a := blocksAncestry{blocks: blocks}
	if got, ok := a.WrapperByCanonical(bare, 6); !ok || got != bare {
		t.Fatalf("bare block must resolve by its own outer id, got %v ok=%v", got, ok)
	}
	// A random inner id never matches a bare block (no aliasing on a bare chain).
	if _, ok := a.WrapperByCanonical(ids.GenerateTestID(), 6); ok {
		t.Fatalf("bare chain must not alias-collapse on an arbitrary canonical")
	}
}
