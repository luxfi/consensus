// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// vote_safety_invariants_test.go — the ADVERSARIAL SAFETY GATES for the signed-vote
// tally and the anti-equivocation guard.
//
// These tests do not change behaviour. They PIN the six invariants the whole
// finality argument rests on, each of which was reached for (and could not be
// cheaply proven) during the 2026-07-31 stall investigations:
//
//	1 TestSafety_UnsignedVoteIsNeverTallied        — an unsigned Chits-derived vote
//	  moves NO tally and α is unreachable from them, however many arrive. Positive
//	  control: the identical vote, from the identical validator, at the identical
//	  position, WITH a signature, counts and finalizes.
//	2 TestSafety_GuardIdempotency_SameCanonical    — re-offering the SAME canonical at
//	  a bound height is true forever, never rewrites the binding, never re-persists.
//	3 TestSafety_ConflictingBinding_RefusedAndStatePreserved — a DIFFERENT canonical at
//	  a bound height is refused, and the refusal leaves memory AND disk byte-identical.
//	4 TestSafety_NoBindingSilentlyDropped          — refusals drop nothing; a Quasar
//	  compaction keeps every binding AT OR ABOVE the certified floor.
//	5 TestSafety_FailClosedPersist_MemoryNeverAheadOfDisk — a failed durable write means
//	  no signature AND a rolled-back map: memory never claims a binding disk lacks.
//	6 TestSafety_SlotKeyIsHeightOnly               — two candidates at one height with
//	  DIFFERENT epochs and DIFFERENT validator-set roots still collide in ONE slot.
//
// The theme is the same in all six: the guard is a VALUE (a durable log of what this
// node already signed), never a lock and never a lease. Nothing in this file clears,
// deletes, or releases a binding — a timeout is not proof that no certificate formed,
// and releasing a binding is the equivocation door.
package chain

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// -----------------------------------------------------------------------------
// probes — read-only views of every counter a vote or a binding can move.
// -----------------------------------------------------------------------------

// voteTally is the COMPLETE set of counters an accept vote can advance. A vote that
// is "not tallied" must leave every one of them untouched: there is no partial
// credit and no side channel by which an unauthenticated vote reaches finality.
type voteTally struct {
	tracked     bool  // still in pendingBlocks (a finalized block leaves the map)
	voteCount   int   // PendingBlock.VoteCount   — the raw accept count
	certVotes   int   // PendingBlock.certVotes   — the cert witness set
	acceptVotes int   // ChainConsensus.Block.acceptVotes — the α predicate's input
	rejectVotes int   // ChainConsensus.Block.rejectVotes
	accepted    bool  // ChainConsensus.Block.accepted (set at acceptVotes >= α)
	vmAccepts   int64 // VM.Accept invocations — the irreversible side effect
}

// readVoteTally snapshots every tally for blk. Takes each owner's own lock; never
// holds two at once (t.mu -> c.mu is the only legal order, and this takes neither
// while holding the other).
func readVoteTally(e *Transitive, blk *verifyOnceBlock) voteTally {
	var v voteTally
	e.mu.RLock()
	if pb, ok := e.pendingBlocks[blk.id]; ok {
		v.tracked = true
		v.voteCount = pb.VoteCount
		v.certVotes = len(pb.certVotes)
	}
	e.mu.RUnlock()

	e.consensus.mu.RLock()
	if cb, ok := e.consensus.blocks[blk.id]; ok {
		v.acceptVotes = cb.acceptVotes()
		v.rejectVotes = cb.rejectVotes()
		v.accepted = cb.accepted
	}
	e.consensus.mu.RUnlock()

	v.vmAccepts = blk.AcceptCalled()
	return v
}

// trackFollowedBlock inserts blk as a tracked, verified, NON-own pending block —
// what followVerifiedBlock establishes for a gossiped block. Deliberately records NO
// self-vote, so every counter starts at zero and the only accepts are the ones a test
// feeds. Returns the position votes must bind to (matching blockPositionLocked for an
// engine with no set-root source).
func trackFollowedBlock(e *Transitive, chainID ids.ID, blk *verifyOnceBlock) VotePosition {
	cb := &Block{
		id:        blk.id,
		parentID:  blk.parentID,
		height:    blk.height,
		timestamp: blk.timestamp.Unix(),
		data:      blk.bytes,
	}
	_ = e.consensus.AddBlock(context.Background(), cb)
	e.mu.Lock()
	e.pendingBlocks[blk.id] = &PendingBlock{
		ConsensusBlock: cb,
		VMBlock:        blk,
		ProposedAt:     time.Now(),
		Round:          0,
	}
	e.mu.Unlock()
	return VotePosition{
		ChainID:  chainID,
		Height:   blk.height,
		Round:    0,
		BlockID:  blk.id,
		ParentID: blk.parentID,
	}
}

// memBindings copies the engine's live binding set and floor under slotMu.
func memBindings(e *Transitive) (map[SlotKey]ids.ID, uint64) {
	e.slotMu.Lock()
	defer e.slotMu.Unlock()
	out := make(map[SlotKey]ids.ID, len(e.committedSlot))
	for k, v := range e.committedSlot {
		out[k] = v
	}
	return out, e.decidedFloor
}

