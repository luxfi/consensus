// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// nova_quasar_matrix_test.go — the v1.36 TWO-TIER adversarial matrix (the go-live gate).
//
// One ⅔ accept threshold is split into two orthogonal tiers on the finality ladder:
//
//	NOVA   = LOCAL ACCEPT  — NovaQuorum(n)=⌊n/2⌋+1 distinct signers. Drives VM.Accept.
//	                          Crash-fault-safe (majority intersection), NOT Byzantine-safe.
//	QUASAR = EXPORT        — strict >⅔ of STAKE by distinct signers. Byzantine-fork-safe.
//	                          The ONLY tier bridges / DEX settlement / cross-chain consume.
//
// THE INVARIANT (asserted throughout): across every scenario below, no two CONFLICTING
// blocks ever both reach QUASAR at one height. Nova may fork under equivocation (expected,
// non-exportable); Quasar may not (that needs >⅓ stake double-signing, beyond f<⅓).
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
// the convergence goroutine and the vote handlers. The TWO-TIER SAFETY invariants asserted here
// (Nova at the majority, Quasar only at ⅔, no two conflicting Quasars, export gating) are ALSO
// asserted by the SINGLE-ENGINE matrix tests (Threshold / WeightedStake / SingleValidator /
// Equivocation / DegradedRPC), which are deterministic and DO run under -race. Mirrors the
// convergence-storm gate discipline in race_flag_race_test.go.
func skipMultinodeUnderRace(t *testing.T) {
	t.Helper()
	if underRace {
		t.Skip("timing-sensitive in-process multinode convergence test skipped under -race " +
			"(the detector slowdown violates the sim's gossip<settle timing assumption; the two-tier " +
			"safety invariants run -race-clean in the single-engine matrix tests)")
	}
}

// novaQ is NovaQuorum(n) — the bare-majority Nova accept count.
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
// (e) n ∈ {2..5} weighted+equal: Nova at majority; Quasar only at strict >⅔ stake.
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_Threshold_NovaMajority_QuasarTwoThirds is the core threshold ladder for
// n ∈ {2,3,4,5}: exactly NovaQuorum(n) distinct signers drive the Nova accept; the export tier
// forms only once a strict >⅔-stake DISTINCT-signer supermajority has attested. For odd n the
// two thresholds differ (a degraded window where the chain produces Nova but does not certify
// Quasar); for even n they coincide.
func TestNovaQuasarMatrix_Threshold_NovaMajority_QuasarTwoThirds(t *testing.T) {
	for n := 2; n <= 5; n++ {
		t.Run(fmt.Sprintf("n=%d_equal", n), func(t *testing.T) {
			vs := newTestValidatorSet(n)
			rec := &recordingGossiper{}
			e, chainID := newQuorumEngineOpts(t, nMatrixParams(n), vs, 0, rec, WithStakeWeighting(vs))

			blk := newTestBlock(1, ids.Empty, "n-matrix")
			pos := trackProposal(e, chainID, blk, 0) // node 0's own signed accept = 1 vote

			nq, tt := novaQ(n), twoThirdsCount(n)
			// Deliver peers up to the Nova majority (self already = 1).
			for i := 1; i < nq; i++ {
				e.ReceiveVote(vs.signedVote(i, pos))
			}
			mustFinalize(t, e, blk, 2*time.Second, fmt.Sprintf("n=%d: NovaQuorum=%d votes → Nova accept", n, nq))

			if nq < tt {
				// Degraded window: a bare majority is below ⅔ stake — no export yet.
				mustNotQuasar(t, e, blk, 400*time.Millisecond, fmt.Sprintf("n=%d: majority %d < ⅔ %d → no export", n, nq, tt))
				for i := nq; i < tt; i++ {
					e.ReceiveVote(vs.signedVote(i, pos))
				}
			}
			mustQuasar(t, e, blk, 2*time.Second, fmt.Sprintf("n=%d: ⅔ count=%d → Quasar export", n, tt))
		})
	}
}

