// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// recovery_test.go — proves the auto-recovery + anti-self-DoS properties:
//
//  1. A follower that receives a block whose PARENT it does not have issues
//     exactly ONE catch-up (ancestor) request per missing parent, and is
//     idempotent (a re-gossip of the same orphan does not re-fire) — so a
//     behind node self-heals via a bounded fetch, not a fetch storm.
//  2. The proposer re-poll backs off EXPONENTIALLY and is CAPPED: the number of
//     RequestVotes over a long window is bounded (logarithmic), NOT linear in
//     time — killing the 250ms poll storm.
//  3. A stuck block past the re-poll cap is ABANDONED (no longer re-polled),
//     not re-solicited forever.
//  4. End-to-end on a multi-node relay network: a follower delivered blocks
//     OUT OF ORDER (child before parent) fetches the missing parent through the
//     catch-up seam and then finalizes the whole frontier — reconciling without
//     a manual snapshot reset.
//  5. The cert gate / VerifyWeighted still holds (a sub-α / sub-stake cert never
//     finalizes) — none of the recovery machinery weakens finality.
package chain

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// -----------------------------------------------------------------------------
// recordingCatchup — counts ancestor-fetch requests and remembers the targets.
// Models the node-layer fetch transport (Get/GetAncestors) the engine drives
// when it detects it is behind. Pure recorder; never delivers anything.
// -----------------------------------------------------------------------------

type recordingCatchup struct {
	mu      sync.Mutex
	calls   int
	missing []ids.ID
	from    []ids.NodeID
}

func (c *recordingCatchup) RequestAncestors(_ ids.ID, _ ids.ID, missingBlockID ids.ID, from ids.NodeID) error {
	c.mu.Lock()
	c.calls++
	c.missing = append(c.missing, missingBlockID)
	c.from = append(c.from, from)
	c.mu.Unlock()
	return nil
}

func (c *recordingCatchup) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *recordingCatchup) lastMissing() (ids.ID, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.missing) == 0 {
		return ids.Empty, false
	}
	return c.missing[len(c.missing)-1], true
}

var _ Catchup = (*recordingCatchup)(nil)

// -----------------------------------------------------------------------------
// 1. Missing-parent gossip ⇒ ONE catch-up request, idempotent.
// -----------------------------------------------------------------------------

func TestRecovery_MissingParentTriggersAncestorFetch(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	cu := &recordingCatchup{}
	rec := &recordingGossiper{}

	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, rec, vs.signerFor(0)),
		WithCatchup(cu),
	)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	rt := &Runtime{Transitive: e, config: NetworkConfig{
		ChainID: chainID, NetworkID: ids.GenerateTestID(), Logger: log.Noop(),
		Gossiper: &certQuorumGossiper{rec: rec}, Catchup: cu,
	}}

	// A child at height 8 whose parent (height 7) we have NEVER tracked: an orphan.
	missingParent := ids.GenerateTestID()
	child := &verifyOnceBlock{
		id: ids.GenerateTestID(), parentID: missingParent, height: 8,
		timestamp: time.Now(), bytes: []byte("orphan-child-8"),
	}
	from := vs.nodeID(1)

	rt.followVerifiedBlock(context.Background(), child, from)

	if got := cu.count(); got != 1 {
		t.Fatalf("missing-parent orphan must trigger exactly ONE ancestor fetch, got %d", got)
	}
	if m, _ := cu.lastMissing(); m != missingParent {
		t.Fatalf("catch-up must target the MISSING PARENT %s, got %s", missingParent, m)
	}

	// Re-gossip of the SAME orphan must NOT re-fire (idempotent — no fetch storm).
	rt.followVerifiedBlock(context.Background(), child, from)
	if got := cu.count(); got != 1 {
		t.Fatalf("re-gossip of same orphan must NOT re-request ancestors (idempotent), got %d", got)
	}
}

