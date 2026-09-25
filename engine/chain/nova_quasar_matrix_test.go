// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// nova_quasar_matrix_test.go — the finality matrix.
//
// A block is accepted (VM.Accept, the ledger's decided height) only on the ⅔ certificate:
// strictly more than ⅔ of signer stake by at least TwoThirdsCount(n) distinct signers, over
// a set of at least minBFTCommittee. That certificate is also the export certificate, so a
// block exports with its accept. A bare majority accepts nothing, whatever its label.
//
// The invariant asserted throughout: across every scenario below, no two conflicting
// blocks are accepted or exported at one height. Two ⅔ certificates at one height share
// 2α−n > f signers, so a second one takes more than ⅓ of stake double-signing, beyond the
// f<⅓ fault bound. A sole validator (K==1) accepts on its own synthesized certificate and
// never exports.
package chain

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// exportTO is a generous bound for EXPORT (Quasar) convergence in the in-process multinode sim.
// Export TRAILS accept and needs the ⅔-th stake vote to gossip and complete the cert, so it takes
// meaningfully longer than accept convergence (emergeTO) — especially under full-suite CPU load.
const exportTO = 25 * time.Second

// skipMultinodeUnderRace skips a timing-sensitive in-process MULTINODE convergence test under
// the race detector. The detector's ~10x, highly-variable slowdown makes the sim's gossip
// latency exceed any bounded convergence settle window — a condition that never occurs on a
// real network (gossip ms, settle hundreds of ms) — and also stresses the FIPS-140 ed25519
// module's concurrent Sign/Verify (all crypto/ed25519 routes through it in Go 1.26) shared by
// the convergence goroutine and the vote handlers. The two-tier safety invariants asserted
// here — accept and export only at ⅔, no two conflicting certificates, export gating —
// are also asserted by the single-engine matrix tests (Threshold, WeightedStake,
// SingleValidator, Equivocation, DegradedRPC), which are deterministic and do run under
// -race. Mirrors the convergence-storm discipline in race_flag_race_test.go.
func skipMultinodeUnderRace(t *testing.T) {
	t.Helper()
	if underRace {
		t.Skip("timing-sensitive in-process multinode convergence test skipped under -race " +
			"(the detector slowdown violates the sim's gossip<settle timing assumption; the two-tier " +
			"safety invariants run -race-clean in the single-engine matrix tests)")
	}
}

// novaQ is NovaQuorum(n) — the bare majority, which accepts nothing.
func novaQ(n int) int { return n/2 + 1 }

// twoThirdsCount is the minimum DISTINCT equal-stake voter count that strictly exceeds ⅔ of
// total stake — the Quasar export threshold for an equal-weight set of n.
func twoThirdsCount(n int) int { return int(config.TwoThirdsStakeFloor(uint64(n))) + 1 }

// nMatrixParams builds K=n consensus params for the single-engine threshold matrix so the live
// committee is exactly n (no BFT floor clamp — the floor only binds when the live count is BELOW
// the preset, which is the transient-restart guard, not this deterministic matrix).
func nMatrixParams(n int) config.Parameters {
	p := dyn5()
	p.K = n
	if n >= 1 {
		p.AlphaPreference = twoThirdsCount(n)
		p.AlphaConfidence = twoThirdsCount(n)
	}
	return p
}

