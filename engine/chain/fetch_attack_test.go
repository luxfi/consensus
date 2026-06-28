// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// fetch_on_unknown_vote_red_test.go — RED-TEAM exploit probes against the
// fetch-on-unknown-vote fix (commit c9a70880a). Each test here is an ATTACK: it
// either demonstrates a break (a finding Blue must fix) or, kept as a hardening
// lock, proves a guard holds. Read the per-test banner for which.
//
// These tests do NOT modify Blue's code. They drive the public/engine API the
// same way Blue's own tests do.
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

// -----------------------------------------------------------------------------
// nonDeliveringCatchup models the ATTACKER'S forged block IDs: it is asked to
// fetch a missing block and NEVER delivers it (the block does not exist). This
// is the realistic adversary — votes carry random BlockIDs that no honest peer
// can ever serve. It still lets claimCatchupLocked run (catchup != nil), so the
// engine records the throttle stamp in catchupRequested for every distinct ID.
// -----------------------------------------------------------------------------
type nonDeliveringCatchup struct {
	mu    sync.Mutex
	calls int
	ids   map[ids.ID]struct{}
}

func newNonDeliveringCatchup() *nonDeliveringCatchup {
	return &nonDeliveringCatchup{ids: map[ids.ID]struct{}{}}
}

func (c *nonDeliveringCatchup) RequestAncestors(_ ids.ID, _ ids.ID, missingBlockID ids.ID, _ ids.NodeID) error {
	c.mu.Lock()
	c.calls++
	c.ids[missingBlockID] = struct{}{}
	c.mu.Unlock()
	// Never deliver anything — the forged block does not exist.
	return nil
}

func (c *nonDeliveringCatchup) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

var _ Catchup = (*nonDeliveringCatchup)(nil)

