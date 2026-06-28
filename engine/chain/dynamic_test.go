// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_dynamic_test.go — proves the DYNAMIC live-set committee (K=N, α=strict-⅔)
// actually finalizes under the engine: 4-of-5 finalizes, 3-of-5 is rejected, one
// laggard is tolerated, two laggards never produce false finality, the proposer's
// self-vote counts, and the re-poll loop recovers a wedged first poll (the devnet
// fix). The committee comes from consensusconfig.FeasibleParams — the SAME path
// the node uses — so these exercise the shipped sizing, not a test constant.
package chain

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/constants"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// dyn5 is the dynamic five-validator committee the node selects today: K=5/α=4
// from FeasibleParams (mainnet/testnet/devnet all collapse to this). Using the
// real sizer proves the params under test are the ones that ship.
func dyn5() config.Parameters { return config.FeasibleParams(constants.LocalID, 5) }

// TestFiveValidatorsAlphaFourFinalizes — the fast path: with the dynamic K=5/α=4
// committee, the proposer's self-vote plus three peer accepts (4 distinct) is a
// quorum and the block finalizes with a gossiped cert. This is the setting all
// three live nets run today.
func TestFiveValidatorsAlphaFourFinalizes(t *testing.T) {
	p := dyn5()
	if p.K != 5 || p.AlphaPreference != 4 {
		t.Fatalf("dynamic five-validator params = K%d/α%d, want K5/α4", p.K, p.AlphaPreference)
	}
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, p, vs, 0, rec)

	blk := newTestBlock(1, ids.Empty, "alpha4")
	pos := trackProposal(e, chainID, blk, 0) // proposer (vdr 0) self-vote recorded

	// Three peer accepts → proposer + 3 = 4 distinct = α. MUST finalize. Wait on
	// VM.Accept (the actual commit), which follows the count flip by a hair.
	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos))
	e.ReceiveVote(vs.signedVote(3, pos))

	if !waitFor(2*time.Second, func() bool { return blk.AcceptCalled() >= 1 }) {
		t.Fatal("LIVENESS: K=5/α=4 block did not finalize after 4 distinct signed accepts")
	}
	if !e.IsAccepted(blk.id) {
		t.Fatal("block must be marked accepted at quorum")
	}
	if blk.AcceptCalled() != 1 {
		t.Fatalf("VM.Accept must run exactly once at quorum, got %d", blk.AcceptCalled())
	}
	rec.mu.Lock()
	gotCerts := len(rec.certs)
	rec.mu.Unlock()
	if gotCerts == 0 {
		t.Fatal("a verified α-of-K cert must be assembled and gossiped at finality")
	}
}

// TestThreeOfFiveRejected proves 3-of-5 is NON-finalizing under the dynamic
// committee — exactly what the spec requires (3/5 = 60% < strict ⅔, and the cert
// AND Parameters.Valid AND the BFT overlap bound all reject it). Proposer + 2
// peers = 3 distinct < α=4 → never accepts.
func TestThreeOfFiveRejected(t *testing.T) {
	p := dyn5()
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, p, vs, 0, rec)

	blk := newTestBlock(1, ids.Empty, "three-of-five")
	pos := trackProposal(e, chainID, blk, 0)

	// Two peer accepts → proposer + 2 = 3 distinct < α=4.
	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos))

	if waitFor(400*time.Millisecond, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("SAFETY VIOLATION: block finalized with only 3-of-5 (below α=4 strict-⅔ quorum)")
	}
	if blk.AcceptCalled() != 0 {
		t.Fatalf("VM.Accept must NOT run below quorum, got %d", blk.AcceptCalled())
	}
	// And the strict-⅔ floor confirms 3 units of stake do NOT exceed floor(2·5/3)=3.
	if 3 > config.TwoThirdsStakeFloor(5) {
		t.Fatal("3-of-5 must NOT strictly exceed the ⅔ stake floor")
	}
}

// TestOneLaggardStillFinalizes proves the dynamic α=4 tolerates ONE
// non-responsive validator: 4 of 5 vote (one laggard silent), quorum reached,
// block finalizes. This is the liveness slack K=N buys — the retired oversized
// K could not tolerate even one laggard on a 5-set.
func TestOneLaggardStillFinalizes(t *testing.T) {
	p := dyn5()
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, p, vs, 0, rec)

	blk := newTestBlock(1, ids.Empty, "one-laggard")
	pos := trackProposal(e, chainID, blk, 0) // proposer = vdr 0

	// Validators 1,2,3 vote; validator 4 is the LAGGARD (never votes). Proposer +
	// 3 = 4 = α. MUST finalize despite the silent validator.
	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos))
	e.ReceiveVote(vs.signedVote(3, pos))

	if !waitFor(2*time.Second, func() bool { return blk.AcceptCalled() >= 1 }) {
		t.Fatal("LIVENESS: block did not finalize with one laggard (4-of-5 is a quorum)")
	}
	if blk.AcceptCalled() != 1 {
		t.Fatalf("VM.Accept exactly once with one laggard, got %d", blk.AcceptCalled())
	}
}