// diskBindings reads what is ACTUALLY on stable storage by reopening the file. The
// live store's Snapshot() is the OPEN-TIME view (Persist never updates it), so a fresh
// open is the only honest probe of disk — the same discipline
// TestQuasarFloor_PromotionAdvancesFloorAtomically uses.
func diskBindings(t *testing.T, path string) (map[SlotKey]ids.ID, uint64) {
	t.Helper()
	s, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("reopen vote-guard %s: %v", path, err)
	}
	return s.Snapshot(), s.FinalizedThrough()
}

// sameBindings reports exact set equality (keys AND canonicals).
func sameBindings(a, b map[SlotKey]ids.ID) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// -----------------------------------------------------------------------------
// countingGuard — a real durable store with a WRITE COUNTER and an arm-able failure.
//
// The counter is what proves the idempotent branch does NOT re-persist and that a
// refusal does NOT write at all: "the binding is unchanged" and "no write was even
// attempted" are different claims, and only the second rules out a torn rewrite.
// Guarded by its own mutex so the store is safe under -race even though every
// production writer already holds slotMu.
// -----------------------------------------------------------------------------

type countingGuard struct {
	mu         sync.Mutex
	inner      VoteGuardStore
	persists   int
	reconciles int
	failWith   error // non-nil ⇒ every durable write fails (models EIO / full disk)
}

func newCountingGuard(t *testing.T, path string) *countingGuard {
	t.Helper()
	inner, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(%s): %v", path, err)
	}
	return &countingGuard{inner: inner}
}

func (g *countingGuard) Persist(bindings map[SlotKey]ids.ID, floor uint64) error {
	g.mu.Lock()
	g.persists++
	fail := g.failWith
	g.mu.Unlock()
	if fail != nil {
		return fail
	}
	return g.inner.Persist(bindings, floor)
}

func (g *countingGuard) Snapshot() map[SlotKey]ids.ID { return g.inner.Snapshot() }
func (g *countingGuard) FinalizedThrough() uint64     { return g.inner.FinalizedThrough() }
func (g *countingGuard) Close() error                 { return g.inner.Close() }

func (g *countingGuard) writes() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.persists + g.reconciles
}

func (g *countingGuard) setFailure(err error) {
	g.mu.Lock()
	g.failWith = err
	g.mu.Unlock()
}

// -----------------------------------------------------------------------------
// 1. UNSIGNED VOTES ARE NEVER TALLIED
// -----------------------------------------------------------------------------

