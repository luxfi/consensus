// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// frontier_rebase_test.go — the ancestry walk must re-test the frontier AFTER a
// canonical rebase.
//
// pathFromTip walks parent links from the certified block down to the finalized tip,
// looping while cur != led.tip. When the exact outer envelope a cert names is untracked,
// the walk stands a LOCAL canonical-equivalent wrapper in its place and assigns cur =
// localID. That assignment moves cur mid-body, so the loop condition no longer holds for
// the value being tested by the checks below it. When the local wrapper IS the finalized
// tip, execution reached `curHeight <= led.height` with cur == led.tip and returned
// ErrConflictsWithFinalizedBranch — refusing a cert that extends the frontier by exactly
// one block, and naming the tip itself as the "losing/pruned sibling branch" it descends
// from.
//
// Observed on lux-testnet luxd-0, which refused every incoming cert at a static tip:
//
//	incoming cert: REFUSED by finality guard (no VM.Accept)
//	error="chain: cert-selected block does not extend the finalized frontier (it descends
//	from a losing/pruned sibling branch): tH96v3tgug2SqQS1uPK6BRo2nUNV6r32SP5RZitUtsQwSHb7Q
//	ancestry reaches oYkSNtSbAaYHfQECmHS1P5VZcrcobANbVLLzcKj2xMheztvPU (height 36060)
//	not finalized tip oYkSNtSbAaYHfQECmHS1P5VZcrcobANbVLLzcKj2xMheztvPU"
//
// The reached ancestor and the finalized tip are the same id. The refusal is self-
// contradictory, and it is self-sustaining: the tip never moves, so the next cert takes
// the same path, so the validator is stranded permanently.

package chain

import (
	"errors"
	"testing"

	"github.com/luxfi/ids"
)

// TestPathFromTip_RebaseOntoTip_Finalizes pins the walk: when the canonical rebase lands
// on the finalized tip itself, the path is complete and the cert finalizes. The pre-fix
// walk returned ErrConflictsWithFinalizedBranch here.
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
