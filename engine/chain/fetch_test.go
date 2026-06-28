// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// fetch_on_unknown_vote_test.go — the VOTE-side self-heal: a vote that arrives
// for a block this node does not yet TRACK must NOT be dropped (the old wedge),
// but BUFFERED and the missing block FETCHED via the catch-up seam; when the
// block lands the buffered votes REPLAY through the normal (signature-gated)
// path and the block finalizes. These are the regression locks that make the
// "vote outran the block, quorum never met, write-path wedged forever" bug
// impossible — while proving buffering never weakens the signature gate or the
// memory bound.
package chain

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// deliveringCatchup is a Catchup stub for the vote-side self-heal: it records the
// missing block IDs it was asked to fetch and, on request, DELIVERS the real
// block back through the runtime's followVerifiedBlock — exactly as a node-layer
// GetAncestors→Put round-trip would, landing the block at the tracking site that
// drains buffered votes. It models "the fetch succeeded".
type deliveringCatchup struct {
	mu        sync.Mutex
	rt        *Runtime
	store     map[ids.ID]*verifyOnceBlock
	requested []ids.ID
	calls     int
}

func (c *deliveringCatchup) RequestAncestors(_ ids.ID, _ ids.ID, missingBlockID ids.ID, from ids.NodeID) error {
	c.mu.Lock()
	c.calls++
	c.requested = append(c.requested, missingBlockID)
	blk := c.store[missingBlockID]
	rt := c.rt
	c.mu.Unlock()
	if blk == nil || rt == nil {
		return nil
	}
	// Deliver the fetched block: this tracks it AND drains its buffered votes.
	rt.followVerifiedBlock(context.Background(), blk, from)
	return nil
}

func (c *deliveringCatchup) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *deliveringCatchup) wasRequested(id ids.ID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.requested {
		if r == id {
			return true
		}
	}
	return false
}

var _ Catchup = (*deliveringCatchup)(nil)

// newVoteSelfHealRuntime builds a started multi-validator Runtime for validator
// `self`, wired with the test validator set (verifier+signer), a cert gossiper,
// and the given Catchup. Returns the runtime + its chainID. This is the runtime
// (not bare engine) form so the engine's requestMissing hook is wired to
// Runtime.requestCatchup — the same path production uses.
func newVoteSelfHealRuntime(t *testing.T, vs *testValidatorSet, self int, cu Catchup) (*Runtime, ids.ID) {
	t.Helper()
	chainID := ids.GenerateTestID()
	rt := NewRuntime(NetworkConfig{
		ChainID:      chainID,
		NetworkID:    ids.GenerateTestID(),
		NodeID:       vs.nodeID(self),
		Logger:       log.Noop(),
		Params:       ptrParams(params5()),
		VoteVerifier: vs,
		VoteSigner:   vs.signerFor(self),
		Gossiper:     &certQuorumGossiper{rec: &recordingGossiper{}},
		Catchup:      cu,
	})
	if err := rt.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = rt.Stop(context.Background()) })
	return rt, chainID
}

func ptrParams(p config.Parameters) *config.Parameters { return &p }

// posFor builds the canonical VotePosition the engine will reconstruct for a
// genesis-extending block tracked via followVerifiedBlock (Round 0, Empty parent,
// no set-root). Votes signed over this position verify when they replay.
func posFor(chainID ids.ID, blk *verifyOnceBlock) VotePosition {
	return VotePosition{
		ChainID:  chainID,
		Height:   blk.height,
		Round:    0,
		BlockID:  blk.id,
		ParentID: blk.parentID,
	}
}

// -----------------------------------------------------------------------------
// TEST 1 — THE SELF-HEAL. Votes arrive BEFORE the block; the engine fetches the
// block, the buffered votes replay, and the block FINALIZES purely from
// votes-arrived-before-block. This is the regression that proves the wedge is
// impossible.
// -----------------------------------------------------------------------------