// -----------------------------------------------------------------------------
// n ∈ {2..5}, weighted and equal: accept and export only at strict >⅔ stake.
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_Threshold_AcceptAndExportAtTwoThirds is the threshold ladder for
// n ∈ {2,3,4,5}: a bare majority accepts nothing, and the block is accepted and exported
// together once a strict >⅔-stake supermajority of distinct signers has signed.
//
// Below minBFTCommittee nothing is accepted however unanimous the set: f=⌊(n−1)/3⌋ is 0 for
// those sizes, and a certificate that tolerates no Byzantine fault is not one a block is
// accepted on. A two- or three-validator chain decides nothing (a sole validator, K==1,
// accepts on its own synthesized certificate — TestNovaQuasarMatrix_SingleValidatorSelfIgnites).
func TestNovaQuasarMatrix_Threshold_AcceptAndExportAtTwoThirds(t *testing.T) {
	for n := 2; n <= 5; n++ {
		t.Run(fmt.Sprintf("n=%d_equal", n), func(t *testing.T) {
			vs := newTestValidatorSet(n)
			rec := &recordingGossiper{}
			e, chainID := newQuorumEngineOpts(t, nMatrixParams(n), vs, 0, rec, WithStakeWeighting(vs))

			blk := newTestBlock(1, ids.Empty, "n-matrix")
			pos := trackProposal(e, chainID, blk, 0) // node 0's own signed accept = 1 vote

			nq, tt := novaQ(n), twoThirdsCount(n)
			// Deliver peers up to the bare majority (self already = 1).
			for i := 1; i < nq; i++ {
				e.ReceiveVote(vs.signedVote(i, pos))
			}
			if nq < tt {
				mustNotFinalize(t, e, blk, 400*time.Millisecond, fmt.Sprintf("n=%d: majority %d < ⅔ %d → no accept", n, nq, tt))
				for i := nq; i < tt; i++ {
					e.ReceiveVote(vs.signedVote(i, pos))
				}
			}
			if n < minBFTCommittee {
				// Every validator has now signed and nothing is accepted: there is no fault
				// budget for a supermajority over this set to be about.
				for i := tt; i < n; i++ {
					e.ReceiveVote(vs.signedVote(i, pos))
				}
				mustNotFinalize(t, e, blk, 400*time.Millisecond, fmt.Sprintf(
					"n=%d: unanimous, and below the minimum Byzantine committee → no accept", n))
				mustNotQuasar(t, e, blk, 100*time.Millisecond, fmt.Sprintf(
					"n=%d: below the minimum Byzantine committee → no export", n))
				return
			}
			mustFinalize(t, e, blk, 2*time.Second, fmt.Sprintf("n=%d: ⅔ count=%d → accept", n, tt))
			mustQuasar(t, e, blk, 2*time.Second, fmt.Sprintf("n=%d: ⅔ count=%d → export", n, tt))
		})
	}
}

// TestNovaQuasarMatrix_WeightedStake_AcceptOnTwoThirdsOfStake proves the threshold is read in
// stake: a four-node head-count holding 40% accepts nothing, and the 60% holder's late vote
// takes the block to 100% of stake, which accepts and exports it.
func TestNovaQuasarMatrix_WeightedStake_AcceptOnTwoThirdsOfStake(t *testing.T) {
	vs := newTestValidatorSet(5)
	// Node 0 holds 60% of stake and ABSTAINS; nodes 1..4 hold 10% each.
	skew := newStakeMap(vs, 60, 10, 10, 10, 10)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngineOpts(t, dyn5(), vs, 0, rec, WithStakeWeighting(skew))

	blk := newTestBlock(1, ids.Empty, "weighted")
	// Track it as a block node 0 did not propose, so the 60% holder does not auto-vote — the accepting
	// coalition is exactly the four 10%-stake nodes.
	cb := &Block{id: blk.id, parentID: blk.parentID, height: blk.height, timestamp: blk.timestamp.Unix(), data: blk.bytes}
	_ = e.consensus.AddBlock(context.Background(), cb)
	e.mu.Lock()
	e.pendingBlocks[blk.id] = &PendingBlock{ConsensusBlock: cb, VMBlock: blk, ProposedAt: time.Now(), Round: 0}
	e.mu.Unlock()
	pos := VotePosition{ChainID: chainID, Height: blk.height, Round: 0, BlockID: blk.id, ParentID: blk.parentID}

	// Nodes 1..4 vote: 4/5 count majority holding only 40% of stake.
	for _, i := range []int{1, 2, 3, 4} {
		e.ReceiveVote(vs.signedVote(i, pos))
	}
	// 40% of stake is below ⅔ — a head-count of four accepts nothing and exports nothing.
	mustNotFinalize(t, e, blk, 2*time.Second, "count majority / 40% stake → no accept")
	mustNotQuasar(t, e, blk, 400*time.Millisecond, "40% stake → no export")

	// The 60%-stake holder votes (late). 40%+60% = 100% > ⅔ ⇒ accepted and exported.
	e.ReceiveVote(vs.signedVote(0, pos))
	mustFinalize(t, e, blk, 2*time.Second, "heavy-stake vote → ⅔ of stake → accept")
	mustQuasar(t, e, blk, 2*time.Second, "heavy-stake late vote → ⅔ export")
}