// A block whose parent IS the finalized tip (or genesis/Empty) is NOT an orphan
// and must NOT trigger any catch-up — only a genuinely-missing parent does.
func TestRecovery_PresentParentNoFetch(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	cu := &recordingCatchup{}
	rec := &recordingGossiper{}

	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, rec, vs.signerFor(0)),
		WithCatchup(cu),
	)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	rt := &Runtime{Transitive: e, config: NetworkConfig{
		ChainID: chainID, NetworkID: ids.GenerateTestID(), Logger: log.Noop(),
		Gossiper: &certQuorumGossiper{rec: rec}, Catchup: cu,
	}}

	// Genesis-extending block: parent is Empty (the initial finalized tip). Not an orphan.
	first := &verifyOnceBlock{
		id: ids.GenerateTestID(), parentID: ids.Empty, height: 1,
		timestamp: time.Now(), bytes: []byte("genesis-child-1"),
	}
	rt.followVerifiedBlock(context.Background(), first, vs.nodeID(1))

	if got := cu.count(); got != 0 {
		t.Fatalf("a block extending the finalized tip must NOT trigger a fetch, got %d", got)
	}
}

// -----------------------------------------------------------------------------
// 2. Re-poll backs off exponentially and is bounded (NOT linear).
// -----------------------------------------------------------------------------

// countingProposer counts RequestVotes calls (the proposer's re-poll = a
// network-wide PushQuery). Never delivers votes — so the block stays stuck and
// the re-poll loop keeps firing, exactly the devnet self-DoS scenario.
type countingProposer struct{ calls int64 }

func (p *countingProposer) Propose(context.Context, BlockProposal) error { return nil }
func (p *countingProposer) RequestVotes(context.Context, VoteRequest) error {
	atomic.AddInt64(&p.calls, 1)
	return nil
}
func (p *countingProposer) count() int64 { return atomic.LoadInt64(&p.calls) }

func TestRecovery_RePollBacksOffExponentially(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	prop := &countingProposer{}

	// Short base interval so the test runs fast; cap and attempt-limit are the
	// load-bearing knobs, not the absolute timing.
	p := params5()
	p.RoundTO = 10 * time.Millisecond
	p.BlockTime = 50 * time.Millisecond // keep the finality poll loop slow/irrelevant here

	e := NewWithConfig(Config{Params: p},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
	)
	e.SetProposer(prop)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	// Track a stuck own-proposal (only the self-vote; never reaches α=4).
	blk := newTestBlock(1, ids.Empty, "stuck-forever")
	_ = trackProposal(e, chainID, blk, 0)

	// Run for ~50 base intervals of wall time. With a FIXED 10ms re-poll this
	// would be ~50 RequestVotes; with exponential backoff + a small attempt cap
	// it must be O(cap) — we assert a hard ceiling far below the linear count.
	window := 50 * p.RoundTO // 500ms
	time.Sleep(window)

	got := prop.count()
	// Linear (a fixed-cadence storm) would be ~ window/RoundTO = 50. EXPONENTIAL
	// backoff (10,20,40,80,160,…) makes the count GEOMETRIC in the window —
	// log2(window/RoundTO) ≈ log2(50) ≈ 6 — so it is bounded far below linear. This
	// is the storm-safety invariant, and it does NOT depend on the attempt cap: an
	// own proposal is re-solicited until it decides (never abandoned), but the
	// backoff keeps that a slow trickle, not a 250ms storm. Ceiling 12 is a safe
	// geometric bound for this window, still ≪ the linear 50.
	const geometricCeiling = 12
	if got > geometricCeiling {
		t.Fatalf("re-poll must be bounded by exponential backoff (geometric ≈%d), got %d over %v — poll storm not fixed",
			geometricCeiling, got, window)
	}
	if got == 0 {
		t.Fatalf("liveness: a stuck own block must be re-polled at least once, got 0")
	}
	t.Logf("re-poll fired %d times over %v (linear would be ~%d) — bounded", got, window, int64(window/p.RoundTO))
}

// -----------------------------------------------------------------------------
// 3. A stuck block past the cap is abandoned (no infinite re-poll), and the cap
//    is per-block: a DIFFERENT stuck block still gets its own attempts.
// -----------------------------------------------------------------------------

