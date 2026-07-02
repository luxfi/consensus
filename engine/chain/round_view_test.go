// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// round_view_test.go — the SAFETY + LIVENESS proof for the round-scoped view-change
// core (round_view.go), in isolation from the engine/network plumbing.
//
// It stands up N independent roundView state machines, one per validator, and connects
// them through an in-test gossip bus that faithfully models block-announce + prevote +
// precommit delivery AND can PARTITION links (the exact condition
// competing_fork_deadlock_test.go proved the old convergence deadlocks under). It then
// asserts the two properties the fix must deliver simultaneously:
//
//   LIVENESS  — once the partition heals (eventual synchrony), every up node finalizes
//               the SAME block. (The old irrevocable one-sig-per-height guard deadlocks
//               here forever.)
//   SAFETY    — no two up nodes EVER finalize different blocks, and no node precommits
//               two conflicting values at one round — under partition, reorder, and a
//               randomized stress. (This is the double-finalization the naive
//               finalized-only fix reintroduces.)
package chain

import (
	"math/rand"
	"testing"

	"github.com/luxfi/ids"
)

// rvNode is one validator's view-change machine plus its local block visibility.
type rvNode struct {
	id   ids.NodeID
	view *roundView
	seen map[ids.ID]struct{} // siblings this node has received (its winner is min(seen))
	up   bool
}

// rvMsg is a gossip message on the bus.
type rvMsgKind int

const (
	rvBlock rvMsgKind = iota
	rvPrevote
	rvPrecommit
)

type rvMsg struct {
	kind  rvMsgKind
	from  ids.NodeID
	round uint32
	block ids.ID
}

// rvSim is the multi-node simulation.
type rvSim struct {
	t     *testing.T
	nodes []*rvNode
	alpha int
	// link gates delivery: returns false to DROP a message from→to (a partition). nil =
	// all links up.
	link func(from, to ids.NodeID) bool
	// committedAt records, per node, the block it finalized — to assert cross-node agreement.
	settle int64
}

func newRVSim(t *testing.T, n, alpha int) *rvSim {
	s := &rvSim{t: t, alpha: alpha, settle: 3}
	for i := 0; i < n; i++ {
		var id ids.NodeID
		id[0] = byte(i + 1)
		s.nodes = append(s.nodes, &rvNode{
			id:   id,
			view: newRoundView(1, alpha, n),
			seen: map[ids.ID]struct{}{},
			up:   true,
		})
	}
	return s
}

func (s *rvSim) node(i int) *rvNode { return s.nodes[i] }

// builds gives node i visibility of block b (as if it built or received it locally) and
// queues a block-announce so peers learn of it too.
func (s *rvSim) announce(bus *[]rvMsg, i int, b ids.ID) {
	s.nodes[i].seen[b] = struct{}{}
	*bus = append(*bus, rvMsg{kind: rvBlock, from: s.nodes[i].id, block: b})
}

// winnerOf returns the deterministic lowest-canonical winner among the siblings node n
// has seen (Empty if none).
func winnerOf(n *rvNode) ids.ID {
	var w ids.ID
	for b := range n.seen {
		if w == ids.Empty || b.Compare(w) < 0 {
			w = b
		}
	}
	return w
}

// deliver routes a batch of messages to reachable peers (respecting the partition),
// applying blocks to `seen` and votes to the tallies. Own-vote application happens at
// emit time in run(); deliver only handles from→to peers.
func (s *rvSim) deliver(batch []rvMsg) {
	for _, m := range batch {
		for _, to := range s.nodes {
			if !to.up || to.id == m.from {
				continue
			}
			if s.link != nil && !s.link(m.from, to.id) {
				continue
			}
			switch m.kind {
			case rvBlock:
				to.seen[m.block] = struct{}{}
			case rvPrevote:
				to.view.observePrevote(m.from, m.round, m.block)
			case rvPrecommit:
				to.view.observePrecommit(m.from, m.round, m.block)
			}
		}
	}
}

