// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// proposer_liveness_test.go — the DURABLE regression suite for the
// down/wedged/forked-designated-proposer LIVENESS halt (Lux mainnet C-Chain,
// stuck producing 1082825 above a finalized 1082824).
//
// ROOT CAUSE (in the engine's terms): a height's designated proposer is
// down/wedged/FORKED (present, sampled, gossiping, but it never produces a
// canonical-extending block that finalizes). A SUBSTITUTE (the next slot's
// designated proposer) builds the canonical block, self-votes, and solicits peer
// votes ONCE. If the α-of-K signed votes do not assemble on that first
// solicitation — the common case when a follower is briefly behind, or the first
// PushQuery is dropped, or (the mainnet reality) one of the five validators is a
// permanent non-voter so the OTHER four must ALL vote with ZERO margin — nothing
// re-solicits the missing votes once `rePollAllPending` ABANDONS the block after
// `maxRePollAttempts` (the Lux-only divergence from avalanchego, which re-polls a
// processing block until it decides and only quiesces at NumProcessing()==0). The
// block then sits in pendingBlocks, never re-solicited, and the chain HALTS.
//
// THE FIX (proven here): an UNDECIDED OWN proposal is NEVER abandoned for re-poll.
// It is re-solicited forever on the bounded (capped) backoff schedule until it
// decides (and is removed from pendingBlocks, so the re-poll naturally quiesces) —
// matching avalanchego's "re-poll while processing, quiesce when decided". This is
// a pure LIVENESS retry: it changes NOTHING about the finality predicate (a block
// still finalizes ONLY on a verified α-of-K cert that clears the stake gate), so
// the safety tests below (sub-quorum, forged, out-of-set, tampered) still reject.
//
// fails-before / passes-after is shown for every behavior: the pre-fix abandon is
// asserted RED, the post-fix persistence GREEN.
package chain

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// params5Prod returns the PRODUCTION 5-validator quorum params: K=5, α=4
// (⌊2·5/3⌋+1 — what bftCommittee derives for a 5-node value network, see
// dynamic_test.go which asserts K5/α4). The finality_test.go params5() uses α=3
// for legacy count tests; the liveness/safety properties here must hold at the
// REAL production threshold where the 4 healthy nodes are the EXACT quorum (zero
// margin — the mainnet condition after a 5th validator forks/wedges).
func params5Prod() config.Parameters {
	p := params5()
	p.AlphaPreference = 4
	p.AlphaConfidence = 4
	return p
}

// reSolicitProbe is a BlockProposer that records how many times the engine
// re-solicits votes for each block (RequestVotes), and fires an optional hook on
// each solicitation. It is the instrument that makes the re-poll behavior
// observable: pre-fix the count plateaus at the abandon cap; post-fix it grows
// without bound while the block is undecided. The hook models a follower that
// only votes once it is (re-)solicited — the production reality, where a node that
// missed the first gossip casts its vote only when the proposer re-sends it.
type reSolicitProbe struct {
	mu        sync.Mutex
	requests  map[ids.ID]int
	proposed  map[ids.ID]int
	onRequest func(blockID ids.ID, cumulative int)
}

func newReSolicitProbe() *reSolicitProbe {
	return &reSolicitProbe{requests: map[ids.ID]int{}, proposed: map[ids.ID]int{}}
}

func (p *reSolicitProbe) Propose(_ context.Context, proposal BlockProposal) error {
	p.mu.Lock()
	p.proposed[proposal.BlockID]++
	p.mu.Unlock()
	return nil
}

func (p *reSolicitProbe) RequestVotes(_ context.Context, req VoteRequest) error {
	p.mu.Lock()
	p.requests[req.BlockID]++
	n := p.requests[req.BlockID]
	hook := p.onRequest
	p.mu.Unlock()
	if hook != nil {
		hook(req.BlockID, n)
	}
	return nil
}

func (p *reSolicitProbe) requestCount(id ids.ID) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests[id]
}

// driveRePoll forces `cycles` re-poll passes over the engine's pending blocks
// synchronously, each pass made "due" by resetting every undecided block's backoff
// window to the distant past. This drives the re-poll attempt counter deterministically
// WITHOUT waiting on the exponential backoff (so the test is fast and not timing-flaky),
// and is isolated from the background ticker by the caller using a long RoundTO so the
// background loop never fires during the synchronous drive.
func driveRePoll(e *Transitive, cycles int) {
	for i := 0; i < cycles; i++ {
		e.mu.Lock()
		for _, pb := range e.pendingBlocks {
			if pb.Decided {
				continue
			}
			pb.lastRePoll = time.Now().Add(-time.Hour)
			pb.rePollBackoff = time.Nanosecond
		}
		e.mu.Unlock()
		e.rePollAllPending(context.Background(), time.Millisecond)
	}
}