// TestTwoLaggardsNoFalseFinality proves two non-responsive validators make the
// quorum genuinely unreachable: proposer + 2 = 3 distinct < α=4, so the block
// must NOT finalize. The engine never fabricates finality from a minority — a
// real ⅓+ outage correctly halts (safety over liveness).
func TestTwoLaggardsNoFalseFinality(t *testing.T) {
	p := dyn5()
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, p, vs, 0, rec)

	blk := newTestBlock(1, ids.Empty, "two-laggards")
	pos := trackProposal(e, chainID, blk, 0)

	// Only validators 1,2 vote; 3 and 4 are laggards. Proposer + 2 = 3 < α=4.
	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos))

	// Give the re-poll loop several rounds to run — it must NOT manufacture
	// finality from a sub-quorum (re-poll is liveness retry, never a force).
	if waitFor(600*time.Millisecond, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("SAFETY VIOLATION: block finalized with two laggards (3-of-5 < α=4) — re-poll must not force")
	}
	if blk.AcceptCalled() != 0 {
		t.Fatalf("VM.Accept must NOT run under a real ⅓+ outage, got %d", blk.AcceptCalled())
	}
	rec.mu.Lock()
	gotCerts := len(rec.certs)
	rec.mu.Unlock()
	if gotCerts != 0 {
		t.Fatalf("no cert may be assembled below quorum, got %d", gotCerts)
	}
}

// TestSelfVoteCounts proves the proposer's OWN signed accept is one of the α
// voters: with α=4, the proposer self-vote + exactly THREE peer votes finalizes
// (4 total). If the self-vote did NOT count, three peers would be only 3 < α and
// the block would hang — so finalizing here proves the self-vote is counted (the
// integration.go followVerifiedBlock → ReceiveVote(ownVote) wiring, kept).
func TestSelfVoteCounts(t *testing.T) {
	p := dyn5()
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngine(t, p, vs, 0, rec)

	blk := newTestBlock(1, ids.Empty, "self-vote")
	pos := trackProposal(e, chainID, blk, 0)

	// The proposer's self-vote is recorded by trackProposal (mirrors
	// buildBlocksLocked). Add exactly THREE peer votes → 4 total ONLY IF the self
	// vote counts.
	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos))
	e.ReceiveVote(vs.signedVote(3, pos))

	if !waitFor(2*time.Second, func() bool { return blk.AcceptCalled() >= 1 }) {
		t.Fatal("self-vote not counted: proposer + 3 peers should be α=4 and finalize")
	}

	// The assembled cert must INCLUDE the proposer's own NodeID as a voter.
	e.mu.RLock()
	pending := e.pendingBlocks[blk.id]
	var cert *QuorumCert
	if pending != nil {
		cert = pending.cert
	}
	// pending may already be deleted post-finalize; fall back to the gossiped cert.
	e.mu.RUnlock()
	if cert == nil {
		rec.mu.Lock()
		if len(rec.certs) > 0 {
			c, err := UnmarshalQuorumCert(rec.certs[len(rec.certs)-1])
			if err == nil {
				cert = c
			}
		}
		rec.mu.Unlock()
	}
	if cert == nil {
		t.Fatal("expected an assembled/gossiped cert to audit the proposer's self-vote")
	}
	sawProposer := false
	for i := range cert.Votes {
		if cert.Votes[i].NodeID == vs.nodeID(0) {
			sawProposer = true
		}
	}
	if !sawProposer {
		t.Fatal("proposer's own NodeID must appear as a voter in the finality cert")
	}
}

// -----------------------------------------------------------------------------
// relayNetwork — N Runtime engines in one process wired into a real
// vote/cert/poll topology: a counting BlockProposer (relayProposer) models the
// proposer's RequestVotes (first poll droppable), a follower's HandleIncomingVote
// reaches every peer, and an assembled GossipCert reaches every peer's
// HandleIncomingCert. This is the multi-node reproduction the devnet
// investigation needs — it exercises the re-poll path end to end.
// -----------------------------------------------------------------------------

type relayNetwork struct {
	mu      sync.Mutex
	engines []*Runtime
	chainID ids.ID
	vs      *testValidatorSet
}

// relayGossiper routes one engine's assembled finality cert to every peer's
// HandleIncomingCert (the engine-level CertGossiper the proposer uses in
// tryFinalizeBlock).
type relayGossiper struct {
	net  *relayNetwork
	self int
}