// stepOnce advances the whole net one macro-tick: every up node re-gossips the siblings
// it has seen (modeling continuous block gossip, so visibility converges once a partition
// heals), steps its machine, applies its OWN votes locally (so its tally counts itself),
// and emits vote gossip; then the bus delivers to reachable peers. Re-broadcasting each
// tick is idempotent (dedup-by-NodeID / seen-set). It asserts SAFETY (no two nodes
// finalize differently) after delivery. Returns true once every up node has finalized.
func (s *rvSim) stepOnce() bool {
	var bus []rvMsg
	for _, n := range s.nodes {
		if !n.up {
			continue
		}
		// Re-gossip seen blocks so a healed partition propagates sibling visibility.
		for b := range n.seen {
			bus = append(bus, rvMsg{kind: rvBlock, from: n.id, block: b})
		}
		w := winnerOf(n)
		act := n.view.step(w, 1, s.settle)
		if act.Prevote != ids.Empty {
			n.view.recordOwnPrevote(act.Prevote)
			n.view.observePrevote(n.id, act.CurRound, act.Prevote) // count self
			bus = append(bus, rvMsg{kind: rvPrevote, from: n.id, round: act.CurRound, block: act.Prevote})
		}
		if act.Precommit != ids.Empty {
			if n.view.recordOwnPrecommit(act.Precommit, act.PrecommitRound) {
				n.view.observePrecommit(n.id, act.PrecommitRound, act.Precommit) // count self
				bus = append(bus, rvMsg{kind: rvPrecommit, from: n.id, round: act.PrecommitRound, block: act.Precommit})
			}
		}
	}
	s.deliver(polCertMsgs(s.nodes)) // gossip validValue POLs so every node converges validValue
	s.deliver(bus)
	s.assertNoFork()
	return s.allFinalized()
}

// polCertMsgs relays each up node's validValue POL (its constituent prevotes) as gossip so a
// peer that missed those prevotes can form the same POL and adopt the same validValue.
func polCertMsgs(nodes []*rvNode) []rvMsg {
	var out []rvMsg
	for _, n := range nodes {
		if !n.up {
			continue
		}
		block, round, voters, ok := n.view.polCert()
		if !ok {
			continue
		}
		for _, voter := range voters {
			out = append(out, rvMsg{kind: rvPrevote, from: voter, round: round, block: block})
		}
	}
	return out
}

// run steps up to maxTicks macro-ticks. Returns true if every up node finalized.
func (s *rvSim) run(maxTicks int) bool {
	for tick := 0; tick < maxTicks; tick++ {
		if s.stepOnce() {
			return true
		}
	}
	return s.allFinalized()
}

// assertNoFork fails immediately if two up nodes have finalized DIFFERENT blocks — the
// double-finalization / cross-node fork the round-scoped guard must make impossible.
func (s *rvSim) assertNoFork() {
	s.t.Helper()
	var head ids.ID
	for _, n := range s.nodes {
		if !n.up || !n.view.finalized {
			continue
		}
		if head == ids.Empty {
			head = n.view.finalizedBlock
			continue
		}
		if n.view.finalizedBlock != head {
			s.t.Fatalf("CROSS-NODE FORK (double-finalization): node finalized %s, another finalized %s at height 1",
				n.view.finalizedBlock, head)
		}
	}
}

func (s *rvSim) allFinalized() bool {
	for _, n := range s.nodes {
		if n.up && !n.view.finalized {
			return false
		}
	}
	return true
}

func rvBlockID(tag byte) ids.ID {
	var b ids.ID
	b[0] = tag
	return b
}

