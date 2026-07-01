// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// convergence_hardening_test.go — white-box gates for the RED-round convergence
// hardening: N4 (operator-tunable settle window) and N1 (proven-loser parent filter).
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// TestConvergence_SettleWindow_OperatorOverride proves N4: an explicit
// Params.ConvergenceSettleWindow wins over the RoundTO/2 auto value (so a WAN operator can
// lengthen the settle without touching round cadence), and a near-zero misconfiguration is
// FLOORED rather than disabling the settle entirely.
func TestConvergence_SettleWindow_OperatorOverride(t *testing.T) {
	vs := newTestValidatorSet(5)

	p := params5Prod()
	p.ConvergenceSettleWindow = 1500 * time.Millisecond
	e, _ := newQuorumEngine(t, p, vs, 0, &recordingGossiper{})
	if got := e.convergenceSettleWindow(); got != 1500*time.Millisecond {
		t.Fatalf("operator settle override not honored: got %s want 1.5s", got)
	}

	p2 := params5Prod()
	p2.ConvergenceSettleWindow = 5 * time.Millisecond // misconfigured near-zero
	e2, _ := newQuorumEngine(t, p2, vs, 0, &recordingGossiper{})
	if got := e2.convergenceSettleWindow(); got < 150*time.Millisecond {
		t.Fatalf("settle floor not applied to a near-zero override: got %s (must be ≥150ms)", got)
	}
}

// TestConvergence_ParentProvenLoser_ExcludesDeadBranch proves N1: a height-H slot whose
// parent LOST its own height's convergence (a strictly-lower-canonical H-1 sibling is
// tracked) is filtered out of the votable set, so this node never wastes its one height-H
// signature on a dead branch — while the untracked finalized-tip parent is NEVER filtered
// (conservative, so the normal path cannot be starved).
func TestConvergence_ParentProvenLoser_ExcludesDeadBranch(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, _ := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})

	lowID := ids.ID{0x01}  // H-1 sibling with the LOWEST canonical → the winner
	highID := ids.ID{0xff} // H-1 sibling with a higher canonical → the loser
	hChild := ids.ID{0x77} // an H block extending the LOSER

	e.mu.Lock()
	e.pendingBlocks[lowID] = &PendingBlock{ConsensusBlock: &Block{id: lowID, parentID: ids.Empty, height: 1, canonicalID: lowID}, ProposedAt: time.Now()}
	e.pendingBlocks[highID] = &PendingBlock{ConsensusBlock: &Block{id: highID, parentID: ids.Empty, height: 1, canonicalID: highID}, ProposedAt: time.Now()}
	e.pendingBlocks[hChild] = &PendingBlock{ConsensusBlock: &Block{id: hChild, parentID: highID, height: 2, canonicalID: hChild}, ProposedAt: time.Now()}
	loserHigh := e.parentIsProvenLoserLocked(highID)
	winnerLow := e.parentIsProvenLoserLocked(lowID)
	untrackedTip := e.parentIsProvenLoserLocked(ids.Empty)
	e.mu.Unlock()

	if !loserHigh {
		t.Fatal("N1: the higher-canonical H-1 sibling MUST be a proven loser (a lower sibling is tracked)")
	}
	if winnerLow {
		t.Fatal("N1: the lowest-canonical H-1 sibling must NOT be a loser (it is the winner)")
	}
	if untrackedTip {
		t.Fatal("N1: an UNTRACKED parent (finalized tip) must NEVER be filtered — conservative, no over-filter")
	}
}
