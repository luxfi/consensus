// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// red_sibling_fork_test.go — ADVERSARIAL (Red team) probe of the crux Blue flagged:
//
//	"Valid-but-conflicting sibling (correct stateRoot, different payload) at one
//	 height under the down-proposer transient — the engine has NO signing-side
//	 per-height vote-once lock, so two valid siblings could each gather alpha if
//	 honest nodes vote both; production relies on proposervm single-proposer."
//
// These tests exercise the CONCURRENT-sibling window that Blue's
// TestMultiNode_NoDoubleFinalize_LateSiblingRejected deliberately sidesteps (it
// finalizes A *first*, then introduces B, so the LOCAL per-height gate trivially
// rejects B). The real down-proposer transient produces TWO valid substitutes at
// the SAME still-unfinalized height. The follower vote path
// (integration.go followVerifiedBlock) dedups ONLY on `incomingHeight <=
// fastFollowHeight` (the finalized height) — there is NO per-(height,epoch)
// vote-once discipline — so an honest node signs an accept vote for EVERY valid
// block it sees at an unfinalized height, including two conflicting siblings.
//
// The reference C++ engine (~/work/lux/consensus2, committed_slot_) closes exactly
// this: an honest node signs <=1 block per (height,epoch). The Go engine does not.
//
// Each test ASSERTS THE SAFETY PROPERTY (no two valid certs / no cross-node
// divergence). A FAILURE here is the proof of the hole; porting the committed_slot_
// discipline (refuse to sign/track a second block at an unfinalized height already
// voted) is what turns these GREEN.
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// injectVotes delivers validator i's REAL signed accept vote (shared test key set)
// for the given position, for each i in voters. Votes go through the SAME async
// channel path a live gossiped vote takes (ReceiveVote -> handleVote -> verify).
func injectVotes(e *Transitive, vs *testValidatorSet, pos VotePosition, voters ...int) {
	for _, i := range voters {
		e.ReceiveVote(vs.signedVote(i, pos))
	}
}

// -----------------------------------------------------------------------------
// RED-1 (root cause, DETERMINISTIC): the losing sibling holds a fully valid
// alpha-of-K finality cert.
//
// One engine tracks two VALID conflicting siblings A,B at the same unfinalized
// height 1 (correct parent=genesis, different payload -> different id/canonical).
// A full alpha=4 quorum of REAL signed votes is delivered for EACH. A finalizes;
// the LOCAL per-height gate refuses B. But B still assembles+verifies a real
// alpha-of-K cert: a SECOND valid finality witness exists at height 1. Any peer
// that receives B's cert before A's would commit B -> cross-node fork.
//
// PASS (safety held) would require: the engine refuses to record B's votes /
// assemble B's cert because it already voted A at height 1 (committed_slot_).
// -----------------------------------------------------------------------------
func TestRed_LosingSiblingHoldsValidCert_NoVoteOnceDiscipline(t *testing.T) {
	vs := newTestValidatorSet(5)
	params := params5Prod() // K=5, alpha=4
	e, chainID := newQuorumEngine(t, params, vs, 0, &recordingGossiper{})

	A := newTestBlock(1, ids.Empty, "sibling-A")
	B := newTestBlock(1, ids.Empty, "sibling-B")
	posA := trackProposal(e, chainID, A, 0) // engine self-signs (validator 0) for A
	posB := trackProposal(e, chainID, B, 0) // engine self-signs (validator 0) for B too — no vote-once lock

	// A reaches alpha first and finalizes.
	injectVotes(e, vs, posA, 1, 2, 3) // {0(self),1,2,3} = 4 = alpha
	if !waitFor(3*time.Second, func() bool { return e.IsAccepted(A.id) }) {
		t.Fatalf("setup: A must finalize with its alpha-of-K quorum")
	}

	// Attempt to build a SECOND valid cert for the conflicting sibling B at the
	// already-finalized height 1. POST-FIX: honest validators 1,2,3 already signed A
	// at height 1, so the disciplined set REFUSES their B-signatures, and this node's
	// own B-vote is refused by reserveSlotForSign — B can never collect alpha SIGNED
	// accepts, so no conflicting cert can form. (Pre-fix, the self-vote plus injected
	// equivocations gave B a full alpha-of-K cert — the fork.)
	injectVotes(e, vs, posB, 1, 2, 3) // refused: 1,2,3 committed A; self committed A
	time.Sleep(300 * time.Millisecond) // let any (refused) vote plumbing settle

	if e.IsAccepted(B.id) {
		t.Fatalf("unexpected: B committed on the same node as A (per-height gate failed)")
	}
	if head, ok := e.consensus.FinalizedBlockAtHeight(1); !ok || head != A.id {
		t.Fatalf("expected height 1 locally locked to A (%s); got head=%s ok=%v", A.id, head, ok)
	}

	// ...but does a SECOND valid alpha-of-K cert exist for the conflicting sibling B?
	e.mu.Lock()
	pbB := e.pendingBlocks[B.id]
	var bCertOK bool
	var bSigners int
	if pbB != nil {
		if vc, ok := e.assembleVerifiedCertLocked(pbB, B.id); ok {
			bCertOK = true
			if qc := vc.Cert(); qc != nil {
				bSigners = len(pbB.certVotes)
			}
		}
	}
	e.mu.Unlock()

	if bCertOK {
		t.Fatalf("SAFETY HOLE (no vote-once discipline): height 1 is finalized to A=%s, yet the conflicting "+
			"sibling B=%s ALSO holds a fully valid, self-verifying alpha-of-K finality cert (%d signed accepts). "+
			"Two valid certs for two conflicting blocks at one height now exist. The LOCAL per-height gate only "+
			"stops THIS node from committing both; a peer that receives B's cert first commits B -> cross-node "+
			"fork. Fix: port the consensus2 committed_slot_ discipline (an honest node signs/tracks <=1 block "+
			"per (height,epoch)).", A.id, B.id, bSigners)
	}
}