func TestVoteForUnknownBlock_FetchesThenFinalizes(t *testing.T) {
	vs := newTestValidatorSet(5)
	cu := &deliveringCatchup{store: map[ids.ID]*verifyOnceBlock{}}
	rt, chainID := newVoteSelfHealRuntime(t, vs, 0, cu)
	cu.mu.Lock()
	cu.rt = rt
	cu.mu.Unlock()

	// A genesis-extending block this node does NOT yet track. Parent is Empty so
	// the missing-PARENT path is inert — we isolate the missing-BYTES (vote) path.
	blk := newTestBlock(1, ids.Empty, "vote-before-block")
	cu.mu.Lock()
	cu.store[blk.id] = blk
	cu.mu.Unlock()
	pos := posFor(chainID, blk)

	// Sanity: the block is genuinely untracked right now.
	if rt.HasPendingBlock(blk.id) {
		t.Fatal("precondition: block must not be tracked yet")
	}

	// Deliver an α-quorum (params5: α=3) of SIGNED accept votes for the UNTRACKED
	// block. Each must be BUFFERED (not dropped) and the FIRST must trigger a
	// catch-up fetch for exactly this blockID.
	rt.ReceiveVote(vs.signedVote(1, pos))
	rt.ReceiveVote(vs.signedVote(2, pos))
	rt.ReceiveVote(vs.signedVote(3, pos))

	// (a) the catch-up was requested for this exact blockID. The delivering stub
	// fetches synchronously inside RequestAncestors, so by the time it has fired
	// the block is delivered+tracked. Wait for the request to be observed.
	if !waitFor(2*time.Second, func() bool { return cu.wasRequested(blk.id) }) {
		t.Fatalf("a vote for an untracked block must trigger a catch-up fetch for that blockID (calls=%d)", cu.count())
	}

	// (c) after delivery, the buffered votes replay, the block reaches the accept
	// quorum, and VM.Accept runs — finalized purely from votes that arrived BEFORE
	// the block. (b) is implied: had the votes been dropped, the count could never
	// reach α and this would never finalize.
	if !waitFor(2*time.Second, func() bool { return rt.IsAccepted(blk.id) }) {
		t.Fatalf("SELF-HEAL FAILED: block did not finalize from votes-before-block "+
			"(accepted=%v, VM.Accept=%d, catchup-calls=%d) — the wedge is back",
			rt.IsAccepted(blk.id), blk.AcceptCalled(), cu.count())
	}
	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("VM.Accept must run exactly once at quorum, got %d", got)
	}

	// The buffer must be fully drained for this block (no leak).
	rt.Transitive.mu.RLock()
	leaked := len(rt.Transitive.bufferedVotes[blk.id])
	rt.Transitive.mu.RUnlock()
	if leaked != 0 {
		t.Fatalf("buffered votes must be drained after the block finalizes, %d remain", leaked)
	}
}

// -----------------------------------------------------------------------------
// TEST 2 — BOUNDS / DoS. A flood of votes for many distinct never-delivered
// block IDs, and many votes for ONE block ID, must never grow the buffer past
// its caps. Memory is bounded regardless of adversarial junk.
// -----------------------------------------------------------------------------

func TestBufferedVotes_Bounded(t *testing.T) {
	vs := newTestValidatorSet(5)
	// No Catchup wired here on purpose: we are testing the BOUND, not the fetch.
	// We drive handleVote directly so the test is deterministic (no channel race).
	chainID := ids.GenerateTestID()
	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
	)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	// (i) Flood many votes for ONE untracked block ID — far past the per-block cap.
	oneID := ids.GenerateTestID()
	for i := 0; i < maxBufferedVotesPerBlock*4; i++ {
		e.handleVote(Vote{BlockID: oneID, NodeID: ids.GenerateTestNodeID(), Accept: true, SignedAt: time.Now(), Signature: []byte("x")})
	}
	e.mu.RLock()
	perBlock := len(e.bufferedVotes[oneID])
	e.mu.RUnlock()
	if perBlock > maxBufferedVotesPerBlock {
		t.Fatalf("per-block buffer exceeded cap: %d > %d", perBlock, maxBufferedVotesPerBlock)
	}
	if perBlock != maxBufferedVotesPerBlock {
		t.Fatalf("per-block buffer should fill to exactly the cap, got %d want %d", perBlock, maxBufferedVotesPerBlock)
	}

	// (ii) Flood votes for many distinct never-delivered block IDs — far past the
	// total distinct-block cap. (oneID above already occupies one key.)
	for i := 0; i < maxBufferedVoteBlocks*3; i++ {
		id := ids.GenerateTestID()
		e.handleVote(Vote{BlockID: id, NodeID: ids.GenerateTestNodeID(), Accept: true, SignedAt: time.Now(), Signature: []byte("x")})
	}
	e.mu.RLock()
	distinct := len(e.bufferedVotes)
	// also re-check every slice respects the per-block cap
	maxSlice := 0
	for _, s := range e.bufferedVotes {
		if len(s) > maxSlice {
			maxSlice = len(s)
		}
	}
	e.mu.RUnlock()

	if distinct > maxBufferedVoteBlocks {
		t.Fatalf("distinct-block buffer exceeded cap: %d > %d (unbounded map — DoS)", distinct, maxBufferedVoteBlocks)
	}
	if distinct != maxBufferedVoteBlocks {
		t.Fatalf("distinct-block buffer should fill to exactly the cap, got %d want %d", distinct, maxBufferedVoteBlocks)
	}
	if maxSlice > maxBufferedVotesPerBlock {
		t.Fatalf("a per-block slice exceeded its cap under the distinct-block flood: %d > %d", maxSlice, maxBufferedVotesPerBlock)
	}
}

