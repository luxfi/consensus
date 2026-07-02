// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// round_view_red_test.go — RED adversarial suite for the round-scoped view-change core.
//
// Blue's round_view_test.go models a WEAK adversary: every node runs the HONEST state
// machine and the "down" node is merely off — never Byzantine. The safety claim, however,
// is stated against a ≤f Byzantine adversary (sound when 2α−n>f). This file supplies the
// missing adversary: a genuine f=1 Byzantine node that EQUIVOCATES — double-prevotes AND
// double-precommits EVERY competing sibling at EVERY round (including future rounds it is
// not at), attempting to manufacture two POLs / two certs at one height. It drives that
// adversary against the real honest roundView machines under adversarial partitions.
//
// The invariant under test is the fork-safety gate: no two honest nodes ever finalize
// different blocks at one height, under ≤f=1 equivocation + partition + reorder.
package chain

import (
	"math/rand"
	"testing"

	"github.com/luxfi/ids"
)

// rvFork reports whether two honest (up) nodes have finalized DIFFERENT blocks. Non-fatal
// (Blue's assertNoFork is fatal); RED needs to detect a fork without aborting so the α=3
// bound-tightness demonstration can assert that a fork DID occur.
func rvFork(nodes []*rvNode) (bool, ids.ID, ids.ID) {
	var head ids.ID
	set := false
	for _, n := range nodes {
		if !n.up || !n.view.finalized {
			continue
		}
		if !set {
			head = n.view.finalizedBlock
			set = true
			continue
		}
		if n.view.finalizedBlock != head {
			return true, head, n.view.finalizedBlock
		}
	}
	return false, ids.Empty, ids.Empty
}

// redStep advances the honest nodes exactly as Blue's stepOnce does (re-gossip seen, step,
// record+observe own votes, emit), then injects the Byzantine node's equivocating votes:
// a prevote AND a precommit for EVERY sibling at EVERY round in [0,maxRound]. The Byzantine
// node (index 4, up=false so the honest loop and deliver skip it as a participant/receiver)
// is exempt from the partition via the caller's link function, modeling a Byzantine peer
// that talks to everyone.
func redStep(s *rvSim, byzID ids.NodeID, sibs []ids.ID, maxRound uint32) {
	var bus []rvMsg
	for _, n := range s.nodes {
		if !n.up {
			continue
		}
		for b := range n.seen {
			bus = append(bus, rvMsg{kind: rvBlock, from: n.id, block: b})
		}
		w := winnerOf(n)
		act := n.view.step(w, 1, s.settle)
		if act.Prevote != ids.Empty {
			n.view.recordOwnPrevote(act.Prevote)
			n.view.observePrevote(n.id, act.CurRound, act.Prevote)
			bus = append(bus, rvMsg{kind: rvPrevote, from: n.id, round: act.CurRound, block: act.Prevote})
		}
		if act.Precommit != ids.Empty {
			if n.view.recordOwnPrecommit(act.Precommit, act.PrecommitRound) {
				n.view.observePrecommit(n.id, act.PrecommitRound, act.Precommit)
				bus = append(bus, rvMsg{kind: rvPrecommit, from: n.id, round: act.PrecommitRound, block: act.Precommit})
			}
		}
	}
	// Byzantine equivocation: EVERY sibling, EVERY round, both phases.
	for _, sib := range sibs {
		for r := uint32(0); r <= maxRound; r++ {
			bus = append(bus, rvMsg{kind: rvPrevote, from: byzID, round: r, block: sib})
			bus = append(bus, rvMsg{kind: rvPrecommit, from: byzID, round: r, block: sib})
		}
	}
	s.deliver(polCertMsgs(s.nodes)) // honest POL gossip (models proposal-carries-POL / anti-entropy)
	s.deliver(bus)
}