// TestSafety_UnsignedVoteIsNeverTallied is the type-level claim the incident turned on,
// proven behaviourally on a K>1 production configuration (K=5, α=4).
//
// The live measurement was 807/807 votes UNSIGNED and 0 finalizations in 6h. Those
// votes are Chits-derived: `Chits` has no signature field at all, so the node layer
// synthesizes an accept bit from a LOCAL re-Verify and hands the engine a Vote with an
// empty Signature (node/chains/manager.go builds it, engine.go's handleVote drops it).
// This test proves the drop is total — MORE unsigned accepts than α move NOTHING — and
// that the ONLY difference between "nothing happens" and "the chain finalizes" is the
// signature. If this test ever fails, an unauthenticated peer message can reach the α
// predicate and finality is forgeable.
func TestSafety_UnsignedVoteIsNeverTallied(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{})

	// PRECONDITION: this is a genuine multi-validator configuration. On K==1 the
	// verifier is bypassed by design (the sole validator's self-vote is the quorum),
	// so proving the gate on K==1 would prove nothing.
	if k := e.consensus.K(); k <= 1 {
		t.Fatalf("precondition: the drop must be proven on a K>1 chain, got K=%d", k)
	}
	alpha := e.consensus.Alpha()
	if alpha < 2 {
		t.Fatalf("precondition: α must be a real quorum, got %d", alpha)
	}
	if e.voteVerifier == nil {
		t.Fatal("precondition: a K>1 engine must carry a vote verifier (Start fail-closes without one)")
	}

	blk := newTestBlock(11367, ids.GenerateTestID(), "unsigned-vs-signed")
	pos := trackFollowedBlock(e, chainID, blk)

	if base := readVoteTally(e, blk); base.voteCount != 0 || base.certVotes != 0 ||
		base.acceptVotes != 0 || base.accepted || base.vmAccepts != 0 {
		t.Fatalf("precondition: a followed (non-own) block must start with every tally at zero, got %+v", base)
	}
	before := e.Stats()["votes_received"].(uint64)

	// --- THE ATTACK: one UNSIGNED accept from EVERY validator. Five accepts is
	// strictly MORE than α=4, so if any of them reached a tally the block would be
	// accepted. This is exactly the fleet-wide shape the incident measured.
	for i := 0; i < 5; i++ {
		e.handleVote(Vote{
			BlockID:  blk.id,
			NodeID:   vs.nodeID(i),
			Accept:   true,
			SignedAt: time.Now(),
			ParentID: pos.ParentID,
			Round:    pos.Round,
		})
	}

	got := readVoteTally(e, blk)
	if got.voteCount != 0 || got.certVotes != 0 || got.acceptVotes != 0 || got.rejectVotes != 0 {
		t.Fatalf("UNSIGNED VOTE TALLIED: 5 unauthenticated accepts moved a counter — %+v. A Chits "+
			"carries no signature, so the peer never attested anything; counting it makes finality "+
			"forgeable by any peer that can send a 32-byte preference.", got)
	}
	if got.accepted || e.consensus.IsAccepted(blk.id) || e.IsAccepted(blk.id) {
		t.Fatalf("UNSIGNED QUORUM: the block reports accepted on %d unsigned votes (α=%d)", 5, alpha)
	}
	if got.vmAccepts != 0 {
		t.Fatalf("UNSIGNED FINALITY: VM.Accept ran %d× with zero authenticated votes", got.vmAccepts)
	}
	// α is unreachable BY CONSTRUCTION, not merely un-reached: the α predicate reads
	// acceptVotes, and unsigned votes never increment it, so no arrival count can ever
	// satisfy it.
	if got.acceptVotes >= alpha {
		t.Fatalf("α REACHED FROM UNSIGNED VOTES: acceptVotes=%d >= α=%d", got.acceptVotes, alpha)
	}
	// The cert layer agrees: with no signed witnesses there is no cert to verify, so the
	// SOLE finalizer cannot run.
	if err := e.TryAccept(context.Background(), blk.id); !errors.Is(err, ErrNoVerifiedQC) {
		t.Fatalf("TryAccept on 5 unsigned votes must answer ErrNoVerifiedQC (no witness ⇒ no cert), got %v", err)
	}

	// --- THE OBSERVABILITY TRAP, pinned. votes_received is an ARRIVAL counter: it
	// advances for every vote that reaches handleVote, including the ones dropped at
	// the signature gate. That is why the fleet's dashboards read healthy through 6h of
	// zero finality. Assert the semantics explicitly so nobody re-derives it from a
	// grafana panel at 3am — and so narrowing it to a tally counter is a deliberate,
	// reviewed change (update the operator runbook with it).
	if after := e.Stats()["votes_received"].(uint64); after != before+5 {
		t.Fatalf("votes_received is an ARRIVAL counter and must advance for dropped votes too: "+
			"%d -> %d, want +5. If this was narrowed to count only TALLIED votes on purpose, that "+
			"changes what every operator dashboard means — update this test and the runbook together.",
			before, after)
	}

	// --- SECOND SHAPE: a real validator's real signature, LIFTED from a different
	// position. Not "unsigned" but equally unauthenticated for THIS block; it must die
	// at the same gate. (An empty signature is the live case; a lifted one is the
	// adversarial case, and the gate must not distinguish.)
	elsewhere := VotePosition{ChainID: chainID, Height: blk.height, Round: 0,
		BlockID: ids.GenerateTestID(), ParentID: pos.ParentID}
	e.handleVote(Vote{
		BlockID:   blk.id,
		NodeID:    vs.nodeID(1),
		Accept:    true,
		SignedAt:  time.Now(),
		Signature: vs.sign(1, elsewhere),
		ParentID:  pos.ParentID,
		Round:     pos.Round,
	})
	if got = readVoteTally(e, blk); got.voteCount != 0 || got.certVotes != 0 || got.acceptVotes != 0 {
		t.Fatalf("REPLAYED SIGNATURE TALLIED: a signature over a DIFFERENT position counted for this "+
			"block — %+v. The canonical message binds the position, so it must not verify here.", got)
	}

	// --- DEFENCE IN DEPTH: even called directly, the cert recorder refuses an unsigned
	// vote. handleVote's gate is the first line; this is the second, and it must survive
	// any future refactor of the first.
	e.mu.Lock()
	pb := e.pendingBlocks[blk.id]
	e.recordCertVoteLocked(pb, Vote{BlockID: blk.id, NodeID: vs.nodeID(2), Accept: true})
	certVotes := len(pb.certVotes)
	e.mu.Unlock()
	if certVotes != 0 {
		t.Fatalf("recordCertVoteLocked admitted an UNSIGNED vote into the cert witness set (%d entries) — "+
			"a cert must be assemblable only from signatures", certVotes)
	}

	// --- POSITIVE CONTROL. The SAME validator, the SAME block, the SAME position, the
	// SAME accept bit — the ONLY difference is a valid signature. It must count.
	signed := vs.signedVote(0, pos)
	if len(signed.Signature) == 0 {
		t.Fatal("harness precondition: the positive control must actually carry a signature")
	}
	e.handleVote(signed)
	got = readVoteTally(e, blk)
	if got.voteCount != 1 || got.certVotes != 1 || got.acceptVotes != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: one SIGNED accept must advance every tally by exactly 1, got %+v. "+
			"Without this the negative half proves nothing — it would be consistent with a dead engine.", got)
	}

	// …and enough signed accepts DO finalize, so the engine is fully live on the exact
	// path the unsigned votes could not touch.
	for i := 1; i < 5; i++ {
		e.handleVote(vs.signedVote(i, pos))
	}
	if !waitFor(2*time.Second, func() bool { return blk.AcceptCalled() >= 1 }) {
		t.Fatalf("POSITIVE CONTROL FAILED: 5 SIGNED accepts (α=%d) did not finalize the block "+
			"(VM.Accept=%d, tally=%+v)", alpha, blk.AcceptCalled(), readVoteTally(e, blk))
	}
	if !e.consensus.IsAccepted(blk.id) {
		t.Fatal("POSITIVE CONTROL FAILED: the block finalized but consensus does not report it accepted")
	}
}

// -----------------------------------------------------------------------------
// 2. GUARD IDEMPOTENCY
// -----------------------------------------------------------------------------