// -----------------------------------------------------------------------------
// n=1 accepts on its own synthesized certificate; the single-validator path still works.
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_SingleValidatorSelfIgnites proves n=1 accepts on the sole validator's
// synthesized certificate (buildSingleValidatorCertLocked, the one below-⅔ witness, and only at
// K==1) — and does NOT reach Quasar.
//
// The sole validator does hold a ⅔ stake supermajority, and it holds a ⅔ supermajority of the
// seats too, so both of the export rung's quorum floors are satisfied by its own signature.
// That is exactly why neither of them is the whole rule: a certificate one key can mint is not
// a claim that a Byzantine supermajority of independent parties agreed, and at n=1 there are no
// independent parties for the claim to be about. Local acceptance, with no peer to fork
// against, is the rung a solo chain gets.
func TestNovaQuasarMatrix_SingleValidatorSelfIgnites(t *testing.T) {
	vs := newTestValidatorSet(1)
	rec := &recordingGossiper{}
	p := dyn5()
	p.K = 1
	// A committee of one has a quorum of one. Shrinking K without shrinking α left
	// α=4 on a 1-node chain — a quorum no committee of that size can ever reach.
	p.AlphaPreference = 1
	p.AlphaConfidence = 1
	e, _ := newQuorumEngineOpts(t, p, vs, 0, rec, WithStakeWeighting(vs))

	blk := newTestBlock(1, ids.Empty, "solo")
	_ = trackProposal(e, chainID(e), blk, 0)
	// Drive the accept funnel (no peer votes arrive to trigger it on a solo chain).
	_ = e.TryAccept(context.Background(), blk.id)
	// The sole validator accepts on its own certificate: there is no peer to agree with.
	mustFinalize(t, e, blk, 2*time.Second, "n=1 accepts alone")
	// And it stops there. One party is not a committee: f=⌊(1−1)/3⌋=0, so its unanimous
	// certificate absorbs no fault and a single compromise forges export finality outright.
	mustNotQuasar(t, e, blk, 400*time.Millisecond,
		"n=1 holds every unit of stake and every seat, and is still not a Byzantine committee → no export")
}

// chainID recovers the engine's chain id (set via WithQuorumCert) for tests that did not capture
// it from the constructor.
func chainID(e *Transitive) ids.ID { return e.chainID }

// -----------------------------------------------------------------------------
// Multinode: 3/5 accepts nothing; 4/5 accepts and exports.
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_ThreeOfFive_AcceptsNothing: with 2 of 5 validators down, the 3 live
// validators sign the block and none accepts it: 3/5 is 60% of stake, not more than ⅔. Both
// the accept tip and the export tip stay Empty. A majority keeps nothing moving; the chain
// waits until ⅔ sign.
func TestNovaQuasarMatrix_ThreeOfFive_AcceptsNothing(t *testing.T) {
	skipMultinodeUnderRace(t)
	net := newSimNet(t, 5, prodParams5())
	net.down(3)
	net.down(4) // 3 up: {0,1,2}

	blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "three-of-five")
	net.build(0, blk)

	// The three sign it (the settle window is 2 s) and it is still not accepted anywhere.
	if !waitFor(emergeTO, func() bool {
		for i := 0; i < 3; i++ {
			if !net.nodes[i].rt.hasSignedHeight(1) {
				return false
			}
		}
		return true
	}) {
		t.Fatal("precondition: the 3 live validators must sign height 1")
	}
	if waitFor(2*time.Second, func() bool { return len(net.headsAtHeight(1)) > 0 }) {
		t.Fatalf("3-of-5 (60%% stake, not more than ⅔) accepted: heads=%v", net.headsAtHeight(1))
	}
	for i, n := range net.nodes {
		if !n.reachable() {
			continue
		}
		if tip := n.quasarTip(); tip != ids.Empty {
			t.Fatalf("node %d exposes an export (Quasar) tip %s under 3/5", i, tip)
		}
		if tip := n.novaTip(); tip != ids.Empty {
			t.Fatalf("node %d exposes an accept tip %s under 3/5", i, tip)
		}
	}
}