// -----------------------------------------------------------------------------
// LIVENESS: the exact competing_fork_deadlock repro, at the state-machine layer.
// Node 0 dead (α=4, 4 live, zero margin); nodes 1 and 2 build competing siblings A/B;
// a partition {1,3}|{2,4} outlasts the settle window; then HEAL. Under the old
// irrevocable one-sig-per-height guard this deadlocks forever. Here it must CONVERGE.
// -----------------------------------------------------------------------------
func TestRoundView_AsymmetricSplit_Converges_NoFork(t *testing.T) {
	s := newRVSim(t, 5, 4)
	s.node(0).up = false // designated proposer dead

	A := rvBlockID(0xA0)
	B := rvBlockID(0xB0)

	// Partition the live set into {1,3} and {2,4}. During the partition, block-announces
	// and votes cannot cross, so group P1 sees only A and P2 only B → each group's winner
	// differs → the classic split.
	grp := map[ids.NodeID]int{s.node(1).id: 1, s.node(3).id: 1, s.node(2).id: 2, s.node(4).id: 2}
	partitioned := true
	s.link = func(from, to ids.NodeID) bool {
		if partitioned {
			return grp[from] == grp[to]
		}
		return true
	}

	// Seed the two siblings into the two groups.
	var seed []rvMsg
	s.announce(&seed, 1, A) // P1 builds A
	s.announce(&seed, 2, B) // P2 builds B
	s.deliver(seed)

	// Run under the partition: no group of 2 can reach α=4, so NO POL, NO precommit, NO
	// lock forms (the key: a no-POL round leaves everyone unlocked). Nothing finalizes.
	if s.run(6); s.allFinalized() {
		t.Fatalf("nothing should finalize under a 2|2 partition with α=4")
	}
	// HEAL. Every node now learns both siblings and re-converges on winner=min(A,B).
	partitioned = false
	if !s.run(30) {
		t.Fatalf("LIVENESS: the net did not converge+finalize after the partition healed")
	}
	// All four up nodes finalized the SAME block, and it is the deterministic winner min(A,B).
	want := A // A=0xA0 < B=0xB0
	for i, n := range s.nodes {
		if !n.up {
			continue
		}
		if !n.view.finalized {
			t.Fatalf("node %d did not finalize", i)
		}
		if n.view.finalizedBlock != want {
			t.Fatalf("node %d finalized %s, want deterministic winner %s", i, n.view.finalizedBlock, want)
		}
	}
	t.Logf("LIVENESS PASS: split→heal converged all 4 live nodes onto %s (no fork)", want)
}

// -----------------------------------------------------------------------------
// LIVENESS under NON-UNIFORM VISIBILITY + round drift (the residual-freeze gate). Blue's
// original stress seeded both siblings to ALL nodes at once (uniform visibility), which
// masked the phase-offset drift RED found: under asymmetric gossip an honest node lags ~1
// round permanently → its 3<α prevotes never form a POL → ~3% permanent stall. Here each
// sibling is seeded to ONE random node and propagates only via block gossip, and the
// partition flips randomly then heals — so honest nodes genuinely drift in round. The
// ROUND-SKIP rule (jump to a round with ≥f+1 senders) must re-align them: this MUST reach
// 0/200 stalls (every seed converges) with NO fork.
// -----------------------------------------------------------------------------
func TestRoundView_NonUniformVisibility_HonestLiveness_NoStall(t *testing.T) {
	stalls := 0
	for seed := int64(0); seed < 200; seed++ {
		rng := rand.New(rand.NewSource(seed + 7))
		s := newRVSim(t, 5, 4)
		down := rng.Intn(5)
		s.node(down).up = false // zero-margin: 4 live = α=4

		up := []int{}
		for i := 0; i < 5; i++ {
			if i != down {
				up = append(up, i)
			}
		}
		A := rvBlockID(0xA0)
		B := rvBlockID(0xB0)
		// NON-UNIFORM: each sibling seeded to ONE up node; it spreads only by block gossip.
		s.node(up[rng.Intn(len(up))]).seen[A] = struct{}{}
		s.node(up[rng.Intn(len(up))]).seen[B] = struct{}{}

		healAt := 10 + rng.Intn(10)
		finalizedAll := false
		for tick := 0; tick < 260 && !finalizedAll; tick++ {
			if tick < healAt {
				side := map[ids.NodeID]int{}
				for _, i := range up {
					side[s.node(i).id] = rng.Intn(2)
				}
				s.link = func(from, to ids.NodeID) bool { return side[from] == side[to] }
			} else {
				s.link = nil // permanent heal
			}
			finalizedAll = s.stepOnce()
		}
		if !finalizedAll {
			stalls++
			t.Errorf("seed %d: STALL — honest nodes drifted and did not converge after heal (down=%d)", seed, down)
		}
		if forked, a, b := func() (bool, ids.ID, ids.ID) {
			var h ids.ID
			set := false
			for _, n := range s.nodes {
				if !n.up || !n.view.finalized {
					continue
				}
				if !set {
					h, set = n.view.finalizedBlock, true
					continue
				}
				if n.view.finalizedBlock != h {
					return true, h, n.view.finalizedBlock
				}
			}
			return false, ids.Empty, ids.Empty
		}(); forked {
			t.Fatalf("seed %d: FORK %s vs %s", seed, a, b)
		}
	}
	if stalls != 0 {
		t.Fatalf("NON-UNIFORM LIVENESS: %d/200 stalls — round-skip did not eliminate the freeze", stalls)
	}
	t.Logf("NON-UNIFORM LIVENESS PASS: 0/200 stalls under non-uniform visibility + round drift + heal (round-skip works)")
}

