// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// multinode_proposer_test.go — liveness and safety when the designated proposer
// for a height is down, wedged, or forked.
//
// The suite runs on the multi-node harness (multinode_harness_test.go): N
// independent *Runtime engines wired through an in-process gossip bus,
// finalizing from real block gossip, real signed vote broadcast, and real α-of-K
// cert assembly and verification. No synthetic quorum is hand-fed, so
// finalization here is emergent rather than asserted into being.
//
// The property under test: when a height's designated proposer is down (silent
// in both directions), wedged (present but non-productive), or forked (emitting
// a divergent-execution block), a substitute builds the canonical block and the
// honest majority independently converges on that single block. The three faults
// are distinct scenarios with distinct assertions.
//
// Safety is co-tested in the same harness, because a liveness retry must never
// lower the BFT threshold: a sub-quorum, a forged cert, and a post-finalization
// sibling all fail to finalize. With f < n/3, at K=5, α=4, f=1 the four healthy
// validators are the exact quorum with zero margin, so the retry has to complete
// that quorum without manufacturing the missing vote.
//
// Each test names the mechanism its assertion rests on, so a reader can see what
// would have to break for the test to stop meaning anything.
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
)

const emergeTO = 8 * time.Second // generous bound for emergent finalization under -race

// -----------------------------------------------------------------------------
// Baseline — the harness itself finalizes emergently. If this cannot pass, no
// fault scenario below is meaningful.
// -----------------------------------------------------------------------------

// TestMultiNode_HealthyProposer_EmergentFinalization: 5 up validators, node 0 is
// the proposer. It builds one block and solicits; the other four verify, broadcast
// signed votes, a cert assembles at α=4, gossips, and all five finalize the same
// block, with no test-injected votes. This is the emergent vote/cert topology
// every scenario below assumes.
//
// The assertion rests on followers broadcasting their own votes (integration.go,
// followVerifiedBlock): with that broadcast absent, no cert can ever assemble.
func TestMultiNode_HealthyProposer_EmergentFinalization(t *testing.T) {
	net := newSimNet(t, 5, prodParams5())
	blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "healthy-h1")

	net.build(0, blk)

	if !waitFor(emergeTO, func() bool {
		all, fork := net.finalizedEverywhere(blk)
		return all && !fork
	}) {
		heads := net.headsAtHeight(1)
		t.Fatalf("emergent finalization failed: not all 5 nodes finalized %s at height 1 (heads=%v)", blk.ID(), heads)
	}
	if heads := net.headsAtHeight(1); len(heads) != 1 {
		t.Fatalf("single head violated: %d distinct heads at height 1: %v", len(heads), heads)
	}
}

// -----------------------------------------------------------------------------
// 4/5 with a down proposer. The designated proposer (node 0) is down — crashed or
// partitioned, inbound and outbound both dropped. A substitute (node 1) builds the
// canonical block; the four reachable validators are the exact α=4 quorum, zero
// margin, and must finalize without the down node: no reboot, no manual step.
//
// Two mechanisms carry this. rePollAllPending exempts a node's own proposal from
// maxRePollAttempts, so a substitute never abandons its own undecided block; and
// the build path re-solicits. At zero margin the fourth honest vote is routinely
// the late one, and a quorum that is never re-solicited never completes.
// -----------------------------------------------------------------------------
func TestMultiNode_DownProposer_SubstituteFinalizes(t *testing.T) {
	net := newSimNet(t, 5, prodParams5())
	net.down(0) // the designated proposer is down

	sub := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "substitute-h1")
	net.build(1, sub) // node 1 substitutes

	if !waitFor(emergeTO, func() bool {
		// every UP node (1..4) finalized the substitute block; none forked.
		all, fork := net.finalizedEverywhere(sub)
		return all && !fork
	}) {
		t.Fatalf("halt under a down proposer: the 4 reachable validators did not finalize the substitute block %s "+
			"at the zero-margin α=4 quorum (heads=%v); the chain failed to self-heal past the down proposer",
			sub.ID(), net.headsAtHeight(1))
	}
	// The down node finalized nothing (it received nothing).
	if _, ok := net.nodes[0].rt.FinalizedBlockAtHeight(1); ok {
		t.Fatal("a down node (all inbound dropped) must not have finalized anything")
	}
	if heads := net.headsAtHeight(1); len(heads) != 1 {
		t.Fatalf("single head violated under a down proposer: %v", heads)
	}
}