// TestNovaQuasarMatrix_FourOfFive_ExportsQuasar: with 4 of 5 up (80% of stake, more than ⅔),
// the block reaches Quasar and every up node exports the same block — no export fork.
func TestNovaQuasarMatrix_FourOfFive_ExportsQuasar(t *testing.T) {
	skipMultinodeUnderRace(t)
	net := newSimNet(t, 5, prodParams5())
	net.down(4) // 4 up: {0,1,2,3}

	blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "four-of-five-quasar")
	net.build(0, blk)

	// The accept converges.
	if !waitFor(emergeTO, func() bool {
		all, fork := net.finalizedEverywhere(blk)
		return all && !fork
	}) {
		t.Fatalf("liveness: 4-of-5 must accept and converge on %s, heads=%v", blk.ID(), net.headsAtHeight(1))
	}
	// The export forms: 4-of-5 (80% of stake, more than ⅔) is the certificate the block was
	// accepted on (assembleCertLocked), and promoteQuasar feeds its votes to the attestor. The
	// wait reads the export FRONTIER, which a Quasar tip carries past every ancestor, so a node
	// that promoted at a successor height is read the way an export consumer reads it.
	ok, chain := waitForExportFrontier(net, blk, exportTO)
	if !ok {
		t.Fatalf("export liveness: 4-of-5 (80%% stake > ⅔) must export %s, tips=%v", blk.ID().String(), net.quasarTips())
	}
	// The invariant: no two conflicting blocks both reach Quasar at one height. Assert it at the
	// HEIGHT, not at the tip. A Quasar tip finalizes all its ancestors, so once the frontier is
	// above blk the tip is legitimately a descendant of blk, and comparing the tip to blk would
	// flag honest frontier progress as a fork. What must not happen is a node exporting a
	// different canonical at blk's own height.
	for i, n := range net.nodes {
		if !n.reachable() {
			continue
		}
		qh, exported := n.quasarHeight()
		if !exported || qh < blk.height {
			continue // has not exported through blk's height yet — nothing to contradict
		}
		// First, the frontier itself is on the canonical line. quasarFrontier is a single
		// {height, canonical, envelope} with no per-height index, and PromoteQuasar is not its
		// only writer: SyncQuasarFrontier seeds it from the VM's durable export record with no
		// Nova-ledger check, and will accept an empty canonical. So a height above blk proves
		// nothing about what was exported; read the tip directly.
		// There is no ids.Empty exemption. A node reporting an export height at or above blk with
		// an empty canonical is the precise shape SyncQuasarFrontier can seed, and excusing it
		// here would skip the one case this check exists for.
		if tip := n.quasarTip(); !chain[tip] {
			t.Fatalf("invariant: node %d reports export height >= %d but its tip %s is not on blk's "+
				"canonical line (ids.Empty means the frontier advanced without naming a canonical)",
				i, blk.height, tip)
		}
		// Second, nothing conflicting was certified at blk's own height. A Quasar tip finalizes
		// its ancestors, so this is where "no two conflicting Quasars at one height" is readable.
		got, held := n.rt.FinalizedBlockAtHeight(blk.height)
		if !held {
			t.Fatalf("invariant: node %d exported through height %d but holds no finalized block at %d",
				i, qh, blk.height)
		}
		if got != blk.ID() {
			t.Fatalf("invariant: node %d exported a conflicting canonical %s at height %d (want %s)",
				i, got, blk.height, blk.ID())
		}
	}
}