// TestRecovery_OwnProposalNeverAbandoned_GossipedBlockAbandoned proves the
// per-source re-poll policy that fixes the down/wedged/forked-proposer halt while
// keeping the gossip-spam guard:
//
//   - An UNDECIDED OWN proposal is NEVER abandoned. This node built it on the
//     finalized tip (its voters HAVE the parent and CAN vote), so the proposer must
//     keep re-soliciting until it finalizes — matching avalanchego (re-poll a
//     processing block until decided). Abandoning it was the mainnet halt.
//   - A NON-OWN (gossiped) block whose voters may be behind its parent STILL
//     abandons after maxRePollAttempts: re-soliciting it is spam that re-poll can't
//     fix (catch-up recovers it). The cap remains for that path.
//
// Driven synchronously (long RoundTO parks the background ticker) so the per-block
// attempt budget is exercised deterministically.
func TestRecovery_OwnProposalNeverAbandoned_GossipedBlockAbandoned(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	prop := &countingProposer{}

	p := params5()
	p.RoundTO = 10 * time.Second // park the background ticker; drive re-poll manually
	p.BlockTime = 50 * time.Millisecond

	e := NewWithConfig(Config{Params: p},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
	)
	e.SetProposer(prop)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	// Own proposal (built locally): only the self-vote, never reaches α — but it is
	// canonical and must keep being driven to finality.
	own := newTestBlock(1, ids.Empty, "own-stuck")
	_ = trackProposal(e, chainID, own, 0)

	// Gossiped follower block (IsOwnProposal=false): models a block received from an
	// ahead peer whose voters we lack — re-poll can't recover it, so it abandons.
	gossiped := newTestBlock(2, ids.Empty, "gossiped-stuck")
	cb := &Block{id: gossiped.id, parentID: gossiped.parentID, height: gossiped.height, timestamp: gossiped.timestamp.Unix(), data: gossiped.bytes}
	_ = e.consensus.AddBlock(context.Background(), cb)
	e.mu.Lock()
	e.pendingBlocks[gossiped.id] = &PendingBlock{
		ConsensusBlock: cb, VMBlock: gossiped, ProposedAt: time.Now(),
		VoteCount: 0, Decided: false, IsOwnProposal: false,
	}
	e.mu.Unlock()

	// Drive far past the attempt cap.
	driveRePoll(e, maxRePollAttempts*4)

	e.mu.RLock()
	ownPB, ownOK := e.pendingBlocks[own.id]
	gosPB, gosOK := e.pendingBlocks[gossiped.id]
	ownAbandoned := ownOK && ownPB.rePollAbandoned
	gosAbandoned := gosOK && gosPB.rePollAbandoned
	ownAttempts := 0
	if ownOK {
		ownAttempts = ownPB.rePollAttempts
	}
	e.mu.RUnlock()

	if !ownOK || !gosOK {
		t.Fatal("both stuck blocks must remain pending (recoverable), not deleted")
	}
	// LIVENESS: the own proposal is never abandoned and is re-solicited past the cap.
	if ownAbandoned {
		t.Fatalf("LIVENESS HALT: own proposal was abandoned for re-poll (attempts=%d) — a substitute's "+
			"canonical block must be re-solicited until it decides, never given up on.", ownAttempts)
	}
	if ownAttempts <= maxRePollAttempts {
		t.Fatalf("own proposal must keep being re-solicited past the cap, attempts=%d", ownAttempts)
	}
	// SPAM GUARD: the gossiped block still abandons after the cap.
	if !gosAbandoned {
		t.Fatal("a gossiped (non-own) block must still be abandoned after the attempt cap — the spam guard " +
			"for a block whose voters are behind its parent must remain.")
	}
}

// -----------------------------------------------------------------------------
// 4. End-to-end: out-of-order delivery on a relay network self-heals.
//    Follower gets child(2) before parent(1); the catch-up seam delivers the
//    missing parent, and the follower then finalizes BOTH via cert gossip.
// -----------------------------------------------------------------------------