// -----------------------------------------------------------------------------
// TEST 3 — SIGNATURE INTEGRITY. A buffered vote is signature-verified on REPLAY
// exactly as a live vote: a BAD-signature (or unsigned) parked vote does NOT
// count toward the quorum when the block lands. Buffering does not bypass the
// gate.
// -----------------------------------------------------------------------------

func TestBufferedVote_StillSignatureGated(t *testing.T) {
	vs := newTestValidatorSet(5)
	cu := &deliveringCatchup{store: map[ids.ID]*verifyOnceBlock{}}
	rt, chainID := newVoteSelfHealRuntime(t, vs, 0, cu)
	cu.mu.Lock()
	cu.rt = rt
	cu.mu.Unlock()

	blk := newTestBlock(1, ids.Empty, "bad-sig-parked")
	cu.mu.Lock()
	cu.store[blk.id] = blk
	cu.mu.Unlock()
	pos := posFor(chainID, blk)

	// Park THREE votes for the untracked block, but corrupt their signatures so
	// none can verify on replay. If buffering bypassed the gate these 3 would meet
	// α=3 and FALSELY finalize the block.
	for _, i := range []int{1, 2, 3} {
		bad := vs.signedVote(i, pos)
		bad.Signature = append([]byte(nil), bad.Signature...)
		bad.Signature[0] ^= 0xFF // flip a bit → signature no longer valid
		rt.ReceiveVote(bad)
	}

	// The fetch still fires (we buffer before verifying — a spam vote costs only a
	// slot), delivering+tracking the block so the parked votes replay.
	if !waitFor(2*time.Second, func() bool { return cu.wasRequested(blk.id) }) {
		t.Fatalf("a (bad-sig) vote for an untracked block must still trigger the fetch")
	}

	// On replay each bad-sig vote hits VerifyVote and is DROPPED. With no valid
	// votes (the follower self=0 has not voted here either — followVerifiedBlock
	// would cast self's vote, that is ONE, still < α=3), the block must NOT
	// finalize. We give the replay ample time; a false finalize would appear here.
	if waitFor(500*time.Millisecond, func() bool { return rt.IsAccepted(blk.id) }) {
		t.Fatalf("SAFETY VIOLATION: a block finalized from BAD-signature buffered votes "+
			"(VM.Accept=%d) — buffering bypassed the signature gate", blk.AcceptCalled())
	}
	if got := blk.AcceptCalled(); got != 0 {
		t.Fatalf("VM.Accept must not run without α valid signatures, got %d", got)
	}

	// Now prove the SAME block finalizes once GENUINE α votes arrive — the gate
	// admits valid votes, so the refusal above was the signature check, not a
	// stuck path. (followVerifiedBlock already cast self's vote 0; add 2 more for
	// α=3.) These are tracked-block votes now, so the normal path counts them.
	rt.ReceiveVote(vs.signedVote(1, pos))
	rt.ReceiveVote(vs.signedVote(2, pos))
	if !waitFor(2*time.Second, func() bool { return rt.IsAccepted(blk.id) }) {
		t.Fatalf("liveness: block did not finalize after GENUINE α votes (Accept=%d) — gate too strict",
			blk.AcceptCalled())
	}
}