// TestSafety_GuardIdempotency_SameCanonical pins the property that makes a restarted
// validator able to RE-ISSUE its outstanding vote: re-offering the canonical it already
// bound is permitted forever, changes nothing, and costs no durable write.
//
// This is what lets a mass-restarted fleet converge without any binding ever being
// released. If the idempotent branch ever returned false, a node that signed at H and
// then restarted could never re-broadcast that vote, and the height could never gather
// α — the permanent halt, arrived at from the opposite direction.
func TestSafety_GuardIdempotency_SameCanonical(t *testing.T) {
	vs := newTestValidatorSet(5)
	path := filepath.Join(t.TempDir(), "vote-guard")
	guard := newCountingGuard(t, path)
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(guard))

	const H = uint64(1098192) // the real mainnet halt height
	A := ids.GenerateTestID()

	if !e.reserveSlotForSign(H, A) {
		t.Fatal("the first binding at an unbound height above the floor must be permitted")
	}
	writesAfterFirst := guard.writes()
	if writesAfterFirst != 1 {
		t.Fatalf("the first binding must cost EXACTLY one durable write (fsync before the signature), got %d",
			writesAfterFirst)
	}
	memAfterFirst, floorAfterFirst := memBindings(e)
	diskAfterFirst, diskFloorAfterFirst := diskBindings(t, path)
	if !sameBindings(memAfterFirst, diskAfterFirst) {
		t.Fatalf("after a successful bind, memory and disk must agree: mem=%v disk=%v", memAfterFirst, diskAfterFirst)
	}

	// Re-offer the SAME canonical many times — a re-solicit storm, a restart replay, a
	// re-gossip of the same block under a new envelope: all legitimate, all idempotent.
	for i := 0; i < 256; i++ {
		if !e.reserveSlotForSign(H, A) {
			t.Fatalf("re-offer #%d of the SAME canonical at a bound height was REFUSED — a node that "+
				"already signed A@%d must be able to re-issue that exact vote, or a restarted fleet "+
				"can never complete the cert it was one vote short of", i, H)
		}
	}

	// Nothing moved: not the binding, not the floor, not the file.
	memNow, floorNow := memBindings(e)
	if !sameBindings(memNow, memAfterFirst) {
		t.Fatalf("an idempotent re-offer MUTATED the binding set: before=%v after=%v", memAfterFirst, memNow)
	}
	if floorNow != floorAfterFirst {
		t.Fatalf("an idempotent re-offer moved the floor: %d -> %d", floorAfterFirst, floorNow)
	}
	if got := guard.writes(); got != writesAfterFirst {
		t.Fatalf("256 idempotent re-offers issued %d extra durable write(s) — the already-durable branch "+
			"must return BEFORE Persist; rewriting the snapshot on every re-solicit is unnecessary fsync "+
			"traffic AND a needless torn-write window", got-writesAfterFirst)
	}
	diskNow, diskFloorNow := diskBindings(t, path)
	if !sameBindings(diskNow, diskAfterFirst) || diskFloorNow != diskFloorAfterFirst {
		t.Fatalf("an idempotent re-offer changed DISK: bindings %v -> %v, floor %d -> %d",
			diskAfterFirst, diskNow, diskFloorAfterFirst, diskFloorNow)
	}

	// The public read-side accessors agree with the guard's own view.
	if !e.hasSignedHeight(H) {
		t.Fatalf("hasSignedHeight(%d) must report true once the height is bound", H)
	}
	if bound, ok := e.committedCanonical(H); !ok || bound != A {
		t.Fatalf("committedCanonical(%d) = (%s,%v), want (%s,true)", H, bound, ok, A)
	}
}

// -----------------------------------------------------------------------------
// 3. CONFLICTING BINDING IS REFUSED — AND THE REFUSAL CORRUPTS NOTHING
// -----------------------------------------------------------------------------