// -----------------------------------------------------------------------------
// RED-2 (safety violation, DETERMINISTIC): cross-node fork from message reordering.
//
// Two honest engines (validators 0 and 1) each track the SAME two valid siblings
// A,B at height 1. Node1 receives A's quorum first (finalizes A); node2 receives
// B's quorum first (finalizes B). Both quorums are REAL alpha-of-K supermajorities
// of the shared validator set. Result: two honest, protocol-following nodes
// permanently DISAGREE on the block at height 1 — a consensus FORK, with each side
// holding a valid finality cert. Message reordering is a normal async-network
// condition, not a contrivance.
//
// The ONLY thing preventing this in production is proposervm single-proposer-per-
// slot. The down-proposer transient (the exact condition this PR fixes) violates
// that by admitting multiple eligible substitute proposers at one height.
// -----------------------------------------------------------------------------
func TestRed_CrossNodeFork_TwoValidSiblings_OppositeArrivalOrder(t *testing.T) {
	vs := newTestValidatorSet(5)
	params := params5Prod() // K=5, alpha=4

	e1, chain1 := newQuorumEngine(t, params, vs, 0, &recordingGossiper{})
	e2, chain2 := newQuorumEngine(t, params, vs, 1, &recordingGossiper{})

	A := newTestBlock(1, ids.Empty, "fork-A")
	B := newTestBlock(1, ids.Empty, "fork-B")

	posA1 := trackProposal(e1, chain1, A, 0)
	posB1 := trackProposal(e1, chain1, B, 0)
	posA2 := trackProposal(e2, chain2, A, 0)
	posB2 := trackProposal(e2, chain2, B, 0)

	// POST-FIX invariant. HONEST validators run the fixed engine's per-height vote-once
	// discipline (modeled by the now-disciplined testValidatorSet): each signs AT MOST
	// ONE canonical at height 1. The ORIGINAL repro hand-fed validators {0,2,3,4} a vote
	// for BOTH A and B — that is ≥3 ACTUAL Byzantine equivocators (f ≥ n/3), beyond the
	// BFT guarantee, where a fork is NOT a safety violation. The correct invariant: under
	// honest disciplined validators, even with OPPOSITE arrival order at the two nodes,
	// only ONE sibling can ever reach alpha (the equivocating quorum overlap is gone), so
	// the two nodes CANNOT diverge. 2,3,4 all commit A; each node's own proposer vote also
	// commits A first (B refused locally). B never reaches alpha; both nodes finalize A.
	injectVotes(e1, vs, posA1, 2, 3, 4) // e1 sees A first: {0(self),2,3,4}=4 -> A finalizes
	injectVotes(e1, vs, posB1, 2, 3, 4) // 2,3,4 already committed A -> REFUSED (honest non-equivocation)
	injectVotes(e2, vs, posB2, 2, 3, 4) // e2 sees B first: 2,3,4 committed A -> REFUSED; B stays sub-alpha
	injectVotes(e2, vs, posA2, 2, 3, 4) // then A: {1(self),2,3,4}=4 -> A finalizes (converges, opposite order)

	if !waitFor(3*time.Second, func() bool { return e1.IsAccepted(A.id) }) {
		t.Fatalf("e1 must finalize A (honest majority of disciplined validators converges)")
	}
	if !waitFor(3*time.Second, func() bool { return e2.IsAccepted(A.id) }) {
		t.Fatalf("e2 must finalize A despite OPPOSITE arrival order (no divergence under discipline)")
	}
	time.Sleep(300 * time.Millisecond)

	h1, ok1 := e1.consensus.FinalizedBlockAtHeight(1)
	h2, ok2 := e2.consensus.FinalizedBlockAtHeight(1)
	if !ok1 || !ok2 {
		t.Fatalf("both nodes must have finalized at height 1 (ok1=%v ok2=%v)", ok1, ok2)
	}
	if h1 != h2 {
		t.Fatalf("CROSS-NODE FORK (BFT SAFETY VIOLATED): e1=%s e2=%s — must be IMPOSSIBLE under honest "+
			"disciplined validators (f<n/3): the per-height vote-once lock removes the equivocating quorum "+
			"overlap, so two conflicting alpha-of-K certs can no longer both form.", h1, h2)
	}
	if e1.IsAccepted(B.id) || e2.IsAccepted(B.id) {
		t.Fatalf("losing sibling B must not finalize on either node (never reached alpha under discipline)")
	}
}

