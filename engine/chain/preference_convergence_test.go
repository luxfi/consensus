// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// TestPreference_ConvergesSiblings_NoDownLeaderHalt proves the down-leader
// liveness fix.
//
// THE BUG: when the slot-leader proposer is down, each healthy validator builds
// its OWN block at the next height (its own clock → its own blkID). The α-of-K
// finality cert needs alpha votes on ONE block, but the votes split across the
// competing siblings, so no cert ever assembles and the chain HALTS. The root was
// Preference() returning the FINALIZED tip — so the VM kept rebuilding the same
// height instead of building on a verified-but-unfinalized block.
//
// THE FIX: Preference() returns the deterministic BUILD tip — the deepest verified
// block extending the finalized chain, lowest-ID sibling per level — so every node
// builds H+1 on the SAME block and the vote converges (matching avalanchego's
// snowman preference, which steers the VM to the preferred non-finalized tip).
func TestPreference_ConvergesSiblings_NoDownLeaderHalt(t *testing.T) {
	ctx := context.Background()
	c := NewChainConsensus(5, 4, 2)

	// Finalized tip at height 36.
	fin := ids.ID{36}
	if err := c.AddBlock(ctx, &Block{id: fin, parentID: ids.Empty, height: 36}); err != nil {
		t.Fatal(err)
	}
	c.ledger = FinalityLedger{tip: fin, height: 36, set: true}

	// Down-leader: two competing VERIFIED siblings at height 37.
	lo := ids.ID{37, 0x01} // lower ID
	hi := ids.ID{37, 0x02} // higher ID
	for _, b := range []*Block{
		{id: lo, parentID: fin, height: 37},
		{id: hi, parentID: fin, height: 37},
	} {
		if err := c.AddBlock(ctx, b); err != nil {
			t.Fatal(err)
		}
	}

	// THE FIX: the build tip is the lowest-ID verified sibling, NOT the finalized
	// tip 36. Pre-fix this returned `fin`, so the VM rebuilt height 37 forever and
	// the chain halted on split votes.
	if got := c.PreferredBuildTip(); got != lo {
		t.Fatalf("Preference() = %s; want lowest-ID build tip %s (NOT finalized tip %s — that is the halt)", got, lo, fin)
	}

	// Deterministic: same tree → same tip on every call, so ALL validators pick the
	// same block and their alpha votes land together.
	for i := 0; i < 8; i++ {
		if got := c.PreferredBuildTip(); got != lo {
			t.Fatalf("Preference() nondeterministic on call %d: %s != %s", i, got, lo)
		}
	}

	// Build H+1 on the chosen sibling: preference follows the DEEPEST verified tip,
	// so the chain keeps extending while the cert for 37 is still being gathered.
	tip38 := ids.ID{38}
	if err := c.AddBlock(ctx, &Block{id: tip38, parentID: lo, height: 38}); err != nil {
		t.Fatal(err)
	}
	if got := c.PreferredBuildTip(); got != tip38 {
		t.Fatalf("Preference() = %s; want deepest verified tip %s", got, tip38)
	}

	// A REJECTED sibling — even with a lower ID — must never be chosen as the build
	// tip (a losing branch can't steer block production).
	rej := ids.ID{37, 0x00}
	if err := c.AddBlock(ctx, &Block{id: rej, parentID: fin, height: 37}); err != nil {
		t.Fatal(err)
	}
	c.blocks[rej].rejected = true
	if got := c.PreferredBuildTip(); got != tip38 {
		t.Fatalf("Preference() = %s; a rejected lower-ID sibling must be skipped (want %s)", got, tip38)
	}
}

// TestPreference_NoChildren_ReturnsFinalizedTip confirms the fix is a strict
// superset of the old behavior: with NO verified children, Preference() still
// returns the finalized tip (the VM builds the next height on it) — so the change
// only ADDS forward progress when a verified child exists, never regresses the
// no-sibling case.
func TestPreference_NoChildren_ReturnsFinalizedTip(t *testing.T) {
	ctx := context.Background()
	c := NewChainConsensus(5, 4, 2)
	fin := ids.ID{42}
	if err := c.AddBlock(ctx, &Block{id: fin, parentID: ids.Empty, height: 42}); err != nil {
		t.Fatal(err)
	}
	c.ledger = FinalityLedger{tip: fin, height: 42, set: true}
	if got := c.PreferredBuildTip(); got != fin {
		t.Fatalf("Preference() = %s; with no verified children want finalized tip %s", got, fin)
	}
}