// -----------------------------------------------------------------------------
// VECTOR 1/3/6: an f=1 equivocator + a 2|2 honest partition. If the bound 2α−n>f holds
// (α=4,n=5: 3>1) the equivocator CANNOT manufacture two POLs, so nothing forks; and once
// the partition heals the honest nodes converge on the deterministic winner despite the
// equivocator still spamming both siblings forever.
// -----------------------------------------------------------------------------
func TestRoundView_EquivocatorPartition_Alpha4_NoFork(t *testing.T) {
	s := newRVSim(t, 5, 4)
	s.node(4).up = false // node 4 = f=1 Byzantine equivocator (driven by redStep)
	byzID := s.node(4).id

	X := rvBlockID(0xA0)
	Y := rvBlockID(0xB0)
	sibs := []ids.ID{X, Y}

	grp := map[ids.NodeID]int{
		s.node(0).id: 1, s.node(1).id: 1,
		s.node(2).id: 2, s.node(3).id: 2,
	}
	partitioned := true
	s.link = func(from, to ids.NodeID) bool {
		if from == byzID || to == byzID {
			return true // Byzantine talks to everyone
		}
		if partitioned {
			return grp[from] == grp[to]
		}
		return true
	}

	// group {0,1} sees only X, group {2,3} sees only Y — the classic split, now with an
	// equivocator bridging both sides to try to push BOTH to α.
	s.node(0).seen[X] = struct{}{}
	s.node(1).seen[X] = struct{}{}
	s.node(2).seen[Y] = struct{}{}
	s.node(3).seen[Y] = struct{}{}

	for tick := 0; tick < 12; tick++ {
		redStep(s, byzID, sibs, 8)
		if forked, a, b := rvFork(s.nodes); forked {
			t.Fatalf("FORK under partition+equivocator at α=4: %s vs %s", a, b)
		}
	}
	for i := 0; i < 4; i++ {
		if s.node(i).view.finalized {
			t.Fatalf("node %d finalized under a 2|2 split at α=4 (equivocator adds only 1 → 3<4): impossible unless a bug", i)
		}
	}

	// HEAL: honest nodes learn both siblings and must converge on min(X,Y)=X.
	partitioned = false
	converged := false
	for tick := 0; tick < 80 && !converged; tick++ {
		redStep(s, byzID, sibs, 8)
		if forked, a, b := rvFork(s.nodes); forked {
			t.Fatalf("FORK after heal at α=4: %s vs %s", a, b)
		}
		converged = s.node(0).view.finalized && s.node(1).view.finalized &&
			s.node(2).view.finalized && s.node(3).view.finalized
	}
	if !converged {
		t.Fatalf("LIVENESS: honest nodes did not converge after heal despite equivocator")
	}
	for i := 0; i < 4; i++ {
		if s.node(i).view.finalizedBlock != X {
			t.Fatalf("node %d finalized %s, want deterministic winner %s", i, s.node(i).view.finalizedBlock, X)
		}
	}
	t.Logf("RED PASS: f=1 equivocator + 2|2 partition at α=4 — no fork; healed to %s", X)
}

// -----------------------------------------------------------------------------
// VECTOR 5 (bound tightness): the SAME equivocator at α=3 (2α−n = 1 = f, NOT > f). Here
// quorum intersection is exactly f, so the single equivocator sits in BOTH α-quorums and
// double-finalizes. This test asserts the fork OCCURS — proving the safety bound is tight
// and that the engine config MUST enforce α=4 for n=5. If this ever stopped forking, the
// safety argument would need re-examination.
// -----------------------------------------------------------------------------
func TestRoundView_EquivocatorPartition_Alpha3_ForksProvingBound(t *testing.T) {
	s := newRVSim(t, 5, 3) // α=3 → 2α−n = 1 = f=1 → bound 2α−n>f VIOLATED
	s.node(4).up = false
	byzID := s.node(4).id

	X := rvBlockID(0xA0)
	Y := rvBlockID(0xB0)
	sibs := []ids.ID{X, Y}

	grp := map[ids.NodeID]int{
		s.node(0).id: 1, s.node(1).id: 1,
		s.node(2).id: 2, s.node(3).id: 2,
	}
	s.link = func(from, to ids.NodeID) bool {
		if from == byzID || to == byzID {
			return true
		}
		return grp[from] == grp[to] // permanent partition
	}
	s.node(0).seen[X] = struct{}{}
	s.node(1).seen[X] = struct{}{}
	s.node(2).seen[Y] = struct{}{}
	s.node(3).seen[Y] = struct{}{}

	forked := false
	var fa, fb ids.ID
	for tick := 0; tick < 12 && !forked; tick++ {
		redStep(s, byzID, sibs, 4)
		forked, fa, fb = rvFork(s.nodes)
	}
	if !forked {
		t.Fatalf("expected a FORK at α=3 (2α−n=f, bound violated) but none occurred — re-examine the safety model/harness")
	}
	t.Logf("BOUND CONFIRMED TIGHT: at α=3 (2α−n=1 = f) ONE equivocator double-finalized %s vs %s. "+
		"The engine config MUST enforce α=4 for n=5 (2α−n=3>1). round_view.go itself does NOT self-check this.", fa, fb)
}