// -----------------------------------------------------------------------------
// A 3/5 partition, seen from both sides: neither side accepts, and the heal decides one
// block.
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_Partition_NeitherSideAccepts splits a 5-net into a 3-side {0,1,2} and
// a 2-side {3,4}. Neither side accepts or exports: 3/5 is 60% of stake, not more than ⅔. On
// heal the block the 3-side signed reaches the other two, they sign it, and all five decide
// and export that one block.
func TestNovaQuasarMatrix_Partition_NeitherSideAccepts(t *testing.T) {
	skipMultinodeUnderRace(t)
	net := newSimNet(t, 5, prodParams5())
	// Partition: isolate {3,4} from the bus (model as down for delivery on both sides).
	net.down(3)
	net.down(4)

	blk1 := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "partition-h1")
	net.build(0, blk1)
	if !waitFor(emergeTO, func() bool {
		for i := 0; i < 3; i++ {
			if !net.nodes[i].rt.hasSignedHeight(1) {
				return false
			}
		}
		return true
	}) {
		t.Fatal("precondition: the 3-side must sign height 1")
	}
	if waitFor(2*time.Second, func() bool { return len(net.headsAtHeight(1)) > 0 }) {
		t.Fatalf("the 3-side (60%% of stake) accepted during the partition: heads=%v", net.headsAtHeight(1))
	}
	if tips := net.quasarTips(); len(tips) != 0 {
		t.Fatalf("no side may export during the partition (neither holds ⅔), got export tips=%v", tips)
	}

	// Heal: bring {3,4} back and hand them the block (its proposer's push, which a quiet
	// height's backoff would otherwise send later). They sign it, and ⅔ is reached.
	net.up(3)
	net.up(4)
	net.send(0, blk1, 3, 4)
	if !waitFor(emergeTO, func() bool {
		all, fork := net.finalizedEverywhere(blk1)
		return all && !fork
	}) {
		t.Fatalf("after the heal all five must decide %s, heads=%v", blk1.ID(), net.headsAtHeight(1))
	}
	if heads := net.headsAtHeight(1); len(heads) != 1 {
		t.Fatalf("two blocks decided at height 1 across the heal: %v", heads)
	}
	// The block was accepted on its ⅔ certificate, so it exports, and on one chain.
	if !waitFor(exportTO, func() bool {
		for _, n := range net.nodes {
			if qh, ok := n.quasarHeight(); !ok || qh < blk1.height {
				return false
			}
		}
		return true
	}) {
		t.Fatalf("after the heal every node must export %s, tipHeights=%v tips=%v",
			blk1.ID(), net.quasarTipHeights(), net.quasarTips())
	}
	if tips := net.quasarTips(); len(tips) != 1 || tips[blk1.ID()] != 5 {
		t.Fatalf("invariant after the heal: export tips=%v, want only %s on all five", tips, blk1.ID())
	}
}

// -----------------------------------------------------------------------------
// One node behind or restarting is a non-event: the other four keep deciding, the rejoining
// node catches up, and export continues. No stall.
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_BehindNodeRejoin_NonEvent takes one of five nodes down, proves the
// remaining 4 (80% of stake) keep deciding across several heights, then
// brings the node back and proves it catches up to the tip while the fleet keeps exporting.
// Taking one node out of a five-node set is a non-event by construction, and this is what that
// costs in practice. (That 4-of-5 exports while the node is away is proven by
// TestNovaQuasarMatrix_FourOfFive_ExportsQuasar; here the subject is production and the
// rejoin.)
func TestNovaQuasarMatrix_BehindNodeRejoin_NonEvent(t *testing.T) {
	skipMultinodeUnderRace(t)
	net := newSimNet(t, 5, prodParams5Fast())
	net.down(4) // node 4 is behind/down

	parentID := ids.Empty
	parentStateRoot := simGenesisRoot()
	var last *simBlock
	const heights = 4
	for h := uint64(1); h <= heights; h++ {
		blk := newHonestBlock(parentID, parentStateRoot, h, "rejoin-h")
		net.build(int((h-1)%4), blk) // rotate the builder across the 4 up nodes
		if !waitFor(emergeTO, func() bool {
			all, fork := net.finalizedEverywhere(blk)
			return all && !fork
		}) {
			t.Fatalf("halted at height %d with node 4 down (4-of-5 up) — ⅔ is up and must keep deciding, heads=%v", h, net.headsAtHeight(h))
		}
		parentID = blk.ID()
		parentStateRoot = blk.stateRoot
		last = blk
	}

	// Rejoin: node 4 comes back and must catch up to the tip via the fetch-missing-ancestor path,
	// finalizing the block the fleet already accepted — with no stall.
	net.nodes[4].setUp(true)
	// Nudge production so gossip/catch-up flows to node 4.
	next := newHonestBlock(last.ID(), last.stateRoot, heights+1, "rejoin-after")
	net.build(0, next)
	if !waitFor(emergeTO, func() bool {
		got, ok := net.nodes[4].rt.FinalizedBlockAtHeight(last.height)
		return ok && got == last.ID()
	}) {
		gotTip, gotH, _ := net.nodes[4].finalizedTip()
		t.Fatalf("rejoin stalled: node 4 did not catch up to height %d block %s after rejoining (its tip=%s@%d) — "+
			"a behind-node rejoin must be a non-event", last.height, last.ID(), gotTip, gotH)
	}
	// The rejoin is a non-event: nothing stalled while node 4 was down, and node 4 caught up on
	// return. Export at ⅔ and the no-second-certificate invariant are proven deterministically,
	// and -race-clean, by TestNovaQuasarMatrix_FourOfFive_ExportsQuasar and
	// TestNovaQuasarMatrix_Equivocation_NoSecondAcceptance.
}