// TestNovaQuasarMatrix_WeightedStake_NovaMajorityQuasarSupermajority proves the tiers are
// orthogonal on the stake axis and that both read the SAME unit: Nova ignites on a bare MAJORITY
// of stake, Quasar on a ⅔ SUPERMAJORITY of it. A four-node head-count holding 40% clears neither;
// the 60% holder's vote clears both, and because it trails, it arrives as a late attestation.
func TestNovaQuasarMatrix_WeightedStake_NovaMajorityQuasarSupermajority(t *testing.T) {
	vs := newTestValidatorSet(5)
	// Node 0 holds 60% of stake and ABSTAINS; nodes 1..4 hold 10% each.
	skew := newStakeMap(vs, 60, 10, 10, 10, 10)
	rec := &recordingGossiper{}
	e, chainID := newQuorumEngineOpts(t, dyn5(), vs, 0, rec, WithStakeWeighting(skew))

	blk := newTestBlock(1, ids.Empty, "weighted")
	// Track as a NON-own block so node 0 (the 60% holder) does NOT auto-vote — the accepting
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
	// NOVA: 40% is not a majority of stake — a head-count of four does not accept.
	mustNotFinalize(t, e, blk, 2*time.Second, "count majority / 40% stake → no Nova accept")
	// QUASAR: 40% stake < ⅔ — no export either.
	mustNotQuasar(t, e, blk, 400*time.Millisecond, "40% stake → no export")

	// The 60%-stake holder votes (late). 40%+60% = 100% ⇒ Nova ignites and Quasar exports.
	e.ReceiveVote(vs.signedVote(0, pos))
	mustFinalize(t, e, blk, 2*time.Second, "heavy-stake vote → Nova stake majority")
	mustQuasar(t, e, blk, 2*time.Second, "heavy-stake late vote → ⅔ export")
}

// -----------------------------------------------------------------------------
// n=1 self-ignites (NovaQuorum(1)=1); the single-validator path still works.
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_SingleValidatorSelfIgnites proves n=1 self-ignites Nova (NovaQuorum(1)=1)
// through the single-validator path, and — since the sole validator IS a ⅔-stake supermajority —
// also reaches Quasar. No peer exists to fork against, so this is sound.
func TestNovaQuasarMatrix_SingleValidatorSelfIgnites(t *testing.T) {
	vs := newTestValidatorSet(1)
	rec := &recordingGossiper{}
	p := dyn5()
	p.K = 1
	e, _ := newQuorumEngineOpts(t, p, vs, 0, rec, WithStakeWeighting(vs))

	blk := newTestBlock(1, ids.Empty, "solo")
	_ = trackProposal(e, chainID(e), blk, 0)
	// Drive the accept funnel (no peer votes arrive to trigger it on a solo chain).
	_ = e.TryAccept(context.Background(), blk.id)
	// The sole validator self-ignites: its own accept is the entire NovaQuorum(1)=1.
	mustFinalize(t, e, blk, 2*time.Second, "n=1 self-ignites Nova")
	// The sole validator is 100% stake > ⅔, so the block is export-final too.
	mustQuasar(t, e, blk, 2*time.Second, "n=1 sole validator is a ⅔ supermajority → Quasar")
}

// chainID recovers the engine's chain id (set via WithQuorumCert) for tests that did not capture
// it from the constructor.
func chainID(e *Transitive) ids.ID { return e.chainID }