// -----------------------------------------------------------------------------
// LIVENESS — the core bug. An undecided own proposal must be re-solicited
// forever, never abandoned, so the substitute's canonical block finalizes once
// the honest majority's votes arrive.
// -----------------------------------------------------------------------------

// TestProposerLiveness_UndecidedOwnProposal_NeverAbandoned is the DIRECT
// fails-before/passes-after proof of the fix. A substitute proposer (node 0)
// builds the canonical next block with only a sub-quorum of votes present (the
// wedged/forked 5th validator never votes, and a 4th honest vote has not yet
// arrived). The engine must keep re-soliciting it forever.
//
// PRE-FIX: rePollAllPending sets rePollAbandoned after maxRePollAttempts (8), then
// SKIPS the block — RequestVotes plateaus near 8 and the block is never re-solicited
// again → permanent halt.
// POST-FIX: the undecided own proposal is never abandoned; RequestVotes keeps firing
// every pass.
func TestProposerLiveness_UndecidedOwnProposal_NeverAbandoned(t *testing.T) {
	vs := newTestValidatorSet(5)
	params := params5Prod()
	params.RoundTO = 10 * time.Second // park the background ticker; we drive re-poll manually
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params, vs, 0, rec)

	cp := newReSolicitProbe()
	e.SetProposer(cp)

	// Node 0 is the substitute: it builds the canonical block at the stuck height.
	blk := newTestBlock(1, ids.Empty, "substitute-canonical")
	pos := trackProposal(e, chainID, blk, 0)
	// Only one peer (node 1) has voted so far → 2 of 5, below α=4. The block is a
	// genuine, healthy, undecided own proposal awaiting the rest of the honest quorum.
	e.ReceiveVote(vs.signedVote(1, pos))

	const cycles = 40 // >> maxRePollAttempts (8)
	driveRePoll(e, cycles)

	e.mu.RLock()
	pb := e.pendingBlocks[blk.id]
	abandoned := pb != nil && pb.rePollAbandoned
	e.mu.RUnlock()
	reqs := cp.requestCount(blk.id)

	if abandoned {
		t.Fatalf("LIVENESS HALT: undecided own proposal was ABANDONED for re-poll after %d attempts "+
			"(rePollAbandoned=true) — the substitute's canonical block stops being re-solicited and the "+
			"chain halts forever. avalanchego re-polls a processing block until it decides.", reqs)
	}
	if reqs <= maxRePollAttempts {
		t.Fatalf("re-solicitation stopped early: RequestVotes fired only %d times over %d passes "+
			"(expected continuous re-solicitation > maxRePollAttempts=%d). A block awaiting the honest "+
			"quorum must keep being solicited.", reqs, cycles, maxRePollAttempts)
	}
}

// TestProposerLiveness_LateHonestVote_FinalizesViaPersistentRePoll is the
// EMERGENT proof: a 4th honest voter (node 3) only casts its vote in RESPONSE to a
// re-solicitation that arrives AFTER the pre-fix abandon point (it was briefly
// behind). The block finalizes iff the proposer keeps re-soliciting past that
// point — i.e. only post-fix. The wedged/forked 5th validator (node 4) never votes.
func TestProposerLiveness_LateHonestVote_FinalizesViaPersistentRePoll(t *testing.T) {
	vs := newTestValidatorSet(5)
	params := params5Prod()
	params.RoundTO = 10 * time.Second
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params, vs, 0, rec)

	cp := newReSolicitProbe()
	e.SetProposer(cp)

	blk := newTestBlock(7, ids.Empty, "late-quorum")
	pos := trackProposal(e, chainID, blk, 0) // node 0 self-vote = 1
	e.ReceiveVote(vs.signedVote(1, pos))     // node 1
	e.ReceiveVote(vs.signedVote(2, pos))     // node 2  → 3 of 5, still < α=4

	// Node 3 is briefly behind: it casts its (4th, quorum-completing) vote ONLY once
	// the proposer has re-solicited past the abandon point. Node 4 (wedged/forked)
	// never votes.
	const lateSolicit = maxRePollAttempts + 4 // 12: only reachable if re-poll never abandons
	var once sync.Once
	cp.onRequest = func(id ids.ID, n int) {
		if id == blk.id && n >= lateSolicit {
			once.Do(func() { e.ReceiveVote(vs.signedVote(3, pos)) })
		}
	}

	// Drive re-poll well past both the abandon cap and the late-solicit threshold.
	driveRePoll(e, 60)

	finalized := waitFor(3*time.Second, func() bool { return e.IsAccepted(blk.id) })
	reqs := cp.requestCount(blk.id)
	if !finalized {
		t.Fatalf("LIVENESS HALT: block never finalized. RequestVotes fired %d times; the late honest "+
			"vote is only solicited at attempt %d, but the pre-fix abandon caps re-poll at "+
			"maxRePollAttempts=%d, so node 3 is never re-solicited and the 4-of-5 quorum never assembles.",
			reqs, lateSolicit, maxRePollAttempts)
	}
}