// -----------------------------------------------------------------------------
// RED-1 → BOUND LOCK (surface #1, the SECOND unbounded map): catchupRequested.
//
// THE ORIGINAL FINDING (pre-fix): catchupRequested had NO delete site anywhere.
// Forging votes for many DISTINCT random BlockIDs that will NEVER be delivered
// made handleVote fire requestMissing → claimCatchupLocked, writing
// catchupRequested[forgedID]=now forever (the buffer capped at 1024 keys, this
// map did not). 8192 forged IDs → 8192 immortal entries → unbounded memory at
// zero signature cost. THIS TEST USED TO FAIL HERE (proving the leak).
//
// THE FIX (Blue): two bounds, both fail-closed —
//
//	(1) handleVote now GATES the fetch on buffer acceptance: a vote dropped at a
//	    buffer cap fires NO fetch, so no catchupRequested entry is created for it.
//	    With the 1024 distinct-block buffer cap, at most 1024 IDs ever claim.
//	(2) claimCatchupLocked HARD-bounds the map at maxCatchupRequested (TTL-sweep
//	    then fail-closed at the cap), plus deletes on track/decide.
//
// POST-FIX INVARIANT (this test now asserts, and PASSES): under the same
// 8192-forged-ID flood, len(catchupRequested) stays ≤ maxCatchupRequested (and in
// fact ≤ the buffer cap, since the fetch is gated on buffer acceptance). The leak
// is closed.
// -----------------------------------------------------------------------------
func TestRED_CatchupRequested_UnboundedUnderForgedVoteIDs(t *testing.T) {
	vs := newTestValidatorSet(5)
	cu := newNonDeliveringCatchup()
	rt, chainID := newVoteSelfHealRuntime(t, vs, 0, cu)
	cu.mu.Lock()
	cu.mu.Unlock()
	_ = chainID

	// Attacker budget: flood far past the 1024 distinct-block buffer cap with
	// votes for distinct random (forged) BlockIDs. We drive the engine directly
	// (handleVote) so the count is deterministic — this is exactly the code the
	// channel path reaches (voteHandlerWithCtx → handleVote). Each vote carries a
	// random BlockID an honest peer can never serve.
	const flood = maxBufferedVoteBlocks * 8 // 8192 distinct forged IDs
	attacker := ids.GenerateTestNodeID()
	for i := 0; i < flood; i++ {
		forged := ids.GenerateTestID()
		// A genuine signature is NOT required to reach the buffer+fetch: handleVote
		// buffers BEFORE any signature work. So a zero-cost unsigned spam vote is the
		// worst case for the map's growth — exactly what we bound.
		rt.Transitive.handleVote(Vote{
			BlockID:  forged,
			NodeID:   attacker,
			Accept:   true,
			SignedAt: time.Now(),
		})
	}

	rt.Transitive.mu.RLock()
	buffered := len(rt.Transitive.bufferedVotes)
	tracked := len(rt.Transitive.catchupRequested)
	rt.Transitive.mu.RUnlock()

	t.Logf("after %d distinct forged vote-IDs: len(bufferedVotes)=%d (cap %d)  "+
		"len(catchupRequested)=%d (hard cap %d)",
		flood, buffered, maxBufferedVoteBlocks, tracked, maxCatchupRequested)

	// The buffer is correctly bounded — confirm the cap held (control).
	if buffered > maxBufferedVoteBlocks {
		t.Fatalf("control: bufferedVotes exceeded its cap (%d > %d)", buffered, maxBufferedVoteBlocks)
	}

	// THE FIXED INVARIANT: catchupRequested is now HARD-bounded. Under any flood of
	// attacker-chosen missing IDs it can never exceed maxCatchupRequested. The leak
	// (one immortal entry per forged ID) is closed.
	if tracked > maxCatchupRequested {
		t.Fatalf("REGRESSION (surface #1): catchupRequested exceeded its hard cap under "+
			"attacker-chosen missing IDs — %d entries vs cap %d. The fetch-gate and/or the "+
			"claimCatchupLocked cap+TTL bound regressed; DoS via memory exhaustion is back. "+
			"catchup-fetch-calls=%d", tracked, maxCatchupRequested, cu.count())
	}

	// Stronger structural check: because the fetch is GATED on buffer acceptance
	// (only the ≤1024 buffered distinct IDs ever claim), the map settles at the
	// buffer cap, well under the hard cap. This pins the fetch-gate specifically —
	// if it regressed, catchupRequested would climb toward maxCatchupRequested even
	// while bufferedVotes stayed at 1024.
	if tracked > maxBufferedVoteBlocks {
		t.Fatalf("fetch-gate regressed: catchupRequested=%d exceeded the buffer cap %d while "+
			"bufferedVotes held at %d — a fetch fired for a vote the buffer REFUSED (pure "+
			"amplification). Expected the gate to cap claims at the buffered-block count.",
			tracked, maxBufferedVoteBlocks, buffered)
	}
}

// -----------------------------------------------------------------------------
// RED-1b → BOUND LOCK (surface #1 part 2, the fetch storm): pre-fix every
// distinct forged ID fired a RequestAncestors with NO aggregate ceiling — N
// forged IDs = N fetches (1:1 amplification). The fix gates the fetch on buffer
// acceptance (handleVote), so only the ≤ maxBufferedVoteBlocks IDs the buffer
// actually parks can fetch; the rest are dropped at the cap and fire nothing.
//
// POST-FIX INVARIANT (now asserted): a flood of distinct forged vote-IDs produces
// at most maxBufferedVoteBlocks fetches within one burst — the storm plateaus at
// the buffer cap instead of tracking the flood. That is the global fetch ceiling.
// -----------------------------------------------------------------------------
func TestRED_FetchStorm_NoGlobalRateLimit(t *testing.T) {
	vs := newTestValidatorSet(5)
	cu := newNonDeliveringCatchup()
	rt, _ := newVoteSelfHealRuntime(t, vs, 0, cu)

	const flood = 4000
	attacker := ids.GenerateTestNodeID()
	for i := 0; i < flood; i++ {
		rt.Transitive.handleVote(Vote{
			BlockID:  ids.GenerateTestID(),
			NodeID:   attacker,
			Accept:   true,
			SignedAt: time.Now(),
		})
	}
	calls := cu.count()
	t.Logf("FETCH CEILING: %d distinct forged vote-IDs produced %d RequestAncestors fetches "+
		"(plateaus at the buffer cap %d, not 1:1 with the flood)", flood, calls, maxBufferedVoteBlocks)
	// The gate caps fetches at the buffered-block count: votes past the buffer cap
	// are dropped and fire no fetch. If this regressed, calls would climb toward the
	// flood size (the old 1:1 amplification).
	if calls > maxBufferedVoteBlocks {
		t.Fatalf("fetch storm un-bounded: %d fetches > buffer cap %d for a %d-ID flood — the "+
			"fetch-gate on buffer acceptance regressed (amplification is back)", calls, maxBufferedVoteBlocks, flood)
	}
}