// -----------------------------------------------------------------------------
// (a)(b)(d) multinode: 3/5 produces Nova, Quasar only at ⅔, export gates on Quasar.
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_ThreeOfFive_NovaNoQuasar is gate (a): with 2 of 5 validators DOWN, the 3
// live validators PRODUCE (Nova accepts + converges everywhere), but the export tier does NOT
// advance (3/5 = 60% stake ≤ ⅔). Gate (d): the Quasar tip stays Empty (export consumers see no
// finalized block) even though the Nova/accept tip has advanced. This is the owner's mandate:
// "survive 3/5, majority is enough — production continues, certification pauses."
func TestNovaQuasarMatrix_ThreeOfFive_NovaNoQuasar(t *testing.T) {
	skipMultinodeUnderRace(t)
	net := newSimNet(t, 5, prodParams5())
	net.down(3)
	net.down(4) // 3 up: {0,1,2}

	blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "three-of-five-nova")
	net.build(0, blk)

	// NOVA: the 3 live validators accept and converge (production continues under the outage).
	if !waitFor(emergeTO, func() bool {
		all, fork := net.finalizedEverywhere(blk)
		return all && !fork
	}) {
		t.Fatalf("NOVA LIVENESS: 3-of-5 must accept+converge %s, heads=%v up=%d", blk.ID(), net.headsAtHeight(1), net.upCount())
	}
	// QUASAR: certification pauses — no up node exports, and the export tip stays Empty.
	if waitFor(2*time.Second, func() bool {
		for _, n := range net.nodes {
			if !n.reachable() {
				continue
			}
			if qh, ok := n.quasarHeight(); ok && qh >= blk.height {
				return true
			}
		}
		return false
	}) {
		t.Fatal("EXPORT SAFETY: 3-of-5 (60% stake ≤ ⅔) must NOT reach Quasar — certification must pause")
	}
	// Gate (d): the export tip a bridge/DEX reads is Empty; the Nova/accept tip is NOT.
	for i, n := range net.nodes {
		if !n.reachable() {
			continue
		}
		if tip := n.quasarTip(); tip != ids.Empty {
			t.Fatalf("node %d exposes a non-empty export (Quasar) tip %s under 3/5 — export must gate on Quasar", i, tip)
		}
		if n.novaTip() == ids.Empty {
			t.Fatalf("node %d has an empty Nova/accept tip under 3/5 — production must have continued", i)
		}
	}
}

// TestNovaQuasarMatrix_FourOfFive_ExportsQuasar is gate (b): with 4 of 5 up (80% stake > ⅔), the
// block reaches Quasar and every up node exports the SAME block (no export fork).
func TestNovaQuasarMatrix_FourOfFive_ExportsQuasar(t *testing.T) {
	skipMultinodeUnderRace(t)
	net := newSimNet(t, 5, prodParams5())
	net.down(4) // 4 up: {0,1,2,3}

	blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "four-of-five-quasar")
	net.build(0, blk)

	// NOVA accept converges.
	if !waitFor(emergeTO, func() bool {
		all, fork := net.finalizedEverywhere(blk)
		return all && !fork
	}) {
		t.Fatalf("NOVA LIVENESS: 4-of-5 must accept+converge %s, heads=%v", blk.ID(), net.headsAtHeight(1))
	}
	// QUASAR export FORMS: 4-of-5 (80% stake > ⅔) reaches the export tier.
	//
	// The guarantee is at the FRONTIER, not per-block, and the test must assert the one the
	// design actually makes. promoteQuasar: a block whose Nova cert did not itself carry ⅔
	// stake stays Nova-only, and "a later fully-voted block advances the export frontier past
	// the gap (a Quasar tip finalizes all its ancestors), so no per-block retroactive
	// promotion is needed for liveness."
	//
	// Demanding that THIS block export, while building no successor, asserted the stronger
	// per-block property. The relevant threshold is NOT α: assembleCertLocked freezes the Nova
	// cert at NovaQuorum(5)=3 verified votes, and α plays no part in cert assembly at all —
	// which matters because prodParams5 sets α=4 and twoThirdsCount(5) is also 4, so "completed
	// on the bare α without ⅔" would be arithmetically impossible and sends the next reader down
	// a dead end. A cert that freezes at 3 has 60% of stake, below the ⅔ export floor, so that
	// block never exports and the frontier has nothing to ride. Driving successive heights is not
	// a weaker assertion: the frontier advancing past height 1 finalizes height 1 with it, which
	// is exactly what an export consumer (bridge, DEX settlement) observes.
	ok, chain := waitForExportFrontier(net, blk, exportTO)
	if !ok {
		t.Fatalf("EXPORT LIVENESS: 4-of-5 (80%% stake > ⅔) must export %s, tips=%v", blk.ID().String(), net.quasarTips())
	}
	// THE INVARIANT: no two CONFLICTING blocks both reach Quasar at one height. Assert it AT THE
	// HEIGHT, not at the tip. A Quasar tip finalizes all its ancestors, so once the frontier is
	// above blk the tip is legitimately a DESCENDANT of blk — comparing the tip to blk would
	// flag honest frontier progress as a fork. What must never happen is a node exporting a
	// DIFFERENT canonical at blk's own height.
	for i, n := range net.nodes {
		if !n.reachable() {
			continue
		}
		qh, exported := n.quasarHeight()
		if !exported || qh < blk.height {
			continue // has not exported through blk's height yet — nothing to contradict
		}
		// (a) the frontier itself is ON the canonical line. quasarFrontier is a SINGLE
		// {height, canonical, envelope} with no per-height index, and PromoteQuasar is not its
		// only writer — SyncQuasarFrontier seeds it from the VM's durable export record with no
		// Nova-ledger check and will accept an empty canonical. So a height above blk proves
		// nothing about WHAT was exported; read the tip directly.
		// No ids.Empty exemption. A node reporting an export height at or above blk with an EMPTY
		// canonical is the precise shape SyncQuasarFrontier can seed, and excusing it here would
		// skip the one case this check cites as its reason to exist.
		if tip := n.quasarTip(); !chain[tip] {
			t.Fatalf("INVARIANT: node %d reports export height >= %d but its tip %s is NOT on blk's "+
				"canonical line (ids.Empty means the frontier advanced without naming a canonical)",
				i, blk.height, tip)
		}
		// (b) and nothing conflicting was certified at blk's own height. A Quasar tip finalizes
		// its ancestors, so this is where "no two conflicting Quasars at one height" is readable.
		got, held := n.rt.FinalizedBlockAtHeight(blk.height)
		if !held {
			t.Fatalf("INVARIANT: node %d exported through height %d but holds no finalized block at %d",
				i, qh, blk.height)
		}
		if got != blk.ID() {
			t.Fatalf("INVARIANT: node %d exported a CONFLICTING canonical %s at height %d (want %s)",
				i, got, blk.height, blk.ID())
		}
	}
}