// -----------------------------------------------------------------------------
// The invariant under equivocation: no second accepting certificate at a height.
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_Equivocation_NoSecondAcceptance constructs the equivocation at the cert
// layer — the deterministic form of the storm — with two conflicting blocks A and B at one
// height. A is signed by {0,1,2,3}, a ⅔ certificate. Two equivocators {1,2} sign B as well, with
// one honest voter {4}: a bare majority for B, which the engine does not assemble into an
// accepting certificate, and which the arrival door refuses at the ⅔ floor whatever its
// label says. A second
// ⅔ certificate would take k ≥ 2α−n = 3 equivocators, more than ⅓ of stake.
func TestNovaQuasarMatrix_Equivocation_NoSecondAcceptance(t *testing.T) {
	const n = 5
	const alpha = 4 // ⅔ count for n=5
	vs := newTestValidatorSet(n)
	e, cid := newQuorumEngine(t, params5(), vs, 0, &recordingGossiper{})

	A := newTestBlock(1, ids.Empty, "equiv-A")
	B := newTestBlock(1, ids.Empty, "equiv-B")
	_ = trackProposal(e, cid, A, 0) // engine (node 0) self-signs A
	_ = trackProposal(e, cid, B, 0) // vote-guard refuses node 0's self-vote for B

	addRaw := func(blockID ids.ID, voters ...int) {
		e.mu.Lock()
		defer e.mu.Unlock()
		pb := e.pendingBlocks[blockID]
		pos := e.blockPositionLocked(pb, blockID)
		for _, i := range voters {
			e.recordCertVoteLocked(pb, Vote{BlockID: blockID, NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos), ParentID: pos.ParentID})
		}
	}
	// The distinct verified signers a block holds.
	verifiedVotes := func(blockID ids.ID) []SignedVote {
		e.mu.Lock()
		defer e.mu.Unlock()
		pb := e.pendingBlocks[blockID]
		pos := e.blockPositionLocked(pb, blockID)
		out := make([]SignedVote, 0, len(pb.certVotes))
		for _, v := range pb.certVotes {
			if e.voteVerifier.VerifyVote(v.NodeID, CanonicalVoteMessage(pos), v.Signature, 0) {
				out = append(out, v)
			}
		}
		return out
	}
	// acceptCertOK: the engine's own accepting certificate assembles for the block.
	acceptCertOK := func(blockID ids.ID) bool {
		e.mu.Lock()
		defer e.mu.Unlock()
		_, ok := e.assembleVerifiedCertLocked(e.pendingBlocks[blockID], blockID)
		return ok
	}
	quasarCertOK := func(blockID ids.ID) bool {
		votes := verifiedVotes(blockID)
		if len(votes) < alpha {
			return false
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		pos := e.blockPositionLocked(e.pendingBlocks[blockID], blockID)
		cert, err := AssembleQuorumCert(pos, Quasar, uint32(alpha), votes)
		return err == nil && cert.Verify(e.voteVerifier, 0) == nil
	}

	// A reaches ⅔ honestly: {0,1,2,3}.
	addRaw(A.id, 1, 2, 3)
	if !quasarCertOK(A.id) || !acceptCertOK(A.id) {
		t.Fatal("A must hold a valid ⅔ certificate, and the engine must assemble it")
	}

	// B: 2 equivocators {1,2}, who also signed A, plus 1 honest voter {4}. A bare majority.
	addRaw(B.id, 1, 2, 4)
	if acceptCertOK(B.id) {
		t.Fatal("a bare majority (3 of 5) assembled an accepting certificate for a conflicting block")
	}
	if quasarCertOK(B.id) {
		t.Fatal("invariant breached: a second conflicting ⅔ certificate formed with only 2 equivocators (below ⅓ stake)")
	}
	// The same three signatures labeled Nova verify as a statement and are still no
	// certificate this node accepts, serves or re-gossips: the door reads them at ⅔.
	votes := verifiedVotes(B.id)
	e.mu.Lock()
	pos := e.blockPositionLocked(e.pendingBlocks[B.id], B.id)
	e.mu.Unlock()
	nova, err := AssembleQuorumCert(pos, Nova, uint32(novaQ(n)), votes)
	if err != nil {
		t.Fatalf("assembling the majority statement: %v", err)
	}
	if err := nova.Verify(e.voteVerifier, 0); err != nil {
		t.Fatalf("control: the majority statement's signatures must verify: %v", err)
	}
	if err := e.verifyCert(nova, 0); err == nil {
		t.Fatal("the arrival door admitted a bare majority under a Nova label")
	}
}

