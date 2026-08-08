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
// THE TEST. No race is needed to pin the ordering. The VM's Accept runs inside
// applyBranchFinalization, AFTER the hoisted rememberAcceptedPos and BEFORE finalizedByCert is
// set — so the probe stands at a point strictly EARLIER than the window, not inside it. Nothing
// removes an acceptedPos entry in between, so present at the probe implies present in the
// window. That is the whole argument, and it is an ordering argument rather than a direct
// observation; the guard below pins the probe's location so the argument cannot rot.
//
// WHAT THIS DOES NOT FIX. There is a SECOND, far wider window with the same symptom, and it is
// the more likely production cause precisely because it is wider: from the cert freezing in
// assembleCertLocked to finalizedByCert being set — an interval that spans ApplyCert AND the
// entire VM.Accept, i.e. the EVM state write. Throughout it the block is STILL PENDING, so
// handleVote never consults finalizedByCert or acceptedPos at all: the vote lands in
// pending.certVotes, assembleCertLocked returns the already-frozen cert, and
// dropPendingBlockLocked deletes the PendingBlock with the vote still inside it.
//
// The position being present there does not help — nothing on that path looks at it. Measured
// in-window: stillPending=true certFrozen=true certVotes=3 finalizedByCert=false
// acceptedPosKnown=true, and 4 of 5 voting reads back ResponsiveStake=0.60 Degraded=true
// QuasarHeight=0. Re-delivering the identical vote exports it, so the vote is DROPPED, not
// rejected. Closing that one means draining pending.certVotes into the attestor as the
// PendingBlock is dropped, and it is tracked as its own change.
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
	var pendingAtAccept bool
	probe := &probeOnAcceptBlock{mockBlock: mb(finalID, 1)}
	probe.onAccept = func() {
		_, posAtAccept = e.lookupAcceptedPos(finalID)
		e.mu.RLock()
		_, finalizedAtAccept = e.finalizedByCert[finalID]
		_, pendingAtAccept = e.pendingBlocks[finalID]
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
	// WHERE THE PROBE ACTUALLY STANDS. VM.Accept runs BEFORE finalizedByCert is set and while the
	// block is STILL PENDING, so this is a point strictly EARLIER than the window — not inside
	// it. The assertion is still sound, because nothing removes an acceptedPos entry between here
	// and the window opening a few statements later: present here implies present there. But it
	// is proof by ordering, not by direct observation, and the guard has to say so — asserting
	// finalizedAtAccept would simply FAIL, which is exactly why it is worth pinning what the
	// probe really sees rather than what it is convenient to assume.
	if !pendingAtAccept || finalizedAtAccept {
		t.Fatalf("probe location moved: expected the pre-window point (pending, not yet in "+
			"finalizedByCert), got pending=%v finalizedByCert=%v. The ordering argument above "+
			"depends on this probe running BEFORE the window opens", pendingAtAccept, finalizedAtAccept)
	}
	// And the record survives the whole call, so a vote arriving after it is attestable too.
	if _, ok := e.lookupAcceptedPos(finalID); !ok {
		t.Fatal("the accepted position must remain resolvable after the finalize completes")
	}
}

// TestVotesDroppedAtCertFreeze_Counted pins the discriminator. certVotes keeps collecting after
// assembleCertLocked freezes the cert; anything above that freeze is carried by the map alone and
// dies when the PendingBlock is dropped. The counter has to see it, because it is the only thing
// that can tell a live fleet whether the wide window actually costs it attestations — a green
// test suite cannot, and 0/40 under contention bounds the flake rate without identifying which
// window produced it.
func TestVotesDroppedAtCertFreeze_Counted(t *testing.T) {
	e := newTestEngine()
	id := ids.GenerateTestID()

	// A cert frozen at two votes, with a third that arrived after the freeze — the shape a
	// trailing ⅔-th vote takes while the block is still pending.
	frozen := &QuorumCert{
		Version:  QuorumCertVersion,
		Type:     QCFinality,
		Tier:     Nova,
		Position: VotePosition{Height: 1, Round: 0, BlockID: id, ParentID: ids.Empty, CanonicalID: id},
		Votes:    []SignedVote{{Accept: true}, {Accept: true}},
	}
	pb := &PendingBlock{
		ConsensusBlock: &Block{id: id, parentID: ids.Empty, height: 1},
		cert:           frozen,
		certVotes: map[ids.NodeID]SignedVote{
			{1}: {Accept: true},
			{2}: {Accept: true},
			{3}: {Accept: true}, // arrived AFTER the freeze — in the map, not in the cert
		},
	}

	e.mu.Lock()
	e.pendingBlocks[id] = pb
	e.dropPendingBlockLocked(id)
	got := e.votesDroppedAtCertFreeze
	e.mu.Unlock()

	if got != 1 {
		t.Fatalf("the vote that arrived after the cert froze must be COUNTED as it is dropped, "+
			"got %d want 1 — without this the wide window is unobservable in production", got)
	}
	if v, ok := e.Stats()["votes_dropped_at_cert_freeze"]; !ok {
		t.Fatal("the counter must be reachable through Stats() — an unexported counter answers nothing")
	} else if v.(uint64) != 1 {
		t.Fatalf("Stats() must report the counter, got %v want 1", v)
	}
}

// TestVotesDroppedAtCertFreeze_QuietWhenNothingLost: a block whose cert carries every vote it
// collected loses nothing, and must not be counted. A counter that ticks on the healthy path is
// worse than none — it would make the production question unanswerable in the other direction.
func TestVotesDroppedAtCertFreeze_QuietWhenNothingLost(t *testing.T) {
	e := newTestEngine()
	id := ids.GenerateTestID()

	pb := &PendingBlock{
		ConsensusBlock: &Block{id: id, parentID: ids.Empty, height: 1},
		cert: &QuorumCert{
			Version:  QuorumCertVersion,
			Type:     QCFinality,
			Tier:     Nova,
			Position: VotePosition{Height: 1, Round: 0, BlockID: id, ParentID: ids.Empty, CanonicalID: id},
			Votes:    []SignedVote{{Accept: true}, {Accept: true}},
		},
		certVotes: map[ids.NodeID]SignedVote{{1}: {Accept: true}, {2}: {Accept: true}},
	}

	e.mu.Lock()
	e.pendingBlocks[id] = pb
	e.dropPendingBlockLocked(id)
	got := e.votesDroppedAtCertFreeze
	e.mu.Unlock()

	if got != 0 {
		t.Fatalf("nothing was lost — the counter must stay at 0, got %d", got)
	}
}