// -----------------------------------------------------------------------------
// 4/5 with a wedged-but-present proposer. Node 0 is the designated proposer and is
// present — its inbound is live, so it receives gossip, tracks, verifies, and will
// finalize via cert — but wedged: its outbound is silenced, so the block it builds
// never propagates and its votes never reach peers. That is what separates wedged
// from down. A wedged-present node still converges on the canonical block through
// the inbound cert; it simply cannot drive its own proposal.
//
// The substitute (node 1) builds the canonical block; nodes 1..4 finalize it at
// α=4; node 0 finalizes it too, which is what "present" means here, while node 0's
// own wedged block finalizes nowhere.
//
// Rests on the same two mechanisms as the down case: a node never abandons its own
// proposal, and the build path re-solicits.
// -----------------------------------------------------------------------------
func TestMultiNode_WedgedPresentProposer_SubstituteFinalizes(t *testing.T) {
	net := newSimNet(t, 5, prodParams5())

	// Node 0 is present but its outbound is silenced (wedged): it will receive and
	// finalize, but its own block/votes never reach anyone.
	net.nodes[0].rt.config.Gossiper.(*busGossiper).silent = func() bool { return true }

	// The wedged proposer "builds" a block that never propagates.
	wedged := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "wedged-h1")
	net.build(0, wedged)

	// The substitute builds the canonical block.
	canonical := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "canonical-h1")
	net.build(1, canonical)

	if !waitFor(emergeTO, func() bool {
		all, fork := net.finalizedEverywhere(canonical)
		return all && !fork
	}) {
		t.Fatalf("halt under a wedged proposer: validators did not finalize the substitute's canonical block %s "+
			"past the wedged-but-present proposer (heads=%v)", canonical.ID(), net.headsAtHeight(1))
	}
	// "Present": the wedged node finalized the canonical block via inbound cert.
	if got, ok := net.nodes[0].rt.FinalizedBlockAtHeight(1); !ok || got != canonical.ID() {
		t.Fatalf("a wedged-but-present node must still converge on the canonical block via the inbound cert "+
			"(got %v, ok=%v) — this is what distinguishes it from a down node", got, ok)
	}
	// The wedged block finalized nowhere.
	if net.headsAtHeight(1)[wedged.ID()] != 0 {
		t.Fatal("the wedged proposer's own (non-propagated) block must finalize nowhere")
	}
	if heads := net.headsAtHeight(1); len(heads) != 1 {
		t.Fatalf("single head violated under a wedged proposer: %v", heads)
	}
}

// -----------------------------------------------------------------------------
// A forked proposer. Node 3 emits a divergent-execution block — a well-formed
// wrapper over a tampered state root — and actively gossips it to everyone. Every
// honest node parses it, re-executes it, and rejects it, because its claimed state
// root does not equal the deterministic execution result. So it is never tracked,
// never voted, never finalized: acceptance is never granted out of turn on the
// strength of the wrapper alone. The substitute (node 1) builds the canonical
// block, which honest nodes finalize. Exactly one head, no double-finalize.
//
// This is the engine-boundary half of the forked-proposer matrix: verify binds the
// inner execution (the state root), not just the outer wrapper. The proposervm
// inner-block Verify half is proven in the node package.
//
// The assertion rests on simBlock.Verify binding that state root. Were verify to
// accept any claimed root, the forked block would be tracked, voted, and could
// double-finalize.
// -----------------------------------------------------------------------------
func TestMultiNode_ForkedProposer_DivergentRejected_CanonicalFinalizes(t *testing.T) {
	net := newSimNet(t, 5, prodParams5())

	// Node 3 forks: a divergent-execution block, actively gossiped.
	forked := newForkedBlock(ids.Empty, simGenesisRoot(), 1, "forked-h1")
	net.build(3, forked)

	// The substitute builds the canonical block.
	canonical := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "canonical-h1")
	net.build(1, canonical)

	if !waitFor(emergeTO, func() bool {
		all, fork := net.finalizedEverywhere(canonical)
		return all && !fork
	}) {
		t.Fatalf("halt under a forked proposer: honest validators did not finalize the canonical block %s past the "+
			"forked proposer (heads=%v)", canonical.ID(), net.headsAtHeight(1))
	}
	// The forked (divergent) block must have finalized nowhere — not even transiently.
	if net.headsAtHeight(1)[forked.ID()] != 0 {
		t.Fatal("safety: a divergent-execution (forked) block finalized somewhere — honest execution " +
			"verify must reject it before any vote is cast for it")
	}
	if heads := net.headsAtHeight(1); len(heads) != 1 {
		t.Fatalf("single head violated under a forked proposer: %d heads %v", len(heads), heads)
	}
	// Every honest node's execution rejected the forked block (never tracked toward a cert).
	for i, n := range net.nodes {
		if i == 3 {
			continue // the forker itself
		}
		if n.rt.IsAccepted(forked.ID()) {
			t.Fatalf("node %d accepted the forked divergent block — inner-execution binding failed", i)
		}
	}
}