// -----------------------------------------------------------------------------
// RED-2 (surface #2, signature-gate bypass on replay — SURGICAL): Blue's own
// gate test only flips a bit. The sharper attacks are SIGNATURE REUSE:
//
//	(a) a vote validly signed as REJECT (pos,false) presented as ACCEPT;
//	(b) a vote validly signed for a DIFFERENT block's position presented for
//	    this block.
//
// Both carry a cryptographically VALID ed25519 signature — by a real validator —
// just over the wrong message. If the gate verified only "is this a real
// validator's signature" without binding decision+position, these would count.
// We park α of each kind for an untracked block; after the block lands and the
// parked votes replay, the block MUST NOT finalize from them.
//
// Kept as a HARDENING LOCK: it currently passes (the guard holds). If anyone
// loosens canonicalVoteMessageFor's decision/position binding, this flips red.
// -----------------------------------------------------------------------------
func TestRED_BufferedVote_RejectSigReusedAsAccept_DoesNotCount(t *testing.T) {
	vs := newTestValidatorSet(5)
	cu := &deliveringCatchup{store: map[ids.ID]*verifyOnceBlock{}}
	rt, chainID := newVoteSelfHealRuntime(t, vs, 0, cu)
	cu.mu.Lock()
	cu.rt = rt
	cu.mu.Unlock()

	blk := newTestBlock(1, ids.Empty, "reject-sig-reuse")
	cu.mu.Lock()
	cu.store[blk.id] = blk
	cu.mu.Unlock()
	pos := posFor(chainID, blk)

	// Build, for validators 1..3, a vote that is a VALID ed25519 signature over the
	// REJECT message (pos,false) but stamped Accept=true on the wire. This is the
	// classic "lift a real signature onto the opposite decision" attack.
	rejectMsg := canonicalVoteMessageFor(pos, false)
	for _, i := range []int{1, 2, 3} {
		sig := signWithValidatorKey(vs, i, rejectMsg) // genuine sig, wrong decision
		rt.ReceiveVote(Vote{
			BlockID:   blk.id,
			NodeID:    vs.nodeID(i),
			Accept:    true, // claims ACCEPT…
			SignedAt:  time.Now(),
			Signature: sig, // …but signed REJECT
			ParentID:  pos.ParentID,
			Round:     pos.Round,
		})
	}

	// The fetch fires (buffer-before-verify), delivering+tracking the block so the
	// parked votes replay through the gate.
	if !waitFor(2*time.Second, func() bool { return cu.wasRequested(blk.id) }) {
		t.Fatalf("a vote for an untracked block must still trigger the fetch")
	}

	// On replay each vote's claimed-accept message (pos,true) ≠ its signed-reject
	// message (pos,false) ⇒ VerifyVote fails ⇒ none count. followVerifiedBlock cast
	// self(0)'s ONE genuine accept, so at most 1 < α=3. Must NOT finalize.
	if waitFor(600*time.Millisecond, func() bool { return rt.IsAccepted(blk.id) }) {
		t.Fatalf("SAFETY VIOLATION (surface #2a): block finalized from REJECT signatures "+
			"reused as ACCEPT (VM.Accept=%d) — the decision bit is not bound", blk.AcceptCalled())
	}
}