// -----------------------------------------------------------------------------
// Degraded-mode RPC visibility.
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_DegradedRPC asserts the RPC snapshot never reports a height as accepted
// or certified below ⅔: at 3 of 5 (60% ≤ ⅔) neither the accept height nor the export height
// advances; a 4th vote (80% > ⅔) accepts and certifies together, and the snapshot reports it
// certified.
func TestNovaQuasarMatrix_DegradedRPC(t *testing.T) {
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, cid := newQuorumEngineOpts(t, dyn5(), vs, 0, rec, WithStakeWeighting(vs))

	blk := newTestBlock(1, ids.Empty, "degraded")
	pos := trackProposal(e, cid, blk, 0) // self = 1 vote
	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos)) // 3 of 5 = 60% stake

	if waitFor(time.Second, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("3/5 (60% stake) accepted")
	}
	s := e.FinalityStatus()
	if s.NovaHeight >= blk.height || s.QuasarHeight >= blk.height {
		t.Fatalf("a height signed by 3/5 was reported accepted or certified: %+v", s)
	}

	// A 4th vote (80% > ⅔) accepts and certifies.
	e.ReceiveVote(vs.signedVote(3, pos))
	if !waitFor(2*time.Second, func() bool {
		s := e.FinalityStatus()
		return !s.Degraded && s.CertificateAvailable && s.NovaHeight >= blk.height && s.QuasarHeight >= blk.height
	}) {
		s := e.FinalityStatus()
		t.Fatalf("after the 4th vote (80%% > ⅔) the chain must accept and certify: %+v", s)
	}
	if s := e.FinalityStatus(); s.ResponsiveStakePct < 0.79 {
		t.Fatalf("responsiveStakePct must be ~0.80 at 4/5, got %v", s.ResponsiveStakePct)
	}
}

// waitForExportFrontier waits for the Quasar EXPORT FRONTIER on any reachable node to reach at
// least `blk`'s height, driving successive honest heights on top of `blk` while it waits.
//
// A block is accepted on its ⅔ certificate and promoteQuasar exports it from those votes, and a
// Quasar tip finalizes all of its ancestors. Driving heights is what a live fleet does, so this
// asserts export liveness the way an export consumer experiences it: the frontier moves, and
// everything at or below it is exported.
func waitForExportFrontier(net *simNet, blk *simBlock, timeout time.Duration) (bool, map[ids.ID]bool) {
	// chain is every block on the canonical line at or above blk, so the caller can assert that
	// whatever the frontier settled on is ON that line — a direct quasarTip() read that survives
	// the frontier moving past blk.
	chain := map[ids.ID]bool{blk.ID(): true}
	exported := func() bool {
		for _, n := range net.nodes {
			if !n.reachable() {
				continue
			}
			if qh, ok := n.quasarHeight(); ok && qh >= blk.height {
				return true
			}
		}
		return false
	}
	if exported() {
		return true, chain
	}

	deadline := time.Now().Add(timeout)
	parentID, parentRoot, height := blk.ID(), blk.stateRoot, blk.height
	for time.Now().Before(deadline) {
		// Rotate the builder across the reachable nodes so no single node's liveness is the
		// thing under test.
		builder := int(height+1) % len(net.nodes)
		for i := 0; i < len(net.nodes) && !net.nodes[builder].reachable(); i++ {
			builder = (builder + 1) % len(net.nodes)
		}
		next := newHonestBlock(parentID, parentRoot, height+1, "export-frontier")
		net.build(builder, next)

		// Advance the cursor only on a converged height. Bumping height while leaving the parent
		// behind would build every later block on a stale parent, and a genuine export stall
		// would then be reported as a pile of unconvergeable garbage instead of as a stall.
		if !waitFor(emergeTO, func() bool {
			all, fork := net.finalizedEverywhere(next)
			return all && !fork
		}) {
			return exported(), chain
		}
		parentID, parentRoot, height = next.ID(), next.stateRoot, next.height
		chain[next.ID()] = true
		if exported() {
			return true, chain
		}
	}
	return exported(), chain
}