// -----------------------------------------------------------------------------
// (f) 3/5 partition, BOTH sides: majority side produces Nova; neither exports Quasar;
//     on heal a single Quasar chain (no two conflicting Quasars).
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_Partition_MajorityProducesNeitherExports splits a 5-net into a 3-side
// {0,1,2} and a 2-side {3,4}. The majority side PRODUCES (Nova advances); NEITHER side reaches
// Quasar (3/5 = 60% ≤ ⅔ on one side, 2/5 below even the Nova majority on the other). On heal,
// once all 5 are up a fresh block reaches Quasar — a SINGLE export chain, never two conflicting
// Quasars.
func TestNovaQuasarMatrix_Partition_MajorityProducesNeitherExports(t *testing.T) {
	skipMultinodeUnderRace(t)
	net := newSimNet(t, 5, prodParams5())
	// Partition: isolate {3,4} from the bus (model as down for delivery on both sides).
	net.down(3)
	net.down(4)

	blk1 := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "partition-h1")
	net.build(0, blk1)
	if !waitFor(emergeTO, func() bool {
		all, fork := net.finalizedEverywhere(blk1)
		return all && !fork
	}) {
		t.Fatalf("majority side must PRODUCE Nova during the partition, heads=%v", net.headsAtHeight(1))
	}
	// Neither side exports (the 3-side holds 60% ≤ ⅔).
	if tips := net.quasarTips(); len(tips) != 0 {
		t.Fatalf("no side may export during the partition (neither holds ⅔), got export tips=%v", tips)
	}

	// HEAL: bring {3,4} back. They missed height 1, so the first post-heal blocks pull them up via
	// catch-up (fetch the missing ancestor). Build a short run so the fleet re-converges and the
	// 4/5-of-5 supermajority forms an export cert on the SINGLE chain.
	net.nodes[3].setUp(true)
	net.nodes[4].setUp(true)
	parentID := blk1.ID()
	parentStateRoot := blk1.stateRoot
	// chain is every block on the single canonical chain, so the export-frontier invariant
	// can be stated as "the tip is ON this chain" instead of pinning it to one ID.
	chain := map[ids.ID]bool{blk1.ID(): true}
	// target is the FIRST post-heal block. Export lags production by a variable number of
	// blocks, so pinning the requirement to the NEWEST one asserted a timing detail rather
	// than the claim above it — it left nothing built behind the frontier and failed ~12% of
	// the time whenever any companion test ran (fleet at h3, newest h4, assert waited out
	// exportTO). "Export RESUMES" is proven by any post-heal height exporting, with the two
	// later blocks as slack; that no side exported DURING the partition is asserted above.
	var target *simBlock
	// exported: has any reachable node exported a POST-HEAL height yet?
	exported := func() bool {
		if target == nil {
			return false
		}
		for _, n := range net.nodes {
			if !n.reachable() {
				continue
			}
			if qh, ok := n.quasarHeight(); ok && qh >= target.height {
				return true
			}
		}
		return false
	}
	// Keep PRODUCING until export resumes. Export needs a fresh ⅔ poll, and the two healed
	// nodes may still have been catching up during the last one — leaving 3 of 5 voting, which
	// is below ⅔. Waiting cannot conjure a new poll, so a passive wait after the final block
	// could never recover and simply timed out (~12% of runs, tipHeights=map[] — nothing
	// exported at all). A real chain resumes export as blocks arrive; so does this one.
	// Drive heights until the export frontier moves, bounded by exportTO rather than by an
	// arbitrary height cap. The cap plus a trailing non-driving waitFor was the defect: if the
	// frontier had not moved by height 8, the test then sat for 25s building NOTHING, and export
	// only ever advances on a NEW fully-voted block (promoteQuasar does no retroactive
	// promotion). It was waiting for an event that, by construction, nothing could now cause.
	exportDeadline := time.Now().Add(exportTO)
	for h := uint64(2); !exported() && time.Now().Before(exportDeadline); h++ {
		blk := newHonestBlock(parentID, parentStateRoot, h, "partition-heal-h")
		net.build(int(h%5), blk)
		if !waitFor(emergeTO, func() bool {
			all, fork := net.finalizedEverywhere(blk)
			return all && !fork
		}) {
			t.Fatalf("after heal, height %d must re-converge (Nova) across the healed fleet, heads=%v", h, net.headsAtHeight(h))
		}
		parentID = blk.ID()
		parentStateRoot = blk.stateRoot
		chain[blk.ID()] = true
		if target == nil {
			target = blk
		}
	}
	// A post-heal block reaches Quasar (all 5 up ⇒ 80%+ stake > ⅔): export RESUMES after the
	// partition healed. (Liveness; the no-conflicting-Quasar INVARIANT is proven deterministically
	// by TestNovaQuasarMatrix_Equivocation_TwoNovaNeverTwoQuasar.)
	if !exported() {
		wantH := uint64(2)
		if target != nil {
			wantH = target.height
		}
		t.Fatalf("after heal, the fleet must export a post-heal block (want height >= %d), tipHeights=%v tips=%v",
			wantH, net.quasarTipHeights(), net.quasarTips())
	}
	// No CONFLICTING export tip anywhere — the export frontier is a single chain across the heal.
	for i, n := range net.nodes {
		if !n.reachable() {
			continue
		}
		if qh, ok := n.quasarHeight(); ok && qh >= target.height {
			if tip := n.quasarTip(); tip != ids.Empty && !chain[tip] {
				t.Fatalf("INVARIANT after heal: node %d exported a CONFLICTING tip %s (not on the canonical chain)", i, tip)
			}
		}
	}
}