func TestRED_BufferedVote_WrongPositionSig_DoesNotCount(t *testing.T) {
	vs := newTestValidatorSet(5)
	cu := &deliveringCatchup{store: map[ids.ID]*verifyOnceBlock{}}
	rt, chainID := newVoteSelfHealRuntime(t, vs, 0, cu)
	cu.mu.Lock()
	cu.rt = rt
	cu.mu.Unlock()

	// The target block (the one we want to falsely finalize) and a DECOY block at a
	// different height whose ACCEPT message the attacker can legitimately collect
	// signatures over (e.g. from a real prior round).
	target := newTestBlock(1, ids.Empty, "target")
	decoy := newTestBlock(2, ids.Empty, "decoy-different-position")
	cu.mu.Lock()
	cu.store[target.id] = target
	cu.mu.Unlock()
	targetPos := posFor(chainID, target)
	decoyPos := posFor(chainID, decoy)

	// Validators 1..3 each produce a VALID accept signature over the DECOY position,
	// which the attacker then replays stamped with the TARGET's BlockID.
	decoyMsg := canonicalVoteMessageFor(decoyPos, true)
	for _, i := range []int{1, 2, 3} {
		sig := signWithValidatorKey(vs, i, decoyMsg) // genuine accept, wrong block
		rt.ReceiveVote(Vote{
			BlockID:   target.id, // claims to be for the TARGET…
			NodeID:    vs.nodeID(i),
			Accept:    true,
			SignedAt:  time.Now(),
			Signature: sig, // …but signed the DECOY position
			ParentID:  targetPos.ParentID,
			Round:     targetPos.Round,
		})
	}

	if !waitFor(2*time.Second, func() bool { return cu.wasRequested(target.id) }) {
		t.Fatalf("a vote for an untracked block must still trigger the fetch")
	}

	// On replay the gate rebuilds (targetPos,true); the signatures are over
	// (decoyPos,true). BlockID/Height differ ⇒ messages differ ⇒ VerifyVote fails.
	if waitFor(600*time.Millisecond, func() bool { return rt.IsAccepted(target.id) }) {
		t.Fatalf("SAFETY VIOLATION (surface #2b): block finalized from signatures over a "+
			"DIFFERENT position (VM.Accept=%d) — position not bound", target.AcceptCalled())
	}
}

// -----------------------------------------------------------------------------
// RED-4 (surface #4, double-count one validator): replay ONE real validator's
// vote twice (once buffered-then-drained, once live) and try to reach α with
// fewer than α DISTINCT validators. The cert map is keyed by NodeID, so the
// replay must collapse to one cert entry. With α-1 distinct validators the block
// MUST NOT finalize regardless of how many times one of them is replayed.
//
// Kept as a HARDENING LOCK. Passes today (NodeID de-dup holds).
// -----------------------------------------------------------------------------
func TestRED_DoubleCountOneValidator_DoesNotReachAlpha(t *testing.T) {
	vs := newTestValidatorSet(5) // α=3 under params5
	cu := &deliveringCatchup{store: map[ids.ID]*verifyOnceBlock{}}
	rt, chainID := newVoteSelfHealRuntime(t, vs, 0, cu)
	cu.mu.Lock()
	cu.rt = rt
	cu.mu.Unlock()

	blk := newTestBlock(1, ids.Empty, "double-count")
	cu.mu.Lock()
	cu.store[blk.id] = blk
	cu.mu.Unlock()
	pos := posFor(chainID, blk)

	// self=0 will cast its own genuine accept when followVerifiedBlock tracks the
	// block (vote #1, distinct). We supply only validator 1's genuine accept, but
	// replay it MANY times (buffered before the block lands, then again live after).
	// Distinct validators = {0,1} = 2 < α=3. No amount of replaying validator 1 can
	// manufacture a third distinct signer.
	v1 := vs.signedVote(1, pos)
	for i := 0; i < 16; i++ {
		rt.ReceiveVote(v1) // buffered while untracked; later counted once-per-NodeID
	}

	if !waitFor(2*time.Second, func() bool { return cu.wasRequested(blk.id) }) {
		t.Fatalf("vote for untracked block must trigger fetch")
	}
	// Hammer the live path too, after the block is tracked.
	for i := 0; i < 16; i++ {
		rt.ReceiveVote(v1)
	}

	if waitFor(700*time.Millisecond, func() bool { return rt.IsAccepted(blk.id) }) {
		t.Fatalf("SAFETY VIOLATION (surface #4): block finalized with only 2 distinct validators "+
			"by replaying one validator's vote (VM.Accept=%d) — NodeID de-dup failed", blk.AcceptCalled())
	}

	// Sanity: a genuine THIRD distinct validator does finalize it (gate not stuck).
	rt.ReceiveVote(vs.signedVote(2, pos))
	if !waitFor(2*time.Second, func() bool { return rt.IsAccepted(blk.id) }) {
		t.Fatalf("liveness: block did not finalize once a 3rd DISTINCT validator voted (Accept=%d)",
			blk.AcceptCalled())
	}
}