func (g *relayGossiper) GossipCert(_ ids.ID, _ ids.ID, certBytes []byte) error {
	g.net.mu.Lock()
	peers := g.net.engines
	g.net.mu.Unlock()
	for i, rt := range peers {
		if i == g.self || rt == nil {
			continue
		}
		_ = rt.HandleIncomingCert(certBytes)
	}
	return nil
}

var _ CertGossiper = (*relayGossiper)(nil)

// relayProposer is a counting BlockProposer that models the proposer's poll.
// The FIRST RequestVotes is dropped (no votes delivered) — the wedged first poll
// at genesis. The SECOND and later RequestVotes (the re-poll) deliver the peer
// votes: each follower's signed accept is fed to the proposer's HandleIncomingVote
// exactly as a re-poll-driven re-broadcast would. So a stuck block recovers IFF
// the re-poll loop issues a second RequestVotes — which is precisely the fix.
type relayProposer struct {
	net       *relayNetwork
	self      int
	pos       VotePosition
	voters    []int // follower indices that vote when polled (on the re-poll)
	mu        sync.Mutex
	calls     int
	delivered bool
}

func (p *relayProposer) Propose(context.Context, BlockProposal) error { return nil }

// RequestVotes counts polls. First poll: drop (wedge). Re-poll: deliver the
// follower votes to the proposer's HandleIncomingVote (idempotent — dedup by
// NodeID in the cert/consensus layers means re-delivery is safe).
func (p *relayProposer) RequestVotes(_ context.Context, req VoteRequest) error {
	p.mu.Lock()
	p.calls++
	call := p.calls
	already := p.delivered
	p.mu.Unlock()

	if call <= 1 || already {
		return nil // first poll is dropped; later polls deliver once
	}

	rt := p.net.engines[p.self]
	for _, v := range p.voters {
		vb, err := encodeSignedVote(p.net.vs.nodeID(v), p.net.vs.sign(v, p.pos))
		if err != nil {
			continue
		}
		rt.HandleIncomingVote(req.BlockID, vb)
	}
	p.mu.Lock()
	p.delivered = true
	p.mu.Unlock()
	return nil
}

func (p *relayProposer) pollCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// newRelayNetwork builds n started Runtime engines sharing one validator set and
// chain, each wired with a relayGossiper.
func newRelayNetwork(t *testing.T, n int, p config.Parameters) *relayNetwork {
	t.Helper()
	net := &relayNetwork{chainID: ids.GenerateTestID(), vs: newTestValidatorSet(n)}
	net.engines = make([]*Runtime, n)
	for i := 0; i < n; i++ {
		g := &relayGossiper{net: net, self: i}
		e := NewWithConfig(Config{Params: p},
			WithQuorumCert(net.chainID, net.vs.nodeID(i), net.vs, g, net.vs.signerFor(i)))
		if err := e.Start(context.Background(), true); err != nil {
			t.Fatalf("engine %d Start: %v", i, err)
		}
		rt := &Runtime{Transitive: e, config: NetworkConfig{ChainID: net.chainID, Logger: log.Noop()}}
		net.engines[i] = rt
		t.Cleanup(func() { _ = e.Stop(context.Background()) })
	}
	return net
}

// trackOwnProposal inserts a verified own-proposal pending block into engine i
// (records its self-vote, mirrors buildBlocksLocked). Returns the vote position.
func (net *relayNetwork) trackOwnProposal(i int, blk *verifyOnceBlock) VotePosition {
	e := net.engines[i].Transitive
	cb := &Block{id: blk.id, parentID: blk.parentID, height: blk.height, timestamp: blk.timestamp.Unix(), data: blk.bytes}
	_ = e.consensus.AddBlock(context.Background(), cb)
	_ = e.consensus.ProcessVote(context.Background(), blk.id, true)
	e.mu.Lock()
	pb := &PendingBlock{ConsensusBlock: cb, VMBlock: blk, ProposedAt: time.Now(), Round: 0, IsOwnProposal: true, VoteCount: 1}
	e.pendingBlocks[blk.id] = pb
	e.recordOwnVoteLocked(pb, blk.id)
	e.mu.Unlock()
	return VotePosition{ChainID: net.chainID, Height: blk.height, Round: 0, BlockID: blk.id, ParentID: blk.parentID}
}

