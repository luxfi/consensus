// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// cert_canonical_aggregation_test.go — FIX #3 (v1.35.29): cert termination via
// per-CANONICAL vote aggregation (the block-288 wrapper-split stall).
//
// THE STALL: a vote is signed over the CANONICAL execution identity, not the outer
// proposervm envelope (canonicalVoteMessageFor), so two validators that executed the
// SAME inner block under DIFFERENT outer wrappers produce byte-identical vote messages.
// But pendingBlocks is OUTER-keyed, so their votes land in DIFFERENT pending blocks —
// and the old assembleCertLocked collected only ONE wrapper's certVotes, splitting the
// quorum: α validators had signed the same inner block, yet no cert ever assembled and
// the height stalled forever. The SIGNING aliased correctly; the AGGREGATION did not.
//
// THE FIX: assembleCertLocked collects verified votes from ALL tracked sibling wrappers
// that share the winner's canonical identity, de-duped by NodeID, and verifies them
// against the one canonical message.
package chain

import (
	"bytes"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// wrapperOf builds a consensus Block that is one OUTER proposervm envelope over a shared
// INNER execution block: distinct outer id/parent, identical canonical fields. Two of
// these are aliases of one inner block, exactly the proposervm re-wrap the fix collapses.
func wrapperOf(inner, parentCanon, execRoot, payloadRoot ids.ID, height uint64) *Block {
	return &Block{
		id:                ids.GenerateTestID(), // distinct OUTER envelope id
		parentID:          ids.GenerateTestID(), // distinct OUTER parent envelope
		height:            height,
		canonicalID:       inner,       // SHARED inner execution commitment
		parentCanonicalID: parentCanon, // SHARED inner parent
		execStateRoot:     execRoot,
		payloadRoot:       payloadRoot,
	}
}

// TestFix3_WrapperSplit_CertAssemblesAcrossOuterEnvelopes is the fail-without/pass-with.
// Two validators sign the SAME inner block under DIFFERENT outer wrappers; each wrapper's
// certVotes holds only ONE vote (below α=2). With per-outer aggregation the cert never
// assembles (the stall). With per-canonical aggregation it MUST assemble from both.
func TestFix3_WrapperSplit_CertAssemblesAcrossOuterEnvelopes(t *testing.T) {
	vs := newTestValidatorSet(3)
	e, _ := newQuorumEngine(t, config.LocalParams(), vs, 0, &recordingGossiper{}) // K=3, α=2
	if a := e.consensus.Alpha(); a != 2 {
		t.Fatalf("precondition: this test needs α=2, got α=%d", a)
	}

	inner := ids.GenerateTestID()
	parentCanon := ids.GenerateTestID()
	execRoot := ids.GenerateTestID()
	payloadRoot := ids.GenerateTestID()
	const height = uint64(288) // the live wrapper-split height

	wa := wrapperOf(inner, parentCanon, execRoot, payloadRoot, height)
	wb := wrapperOf(inner, parentCanon, execRoot, payloadRoot, height)
	if wa.id == wb.id {
		t.Fatal("wrappers must have DISTINCT outer ids")
	}
	if wa.canonicalRep() != wb.canonicalRep() {
		t.Fatal("wrappers must share the SAME canonical identity (they wrap one inner block)")
	}

	e.mu.Lock()
	pa := &PendingBlock{ConsensusBlock: wa, ProposedAt: time.Now()}
	pb := &PendingBlock{ConsensusBlock: wb, ProposedAt: time.Now()}
	e.pendingBlocks[wa.id] = pa
	e.pendingBlocks[wb.id] = pb

	posA := e.blockPositionLocked(pa, wa.id)
	posB := e.blockPositionLocked(pb, wb.id)
	e.mu.Unlock()

	// The aliasing precondition: the two wrappers produce BYTE-IDENTICAL vote messages
	// (the message binds the canonical id, not the outer envelope), so a signature over
	// one verifies against the other.
	if !bytes.Equal(CanonicalVoteMessage(posA), CanonicalVoteMessage(posB)) {
		t.Fatal("two wrappers of one inner block MUST sign byte-identical vote messages")
	}

	// Validator 0 signs wrapper A; validator 1 signs wrapper B. Each vote lands in a
	// DIFFERENT outer-keyed pending block.
	e.mu.Lock()
	e.recordCertVoteLocked(pa, Vote{BlockID: wa.id, NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, posA)})
	e.recordCertVoteLocked(pb, Vote{BlockID: wb.id, NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, posB)})

	// FAIL-WITHOUT precondition: neither wrapper alone holds α=2 votes.
	if len(pa.certVotes) != 1 || len(pb.certVotes) != 1 {
		e.mu.Unlock()
		t.Fatalf("each wrapper must hold exactly ONE vote (A=%d, B=%d) — else the test proves nothing",
			len(pa.certVotes), len(pb.certVotes))
	}

	// PASS-WITH: assembleCertLocked aggregates across the sibling wrappers → a cert forms.
	certA := e.assembleCertLocked(pa, wa.id)
	certB := e.assembleCertLocked(pb, wb.id)
	e.mu.Unlock()

	if certA == nil {
		t.Fatal("WRAPPER-SPLIT STALL: no cert assembled from wrapper A — the two per-wrapper " +
			"votes for the same inner block failed to aggregate (α=2 unreachable per-outer)")
	}
	if len(certA.Votes) != 2 {
		t.Fatalf("cert must aggregate BOTH validators' votes across wrappers, got %d voters", len(certA.Votes))
	}
	if certB == nil || len(certB.Votes) != 2 {
		t.Fatal("aggregation must be symmetric: wrapper B must also assemble the 2-vote cert")
	}
	// The assembled cert must VERIFY (the signatures are over the shared canonical message).
	if err := certA.Verify(vs, 0); err != nil {
		t.Fatalf("aggregated cert must verify: %v", err)
	}
}