// -----------------------------------------------------------------------------
// RED-3 (surface #3, finalized re-buffer race): hammer late votes for a block
// CONCURRENTLY with its finalize, under -race. If delete(pendingBlocks) and
// add(finalizedByCert) were not atomic under the same lock, a late vote could
// slip into the !exists branch BEFORE finalizedByCert is set → re-buffer + fetch
// for an already-final block → an immortal leak. We assert: after the dust
// settles, no buffered votes and no growth of catchupRequested for the finalized
// ID (it was tracked, so claimCatchupLocked early-returns once known).
//
// Kept as a HARDENING LOCK. The -race detector is the real assertion.
// -----------------------------------------------------------------------------
func TestRED_LateVoteVsFinalize_NoReBufferLeak(t *testing.T) {
	vs := newTestValidatorSet(5)
	cu := &deliveringCatchup{store: map[ids.ID]*verifyOnceBlock{}}
	rt, chainID := newVoteSelfHealRuntime(t, vs, 0, cu)
	cu.mu.Lock()
	cu.rt = rt
	cu.mu.Unlock()

	blk := newTestBlock(1, ids.Empty, "late-vote-race")
	cu.mu.Lock()
	cu.store[blk.id] = blk
	cu.mu.Unlock()
	pos := posFor(chainID, blk)

	// Deliver the block + a genuine α quorum so it finalizes…
	rt.followVerifiedBlock(context.Background(), blk, vs.nodeID(1))
	var wg sync.WaitGroup
	// …while a flood of LATE genuine votes for the same block races the finalize.
	for _, i := range []int{1, 2, 3, 4} {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 50; k++ {
				rt.ReceiveVote(vs.signedVote(i, pos))
			}
		}()
	}
	wg.Wait()

	if !waitFor(2*time.Second, func() bool { return rt.IsAccepted(blk.id) }) {
		t.Fatalf("block did not finalize under the late-vote race (Accept=%d)", blk.AcceptCalled())
	}
	// Give late votes time to drain through the channel post-finalize.
	time.Sleep(150 * time.Millisecond)

	rt.Transitive.mu.RLock()
	leakedBuf := len(rt.Transitive.bufferedVotes[blk.id])
	_, stillTracked := rt.Transitive.catchupRequested[blk.id]
	rt.Transitive.mu.RUnlock()
	if leakedBuf != 0 {
		t.Fatalf("FINDING (surface #3): %d votes re-buffered for an already-finalized block — "+
			"delete(pendingBlocks)/add(finalizedByCert) raced", leakedBuf)
	}
	// A finalized block being in catchupRequested is not itself a leak (it was a
	// real block that we tracked), but log it for the report.
	t.Logf("post-finalize: bufferedVotes[blk]=%d, catchupRequested has blk=%v, VM.Accept=%d",
		leakedBuf, stillTracked, blk.AcceptCalled())
}