// -----------------------------------------------------------------------------
// SAFETY — the fix must NOT lower the BFT threshold. Persistent re-solicitation
// re-sends a block; it can NEVER manufacture a vote, so a sub-quorum, a forged
// vote, or an out-of-set signer still cannot finalize.
// -----------------------------------------------------------------------------

// TestProposerSafety_SubQuorumNeverFinalizes_EvenWithPersistentRePoll proves the
// liveness retry does not weaken safety: 3 of 5 (< α=4) is re-solicited forever and
// STILL never finalizes. A genuine minority cannot finalize no matter how long it
// re-polls.
func TestProposerSafety_SubQuorumNeverFinalizes_EvenWithPersistentRePoll(t *testing.T) {
	vs := newTestValidatorSet(5)
	params := params5Prod()
	params.RoundTO = 10 * time.Second
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params, vs, 0, rec)
	e.SetProposer(newReSolicitProbe())

	blk := newTestBlock(3, ids.Empty, "minority")
	pos := trackProposal(e, chainID, blk, 0) // self = 1
	e.ReceiveVote(vs.signedVote(1, pos))     // 2
	e.ReceiveVote(vs.signedVote(2, pos))     // 3 of 5 < α=4

	driveRePoll(e, 60) // re-solicit far past the old abandon cap

	if waitFor(500*time.Millisecond, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("SAFETY VIOLATION: a sub-quorum (3 of 5, below α=4) finalized — persistent re-poll " +
			"must re-SOLICIT only; it must never lower the α-of-K threshold.")
	}
	rec.mu.Lock()
	certs := len(rec.certs)
	rec.mu.Unlock()
	if certs != 0 {
		t.Fatalf("no cert may be produced below α; got %d", certs)
	}
}

// TestProposerSafety_ForgedAndOutOfSetVotes_NeverCount proves re-solicitation
// cannot be exploited: an out-of-set signer and a real validator's signature lifted
// to the WRONG position are both dropped, so even re-solicited forever the block
// stays at its 3 genuine votes (< α) and never finalizes.
func TestProposerSafety_ForgedAndOutOfSetVotes_NeverCount(t *testing.T) {
	vs := newTestValidatorSet(5)
	params := params5Prod()
	params.RoundTO = 10 * time.Second
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, params, vs, 0, rec)
	e.SetProposer(newReSolicitProbe())

	blk := newTestBlock(9, ids.Empty, "forged-attack")
	pos := trackProposal(e, chainID, blk, 0) // self = 1
	e.ReceiveVote(vs.signedVote(1, pos))     // 2
	e.ReceiveVote(vs.signedVote(2, pos))     // 3 (genuine)

	// (a) out-of-set signer: keys unknown to vs.
	outsider := newTestValidatorSet(1)
	e.ReceiveVote(Vote{
		BlockID: blk.id, NodeID: outsider.nodeID(0), Accept: true, SignedAt: time.Now(),
		Signature: outsider.sign(0, pos), ParentID: pos.ParentID, Round: pos.Round,
	})
	// (b) real validator (node 3) signature over a DIFFERENT (wrong) position.
	wrong := pos
	wrong.Height++
	e.ReceiveVote(Vote{
		BlockID: blk.id, NodeID: vs.nodeID(3), Accept: true, SignedAt: time.Now(),
		Signature: vs.sign(3, wrong), ParentID: pos.ParentID, Round: pos.Round,
	})

	driveRePoll(e, 60)

	if waitFor(500*time.Millisecond, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("SAFETY VIOLATION: forged/out-of-set/wrong-position votes were counted toward the quorum.")
	}
}