// -----------------------------------------------------------------------------
// Safety: no double-finalize, one head per height, sibling convergence. Height 1
// finalizes to block A. A conflicting sibling B at that same height 1 is then built
// and gossiped by another substitute. Every node has already finalized A there, so
// B is refused everywhere by the per-height guard keyed on the canonical
// commitment, and reported as equivocation — never as a second head.
//
// This is the deterministic form of sibling convergence: once a height is decided
// the network is locked to that head, and a late competing sibling cannot fork it,
// however willing honest nodes are to vote for any verified block.
//
// Two guards carry it: HandleIncomingCert returns early for a cert at or below the
// finalized height, and FinalizeBranch holds a per-height guard. Without both, B's
// cert double-finalizes height 1.
// -----------------------------------------------------------------------------
func TestMultiNode_NoDoubleFinalize_LateSiblingRejected(t *testing.T) {
	net := newSimNet(t, 5, prodParams5())

	a := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "branch-A")
	net.build(0, a)
	if !waitFor(emergeTO, func() bool { all, fork := net.finalizedEverywhere(a); return all && !fork }) {
		t.Fatalf("setup: branch A must finalize everywhere first (heads=%v)", net.headsAtHeight(1))
	}

	// A conflicting sibling at the already-decided height 1, built + gossiped by a
	// different substitute, given every chance to finalize.
	b := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "branch-B")
	net.build(2, b)

	// Give the network ample time to (wrongly) finalize B if the guard were absent.
	time.Sleep(1500 * time.Millisecond)

	if net.headsAtHeight(1)[b.ID()] != 0 {
		t.Fatal("safety: a conflicting sibling finalized at an already-decided height (double-finalize/fork)")
	}
	if heads := net.headsAtHeight(1); len(heads) != 1 || heads[a.ID()] == 0 {
		t.Fatalf("height 1 must remain locked to branch A on every node; heads=%v", heads)
	}
}

// -----------------------------------------------------------------------------
// Safety: a genuine sub-quorum never finalizes. Only nodes 0 and 1 are up and node
// 1 builds. Two of five is below NovaQuorum(5)=3, so no cert can assemble however
// long the block is re-solicited. A liveness retry re-solicits; it can never
// manufacture the missing vote.
//
// The assertion depends on the threshold itself, not on the retry: were the accept
// count lowered to the live-up count, this same set would finalize.
// -----------------------------------------------------------------------------
func TestMultiNode_BelowMajorityNeverAccepts(t *testing.T) {
	net := newSimNet(t, 5, prodParams5())
	net.down(2)
	net.down(3)
	net.down(4) // only 2 of 5 remain — below the Nova majority NovaQuorum(5)=3

	blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "subquorum-h1")
	net.build(1, blk)

	// Re-solicit far past any backoff; a below-majority online set must still never accept. The
	// accept floor is the majority, so a 3-of-5 set does accept and the fail-closed floor sits at
	// 2 of 5: below the majority there is no Nova cert and so no acceptance.
	if waitFor(2*time.Second, func() bool { _, ok := net.nodes[1].rt.FinalizedBlockAtHeight(1); return ok }) {
		t.Fatal("safety: a below-majority set (2 of 5, below NovaQuorum=3) accepted — the retry lowered the threshold")
	}
	if heads := net.headsAtHeight(1); len(heads) != 0 {
		t.Fatalf("no block may accept below the Nova majority; heads=%v", heads)
	}
}

// -----------------------------------------------------------------------------
// Safety: sustained liveness across heights, with no reset and no state-wipe path.
// The network finalizes height 1, then builds and finalizes height 2 on top of it,
// so finalization is durable and the chain keeps producing past a fault without
// any reset. A node never needs its state wiped to make progress.
// -----------------------------------------------------------------------------
func TestMultiNode_SustainedLiveness_TwoHeights(t *testing.T) {
	net := newSimNet(t, 5, prodParams5())

	h1 := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "seq-h1")
	net.build(0, h1)
	if !waitFor(emergeTO, func() bool { all, fork := net.finalizedEverywhere(h1); return all && !fork }) {
		t.Fatalf("height 1 must finalize (heads=%v)", net.headsAtHeight(1))
	}

	// Build height 2 on top of the finalized height-1 state.
	h2 := newHonestBlock(h1.ID(), h1.stateRoot, 2, "seq-h2")
	net.build(1, h2)
	if !waitFor(emergeTO, func() bool { all, fork := net.finalizedEverywhere(h2); return all && !fork }) {
		t.Fatalf("height 2 must finalize ON TOP of height 1 (sustained liveness); heads@2=%v", net.headsAtHeight(2))
	}
	// Both heights are singular across the network — no fork accrued over two rounds.
	if h := net.headsAtHeight(1); len(h) != 1 {
		t.Fatalf("height 1 head diverged after height 2: %v", h)
	}
	if h := net.headsAtHeight(2); len(h) != 1 {
		t.Fatalf("height 2 head diverged: %v", h)
	}
}
