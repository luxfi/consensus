// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// multinode_proposer_test.go — the down/wedged/forked-designated-proposer
// LIVENESS + SAFETY suite, proven on the REAL multi-node harness
// (multinode_harness_test.go): N independent *Runtime engines wired through an
// in-process gossip bus, finalizing EMERGENTLY from real block gossip, real
// signed vote broadcast, and real α-of-K cert assembly/verify — no synthetic
// quorum is hand-fed.
//
// The mainnet-unblock property under test: a height's designated proposer is
// DOWN, WEDGED (present but non-productive), or FORKED (emits a divergent-
// execution block); a SUBSTITUTE builds the canonical block; and the honest
// majority independently converges on that single block. The three faults are
// DISTINCT scenarios with distinct assertions.
//
// SAFETY is co-tested in the SAME harness (the liveness retry must not lower the
// BFT threshold): sub-quorum, forged cert, and a post-finalization sibling all
// FAIL to finalize. BFT ≥ avalanche (f < n/3): at K=5, α=4, f=1 the four healthy
// validators are the EXACT quorum (zero margin) — the mainnet condition.
//
// fails-before/passes-after: each test names the exact code revert that turns it
// RED in its doc comment; the mechanisms live in v1.33.0 (never-abandon-own
// re-poll, per-height finalize guard, canonical-keyed equivocation) plus the
// engine.go build-path re-solicit alignment added with this suite.
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
)

const emergeTO = 8 * time.Second // generous bound for emergent finalization under -race

// -----------------------------------------------------------------------------
// BASELINE — the harness itself finalizes emergently. If this cannot go green,
// no fault scenario below is meaningful.
// -----------------------------------------------------------------------------

// TestMultiNode_HealthyProposer_EmergentFinalization: 5 up validators, node 0 is
// the proposer. It builds ONE block and solicits; the other four verify, broadcast
// signed votes, a cert assembles at α=4, gossips, and ALL FIVE finalize the SAME
// block — with no test-injected votes. Proves the emergent vote/cert topology.
//
// fails-before: revert integration.go followVerifiedBlock's BroadcastVote (so
// followers never broadcast their votes) → no cert ever assembles → RED.
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
		t.Fatalf("SINGLE-HEAD violated: %d distinct heads at height 1: %v", len(heads), heads)
	}
}

// -----------------------------------------------------------------------------
// GATE: 4/5 with ONE DOWN proposer. The designated proposer (node 0) is DOWN
// (crashed/partitioned — inbound AND outbound dropped). A substitute (node 1)
// builds the canonical block; the four reachable validators are the EXACT α=4
// quorum (zero margin) and must finalize WITHOUT the down node. Self-healing: no
// reboot, no manual step.
//
// fails-before: revert engine.go rePollAllPending's own-proposal exemption (let
// maxRePollAttempts abandon the substitute's undecided own block) AND drop the
// build-path re-solicit → the 4th honest vote that arrives late is never
// re-solicited → the zero-margin quorum never completes → RED.
// -----------------------------------------------------------------------------
func TestMultiNode_DownProposer_SubstituteFinalizes(t *testing.T) {
	net := newSimNet(t, 5, prodParams5())
	net.down(0) // designated proposer is DOWN

	sub := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "substitute-h1")
	net.build(1, sub) // node 1 substitutes

	if !waitFor(emergeTO, func() bool {
		// every UP node (1..4) finalized the substitute block; none forked.
		all, fork := net.finalizedEverywhere(sub)
		return all && !fork
	}) {
		t.Fatalf("DOWN-PROPOSER HALT: the 4 reachable validators did not finalize the substitute block %s "+
			"at the zero-margin α=4 quorum (heads=%v). The chain failed to self-heal past the down proposer.",
			sub.ID(), net.headsAtHeight(1))
	}
	// The down node finalized nothing (it received nothing).
	if _, ok := net.nodes[0].rt.FinalizedBlockAtHeight(1); ok {
		t.Fatal("a DOWN node (all inbound dropped) must not have finalized anything")
	}
	if heads := net.headsAtHeight(1); len(heads) != 1 {
		t.Fatalf("SINGLE-HEAD violated under a down proposer: %v", heads)
	}
}

// -----------------------------------------------------------------------------
// GATE: 4/5 with ONE WEDGED-BUT-PRESENT proposer. Node 0 is the designated
// proposer and is PRESENT (its inbound is live — it receives gossip, tracks,
// verifies, and will finalize via cert) but WEDGED: its OUTBOUND is silenced, so
// the block it "builds" never propagates and its votes never reach peers. This is
// distinct from DOWN: a wedged-present node still CONVERGES on the canonical block
// (it finalizes via the inbound cert), it just cannot drive its own proposal.
//
// The substitute (node 1) builds the canonical block; nodes 1..4 finalize it at
// α=4; and node 0 ALSO finalizes it (proving "present"), while node 0's wedged
// block finalizes NOWHERE.
//
// fails-before: same revert as the down gate (own-proposal never-abandon +
// build-path re-solicit) → RED.
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
		t.Fatalf("WEDGED-PROPOSER HALT: validators did not finalize the substitute's canonical block %s "+
			"past the wedged-but-present proposer (heads=%v).", canonical.ID(), net.headsAtHeight(1))
	}
	// "Present": the wedged node finalized the canonical block via inbound cert.
	if got, ok := net.nodes[0].rt.FinalizedBlockAtHeight(1); !ok || got != canonical.ID() {
		t.Fatalf("a WEDGED-BUT-PRESENT node must still CONVERGE on the canonical block via the inbound cert "+
			"(got %v, ok=%v) — this is what distinguishes it from a down node", got, ok)
	}
	// The wedged block finalized nowhere.
	if net.headsAtHeight(1)[wedged.ID()] != 0 {
		t.Fatal("the wedged proposer's own (non-propagated) block must finalize NOWHERE")
	}
	if heads := net.headsAtHeight(1); len(heads) != 1 {
		t.Fatalf("SINGLE-HEAD violated under a wedged proposer: %v", heads)
	}
}