// signWithValidatorKey exposes a raw "sign THIS message with validator i's key"
// for the signature-reuse attacks (vs.sign binds to a position; here we sign an
// arbitrary attacker-chosen message). It reuses the harness signer.
func signWithValidatorKey(vs *testValidatorSet, i int, msg []byte) []byte {
	sig, err := vs.signerFor(i).SignVote(msg)
	if err != nil {
		panic(err)
	}
	return sig
}

// -----------------------------------------------------------------------------
// RED-1c → BOUND LOCK (surface #1 part 2, the per-block cap vs LARGE sets):
// PRE-FIX maxBufferedVotesPerBlock was 256, but node/config/tokenomics.go defines
// a 500-validator tier (and an unlimited tier). On a K=N / α=⌈⅔N⌉ chain with
// N=400, α≈267 > 256, so votes 257..N were DROPPED at the cap and the buffered
// fast-path could not finalize a large net in the gossip-race window. THIS TEST
// USED TO ASSERT exactly 256 survived (the drop).
//
// THE FIX (Blue): raised maxBufferedVotesPerBlock to 512 — ≥ the 500-validator
// tier with margin (α(500)≈334 < 512). Now every genuine validator in a supported
// set parks, so the buffered fast-path can reach α without re-poll.
//
// POST-FIX INVARIANT (now asserted): all N=400 distinct genuine votes survive
// (none dropped), and α(N) ≤ cap so the fast-path is sufficient. We also pin that
// the cap covers the full 500-tier's supermajority.
// -----------------------------------------------------------------------------
func TestRED_PerBlockCap_DropsLegitVotesForLargeValidatorSet(t *testing.T) {
	const N = 400 // a realistic large set within the tokenomics 500 tier
	vs := newTestValidatorSet(N)

	// Bare engine (no Catchup needed — we are measuring the BUFFER, not the fetch).
	// K and α are immaterial to bufferVoteLocked; we only need the cap behavior,
	// which is a fixed const. Drive handleVote directly for determinism.
	chainID := ids.GenerateTestID()
	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
	)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	// One untracked block; N distinct GENUINE validators each cast a real accept.
	blkID := ids.GenerateTestID()
	parentID := ids.Empty
	pos := VotePosition{ChainID: chainID, Height: 1, Round: 0, BlockID: blkID, ParentID: parentID}
	for i := 0; i < N; i++ {
		v := vs.signedVote(i, pos) // genuine signature by a distinct validator
		e.handleVote(v)
	}

	e.mu.RLock()
	parked := len(e.bufferedVotes[blkID])
	e.mu.RUnlock()

	t.Logf("LARGE-SET: %d distinct genuine validators voted for one untracked block; "+
		"%d survived the per-block cap (%d). %d dropped.",
		N, parked, maxBufferedVotesPerBlock, N-parked)

	// FIXED: the whole supported set parks — no legitimate vote is dropped.
	if parked != N {
		t.Fatalf("FIX-3 REGRESSION: %d of %d genuine votes survived the per-block cap %d — "+
			"the cap dropped legitimate validator votes for a supported-size net (large-net "+
			"liveness wedge is back)", parked, N, maxBufferedVotesPerBlock)
	}
	// The buffered fast-path is now SUFFICIENT for this net: α(N) ≤ cap, so a full
	// α-quorum of buffered accepts can replay and finalize without depending on
	// re-poll re-delivery.
	alphaForN := (2*N + 2) / 3 // ⌈2N/3⌉ ≈ strict-⅔ headcount
	if alphaForN > maxBufferedVotesPerBlock {
		t.Fatalf("FIX-3 REGRESSION: for N=%d, α≈%d still exceeds cap %d — the buffered fast-path "+
			"cannot finalize a supported-size net", N, alphaForN, maxBufferedVotesPerBlock)
	}
	// Pin the design target: the cap covers the full 500-validator tokenomics tier's
	// supermajority with margin, so no supported net is wedged at the buffer.
	const tokenomicsMaxTierValidators = 500 // node/config/tokenomics.go MaxValidators tier
	alpha500 := (2*tokenomicsMaxTierValidators + 2) / 3
	if alpha500 > maxBufferedVotesPerBlock {
		t.Fatalf("per-block cap %d does not cover the 500-validator tier supermajority α≈%d — "+
			"raise maxBufferedVotesPerBlock", maxBufferedVotesPerBlock, alpha500)
	}
}

