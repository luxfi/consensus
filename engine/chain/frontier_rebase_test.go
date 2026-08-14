// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// frontier_rebase_test.go — the ancestry walk re-tests the frontier after a canonical
// rebase.
//
// pathFromTip walks parent links from the certified block down to the finalized tip,
// looping while cur != led.tip. A cert may name an outer envelope this node never
// tracked; when it does, the walk substitutes a local canonical-equivalent wrapper and
// assigns cur = localID. That assignment moves cur mid-body, past the loop condition, so
// every check below it tests a value the condition was never applied to. If the
// substituted wrapper is itself the finalized tip, the walk falls into
// `curHeight <= led.height` holding cur == led.tip and reports
// ErrConflictsWithFinalizedBranch: a cert that extends the frontier by exactly one block
// is refused, with the tip named as the losing sibling branch it supposedly descends from.
//
// The refusal is self-contradictory — the ancestor reached and the finalized tip are one
// id — and self-sustaining: refusing the cert leaves the tip where it was, so the next
// cert walks the same path to the same refusal. A frontier rebase must therefore re-test
// the loop condition against the substituted value before any check consumes it.

package chain

import (
	"errors"
	"testing"

	"github.com/luxfi/ids"
)

// TestPathFromTip_RebaseOntoTip_Finalizes pins the walk: when the canonical rebase lands
// on the finalized tip itself, the path is complete and the cert finalizes. A walk that
// does not re-test the frontier after the rebase refuses it as a conflicting branch.
func TestPathFromTip_RebaseOntoTip_Finalizes(t *testing.T) {
	const tipHeight = 36060

	tipInner := ids.GenerateTestID()  // the tip's inner execution commitment
	tip := ids.GenerateTestID()       // the tip envelope this node holds
	tipParent := ids.GenerateTestID() // one below the frontier
	remoteTip := ids.GenerateTestID() // a sibling envelope of the SAME inner block, unheld
	certBlock := ids.GenerateTestID() // the certified block at tipHeight+1

	led := seedLedger(tip, tipInner, tipHeight)

	// This node tracks its own tip envelope, never the remote sibling wrapper the cert
	// names as parent. Both wrap tipInner at tipHeight, so they are interchangeable.
	dag := mapAncestry{
		tip: {parent: tipParent, height: tipHeight, canonical: tipInner},
		certBlock: {
			parent: remoteTip, height: tipHeight + 1,
			canonical: certBlock, parentCanonical: tipInner,
		},
	}

	cert := Cert{
		Block: certBlock, Parent: remoteTip, Height: tipHeight + 1,
		Canonical: certBlock, ParentCanonical: tipInner,
	}

	next, _, err := Finalize(led, cert, dag)
	if err != nil {
		if errors.Is(err, ErrConflictsWithFinalizedBranch) {
			t.Fatalf("a cert extending the tip by one block was refused as a losing "+
				"branch — the walk rebased onto the tip and did not re-test the "+
				"frontier: %v", err)
		}
		t.Fatalf("finalize must succeed; got %v", err)
	}
	if next.height != tipHeight+1 {
		t.Fatalf("finalized height = %d, want %d", next.height, tipHeight+1)
	}
	if next.tip != certBlock {
		t.Fatalf("finalized tip = %s, want the certified block %s", next.tip, certBlock)
	}
}

// TestPathFromTip_RebaseOntoNonTip_StillRefuses pins the other side: a rebase that lands
// BELOW the frontier on a block that is genuinely not the tip is still a conflict. The
// re-test must not soften the guard into accepting real sibling branches.
func TestPathFromTip_RebaseOntoNonTip_StillRefuses(t *testing.T) {
	const tipHeight = 36060

	tip := ids.GenerateTestID()
	tipInner := ids.GenerateTestID()

	loserInner := ids.GenerateTestID() // a DIFFERENT inner block at the same height
	loser := ids.GenerateTestID()      // the losing sibling this node happens to track
	loserParent := ids.GenerateTestID()
	remoteLoser := ids.GenerateTestID() // unheld envelope of the losing block
	certBlock := ids.GenerateTestID()

	led := seedLedger(tip, tipInner, tipHeight)

	dag := mapAncestry{
		tip:   {parent: ids.GenerateTestID(), height: tipHeight, canonical: tipInner},
		loser: {parent: loserParent, height: tipHeight, canonical: loserInner},
		certBlock: {
			parent: remoteLoser, height: tipHeight + 1,
			canonical: certBlock, parentCanonical: loserInner,
		},
	}

	cert := Cert{
		Block: certBlock, Parent: remoteLoser, Height: tipHeight + 1,
		Canonical: certBlock, ParentCanonical: loserInner,
	}

	if _, _, err := Finalize(led, cert, dag); !errors.Is(err, ErrConflictsWithFinalizedBranch) {
		t.Fatalf("a cert descending from a losing sibling at the frontier height must "+
			"still be refused; got %v", err)
	}
}