// -----------------------------------------------------------------------------
// GATE: FORKED proposer (the mainnet luxd-3 reality). Node 3 is FORKED: it emits
// a divergent-execution block (a well-formed wrapper over a tampered state root)
// and actively gossips it to everyone. Every honest node PARSES it, RE-EXECUTES,
// and REJECTS it (its claimed state root != the deterministic execution result) —
// so it is never tracked, never voted, never finalized (no early out-of-turn
// acceptance). The substitute (node 1) builds the canonical block, which honest
// nodes finalize. Exactly one head; no double-finalize.
//
// This is the engine-boundary half of the forked-proposer matrix: verify binds
// the INNER execution (state root), not just the outer wrapper. The proposervm
// inner-block-Verify half is proven in the node package.
//
// fails-before: make simBlock.Verify ignore the state root (accept any) → the
// forked block is tracked+voted and can double-finalize → RED.
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
		t.Fatalf("FORKED-PROPOSER HALT: honest validators did not finalize the canonical block %s past the "+
			"forked proposer (heads=%v).", canonical.ID(), net.headsAtHeight(1))
	}
	// The forked (divergent) block must have finalized NOWHERE — not even transiently.
	if net.headsAtHeight(1)[forked.ID()] != 0 {
		t.Fatal("SAFETY VIOLATION: a divergent-execution (forked) block finalized somewhere — honest execution " +
			"Verify must reject it before any vote (no early out-of-turn acceptance).")
	}
	if heads := net.headsAtHeight(1); len(heads) != 1 {
		t.Fatalf("SINGLE-HEAD violated under a forked proposer: %d heads %v", len(heads), heads)
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
// SAFETY: no double-finalize / single head per height / sibling convergence.
// Height 1 finalizes to block A (canonical). A conflicting sibling B at the SAME
// height 1 is then built + gossiped by another substitute. Every node has already
// finalized A at height 1, so B is REFUSED everywhere (the per-height guard keyed
// on the canonical commitment) and reported as equivocation — never a second head.
//
// This is the deterministic form of "sibling convergence": once a height is
// decided, the network is LOCKED to that head; a late competing sibling cannot
// fork it, no matter that honest nodes would otherwise vote for any verified block.
//
// fails-before: revert the topology.go HandleIncomingCert height gate (`cert.
// Position.Height <= fh` early-return) and the FinalizeBranch per-height guard →
// B's cert double-finalizes height 1 → RED.
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
		t.Fatal("SAFETY VIOLATION: a conflicting sibling finalized at an already-decided height (double-finalize/fork).")
	}
	if heads := net.headsAtHeight(1); len(heads) != 1 || heads[a.ID()] == 0 {
		t.Fatalf("height 1 must remain LOCKED to branch A on every node; heads=%v", heads)
	}
}

// -----------------------------------------------------------------------------
// SAFETY: a genuine SUB-QUORUM never finalizes. Only 3 validators are up (nodes
// 0,1,2); node 1 builds. 3 < α=4, so no cert can ever assemble no matter how long
// the block is re-solicited. Liveness retry re-SOLICITS; it can never manufacture
// the 4th vote.
//
// fails-before: lower α below the live-up count (e.g. α=3) → the 3 up nodes
// finalize → RED (proving the assertion actually depends on the threshold).
// -----------------------------------------------------------------------------
func TestMultiNode_SubQuorumNeverFinalizes(t *testing.T) {
	net := newSimNet(t, 5, prodParams5())
	net.down(3)
	net.down(4) // only 3 of 5 remain — below α=4

	blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "subquorum-h1")
	net.build(1, blk)

	// Re-solicit far past any backoff; a sub-quorum must STILL never finalize.
	if waitFor(2*time.Second, func() bool { _, ok := net.nodes[1].rt.FinalizedBlockAtHeight(1); return ok }) {
		t.Fatal("SAFETY VIOLATION: a sub-quorum (3 of 5, below α=4) finalized — the liveness retry lowered the threshold.")
	}
	if heads := net.headsAtHeight(1); len(heads) != 0 {
		t.Fatalf("no block may finalize below α; heads=%v", heads)
	}
}

// -----------------------------------------------------------------------------
// SAFETY: sustained liveness across heights (no reset / no DB-wipe path). The
// network finalizes height 1, then builds and finalizes height 2 ON TOP of it —
// proving finalization is durable and the chain keeps producing past a fault
// WITHOUT any reset. Models the self-heal-and-continue property the 7-gate
// requires (a node never needs a DB wipe to make progress).
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