// -----------------------------------------------------------------------------
// VECTOR 2/6 (cross-round + future-round injection): all honest nodes first reach consensus
// on X while the equivocator spams Y prevotes/precommits at EVERY round (including rounds
// ahead of the honest current round) from the very start, trying to (a) manufacture a
// POL(Y) at a high round to unlock X-locked nodes, or (b) push a Y precommit-quorum. The
// invariant: Y never reaches a POL and never finalizes anywhere.
// -----------------------------------------------------------------------------
func TestRoundView_EquivocatorFutureRounds_CannotForge_POL(t *testing.T) {
	s := newRVSim(t, 5, 4)
	s.node(4).up = false
	byzID := s.node(4).id

	X := rvBlockID(0xA0)
	Y := rvBlockID(0xB0)
	sibs := []ids.ID{X, Y}

	s.link = func(from, to ids.NodeID) bool { return true } // fully connected; only the equivocator is adversarial

	// All 4 honest nodes see X (the honest sibling). Y exists ONLY as the equivocator's
	// fabricated votes — no honest node ever prevotes Y.
	for i := 0; i < 4; i++ {
		s.node(i).seen[X] = struct{}{}
	}

	converged := false
	for tick := 0; tick < 60 && !converged; tick++ {
		redStep(s, byzID, sibs, 20) // Y prevotes/precommits at rounds 0..20 every tick
		if forked, a, b := rvFork(s.nodes); forked {
			t.Fatalf("FORK: equivocator forged a competing finalize %s vs %s", a, b)
		}
		// Y must NEVER acquire a POL at any honest node (only 1 Byz vote for Y → <α).
		for i := 0; i < 4; i++ {
			if _, ok := s.node(i).view.havePOL[Y]; ok {
				t.Fatalf("node %d formed a POL for the equivocator's phantom Y — false POL", i)
			}
			if s.node(i).view.finalized && s.node(i).view.finalizedBlock == Y {
				t.Fatalf("node %d finalized the phantom Y", i)
			}
		}
		converged = s.node(0).view.finalized && s.node(1).view.finalized &&
			s.node(2).view.finalized && s.node(3).view.finalized
	}
	if !converged {
		t.Fatalf("LIVENESS: honest nodes did not finalize X under a future-round-spamming equivocator")
	}
	for i := 0; i < 4; i++ {
		if s.node(i).view.finalizedBlock != X {
			t.Fatalf("node %d finalized %s, want X", i, s.node(i).view.finalizedBlock)
		}
	}
	t.Logf("RED PASS: equivocator cannot forge a POL(Y) at any round; all honest finalized X")
}

// -----------------------------------------------------------------------------
// VECTOR 1/3/6 (randomized): Blue's 200-seed stress with a REAL f=1 equivocator added.
// Random partitions flip each tick then heal; the equivocator double-votes both siblings at
// all rounds throughout. Invariant every tick: no two honest nodes finalize differently.
// Then liveness: once healed, all honest converge.
// -----------------------------------------------------------------------------
func TestRoundView_ByzantineEquivocatorStress_Alpha4_NoFork(t *testing.T) {
	for seed := int64(0); seed < 200; seed++ {
		rng := rand.New(rand.NewSource(seed))
		s := newRVSim(t, 5, 4)
		s.node(4).up = false // node 4 = f=1 Byzantine equivocator
		byzID := s.node(4).id

		X := rvBlockID(0xA0)
		Y := rvBlockID(0xB0)
		sibs := []ids.ID{X, Y}

		s.node(rng.Intn(4)).seen[X] = struct{}{}
		s.node(rng.Intn(4)).seen[Y] = struct{}{}

		healAt := 14
		var side map[ids.NodeID]int
		s.link = func(from, to ids.NodeID) bool {
			if from == byzID || to == byzID {
				return true
			}
			if side == nil {
				return true
			}
			return side[from] == side[to]
		}

		finalizedAll := false
		for tick := 0; tick < 200 && !finalizedAll; tick++ {
			if tick < healAt {
				side = map[ids.NodeID]int{}
				for i := 0; i < 4; i++ {
					side[s.node(i).id] = rng.Intn(2)
				}
			} else {
				side = nil // permanent heal
			}
			redStep(s, byzID, sibs, 16)
			if forked, a, b := rvFork(s.nodes); forked {
				t.Fatalf("seed %d: FORK with f=1 equivocator at α=4: %s vs %s", seed, a, b)
			}
			finalizedAll = s.node(0).view.finalized && s.node(1).view.finalized &&
				s.node(2).view.finalized && s.node(3).view.finalized
		}
		if !finalizedAll {
			t.Fatalf("seed %d: LIVENESS — honest nodes did not converge after heal despite equivocator", seed)
		}
		// Cross-check: all honest agree.
		if forked, a, b := rvFork(s.nodes); forked {
			t.Fatalf("seed %d: post-run FORK %s vs %s", seed, a, b)
		}
	}
	t.Logf("BYZANTINE STRESS PASS: 200 seeds, f=1 equivocator (double-prevote+double-precommit, every sibling, every round) + random partition, α=4 — no fork, always converged")
}