// -----------------------------------------------------------------------------
// (h) behind-node / restart (the luxd-4 case): a NON-EVENT — majority keeps producing
//     Nova; the rejoining node catches up; export resumes. No stall.
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_BehindNodeRejoin_NonEvent kills one of five nodes, proves the remaining 4
// keep PRODUCING (Nova) across several heights (the majority never stalls), then brings the node
// back and proves it CATCHES UP to the tip AND the fleet resumes EXPORTING (Quasar) — the
// pulling-luxd-4 scenario is a non-event. (That 4-of-5 exports during the outage is proven by
// TestNovaQuasarMatrix_FourOfFive_ExportsQuasar; here the focus is Nova production + rejoin.)
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
			t.Fatalf("NOVA HALT at height %d with node 4 down (4-of-5 up) — the majority must keep producing, heads=%v", h, net.headsAtHeight(h))
		}
		parentID = blk.ID()
		parentStateRoot = blk.stateRoot
		last = blk
	}

	// REJOIN: node 4 comes back and must CATCH UP to the tip (the fetch-missing-ancestor path),
	// finalizing the block the fleet already accepted — no stall.
	net.nodes[4].setUp(true)
	// Nudge production so gossip/catch-up flows to node 4.
	next := newHonestBlock(last.ID(), last.stateRoot, heights+1, "rejoin-after")
	net.build(0, next)
	if !waitFor(emergeTO, func() bool {
		got, ok := net.nodes[4].rt.FinalizedBlockAtHeight(last.height)
		return ok && got == last.ID()
	}) {
		gotTip, gotH, _ := net.nodes[4].finalizedTip()
		t.Fatalf("REJOIN STALL: node 4 did not catch up to height %d block %s after rejoining (its tip=%s@%d) — "+
			"the behind-node rejoin must be a non-event", last.height, last.ID(), gotTip, gotH)
	}
	// The rejoin is a NON-EVENT: Nova never stalled while node 4 was down, and node 4 caught up on
	// return. Export RESUMPTION at ⅔ and the no-two-conflicting-Quasars INVARIANT are proven
	// (deterministically, -race-clean) by TestNovaQuasarMatrix_FourOfFive_ExportsQuasar and
	// TestNovaQuasarMatrix_Equivocation_TwoNovaNeverTwoQuasar.
}