// TestSafety_ConflictingBinding_RefusedAndStatePreserved is the anti-equivocation core.
//
// Refusing is only half the property. The half that is never tested — and the half a
// bug would hide in — is that the refusal is a pure read: it must not rewrite the
// binding, must not evict it, must not touch neighbouring heights, and must not issue a
// durable write at all. A refusal that corrupted the slot would convert "this node
// declines to equivocate" into "this node forgot what it signed", which is the
// equivocation door with extra steps.
func TestSafety_ConflictingBinding_RefusedAndStatePreserved(t *testing.T) {
	vs := newTestValidatorSet(5)
	path := filepath.Join(t.TempDir(), "vote-guard")
	guard := newCountingGuard(t, path)
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(guard))

	const H = uint64(4242)
	A := ids.GenerateTestID()
	below := ids.GenerateTestID() // neighbour at H-1
	above := ids.GenerateTestID() // neighbour at H+1

	for _, bind := range []struct {
		h uint64
		c ids.ID
	}{{H - 1, below}, {H, A}, {H + 1, above}} {
		if !e.reserveSlotForSign(bind.h, bind.c) {
			t.Fatalf("precondition: binding height %d must be permitted", bind.h)
		}
	}
	memBefore, floorBefore := memBindings(e)
	diskBefore, diskFloorBefore := diskBindings(t, path)
	writesBefore := guard.writes()

	// A storm of DIFFERENT canonicals at the bound height — a proposer replaced, a
	// rebuild with a new envelope id, a Byzantine sibling. Every one must be refused,
	// and the state after each must be identical to the state before.
	for i := 0; i < 64; i++ {
		B := ids.GenerateTestID()
		if e.reserveSlotForSign(H, B) {
			t.Fatalf("EQUIVOCATION: a CONFLICTING canonical at already-bound height %d was admitted "+
				"(bound=%s offered=%s). One signature per height is what makes two conflicting certs "+
				"at one height impossible.", H, A, B)
		}
		memNow, floorNow := memBindings(e)
		if !sameBindings(memNow, memBefore) {
			t.Fatalf("refusal #%d MUTATED the binding set: before=%v after=%v — a refusal must be a "+
				"pure read; losing the binding is how a node forgets it already signed", i, memBefore, memNow)
		}
		if floorNow != floorBefore {
			t.Fatalf("refusal #%d moved the floor: %d -> %d", i, floorBefore, floorNow)
		}
	}

	// The refusal path never reaches Persist — proven by the write counter, not just by
	// the resulting bytes (identical bytes cannot rule out a rewrite).
	if got := guard.writes(); got != writesBefore {
		t.Fatalf("64 refusals issued %d durable write(s); a refusal must not write at all", got-writesBefore)
	}
	diskAfter, diskFloorAfter := diskBindings(t, path)
	if !sameBindings(diskAfter, diskBefore) || diskFloorAfter != diskFloorBefore {
		t.Fatalf("refusals changed DISK: bindings %v -> %v, floor %d -> %d",
			diskBefore, diskAfter, diskFloorBefore, diskFloorAfter)
	}

	// The ORIGINAL binding is intact and still authoritative in both directions.
	if bound, ok := e.committedCanonical(H); !ok || bound != A {
		t.Fatalf("the original binding at %d was lost or changed: got (%s,%v), want (%s,true)", H, bound, ok, A)
	}
	if !e.reserveSlotForSign(H, A) {
		t.Fatalf("after 64 refusals the ORIGINAL canonical must still be idempotently accepted at %d — "+
			"the refusals poisoned the slot", H)
	}
	// And the neighbours were never collateral damage: each height is an independent slot.
	for _, n := range []struct {
		h uint64
		c ids.ID
	}{{H - 1, below}, {H + 1, above}} {
		if bound, ok := e.committedCanonical(n.h); !ok || bound != n.c {
			t.Fatalf("neighbouring height %d was disturbed by refusals at %d: got (%s,%v), want (%s,true)",
				n.h, H, bound, ok, n.c)
		}
	}
}

// -----------------------------------------------------------------------------
// 4. NO BINDING IS EVER SILENTLY DROPPED
// -----------------------------------------------------------------------------

// TestSafety_NoBindingSilentlyDropped covers the two ways a binding could vanish:
// a refusal (proven above to be a pure read — re-asserted here across a whole set) and
// a Quasar compaction.
//
// Compaction is the ONLY sanctioned removal, and it is sanctioned precisely because a
// ⅔-by-stake certificate makes those heights irreversibly decided — the durable floor
// then refuses them, so nothing is actually forgotten. The line that must never move is
// STRICTLY BELOW: a binding AT the certified floor, and every binding above it, is the
// window where a restarted node still needs its own vote memory. An inclusive prune here
// is the prune-then-resign fork.
func TestSafety_NoBindingSilentlyDropped(t *testing.T) {
	vs := newTestValidatorSet(5)
	path := filepath.Join(t.TempDir(), "vote-guard")
	guard := newCountingGuard(t, path)
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(guard))

	const Q = uint64(1000)
	heights := []uint64{Q - 100, Q - 1, Q, Q + 1, Q + 50}
	bound := make(map[uint64]ids.ID, len(heights))
	for _, h := range heights {
		c := ids.GenerateTestID()
		if !e.reserveSlotForSign(h, c) {
			t.Fatalf("precondition: binding height %d must be permitted", h)
		}
		bound[h] = c
	}

	// (a) REFUSALS DROP NOTHING — at every bound height at once.
	memBefore, _ := memBindings(e)
	for _, h := range heights {
		if e.reserveSlotForSign(h, ids.GenerateTestID()) {
			t.Fatalf("EQUIVOCATION: a conflicting canonical at bound height %d was admitted", h)
		}
	}
	memAfterRefusals, _ := memBindings(e)
	if !sameBindings(memAfterRefusals, memBefore) {
		t.Fatalf("a refusal dropped a binding: before=%v after=%v", memBefore, memAfterRefusals)
	}
	if len(memAfterRefusals) != len(heights) {
		t.Fatalf("binding count changed across refusals: %d -> %d", len(heights), len(memAfterRefusals))
	}

	// (b) COMPACTION KEEPS EVERYTHING AT OR ABOVE THE CERTIFIED FLOOR.
	if err := e.compactVoteGuardThroughQuasar(Q); err != nil {
		t.Fatalf("compactVoteGuardThroughQuasar(%d): %v", Q, err)
	}
	memAfterCompact, floorAfterCompact := memBindings(e)
	if floorAfterCompact != Q {
		t.Fatalf("a ⅔-certified height must advance the in-memory floor to %d, got %d", Q, floorAfterCompact)
	}
	for _, h := range []uint64{Q, Q + 1, Q + 50} {
		if got, ok := memAfterCompact[SlotKey{Height: h}]; !ok || got != bound[h] {
			t.Fatalf("binding at %d is AT OR ABOVE the certified floor %d and MUST survive compaction "+
				"(got %s,%v want %s). The prune is STRICTLY below: deleting the just-certified height's "+
				"own slot is the prune-then-resign fork.", h, Q, got, ok, bound[h])
		}
	}
	for _, h := range []uint64{Q - 100, Q - 1} {
		if _, ok := memAfterCompact[SlotKey{Height: h}]; ok {
			t.Fatalf("binding at %d is strictly below the certified floor %d and should have been "+
				"compacted away (the durable floor covers it)", h, Q)
		}
	}
	// The same view landed on disk, atomically with the floor.
	diskAfterCompact, diskFloor := diskBindings(t, path)
	if diskFloor != Q {
		t.Fatalf("the certified floor did not reach disk: %d want %d", diskFloor, Q)
	}
	if !sameBindings(diskAfterCompact, memAfterCompact) {
		t.Fatalf("memory and disk disagree after compaction: mem=%v disk=%v", memAfterCompact, diskAfterCompact)
	}

	// (c) COMPACTION IS IDEMPOTENT AND MONOTONIC. Re-running it at the same height, or
	// at a LOWER one (a stale attestation, an out-of-order promotion), must never drop a
	// surviving binding and must never lower the floor — the floor is the one value this
	// node can never take back.
	if err := e.compactVoteGuardThroughQuasar(Q); err != nil {
		t.Fatalf("re-compaction at the same height must succeed: %v", err)
	}
	if err := e.compactVoteGuardThroughQuasar(Q - 500); err != nil {
		t.Fatalf("compaction at a LOWER height must not error: %v", err)
	}
	memFinal, floorFinal := memBindings(e)
	if floorFinal != Q {
		t.Fatalf("the durable floor is MONOTONIC: a stale/lower compaction moved it %d -> %d", Q, floorFinal)
	}
	if !sameBindings(memFinal, memAfterCompact) {
		t.Fatalf("a repeated/stale compaction dropped a surviving binding: %v -> %v",
			memAfterCompact, memFinal)
	}

	// (d) AND THE SURVIVORS STILL GUARD. Above the floor, one signature per height is
	// still enforced individually — that is the whole reason those bindings are kept.
	for _, h := range []uint64{Q, Q + 1, Q + 50} {
		if e.reserveSlotForSign(h, ids.GenerateTestID()) {
			t.Fatalf("a surviving binding at %d stopped refusing conflicts after compaction", h)
		}
	}
}