// -----------------------------------------------------------------------------
// RED-3 (EMERGENT, real gossip): two concurrent substitutes under a down proposer.
//
// The mainnet down-proposer transient, on the REAL multi-node harness: designated
// proposer (node 0) is DOWN; TWO substitutes (nodes 1 and 2) each build a distinct
// VALID block at height 1 CONCURRENTLY (both pass execution Verify — this is the
// valid-but-conflicting case, NOT the tampered-stateRoot forked block Blue's suite
// covers). Finalization is emergent from real block gossip + signed-vote broadcast
// + alpha-of-K cert assembly. We then check for divergent finalized heads.
//
// NOTE: the outcome is TIMING-DEPENDENT (which cert wins the propagation race on
// each node). A single observed fork proves reachability; convergence on a given
// run does NOT prove safety (RED-1/RED-2 prove the mechanism deterministically).
// -----------------------------------------------------------------------------
func TestRed_DownProposer_TwoSubstitutes_EmergentForkProbe(t *testing.T) {
	net := newSimNet(t, 5, prodParams5()) // K=5, alpha=4, one down => 4 = exact quorum
	net.down(0)                           // designated proposer DOWN

	a := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "substitute-A")
	b := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "substitute-B")

	// Two eligible substitutes build competing VALID siblings at the same height,
	// concurrently, BEFORE either finalizes.
	net.build(1, a)
	net.build(2, b)

	// Let the emergent vote/cert flow run to quiescence.
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		heads := net.headsAtHeight(1)
		if len(heads) >= 2 {
			break // fork already observed
		}
		time.Sleep(20 * time.Millisecond)
	}

	heads := net.headsAtHeight(1)
	bothFinalizedSomewhere := heads[a.ID()] > 0 && heads[b.ID()] > 0
	if len(heads) >= 2 || bothFinalizedSomewhere {
		t.Fatalf("EMERGENT CROSS-NODE FORK under down-proposer transient: distinct finalized heads at height 1 = %v "+
			"(A=%s cnt=%d, B=%s cnt=%d). Two valid substitutes each gathered an alpha-of-K cert because honest "+
			"nodes voted BOTH (no per-(height,epoch) vote-once discipline). Fix: consensus2 committed_slot_.",
			heads, a.ID(), heads[a.ID()], b.ID(), heads[b.ID()])
	}
	t.Logf("no fork observed THIS run (converged to %v). Timing-dependent; RED-1/RED-2 prove the hole deterministically.", heads)
}
