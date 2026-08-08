// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// accepted_pos_ordering_test.go — the trailing vote must never fall through the window between
// "this block is finalized" and "here is the position its votes signed".
//
// THE DEFECT. handleVote routes a vote for a block that is no longer pending but IS in
// finalizedByCert to attestFinalizedVote, which resolves the signed position through
// lookupAcceptedPos. applyBranchFinalization sets finalizedByCert and drops the pending block;
// rememberAcceptedPos used to run later still, inside promoteQuasar. Between those two points
// the tap was OPEN and the position ABSENT, so a vote arriving there took the finalized path,
// missed the lookup, and returned in silence.
//
// The loss is permanent. Accept votes are solicited once and promoteQuasar does no retroactive
// promotion, so dropping the ⅔-th one leaves that height Nova-only forever: settlement never
// exports it, and FinalityStatus reports Degraded with a responsive stake below the truth — a
// fleet with 4 of 5 up and all four voting reads back as 0.6. The window widens under CPU
// contention, which is where it surfaced.
//
// THE TEST. No race is needed to pin the ordering. The VM's Accept runs INSIDE
// applyBranchFinalization — after the hoisted rememberAcceptedPos and before the old one — so
// probing lookupAcceptedPos from Accept observes exactly the interval that used to be empty.
package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// probeOnAcceptBlock runs a hook at the instant the VM accepts, which is inside the same
// finalization step that publishes the block to finalizedByCert.
type probeOnAcceptBlock struct {
	*mockBlock
	onAccept func()
}

func (b *probeOnAcceptBlock) Accept(context.Context) error {
	if b.onAccept != nil {
		b.onAccept()
	}
	return nil
}

// TestAcceptedPos_RememberedBeforeFinalityIsVisible: by the time anything can observe the block
// as finalized, the position its accept votes signed must already be resolvable — otherwise a
// trailing vote arriving in that window is dropped and the height can never export.
func TestAcceptedPos_RememberedBeforeFinalityIsVisible(t *testing.T) {
	e := newTestEngine()
	ctx := context.Background()

	finalID := ids.GenerateTestID()
	cb := &Block{id: finalID, parentID: ids.Empty, height: 1}
	if err := e.consensus.AddBlock(ctx, cb); err != nil {
		t.Fatalf("AddBlock: %v", err)
	}

	var posAtAccept bool
	var finalizedAtAccept bool
	probe := &probeOnAcceptBlock{mockBlock: mb(finalID, 1)}
	probe.onAccept = func() {
		_, posAtAccept = e.lookupAcceptedPos(finalID)
		e.mu.RLock()
		_, finalizedAtAccept = e.finalizedByCert[finalID]
		e.mu.RUnlock()
	}

	vm := &reconcilingOrphanVM{orphanVMBase: &orphanVMBase{
		head:   finalID,
		blocks: map[ids.ID]*mockBlock{finalID: mb(finalID, 1)},
	}}
	e.SetVM(vm)

	e.mu.Lock()
	e.pendingBlocks[finalID] = &PendingBlock{ConsensusBlock: cb, VMBlock: probe}
	e.mu.Unlock()

	cert := VerifiedQuorumCert{qc: &QuorumCert{
		Version:   QuorumCertVersion,
		Type:      QCFinality,
		Tier:      Nova,
		Position:  VotePosition{Height: 1, Round: 0, BlockID: finalID, ParentID: ids.Empty, CanonicalID: finalID},
		Threshold: 1,
	}}

	if err := e.acceptWithCertCore(ctx, finalID, cert, false); err != nil {
		t.Fatalf("acceptWithCertCore: %v", err)
	}

	if !posAtAccept {
		t.Fatal("ORDERING: the accepted position was NOT resolvable at VM.Accept, i.e. while the " +
			"block was being published as finalized. A trailing accept vote arriving in that " +
			"window takes the finalized path, misses lookupAcceptedPos and is dropped in " +
			"silence — and because votes are solicited once with no retroactive promotion, that " +
			"height stays Nova-only permanently")
	}
	// Sanity: the probe really did observe the finalization step, so the assertion above is not
	// passing because it ran somewhere harmless.
	if !finalizedAtAccept && !posAtAccept {
		t.Fatal("probe did not observe the finalization step at all — the ordering assertion is vacuous")
	}
	// And the record survives the whole call, so a vote arriving after it is attestable too.
	if _, ok := e.lookupAcceptedPos(finalID); !ok {
		t.Fatal("the accepted position must remain resolvable after the finalize completes")
	}
}