// -----------------------------------------------------------------------------
// 5. FAIL-CLOSED PERSIST
// -----------------------------------------------------------------------------

// TestSafety_FailClosedPersist_MemoryNeverAheadOfDisk proves the invariant that makes
// the guard trustworthy across a crash: THE IN-MEMORY BINDING SET IS NEVER A SUPERSET OF
// WHAT DISK HOLDS.
//
// If memory could hold a binding disk lacks, a crash right after the signature would
// come back with no memory of a vote this node actually cast — and the node would then
// happily sign a conflicting sibling at that height. So a failed durable write must
// produce BOTH: no signature (return false) and no residual in-memory claim.
//
// TestBlue_VoteGuard_PersistFailure_FailsClosed proves the return value and the single
// rolled-back key. This adds the part that matters operationally: the failure is
// SURVIVABLE and CONTAINED — pre-existing bindings are untouched, memory equals disk
// exactly, and the node resumes binding the moment the disk comes back.
func TestSafety_FailClosedPersist_MemoryNeverAheadOfDisk(t *testing.T) {
	vs := newTestValidatorSet(5)
	path := filepath.Join(t.TempDir(), "vote-guard")
	guard := newCountingGuard(t, path)
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(guard))

	const H1, H2, H3 = uint64(700), uint64(701), uint64(702)
	A := ids.GenerateTestID()

	if !e.reserveSlotForSign(H1, A) {
		t.Fatal("precondition: the healthy bind at H1 must succeed")
	}
	memHealthy, floorHealthy := memBindings(e)
	diskHealthy, _ := diskBindings(t, path)
	if !sameBindings(memHealthy, diskHealthy) {
		t.Fatalf("precondition: mem=%v disk=%v must already agree", memHealthy, diskHealthy)
	}

	// --- THE DISK DIES.
	errDisk := errors.New("simulated durable-write failure (EIO)")
	guard.setFailure(errDisk)

	for _, h := range []uint64{H2, H3} {
		if e.reserveSlotForSign(h, ids.GenerateTestID()) {
			t.Fatalf("FAIL-OPEN: reserveSlotForSign(%d) permitted a signature while the durable write "+
				"was failing. No durable memory ⇒ no signature: a crash would then forget a vote this "+
				"node cast, and it would sign a conflicting sibling on the way back up.", h)
		}
	}

	memFailed, floorFailed := memBindings(e)
	diskFailed, _ := diskBindings(t, path)
	if !sameBindings(memFailed, memHealthy) {
		t.Fatalf("a failed write left RESIDUE in memory: %v -> %v. The rollback must restore the map "+
			"exactly, or memory claims a binding disk does not hold.", memHealthy, memFailed)
	}
	if !sameBindings(memFailed, diskFailed) {
		t.Fatalf("MEMORY AHEAD OF DISK: mem=%v disk=%v", memFailed, diskFailed)
	}
	if floorFailed != floorHealthy {
		t.Fatalf("a failed binding write moved the floor: %d -> %d", floorHealthy, floorFailed)
	}
	// The PRE-EXISTING binding is untouched — a disk fault must not cost this node the
	// equivocation memory it already has.
	if bound, ok := e.committedCanonical(H1); !ok || bound != A {
		t.Fatalf("the pre-existing binding at %d was lost during the disk fault: (%s,%v) want (%s,true)",
			H1, bound, ok, A)
	}
	if !e.reserveSlotForSign(H1, A) {
		t.Fatalf("the ALREADY-DURABLE binding at %d must stay idempotently signable during a disk fault "+
			"— it needs no new write", H1)
	}

	// A compaction under the same fault also fails closed: it returns the error (so the
	// caller withholds the export frontier) and rolls the floor back.
	if err := e.compactVoteGuardThroughQuasar(H1); !errors.Is(err, errDisk) {
		t.Fatalf("compactVoteGuardThroughQuasar must surface the durable-write error (the caller uses "+
			"it to withhold the export-frontier notification), got %v", err)
	}
	memAfterBadCompact, floorAfterBadCompact := memBindings(e)
	if floorAfterBadCompact != floorHealthy {
		t.Fatalf("the floor advanced to %d despite a FAILED durable write — memory must never claim a "+
			"floor disk does not carry", floorAfterBadCompact)
	}
	diskAfterBadCompact, _ := diskBindings(t, path)
	if !sameBindings(memAfterBadCompact, diskAfterBadCompact) {
		t.Fatalf("MEMORY AHEAD OF DISK after a failed compaction: mem=%v disk=%v",
			memAfterBadCompact, diskAfterBadCompact)
	}

	// --- THE DISK COMES BACK. The node resumes immediately; the fault cost it votes,
	// never safety, and required no operator intervention and no binding to be cleared.
	guard.setFailure(nil)
	B := ids.GenerateTestID()
	if !e.reserveSlotForSign(H2, B) {
		t.Fatalf("after the durable store recovered, binding %d must be permitted again", H2)
	}
	memRecovered, _ := memBindings(e)
	diskRecovered, _ := diskBindings(t, path)
	if !sameBindings(memRecovered, diskRecovered) {
		t.Fatalf("mem/disk diverged after recovery: mem=%v disk=%v", memRecovered, diskRecovered)
	}
	if got, ok := diskRecovered[SlotKey{Height: H2}]; !ok || got != B {
		t.Fatalf("the recovered binding did not reach disk: (%s,%v) want (%s,true)", got, ok, B)
	}
}