// -----------------------------------------------------------------------------
// TEST 4 — FETCH GATED ON BUFFER ACCEPTANCE. The fetch (requestMissing) fires
// ONLY for a vote the buffer actually parked. Once the distinct-block buffer is
// saturated at its cap, a vote for a NEW (forged) block ID is dropped by
// bufferVoteLocked — and must therefore fire NO fetch. Firing a fetch for a vote
// we refused to buffer is pure amplification with no payoff (nothing is parked
// for the fetched block to drain into). This pins FIX 2: the bounded global fetch
// ceiling is "fetches ≤ buffered distinct blocks".
// -----------------------------------------------------------------------------

func TestFetchNotFiredWhenBufferRejected(t *testing.T) {
	vs := newTestValidatorSet(5)
	// Bare engine: we drive handleVote directly (synchronous, deterministic) and
	// stub the engine's requestMissing hook with a counter. A bare engine leaves
	// requestMissing nil (only NewRuntime wires it), so the stub is the sole caller.
	chainID := ids.GenerateTestID()
	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
	)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	// Record every fetch request. handleVote calls this inline (same goroutine)
	// after releasing t.mu, so a plain counter is race-free under direct driving.
	var fetches int
	e.mu.Lock()
	e.requestMissing = func(missingID ids.ID, from ids.NodeID) { fetches++ }
	e.mu.Unlock()

	attacker := ids.GenerateTestNodeID()

	// Saturate the distinct-block buffer to EXACTLY its cap with distinct forged
	// IDs. Each is a new key the buffer accepts, so each fires exactly one fetch.
	for i := 0; i < maxBufferedVoteBlocks; i++ {
		e.handleVote(Vote{BlockID: ids.GenerateTestID(), NodeID: attacker, Accept: true, SignedAt: time.Now()})
	}

	e.mu.RLock()
	distinct := len(e.bufferedVotes)
	e.mu.RUnlock()
	if distinct != maxBufferedVoteBlocks {
		t.Fatalf("precondition: buffer should be saturated at %d distinct blocks, got %d", maxBufferedVoteBlocks, distinct)
	}
	fetchesAtCap := fetches
	if fetchesAtCap != maxBufferedVoteBlocks {
		t.Fatalf("precondition: %d accepted votes should each fire one fetch, got %d", maxBufferedVoteBlocks, fetchesAtCap)
	}

	// Now send MORE votes for NEW (distinct, never-before-seen) forged block IDs.
	// The buffer is full → bufferVoteLocked returns false for each → the fetch must
	// be GATED OFF. The fetch counter must NOT advance.
	for i := 0; i < 500; i++ {
		e.handleVote(Vote{BlockID: ids.GenerateTestID(), NodeID: attacker, Accept: true, SignedAt: time.Now()})
	}

	if fetches != fetchesAtCap {
		t.Fatalf("FIX-2 REGRESSION: %d fetches fired for votes the saturated buffer REFUSED "+
			"(was %d at cap, now %d) — a rejected vote must fire NO fetch (pure amplification)",
			fetches-fetchesAtCap, fetchesAtCap, fetches)
	}

	// Sanity: a vote for an ALREADY-BUFFERED block ID (under its per-block cap) is
	// still accepted and still fetches (the gate keys on buffer acceptance, not on
	// "new ID"). Pick one of the saturated keys and add a second vote for it.
	e.mu.RLock()
	var anExisting ids.ID
	for id := range e.bufferedVotes {
		anExisting = id
		break
	}
	e.mu.RUnlock()
	e.handleVote(Vote{BlockID: anExisting, NodeID: ids.GenerateTestNodeID(), Accept: true, SignedAt: time.Now()})
	if fetches != fetchesAtCap+1 {
		t.Fatalf("a vote ACCEPTED into an existing (under-cap) buffer key must still fetch: "+
			"expected %d, got %d", fetchesAtCap+1, fetches)
	}
}