// -----------------------------------------------------------------------------
// TEST (L-1 lock) — SINGLE-BYZANTINE-ID BUFFER CROWD-OUT IS CLOSED. Before the
// fix, bufferVoteLocked appended every arrival, so ONE Byzantine validator (one
// NodeID) could send maxBufferedVotesPerBlock junk votes for a block a genuine
// proposer is about to gossip — filling the per-block slice and crowding genuine
// validators' votes out of the buffer fast-path (liveness-only, recoverable via
// re-poll, but hardened here). The fix dedups by (BlockID, NodeID), the dual of
// certVotes' NodeID keying: one NodeID occupies at most ONE slot per block, so it
// cannot crowd anyone out. This attack PROVES the closure.
// -----------------------------------------------------------------------------

func TestRED_BufferedVote_DedupByNodeID(t *testing.T) {
	vs := newTestValidatorSet(5)
	// Bare engine — we measure the BUFFER, not the fetch; drive handleVote directly
	// for determinism (synchronous, same path the channel reaches).
	chainID := ids.GenerateTestID()
	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
	)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	// One untracked block a genuine proposer is about to gossip.
	blkID := ids.GenerateTestID()
	pos := VotePosition{ChainID: chainID, Height: 1, Round: 0, BlockID: blkID, ParentID: ids.Empty}

	// ATTACK: a single Byzantine validator (validator 1) sprays 600 votes — well past
	// the per-block cap (512) — all for this one block. Pre-fix every one appended and
	// the slice hit the cap from this lone NodeID alone, locking out everyone else.
	attacker := 1
	const spray = 600
	for i := 0; i < spray; i++ {
		v := vs.signedVote(attacker, pos)
		e.handleVote(v)
	}

	e.mu.RLock()
	afterAttack := len(e.bufferedVotes[blkID])
	e.mu.RUnlock()

	// DEDUP: one NodeID → exactly ONE parked slot, no matter how many it sent.
	if afterAttack != 1 {
		t.Fatalf("L-1 BREAK: one Byzantine NodeID sprayed %d votes for one block and parked %d "+
			"entries — must dedup to exactly 1 (single-ID crowd-out is open)", spray, afterAttack)
	}

	// LIVENESS: a DIFFERENT genuine validator's vote for the SAME block must still be
	// accepted into the buffer — proving the attacker (even after >512 sprayed) did
	// NOT consume the per-block budget and crowd the honest voter out.
	genuine := 2
	e.handleVote(vs.signedVote(genuine, pos))

	e.mu.RLock()
	afterGenuine := len(e.bufferedVotes[blkID])
	e.mu.RUnlock()

	if afterGenuine != 2 {
		t.Fatalf("L-1 CROWD-OUT: after one NodeID sprayed %d votes, a genuine distinct validator's "+
			"vote for the same block was crowded out (slice=%d, want 2) — the buffer budget is "+
			"consumed by a single Byzantine ID", spray, afterGenuine)
	}

	// And the attacker spraying AGAIN after the genuine vote landed still replaces its
	// own single slot (no growth) — the genuine voter stays parked.
	for i := 0; i < spray; i++ {
		e.handleVote(vs.signedVote(attacker, pos))
	}
	e.mu.RLock()
	final := len(e.bufferedVotes[blkID])
	e.mu.RUnlock()
	if final != 2 {
		t.Fatalf("L-1 BREAK: re-spray by the attacker grew the slice to %d (want 2) — dedup must "+
			"REPLACE the attacker's slot in place, never append", final)
	}

	t.Logf("L-1 lock: 1 Byzantine NodeID × %d votes → 1 slot; +1 genuine distinct validator → 2 "+
		"slots; re-spray → still 2. Single-ID crowd-out is closed.", spray)
}

func ptrParamsRed(p config.Parameters) *config.Parameters { return &p }

var _ = ptrParamsRed
var _ = log.Noop