// -----------------------------------------------------------------------------
// 6. THE SLOT KEY IS HEIGHT — AND ONLY HEIGHT
// -----------------------------------------------------------------------------

// epochSetRoots is a deterministic ValidatorSetRootSource that gives every P-chain
// epoch height its OWN distinct set-root, so two candidates at one consensus height can
// be made to claim genuinely different validator-set epochs.
type epochSetRoots struct{}

func (epochSetRoots) ValidatorSetRoot(epochHeight uint64) ids.ID {
	if epochHeight == 0 {
		return ids.Empty // the bare / pre-fork block: no epoch bound
	}
	var id ids.ID
	id[0] = byte(epochHeight)
	id[1] = byte(epochHeight >> 8)
	return id
}

// TestSafety_SlotKeyIsHeightOnly pins the owner's non-negotiable constraint: the
// anti-equivocation slot key is HEIGHT-KEYED and must never grow an epoch or
// validator-set-root component.
//
// That value is proposer-influenced. When the key WAS (height, epoch), two honest
// sibling blocks at one consensus height pinned different proposervm P-chain heights —
// a bare/pre-fork block reports 0, a wrapped block reports P — so they landed in
// DIFFERENT slots and ONE HONEST VALIDATOR SIGNED BOTH. Two α-of-K certs formed at
// height 7 and the fleet took the equivocation exit(1).
//
// Two proofs, because either alone is weak:
//   - STRUCTURAL: SlotKey has exactly one field. A key that cannot express an epoch
//     cannot be fragmented by one — the constraint is enforced by the type, not by
//     discipline.
//   - BEHAVIOURAL, THROUGH THE REAL SIGN PATH: two tracked blocks at one height with
//     different epochs AND provably different set-roots, driven through
//     recordOwnVoteLocked (not a direct reserveSlotForSign call) — only the first gets
//     this node's signature.
func TestSafety_SlotKeyIsHeightOnly(t *testing.T) {
	// --- STRUCTURAL.
	st := reflect.TypeOf(SlotKey{})
	if st.NumField() != 1 {
		names := make([]string, st.NumField())
		for i := range names {
			names[i] = st.Field(i).Name
		}
		t.Fatalf("SlotKey must have EXACTLY one field (Height); got %d: %v. Adding an epoch or "+
			"validator-set-root component makes the anti-equivocation key proposer-influenced, which "+
			"is what let one honest validator sign two siblings at one height.", st.NumField(), names)
	}
	if f := st.Field(0); f.Name != "Height" || f.Type.Kind() != reflect.Uint64 {
		t.Fatalf("SlotKey's sole field must be Height uint64, got %s %s", f.Name, f.Type)
	}

	// --- BEHAVIOURAL, through the production sign path.
	vs := newTestValidatorSet(5)
	path := filepath.Join(t.TempDir(), "vote-guard")
	guard := newCountingGuard(t, path)
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{},
		WithVoteGuard(guard), WithValidatorSetRoot(epochSetRoots{}))

	const H = uint64(7) // the real fresh-net double-finalization height

	// Three candidates at the SAME consensus height H:
	//   wrapped — epoch 900, canonical CA           (the block this node signs)
	//   bare    — epoch 0,   canonical CB           (a pre-fork envelope: epoch Empty)
	//   requel  — epoch 1500, canonical CA          (the SAME inner block, new envelope+epoch)
	CA, CB := ids.GenerateTestID(), ids.GenerateTestID()
	mk := func(epoch uint64, canonical ids.ID) *PendingBlock {
		blk := newTestBlock(H, ids.GenerateTestID(), "slot-key")
		cb := &Block{
			id:           blk.id,
			parentID:     blk.parentID,
			height:       H,
			timestamp:    blk.timestamp.Unix(),
			data:         blk.bytes,
			pChainHeight: epoch,
			canonicalID:  canonical,
		}
		_ = e.consensus.AddBlock(context.Background(), cb)
		pb := &PendingBlock{ConsensusBlock: cb, VMBlock: blk, ProposedAt: time.Now()}
		e.mu.Lock()
		e.pendingBlocks[blk.id] = pb
		e.mu.Unlock()
		return pb
	}
	wrapped := mk(900, CA)
	bare := mk(0, CB)
	requel := mk(1500, CA)

	// POSITIVE CONTROL for the premise: the three candidates really do occupy different
	// validator-set epochs. Without this the collision below could be an artifact of the
	// positions being identical, and the test would prove nothing.
	e.mu.RLock()
	posWrapped := e.blockPositionLocked(wrapped, wrapped.ConsensusBlock.id)
	posBare := e.blockPositionLocked(bare, bare.ConsensusBlock.id)
	posRequel := e.blockPositionLocked(requel, requel.ConsensusBlock.id)
	e.mu.RUnlock()
	if posWrapped.Height != H || posBare.Height != H || posRequel.Height != H {
		t.Fatalf("premise: all three candidates must sit at height %d, got %d/%d/%d",
			H, posWrapped.Height, posBare.Height, posRequel.Height)
	}
	if posWrapped.ValidatorSetRoot == posBare.ValidatorSetRoot ||
		posWrapped.ValidatorSetRoot == posRequel.ValidatorSetRoot ||
		posBare.ValidatorSetRoot == posRequel.ValidatorSetRoot {
		t.Fatalf("premise FAILED: the candidates must carry DISTINCT validator-set roots or the "+
			"collision proves nothing — wrapped=%s bare=%s requel=%s",
			posWrapped.ValidatorSetRoot, posBare.ValidatorSetRoot, posRequel.ValidatorSetRoot)
	}

	// Sign the wrapped candidate through the REAL path (binds the slot, signs, records).
	e.mu.Lock()
	e.recordOwnVoteLocked(wrapped, wrapped.ConsensusBlock.id)
	wrappedVotes := len(wrapped.certVotes)
	e.mu.Unlock()
	if wrappedVotes != 1 {
		t.Fatalf("the first candidate at height %d must collect this node's signature, got %d cert votes",
			H, wrappedVotes)
	}
	if bound, ok := e.committedCanonical(H); !ok || bound != CA {
		t.Fatalf("height %d must be bound to the signed canonical %s, got (%s,%v)", H, CA, bound, ok)
	}

	// THE FIX UNDER TEST: the DIFFERENT-epoch, DIFFERENT-canonical sibling at the SAME
	// height is REFUSED. Under (height,epoch) keying this was a fresh slot and this node
	// signed twice.
	e.mu.Lock()
	e.recordOwnVoteLocked(bare, bare.ConsensusBlock.id)
	bareVotes := len(bare.certVotes)
	e.mu.Unlock()
	if bareVotes != 0 {
		t.Fatalf("DOUBLE-VOTE REGRESSION: this node signed a SECOND candidate at height %d because it "+
			"claimed a different validator-set epoch (bare set-root=%s vs signed=%s). The slot must be "+
			"epoch-BLIND: one signature per consensus height, full stop — otherwise a bare sibling and a "+
			"wrapped sibling each gather an α-of-K cert at one height and the fleet takes the "+
			"equivocation exit(1).", H, posBare.ValidatorSetRoot, posWrapped.ValidatorSetRoot)
	}
	if bound, ok := e.committedCanonical(H); !ok || bound != CA {
		t.Fatalf("the refusal disturbed the binding at %d: (%s,%v) want (%s,true)", H, bound, ok, CA)
	}

	// AND THE SYMMETRIC HALF: an epoch change must not FRAGMENT the slot in the other
	// direction either. The same inner block re-enveloped under a NEW epoch is the same
	// decision — it must remain idempotently signable, or a validator-set change would
	// strand this node's outstanding vote.
	e.mu.Lock()
	e.recordOwnVoteLocked(requel, requel.ConsensusBlock.id)
	requelVotes := len(requel.certVotes)
	e.mu.Unlock()
	if requelVotes != 1 {
		t.Fatalf("the SAME canonical %s re-offered at height %d under a different epoch (set-root %s) "+
			"must remain idempotently signable, got %d cert votes — the slot is keyed on HEIGHT and "+
			"valued by CANONICAL; the epoch influences neither", CA, H, posRequel.ValidatorSetRoot, requelVotes)
	}
	// One binding at H, one durable write for it: the epochs never fragmented the slot.
	mem, _ := memBindings(e)
	if len(mem) != 1 {
		t.Fatalf("three candidates at ONE height produced %d binding slots (%v) — the key must collapse "+
			"them all to SlotKey{Height:%d}", len(mem), mem, H)
	}
	if guard.writes() != 1 {
		t.Fatalf("three candidates at one height cost %d durable writes; only the FIRST binding writes "+
			"(the refusal and the idempotent re-offer must not)", guard.writes())
	}
}