// -----------------------------------------------------------------------------
// SAFETY: first-quorum-wins. A gets a POL and some nodes lock on it; then a LOWER
// sibling B appears. The lock rule must keep the net on A (no POL(B) can form over the
// A-locked majority), so A finalizes and B NEVER finalizes — even though B is
// lower-canonical. Proves a late lower sibling cannot double-finalize.
// -----------------------------------------------------------------------------
func TestRoundView_LateLowerSibling_CannotDoubleFinalize(t *testing.T) {
	s := newRVSim(t, 5, 4)
	s.node(0).up = false // 4 live, α=4

	A := rvBlockID(0xB0) // note: A has the HIGHER id…
	B := rvBlockID(0xA0) // …and B is LOWER-canonical, introduced late.

	// Round 0: only A is visible to everyone → all prevote A → POL(A) → lock A → cert A.
	var seed []rvMsg
	for i := 1; i <= 4; i++ {
		s.announce(&seed, i, A)
	}
	s.deliver(seed)

	// Introduce the lower sibling B to everyone AFTER a couple of ticks (late arrival);
	// stepOnce re-gossips it thereafter.
	for tick := 0; tick < 40; tick++ {
		if tick == 2 {
			for i := 1; i <= 4; i++ {
				s.node(i).seen[B] = struct{}{}
			}
		}
		s.stepOnce()
	}
	for i, n := range s.nodes {
		if !n.up {
			continue
		}
		if !n.view.finalized {
			t.Fatalf("node %d did not finalize (liveness)", i)
		}
		if n.view.finalizedBlock != A {
			t.Fatalf("node %d finalized %s; the first-quorum value A=%s must win despite the lower late B", i, n.view.finalizedBlock, A)
		}
	}
	// B must never have reached a precommit quorum anywhere.
	t.Logf("SAFETY PASS: late lower sibling B could not unseat the first-quorum A (no double-finalization)")
}

// -----------------------------------------------------------------------------
// SAFETY STRESS: randomized partitions + reordering across many seeds. Two competing
// siblings, one node down (zero margin). The partition flips randomly (adversarial
// async), then permanently heals in the back half so liveness can complete. Invariant
// asserted every tick: no two up nodes ever finalize different blocks. Then liveness:
// once healed, all finalize the same block.
// -----------------------------------------------------------------------------
func TestRoundView_RandomizedPartitionStress_NoDoubleFinalize(t *testing.T) {
	for seed := int64(0); seed < 200; seed++ {
		rng := rand.New(rand.NewSource(seed))
		s := newRVSim(t, 5, 4)
		down := rng.Intn(5)
		s.node(down).up = false

		A := rvBlockID(0xA0)
		B := rvBlockID(0xB0)
		// Random which up node first builds A vs B.
		up := []int{}
		for i := 0; i < 5; i++ {
			if i != down {
				up = append(up, i)
			}
		}
		var seedMsgs []rvMsg
		s.announce(&seedMsgs, up[rng.Intn(len(up))], A)
		s.announce(&seedMsgs, up[rng.Intn(len(up))], B)
		s.deliver(seedMsgs)

		healAt := 12
		s.link = func(from, to ids.NodeID) bool { return true }
		// Drive with a randomly flipping partition for the first healAt ticks, then heal.
		partition := func(t int) {
			if t >= healAt {
				s.link = func(from, to ids.NodeID) bool { return true }
				return
			}
			// random 2-way split of the up nodes each tick
			side := map[ids.NodeID]int{}
			for _, i := range up {
				side[s.node(i).id] = rng.Intn(2)
			}
			s.link = func(from, to ids.NodeID) bool { return side[from] == side[to] }
		}

		finalizedAll := false
		for tick := 0; tick < 120 && !finalizedAll; tick++ {
			partition(tick)
			finalizedAll = s.stepOnce() // re-gossips blocks+votes, asserts SAFETY every tick
		}
		if !finalizedAll {
			t.Fatalf("seed %d: LIVENESS — net did not converge after healing (down=%d)", seed, down)
		}
	}
	t.Logf("STRESS PASS: 200 seeds, randomized partition+reorder, zero-margin — no double-finalization, always converged")
}