// TestVoteRepollPreventsStall is the devnet-wedge reproduction + fix, in the
// relay network. A proposer builds a block; its FIRST poll is dropped (no peer
// votes — the genesis stall, where the single RequestVotes was lost and nothing
// re-solicited). With only its self-vote the block is wedged (1 < α=4). The
// re-poll loop fires a SECOND RequestVotes; the relayProposer then delivers the
// peer votes and the block finalizes via a real α-of-K cert. Pre-fix (no re-poll)
// the second poll never happens and the block hangs forever — this test fails.
func TestVoteRepollPreventsStall(t *testing.T) {
	p := config.FeasibleParams(constants.LocalID, 5) // K=5/α=4, RoundTO=5ms
	net := newRelayNetwork(t, 5, p)

	blk := newTestBlock(1, ids.Empty, "repoll-stall")
	pos := net.trackOwnProposal(0, blk)

	// Wire the counting proposer onto engine 0. Followers 1,2,3 vote on the re-poll
	// (proposer self + 3 = 4 = α). The first poll is dropped.
	rp := &relayProposer{net: net, self: 0, pos: pos, voters: []int{1, 2, 3}}
	proposer := net.engines[0].Transitive
	proposer.mu.Lock()
	proposer.proposer = rp
	proposer.mu.Unlock()

	// Simulate the proposer's ONE post-proposal poll being dropped (the
	// relayProposer drops its first RequestVotes — no votes delivered). At this
	// instant the block has ONLY the proposer's self-vote (1 < α=4): it is wedged.
	// The ONLY way it can ever finalize is a SECOND RequestVotes — which the engine
	// issues exclusively from the re-poll loop. Pre-fix that loop did not exist, so
	// the block would hang here forever (the devnet stall).
	_ = rp.RequestVotes(context.Background(), VoteRequest{BlockID: blk.id, BlockData: blk.bytes})
	if proposer.IsAccepted(blk.id) {
		t.Fatal("SAFETY: finalized on the self-vote alone (1 < α=4) before any re-poll")
	}
	if got := rp.pollCount(); got != 1 {
		t.Fatalf("setup: expected exactly the one dropped poll before re-poll, got %d", got)
	}

	// The re-poll loop (interval RoundTO=5ms) must issue a SECOND RequestVotes,
	// which delivers the peer votes → quorum → real α-of-K cert → VM.Accept. Wait
	// on VM.Accept (the actual finalization), not just consensus.IsAccepted, since
	// the VM commit follows the count flip by a hair.
	if !waitFor(2*time.Second, func() bool { return blk.AcceptCalled() >= 1 }) {
		t.Fatal("WEDGE: re-poll did not recover the stalled block — no second RequestVotes was issued")
	}
	if !proposer.IsAccepted(blk.id) {
		t.Fatal("block must be marked accepted after re-poll recovery")
	}
	// Recovery MUST have come from a re-poll (second+ RequestVotes), proving the
	// fix is what unstuck it — not the dropped first poll.
	if rp.pollCount() < 2 {
		t.Fatalf("re-poll must issue a SECOND RequestVotes to unstick the block, got %d polls", rp.pollCount())
	}
	// Exactly one VM.Accept (the two finalize paths are mutually idempotent via
	// pending.Decided; settle briefly to confirm no double-Accept).
	time.Sleep(20 * time.Millisecond)
	if blk.AcceptCalled() != 1 {
		t.Fatalf("VM.Accept exactly once after re-poll recovery, got %d", blk.AcceptCalled())
	}
}

// TestRePollNeverForcesSubQuorum is the safety companion: even after many re-poll
// intervals, a proposer whose peers NEVER vote (re-poll re-solicits but no votes
// ever come back) is NEVER finalized — the re-poll re-requests, it never forces.
func TestRePollNeverForcesSubQuorum(t *testing.T) {
	p := config.FeasibleParams(constants.LocalID, 5)
	net := newRelayNetwork(t, 5, p)

	blk := newTestBlock(1, ids.Empty, "repoll-no-force")
	pos := net.trackOwnProposal(0, blk)

	// A proposer with NO voters: every re-poll delivers nothing.
	rp := &relayProposer{net: net, self: 0, pos: pos, voters: nil}
	proposer := net.engines[0].Transitive
	proposer.mu.Lock()
	proposer.proposer = rp
	proposer.mu.Unlock()

	// Let the re-poll loop run for many intervals (RoundTO=5ms → 150ms ≫ interval).
	if waitFor(150*time.Millisecond, func() bool { return proposer.IsAccepted(blk.id) }) {
		t.Fatal("SAFETY VIOLATION: re-poll forced finality with no peer votes (only the self-vote)")
	}
	if blk.AcceptCalled() != 0 {
		t.Fatalf("VM.Accept must never run without a real quorum, got %d", blk.AcceptCalled())
	}
	// The re-poll MUST have been re-soliciting (proving it ran and still didn't force).
	if rp.pollCount() < 2 {
		t.Fatalf("re-poll loop should have re-issued RequestVotes, got %d polls", rp.pollCount())
	}
}