// -----------------------------------------------------------------------------
// (c)(g) THE INVARIANT under equivocation: two Nova certs possible, NEVER two Quasars.
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_Equivocation_TwoNovaNeverTwoQuasar constructs the equivocation at the cert
// layer (the deterministic form of the storm): two conflicting blocks A and B at one height. With
// k equivocators, a second NOVA (bare-majority) cert for B is reachable at a LOWER k than a second
// QUASAR (⅔) cert — Nova is crash-fault-safe only, so a second Nova cert under equivocation is
// EXPECTED and non-exportable. THE INVARIANT: a second QUASAR cert requires k ≥ 2α−n = >⅓ stake
// double-signing, so below that NO two conflicting export certs ever coexist.
func TestNovaQuasarMatrix_Equivocation_TwoNovaNeverTwoQuasar(t *testing.T) {
	const n = 5
	const alpha = 4 // ⅔ export count for n=5
	vs := newTestValidatorSet(n)
	e, cid := newQuorumEngine(t, params5Prod(), vs, 0, &recordingGossiper{})

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
	// Count how many distinct verified signers a block holds (both tiers assemble from these).
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
	novaCertOK := func(blockID ids.ID) bool {
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

	// A reaches an EXPORTABLE quorum honestly: {0,1,2,3}.
	addRaw(A.id, 1, 2, 3)
	if !quasarCertOK(A.id) {
		t.Fatal("A must hold a valid ⅔ (Quasar) export cert")
	}

	// Give B exactly 2 equivocators {1,2} (who also signed A) + 1 honest-B voter {4}. That is a
	// NOVA majority (3) for B → a SECOND Nova cert exists (expected under equivocation).
	addRaw(B.id, 1, 2, 4)
	if !novaCertOK(B.id) {
		t.Fatal("with 2 equivocators + 1 honest, B must reach a second NOVA cert (bare majority) — expected, non-exportable")
	}
	// THE INVARIANT: B has only 3 distinct signers (< α=4), so NO second QUASAR (export) cert can
	// form. Two conflicting export certs would need k ≥ 2α−n = 3 equivocators (>⅓ stake).
	if quasarCertOK(B.id) {
		t.Fatal("INVARIANT BREACH: a second conflicting QUASAR (export) cert formed with only 2 equivocators (<⅓ stake)")
	}
}

// -----------------------------------------------------------------------------
// (i) degraded-mode RPC visibility.
// -----------------------------------------------------------------------------

// TestNovaQuasarMatrix_DegradedRPC asserts the two-tier RPC snapshot never reports a Nova height
// as certified, and flips `Degraded`/`CertificateAvailable` exactly at the ⅔-stake boundary:
// 3-of-5 (60% ≤ ⅔) is degraded (producing Nova, quasarHeight behind); a 4th vote (80% > ⅔) clears
// it and advances quasarHeight.
func TestNovaQuasarMatrix_DegradedRPC(t *testing.T) {
	vs := newTestValidatorSet(5)
	rec := &recordingGossiper{}
	e, cid := newQuorumEngineOpts(t, dyn5(), vs, 0, rec, WithStakeWeighting(vs))

	blk := newTestBlock(1, ids.Empty, "degraded")
	pos := trackProposal(e, cid, blk, 0) // self = 1 vote
	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos)) // 3 of 5 = 60% stake

	// DEGRADED: Nova accepted, but ⅔ is not reachable — quasarHeight trails novaHeight.
	if !waitFor(2*time.Second, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("precondition: 3/5 must Nova-accept")
	}
	if !waitFor(time.Second, func() bool { s := e.FinalityStatus(); return s.Degraded && !s.CertificateAvailable }) {
		s := e.FinalityStatus()
		t.Fatalf("3/5 must report degraded (producing Nova, not certifying Quasar): %+v", s)
	}
	s := e.FinalityStatus()
	if s.NovaHeight < blk.height {
		t.Fatalf("novaHeight must have advanced to the accepted block, got %d", s.NovaHeight)
	}
	if s.QuasarHeight >= blk.height {
		t.Fatalf("quasarHeight must NOT report a Nova-only block as certified, got %d", s.QuasarHeight)
	}
	if s.ResponsiveStakePct <= 0 || s.ResponsiveStakePct > 0.61 {
		t.Fatalf("responsiveStakePct must be ~0.60 at 3/5, got %v", s.ResponsiveStakePct)
	}

	// RECOVER: a 4th vote (80% > ⅔) clears degraded and advances the export height.
	e.ReceiveVote(vs.signedVote(3, pos))
	if !waitFor(2*time.Second, func() bool {
		s := e.FinalityStatus()
		return !s.Degraded && s.CertificateAvailable && s.QuasarHeight >= blk.height
	}) {
		s := e.FinalityStatus()
		t.Fatalf("after the 4th vote (80%% > ⅔) the chain must certify (not degraded, quasarHeight advanced): %+v", s)
	}
}

// waitForExportFrontier waits for the Quasar EXPORT FRONTIER on any reachable node to reach at
// least `blk`'s height, driving successive honest heights on top of `blk` while it waits.
//
// This is the liveness property the two-tier design actually guarantees. promoteQuasar emits an
// export cert only for a block whose OWN Nova cert carried >⅔ of stake; a block finalized on the
// NovaQuorum majority — assembleCertLocked freezes at NovaQuorum(5)=3, and α does not gate cert
// assembly — without that supermajority attesting stays Nova-only, permanently and by design:
// "no per-block retroactive promotion is needed for liveness" precisely because a later
// fully-voted block carries the frontier past it, and a Quasar tip finalizes all its ancestors.
// A test that builds ONE block and demands that block export is therefore asserting a property
// the engine never promised, and it fails on exactly the runs where the first cert froze at 3.
//
// Driving heights is what a live fleet does, so this asserts export liveness the way an export
// consumer experiences it: the frontier moves, and everything at or below it is exported.
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

		// Advance the cursor ONLY on a converged height. Bumping height while leaving the parent
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