// TestFix3_PendingByCanonical_ResolvesInnerToOuter proves the inner→outer resolve the
// durable clone-recovery re-sign needs: committedSlot stores the INNER canonical id, but
// pendingBlocks is OUTER-keyed. A direct pendingBlocks[inner] lookup MISSES for a wrapped
// block (inner ≠ outer) — the v4 fallback's silent no-op on real wrappers. The resolver
// must find the tracked outer wrapper by its canonical identity.
func TestFix3_PendingByCanonical_ResolvesInnerToOuter(t *testing.T) {
	vs := newTestValidatorSet(3)
	e, _ := newQuorumEngine(t, config.LocalParams(), vs, 0, &recordingGossiper{})

	inner := ids.GenerateTestID()
	w := wrapperOf(inner, ids.GenerateTestID(), ids.GenerateTestID(), ids.GenerateTestID(), 42)
	if w.id == inner {
		t.Fatal("precondition: a wrapped block has outer id ≠ inner canonical")
	}

	e.mu.Lock()
	e.pendingBlocks[w.id] = &PendingBlock{ConsensusBlock: w, ProposedAt: time.Now()}

	// The old direct lookup on the INNER id misses (outer-keyed map).
	_, directHit := e.pendingBlocks[inner]
	// The resolver finds the tracked OUTER wrapper by canonical identity.
	got, gotID := e.pendingByCanonicalLocked(inner)
	e.mu.Unlock()

	if directHit {
		t.Fatal("precondition: pendingBlocks[inner] must MISS for a wrapped block")
	}
	if got == nil || gotID != w.id {
		t.Fatalf("pendingByCanonicalLocked(inner) must resolve to the outer wrapper %s, got id=%s (nil=%v)",
			w.id, gotID, got == nil)
	}

	// A decided or absent canonical resolves to nothing.
	e.mu.Lock()
	e.pendingBlocks[w.id].Decided = true
	gotDecided, _ := e.pendingByCanonicalLocked(inner)
	gotAbsent, _ := e.pendingByCanonicalLocked(ids.GenerateTestID())
	e.mu.Unlock()
	if gotDecided != nil {
		t.Fatal("a DECIDED wrapper must not be returned (its height is settled)")
	}
	if gotAbsent != nil {
		t.Fatal("an untracked canonical must resolve to nil")
	}
}