func TestRecovery_OutOfOrderFollowerCatchesUpAndFinalizes(t *testing.T) {
	net := newRelayNetwork(t, 5, params5())

	// Build a 2-block chain at the proposer (engine 0): genesis-child A(1) then B(2).
	a := &verifyOnceBlock{id: ids.GenerateTestID(), parentID: ids.Empty, height: 1, timestamp: time.Now(), bytes: []byte("A1")}
	b := &verifyOnceBlock{id: ids.GenerateTestID(), parentID: a.id, height: 2, timestamp: time.Now(), bytes: []byte("B2")}

	// A catch-up seam that, when asked for a missing ancestor, re-delivers that
	// block to the asking follower via followVerifiedBlock (models GetAncestors→Put).
	store := map[ids.ID]*verifyOnceBlock{a.id: a, b.id: b}
	cu := &relayCatchup{net: net, store: store}
	for _, rt := range net.engines {
		rt.config.Catchup = cu
		rt.Transitive.catchup = cu
	}

	follower := net.engines[1]

	// Deliver B(2) FIRST to the follower — its parent A(1) is missing ⇒ orphan.
	follower.followVerifiedBlock(context.Background(), b, net.vs.nodeID(0))

	// The catch-up seam must have fetched A(1) (the missing parent) and, once A is
	// present, B is no longer an orphan. Drive votes so both finalize.
	// Proposer assembles certs for A and B from follower votes; relayGossiper
	// distributes them. We simulate the α votes arriving for both heights.
	posA := VotePosition{ChainID: net.chainID, Height: 1, Round: 0, BlockID: a.id, ParentID: ids.Empty}
	posB := VotePosition{ChainID: net.chainID, Height: 2, Round: 0, BlockID: b.id, ParentID: a.id}

	// Engine 0 tracks both as own proposals so it can assemble+gossip certs.
	net.trackOwnProposal(0, a)
	deliverVotes(net, 0, posA, []int{1, 2, 3})
	// After A finalizes everywhere, track + finalize B.
	if !waitFor(2*time.Second, func() bool { return follower.IsAccepted(a.id) }) {
		t.Fatal("follower did not finalize A(1) after catch-up + votes")
	}
	net.trackOwnProposal(0, b)
	deliverVotes(net, 0, posB, []int{1, 2, 3})
	if !waitFor(2*time.Second, func() bool { return follower.IsAccepted(b.id) }) {
		t.Fatal("follower did not finalize B(2) after its parent was caught up")
	}

	// The catch-up seam must have been exercised (the missing parent was fetched).
	if cu.fetches() == 0 {
		t.Fatal("expected the catch-up seam to fetch the missing parent at least once")
	}
}

// relayCatchup re-delivers a requested missing block to the asking follower via
// followVerifiedBlock — modelling the node-layer GetAncestors→Put round-trip.
type relayCatchup struct {
	net     *relayNetwork
	store   map[ids.ID]*verifyOnceBlock
	mu      sync.Mutex
	count   int
}

func (c *relayCatchup) RequestAncestors(_ ids.ID, _ ids.ID, missingBlockID ids.ID, from ids.NodeID) error {
	c.mu.Lock()
	c.count++
	blk := c.store[missingBlockID]
	c.mu.Unlock()
	if blk == nil {
		return nil
	}
	// Deliver to whichever follower asked. We find it by the `from` being the
	// proposer; deliver to all non-proposer engines (the asking follower included).
	for i, rt := range c.net.engines {
		if i == 0 || rt == nil {
			continue
		}
		rt.followVerifiedBlock(context.Background(), blk, from)
	}
	return nil
}

func (c *relayCatchup) fetches() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

var _ Catchup = (*relayCatchup)(nil)

// deliverVotes feeds α follower votes for pos to the proposer (engine `proposer`)
// so it assembles + gossips the cert, finalizing the block network-wide.
func deliverVotes(net *relayNetwork, proposer int, pos VotePosition, voters []int) {
	rt := net.engines[proposer]
	for _, v := range voters {
		vb, err := encodeSignedVote(net.vs.nodeID(v), net.vs.sign(v, pos))
		if err != nil {
			continue
		}
		rt.HandleIncomingVote(pos.BlockID, vb)
	}
}

// -----------------------------------------------------------------------------
// 5. Recovery machinery does NOT weaken the cert gate: a sub-α cert never
//    finalizes even with the catch-up + re-poll paths active.
// -----------------------------------------------------------------------------

func TestRecovery_CertGateStillHolds_SubAlphaNeverFinalizes(t *testing.T) {
	vs := newTestValidatorSet(5)
	cu := &recordingCatchup{}
	e, chainID := newQuorumEngineOpts(t, params5(), vs, 0, &recordingGossiper{}, WithCatchup(cu))

	blk := newTestBlock(1, ids.Empty, "sub-alpha")
	pos := trackProposal(e, chainID, blk, 0)

	// Only 2 distinct signed votes (self + one peer) — below α=4. Even with the
	// re-poll loop running, this must NEVER finalize.
	vb, _ := encodeSignedVote(vs.nodeID(1), vs.sign(1, pos))
	rt := &Runtime{Transitive: e, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}
	rt.HandleIncomingVote(blk.id, vb)

	if waitFor(300*time.Millisecond, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("SAFETY: a sub-alpha block finalized — the cert gate was weakened by the recovery path")
	}
	_ = config.Parameters{}
}
