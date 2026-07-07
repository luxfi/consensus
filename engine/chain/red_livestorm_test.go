// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// red_livestorm_test.go — RED adversarial live-storm repros for consensus v1.35.29
// (ship/fix-bls-v2). A green unit suite is NOT acceptance; these stand up real
// multi-node *Runtime fleets and drive the exact conditions FIX #1 / FIX #3 target.
//
//	(a) TRANSIENT-COUNT RESTART  — FIX #1 self-finality floor: a validator sampler
//	    whose Count() transiently reads 1 during a boot window must NOT let any node
//	    self-finalize (the mainnet 1085013 fork), and the chain must resume finalizing
//	    once the set resolves. Contrast: a GENUINE single-validator chain (presetK≤1)
//	    MUST still self-finalize — the floor discriminates on the live count, not preset.
//
//	(b) WRAPPER-SPLIT             — FIX #3 canonical vote aggregation: ≥2 nodes wrap the
//	    SAME inner execution block under DIFFERENT proposervm-style outer envelopes; the
//	    α-of-K votes split across the outer-keyed pending blocks, yet the height MUST
//	    finalize on every node (the block-288 self-heal).
//
//	(probe) PARTIAL-EPOCH STAKE   — RED vector #1: does the floor+VerifyWeighted CLOSE the
//	    subset-stake self-accept CLASS, or is a complete stake feed REQUIRED? Demonstrates
//	    the consensus layer TRUSTS its StakeSource — a shared partial epoch set lets an
//	    α-sized coalition self-accept below ⅔ of the TRUE set. (Not exploitable in prod
//	    because the node's height-indexed GetValidatorSet is atomic-or-fail-closed; this
//	    pins that as a LOAD-BEARING external invariant with no consensus-level backstop.)
//
// Run under -race.
package chain

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// -----------------------------------------------------------------------------
// transientCountSampler — a ValidatorSampler whose Count() is flippable at runtime,
// modeling validators.Manager.Count(net)=len(m.validators[net]) UNDER-reporting during
// a restart window (reads 1) then resolving to the real set (reads N). This is the exact
// volatile feed that drove K=1/α=1 → self-finality at mainnet 1085013.
// -----------------------------------------------------------------------------

type transientCountSampler struct{ n atomic.Int64 }

func newTransientCountSampler(boot int64) *transientCountSampler {
	s := &transientCountSampler{}
	s.n.Store(boot)
	return s
}
func (s *transientCountSampler) set(n int64)        { s.n.Store(n) }
func (s *transientCountSampler) Count(_ ids.ID) int { return int(s.n.Load()) }
func (s *transientCountSampler) Sample(_ ids.ID, k int) ([]ids.NodeID, error) {
	return nil, nil // unused by the finality path under test
}

var _ ValidatorSampler = (*transientCountSampler)(nil)

// anyHeadAtHeight reports every distinct finalized id any UP node holds at height h.
// EMPTY ⇒ no node has finalized anything at h (the safety assertion for the floor).
func guardedHeads(g *guardedNet, h uint64) map[ids.ID]int {
	heads := map[ids.ID]int{}
	for _, n := range g.nodes {
		if got, ok := n.rt.FinalizedBlockAtHeight(h); ok {
			heads[got]++
		}
	}
	return heads
}

// TestRed_TransientCountRestart_FloorHoldsThenResumes is the FIX #1 live-storm.
//
// A 5-validator chain (presetK=5) whose sampler transiently reads Count()=1 boots the
// FLOORED committee K=4/α=3 (NOT the self-finalizing K=1/α=1). With finality gossip
// partitioned so each node holds only its OWN vote, NO node — not even the block's own
// proposer — may finalize (the 1085013 self-finalize is closed). Once the count resolves
// to 5 and the partition heals, the chain finalizes 4-of-5 and resumes. No node ever
// diverges.
func TestRed_TransientCountRestart_FloorHoldsThenResumes(t *testing.T) {
	const n = 5
	samplers := make([]ValidatorSampler, n)
	transient := make([]*transientCountSampler, n)
	for i := range samplers {
		transient[i] = newTransientCountSampler(1) // the restart under-read
		samplers[i] = transient[i]
	}

	params := prodParams5() // presetK=5, α=4, RoundTO parked at 30s
	g := newGuardedNetSampled(t, n, params, samplers)

	// The floor is ACTIVE: a presetK=5 chain reading Count()=1 boots K=4/α=3, never K=1.
	for i, node := range g.nodes {
		if k := node.rt.Transitive.consensus.K(); k != 4 {
			t.Fatalf("node %d: transient Count()=1 with presetK=5 must FLOOR to K=4 (minimal BFT), got K=%d "+
				"(K=1 is the 1085013 self-finality committee)", i, k)
		}
		if a := node.rt.Transitive.consensus.Alpha(); a < 3 {
			t.Fatalf("node %d: floored α must be ≥3, got %d (α=1 self-finalizes)", i, a)
		}
	}
	t.Logf("all %d nodes booted the FLOORED committee K=4/α=3 under transient Count()=1 (not the K=1 fork committee)", n)

	// PARTITION finality (blocks + prevotes gossip, accept-votes + certs do NOT): each node
	// holds ONLY its OWN signed accept for H1 — the exact isolation under which a K=1 engine
	// self-finalized divergent blocks at 1085013. node 0 proposes H1.
	g.bus.setLink(func(_, _ ids.NodeID, kind busMsgKind) bool { return kind != msgVote && kind != msgCert })
	h1 := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "red-transient-h1")
	g.deliverBlockToAll(h1)

	if !waitFor(emergeTO, func() bool { return g.allBound(1) }) {
		t.Fatalf("precondition: every node must TRACK+bind H1 (each casting its own self-vote)")
	}

	// THE SAFETY PROPERTY (FIX #1): under transient Count()=1 with only self-votes, NO node
	// advances finality — not even node 0, whose own proposal it is (finalizeOwnProposal →
	// K=4 → needs a real α-of-K + ⅔-stake, never the lone self-vote). Zero finalized heads ⇒
	// no self-finalize and (a fortiori) no divergent VM.Accept. Hold the window open to be
	// sure the floored committee never advances on the self-vote the OLD K=1 sizer accepted.
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if heads := guardedHeads(g, 1); len(heads) != 0 {
			t.Fatalf("SELF-FINALITY: %d finalized head(s) at H1 under transient Count()=1 with partitioned votes "+
				"— the floor did NOT hold (a lone self-vote finalized): %v", len(heads), heads)
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Logf("under transient Count()=1 + partitioned finality: 0 nodes self-finalized H1 — the 1085013 self-finality is CLOSED at the sizer (no divergent Accept possible)")

	// RESOLVE the set and RESTART the fleet from persisted state (the task's kill+restart):
	// Count→5 so reconstruction sizes the genuine K=5/α=4, and the fsync'd guards re-seed
	// committedSlot[1] with certVotes EMPTY. HEAL finality gossip. The clone-recovery re-sign
	// fallback (each node re-signs its DURABLE committedSlot[1] canonical) now re-contributes
	// the votes the partition dropped, so the real 4-of-5 ⅔-stake cert forms — the chain
	// RESUMES. (Without the restart, each node's own vote lingers in certVotes and the
	// self-voted-slot suppression would not re-broadcast it; the restart is the recovery path.)
	for _, s := range transient {
		s.set(5)
	}
	g.restartAll()
	if k := g.nodes[0].rt.Transitive.consensus.K(); k != 5 {
		t.Fatalf("after Count→5 + restart, the genuine committee must size to K=5, got K=%d", k)
	}
	g.bus.setLink(nil)
	g.deliverBlockToAll(h1) // re-gossip so each restarted node re-tracks H1's pending block

	if !waitFor(reconvergeTO, func() bool { return g.allFinalized(h1) }) {
		t.Fatalf("LIVENESS: H1 did NOT finalize within %s after the set resolved to 5 and the fleet restarted "+
			"— the chain failed to RESUME after the transient window", reconvergeTO)
	}
	// No divergent head: exactly ONE finalized id at H1 across the fleet.
	if heads := guardedHeads(g, 1); len(heads) != 1 {
		t.Fatalf("FORK after resume: expected exactly one finalized head at H1, got %v", heads)
	}
	t.Logf("after Count→5 + restart + heal: H1 FINALIZED 5/5 on the real ⅔-stake cert (K=5/α=4) — chain RESUMED, no divergence")
}

// TestRed_GenuineSingleValidator_SelfFinalizes_NotBrokenByFloor is the discriminator /
// fail-without contrast (RED vector #6). The floor keys on the LIVE count, not the preset:
//   - presetK=1 (a genuine single: --dev / launch-single / SingleValidatorParams) reading
//     Count()=1 STAYS K=1 and self-finalizes on its own accept.
//   - presetK=20 (a sybil-protected chain with only 1 live validator) is FLOORED to K=4 and
//     HALTS — the expected, correct behavior (NOT a regression: a 1-of-20 self-finalize is
//     exactly the fork).
//
// It then drives the K=1 self-finalize MECHANISM directly (the path the floor SUPPRESSES for
// presetK>1) so the transient-count test above is proven to exercise the floor, not luck.
func TestRed_GenuineSingleValidator_SelfFinalizes_NotBrokenByFloor(t *testing.T) {
	// (1) Construction discrimination through the REAL NewRuntime → bftCommittee path.
	for _, tc := range []struct {
		presetK, wantK int
		why            string
	}{
		{1, 1, "genuine single — stays K=1, self-finalizes (no peer to fork against)"},
		{20, 4, "sybil-preset, 1 live validator — FLOORED to K=4, HALTS (not a 1-of-20 self-finalize)"},
	} {
		params := config.LocalBFTParams()
		params.K = tc.presetK
		if tc.presetK == 1 {
			params.AlphaPreference, params.AlphaConfidence = 1, 1 // α≤K for the unclamped genuine single
		}
		rt := NewRuntime(NetworkConfig{
			ChainID:    ids.GenerateTestID(),
			NetworkID:  ids.Empty,
			NodeID:     ids.GenerateTestNodeID(),
			Validators: newTransientCountSampler(1), // the transient restart under-read
			Params:     &params,
		})
		if k := rt.Transitive.consensus.K(); k != tc.wantK {
			t.Fatalf("presetK=%d, live Count()=1: want K=%d (%s), got K=%d", tc.presetK, tc.wantK, tc.why, k)
		}
	}
	t.Logf("floor discriminates on live count: presetK=1→K=1 (self-finalize preserved); presetK=20+count=1→K=4 (HALT, expected)")

	// (2) The K=1 self-finalize mechanism the floor suppresses for presetK>1 (matches
	// TestSingleValidator_StillFinalizes): a genuine single-validator engine finalizes its
	// own block on the 1-of-1 quorum.
	e := NewWithParams(config.Parameters{K: 1, AlphaPreference: 1, AlphaConfidence: 1, Beta: 1})
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("K=1 Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	blk := newTestBlock(1, ids.Empty, "red-solo")
	_ = trackProposal(e, ids.Empty, blk, 0)
	e.finalizeOwnProposal(context.Background(), blk.id)
	if !e.IsAccepted(blk.id) {
		t.Fatal("REGRESSION: a genuine single-validator (K=1) must self-finalize its own block (1-of-1 quorum)")
	}
	if blk.AcceptCalled() != 1 {
		t.Fatalf("K=1 VM.Accept exactly once, got %d", blk.AcceptCalled())
	}
	t.Logf("genuine single (K=1) self-finalized its own block — floor does not regress the launch-single path")
}

// -----------------------------------------------------------------------------
// wrapBlock / wrapVM — a proposervm-style OUTER envelope over a shared INNER execution
// block. It implements canonicalCommitter, so two wrappers of ONE inner block are ALIASES
// (same CanonicalID) with DISTINCT outer ids — exactly what pendingBlocks keys apart and
// FIX #3 must re-aggregate. The wire bytes carry the outer id so every node ParseBlock's
// the byte-identical wrapper.
// -----------------------------------------------------------------------------

type wrapBlock struct {
	outerID     ids.ID
	innerID     ids.ID
	parentOuter ids.ID
	parentInner ids.ID
	height      uint64
	ts          int64
	execRoot    ids.ID
	payloadRoot ids.ID
	payload     []byte
	parentState ids.ID // resolved by the VM for execution Verify (not on the wire)
}

func (b *wrapBlock) Bytes() []byte {
	buf := make([]byte, 0, 32*6+16+len(b.payload))
	buf = append(buf, b.outerID[:]...)
	buf = append(buf, b.innerID[:]...)
	buf = append(buf, b.parentOuter[:]...)
	buf = append(buf, b.parentInner[:]...)
	var u64 [8]byte
	binary.BigEndian.PutUint64(u64[:], b.height)
	buf = append(buf, u64[:]...)
	binary.BigEndian.PutUint64(u64[:], uint64(b.ts))
	buf = append(buf, u64[:]...)
	buf = append(buf, b.execRoot[:]...)
	buf = append(buf, b.payloadRoot[:]...)
	buf = append(buf, b.payload...)
	return buf
}

func (b *wrapBlock) ID() ids.ID           { return b.outerID }
func (b *wrapBlock) Parent() ids.ID       { return b.parentOuter }
func (b *wrapBlock) ParentID() ids.ID     { return b.parentOuter }
func (b *wrapBlock) Height() uint64       { return b.height }
func (b *wrapBlock) Timestamp() time.Time { return time.Unix(b.ts, 0) }
func (b *wrapBlock) Status() uint8        { return 0 }
func (b *wrapBlock) Verify(context.Context) error {
	if want := expectedStateRoot(b.parentState, b.payload); b.execRoot != want {
		return errors.New("wrap: divergent execution (forked inner block)")
	}
	return nil
}
func (b *wrapBlock) Accept(context.Context) error { return nil }
func (b *wrapBlock) Reject(context.Context) error { return nil }

// canonicalCommitter — the four inner-execution reads the engine collapses aliases on.
func (b *wrapBlock) CanonicalID() ids.ID        { return b.innerID }
func (b *wrapBlock) ParentCanonicalID() ids.ID  { return b.parentInner }
func (b *wrapBlock) ExecutionStateRoot() ids.ID { return b.execRoot }
func (b *wrapBlock) PayloadRoot() ids.ID        { return b.payloadRoot }

var _ block.Block = (*wrapBlock)(nil)

// wrapVM is a per-node BlockBuilder that parses wrapBlock wire bytes deterministically and
// records seen block state roots so a follower can execution-verify a received wrapper.
type wrapVM struct {
	mu        sync.Mutex
	toBuild   *wrapBlock
	stateByID map[ids.ID]ids.ID
	blockByID map[ids.ID]*wrapBlock
	lastAcc   ids.ID
}

func newWrapVM() *wrapVM {
	return &wrapVM{
		stateByID: map[ids.ID]ids.ID{ids.Empty: simGenesisRoot()},
		blockByID: map[ids.ID]*wrapBlock{},
		lastAcc:   ids.Empty,
	}
}

func (vm *wrapVM) BuildBlock(context.Context) (block.Block, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	if vm.toBuild == nil {
		return nil, errors.New("wrap: nothing to build")
	}
	return vm.toBuild, nil
}

func (vm *wrapVM) ParseBlock(_ context.Context, data []byte) (block.Block, error) {
	if len(data) < 32*6+16 {
		return nil, errors.New("wrap: short block bytes")
	}
	b := &wrapBlock{}
	off := 0
	rd := func() ids.ID { var id ids.ID; copy(id[:], data[off:off+32]); off += 32; return id }
	b.outerID = rd()
	b.innerID = rd()
	b.parentOuter = rd()
	b.parentInner = rd()
	b.height = binary.BigEndian.Uint64(data[off : off+8])
	off += 8
	b.ts = int64(binary.BigEndian.Uint64(data[off : off+8]))
	off += 8
	b.execRoot = rd()
	b.payloadRoot = rd()
	b.payload = append([]byte(nil), data[off:]...)

	vm.mu.Lock()
	ps, ok := vm.stateByID[b.parentOuter]
	if !ok {
		ps = simGenesisRoot()
	}
	b.parentState = ps
	vm.blockByID[b.outerID] = b
	vm.stateByID[b.outerID] = b.execRoot
	vm.mu.Unlock()
	return b, nil
}

func (vm *wrapVM) GetBlock(_ context.Context, id ids.ID) (block.Block, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	if b, ok := vm.blockByID[id]; ok {
		return b, nil
	}
	return nil, errors.New("wrap: not found")
}
func (vm *wrapVM) LastAccepted(context.Context) (ids.ID, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	return vm.lastAcc, nil
}
func (vm *wrapVM) SetPreference(context.Context, ids.ID) error { return nil }

var _ BlockBuilder = (*wrapVM)(nil)

// newWrapperOf builds one OUTER envelope over the shared inner (innerID, execRoot,
// payloadRoot) at height on top of genesis. Distinct outer id ⇒ a distinct pending block.
func newWrapperOf(innerID, execRoot, payloadRoot ids.ID, height uint64, tag string) *wrapBlock {
	return &wrapBlock{
		outerID:     ids.GenerateTestID(), // distinct outer envelope
		innerID:     innerID,              // SHARED inner execution identity
		parentOuter: ids.Empty,            // build on the finalized/genesis tip so it can finalize
		parentInner: ids.Empty,
		height:      height,
		ts:          time.Now().Unix(),
		execRoot:    execRoot,
		payloadRoot: payloadRoot,
		payload:     []byte(tag),
	}
}

// wrapFleet is a compact N-node fleet of REAL *Runtime engines over wrapVMs, wired to the
// shared simBus (reusing busGossiper/simNode/the delivery loop — only the VM differs, so
// simNode.vm is left nil, unused by run()).
type wrapFleet struct {
	t     *testing.T
	vs    *testValidatorSet
	bus   *simBus
	nodes []*simNode
	vms   []*wrapVM
}

func newWrapFleet(t *testing.T, n int, params config.Parameters) *wrapFleet {
	t.Helper()
	f := &wrapFleet{t: t, vs: newTestValidatorSet(n), bus: newSimBus()}
	chainID := ids.GenerateTestID()
	for i := 0; i < n; i++ {
		vm := newWrapVM()
		cfg := NetworkConfig{
			ChainID:      chainID,
			NetworkID:    ids.Empty,
			NodeID:       f.vs.nodeID(i),
			Logger:       log.Noop(),
			Gossiper:     &busGossiper{bus: f.bus, self: f.vs.nodeID(i)},
			VM:           vm,
			Params:       &params,
			VoteVerifier: f.vs,
			VoteSigner:   f.vs.signerFor(i),
			StakeSource:  f.vs,
		}
		rt := NewRuntime(cfg)
		if err := rt.Start(context.Background(), true); err != nil {
			t.Fatalf("wrap node %d Start: %v", i, err)
		}
		node := &simNode{nodeID: f.vs.nodeID(i), rt: rt, inbox: make(chan busMsg, 4096), stop: make(chan struct{}), up: true}
		f.bus.register(node)
		f.nodes = append(f.nodes, node)
		f.vms = append(f.vms, vm)
	}
	for _, node := range f.nodes {
		node.wg.Add(1)
		go node.run()
	}
	t.Cleanup(f.shutdown)
	return f
}

func (f *wrapFleet) shutdown() {
	for _, n := range f.nodes {
		select {
		case <-n.stop:
		default:
			close(n.stop)
		}
	}
	for _, n := range f.nodes {
		n.wg.Wait()
		_ = n.rt.Stop(context.Background())
	}
}

// deliverTo enqueues a wrapper's bytes to node i's inbox (a FIFO the run loop drains in
// order — so per-node processing order is exactly the enqueue order).
func (f *wrapFleet) deliverTo(i int, b *wrapBlock) {
	f.nodes[i].enqueue(busMsg{kind: msgBlock, from: f.vs.nodeID(0), payload: b.Bytes()})
}

// TestRed_WrapperSplit_LiveFleetFinalizes is the FIX #3 live-storm (block-288 self-heal).
//
// Two wrappers A, B of ONE inner block C are delivered to a 5-node fleet so that nodes
// {0,1} process A FIRST (vote under A) and nodes {2,3,4} process B FIRST (vote under B).
// Every node then receives BOTH wrappers, so the α-of-K votes for canonical C are SPLIT
// across the outer-keyed pending blocks (A: {0,1}=2, B: {2,3,4}=3 — NEITHER reaches α=4).
// FIX #3 aggregates the votes by canonical identity, so a single 5-vote cert forms and the
// height MUST finalize on every node (per-outer aggregation would stall forever).
func TestRed_WrapperSplit_LiveFleetFinalizes(t *testing.T) {
	const n = 5
	params := prodParams5()                 // K=5/α=4 — a single wrapper's ≤3 votes can NEVER reach α alone
	params.RoundTO = 400 * time.Millisecond // let the re-poll re-offer drive convergence
	f := newWrapFleet(t, n, params)

	inner := ids.GenerateTestID()
	execRoot := expectedStateRoot(simGenesisRoot(), []byte("red-288")) // honest inner execution
	payloadRoot := ids.GenerateTestID()
	const h = uint64(1)

	wa := newWrapperOf(inner, execRoot, payloadRoot, h, "red-288")
	wb := newWrapperOf(inner, execRoot, payloadRoot, h, "red-288")
	if wa.ID() == wb.ID() {
		t.Fatal("wrappers must have DISTINCT outer ids")
	}
	if wa.CanonicalID() != wb.CanonicalID() {
		t.Fatal("wrappers must SHARE the inner canonical id (they wrap one inner block)")
	}

	// Split the fleet's first-seen wrapper: {0,1}→A first, {2,3,4}→B first (each votes under
	// the wrapper it saw first). Then give every node the OTHER wrapper too, so all can
	// AGGREGATE across both — but no node re-votes (one signature per canonical per height).
	for _, i := range []int{0, 1} {
		f.deliverTo(i, wa)
	}
	for _, i := range []int{2, 3, 4} {
		f.deliverTo(i, wb)
	}
	// brief settle so the first-seen vote binds before the sibling wrapper arrives
	time.Sleep(150 * time.Millisecond)
	for _, i := range []int{0, 1} {
		f.deliverTo(i, wb)
	}
	for _, i := range []int{2, 3, 4} {
		f.deliverTo(i, wa)
	}

	// EVERY node must finalize the height — on canonical C (or one of its A/B wrappers),
	// never a third block, and all nodes must AGREE (no fork).
	valid := map[ids.ID]bool{inner: true, wa.ID(): true, wb.ID(): true}
	ok := waitFor(emergeTO, func() bool {
		for _, node := range f.nodes {
			got, ok := node.rt.FinalizedBlockAtHeight(h)
			if !ok || !valid[got] {
				return false
			}
		}
		return true
	})
	if !ok {
		// Diagnose: show each node's finalized head (Empty ⇒ stalled = the wrapper-split bug).
		for i, node := range f.nodes {
			got, has := node.rt.FinalizedBlockAtHeight(h)
			t.Logf("  node %d finalized@H=%v (has=%v)  [inner=%v A=%v B=%v]", i, got, has, inner, wa.ID(), wb.ID())
		}
		t.Fatalf("WRAPPER-SPLIT STALL: not every node finalized canonical C within %s — the α-of-K votes "+
			"split across outer wrappers A/B and did NOT aggregate (FIX #3 regressed)", emergeTO)
	}
	// No FORK: every node finalized the IDENTICAL id (a wrapper of, or the canonical of, C).
	head0, _ := f.nodes[0].rt.FinalizedBlockAtHeight(h)
	for i, node := range f.nodes {
		got, _ := node.rt.FinalizedBlockAtHeight(h)
		if got != head0 {
			t.Fatalf("FORK at H: node 0 finalized %v but node %d finalized %v (divergent heads across the wrapper split)", head0, i, got)
		}
	}
	t.Logf("all %d nodes finalized ONE agreed block %v at H across the A/B wrapper split (votes A:{0,1} + B:{2,3,4} "+
		"aggregated by canonical into a single ⅔-stake cert) — block-288 wrapper-split self-heal holds", n, head0)
}

// -----------------------------------------------------------------------------
// PARTIAL-EPOCH STAKE PROBE (RED vector #1).
//
// partialEpochSet models a height-indexed validator state that resolves to a SUBSET at the
// epoch: only the `present` indices have a resolvable pubkey (VerifyVote) AND stake
// (Weight/TotalStake) — the byte-consistent partial read (verify, stake, set-root all
// funnel through ONE set@epoch in the node's validatorSetAtHeight). It is backed by a real
// testValidatorSet so present members' signatures verify.
// -----------------------------------------------------------------------------

type partialEpochSet struct {
	vs      *testValidatorSet
	present map[int]bool
}

func newPartialEpochSet(vs *testValidatorSet, present ...int) *partialEpochSet {
	m := map[int]bool{}
	for _, i := range present {
		m[i] = true
	}
	return &partialEpochSet{vs: vs, present: m}
}
func (p *partialEpochSet) idx(nodeID ids.NodeID) (int, bool) {
	for i := range p.vs.ids {
		if p.vs.ids[i] == nodeID {
			return i, true
		}
	}
	return 0, false
}
func (p *partialEpochSet) VerifyVote(nodeID ids.NodeID, msg, sig []byte, epoch uint64) bool {
	i, ok := p.idx(nodeID)
	if !ok || !p.present[i] {
		return false // absent from the partial epoch set ⇒ pubkey unresolvable
	}
	return p.vs.VerifyVote(nodeID, msg, sig, epoch)
}
func (p *partialEpochSet) Weight(nodeID ids.NodeID, _ uint64) uint64 {
	i, ok := p.idx(nodeID)
	if !ok || !p.present[i] {
		return 0
	}
	return 1
}
func (p *partialEpochSet) TotalStake(_ uint64) uint64 { return uint64(len(p.present)) }

var (
	_ VoteVerifier = (*partialEpochSet)(nil)
	_ StakeSource  = (*partialEpochSet)(nil)
)

// TestRed_WrapperEquivocation_NoDoubleCert is RED vector #3: FIX #3 aggregates votes by
// CANONICAL identity and de-dups by NodeID (first-iterated, map-random). A Byzantine node
// that signs TWO DIFFERENT inner canonicals X and Y under two wrappers at ONE height must
// NOT be able to turn the aggregation into two conflicting certs. With the honest votes
// split 2-2 across X/Y and the Byzantine vote on BOTH, each canonical reaches the α=3 COUNT
// quorum — but the ⅔-by-stake VerifyWeighted gate (measured on the TRUE 5-set) rejects BOTH
// at 60%, so NEITHER finalizes. The per-canonical aggregation never merges X's and Y's
// votes; the map-random de-dup only ever chooses among byte-identical votes for ONE
// canonical. This is the safety authority the wrapper-aggregation cannot relax.
func TestRed_WrapperEquivocation_NoDoubleCert(t *testing.T) {
	vs := newTestValidatorSet(5)                                                                                 // equal unit stake ⇒ ⅔-of-5 needs 4 voters
	e, _ := newQuorumEngineOpts(t, config.LocalBFTParams(), vs, 0, &recordingGossiper{}, WithStakeWeighting(vs)) // K=4/α=3
	if a := e.consensus.Alpha(); a != 3 {
		t.Fatalf("precondition: this test needs the floored α=3, got α=%d", a)
	}
	const height = uint64(1)
	innerX := ids.GenerateTestID()
	innerY := ids.GenerateTestID()
	if innerX == innerY {
		t.Fatal("X and Y must be DIFFERENT canonicals")
	}
	// Two wrappers of two DIFFERENT inner blocks, both extending the finalized tip at height 1.
	wX := wrapperOf(innerX, ids.Empty, ids.GenerateTestID(), ids.GenerateTestID(), height)
	wY := wrapperOf(innerY, ids.Empty, ids.GenerateTestID(), ids.GenerateTestID(), height)

	e.mu.Lock()
	pX := &PendingBlock{ConsensusBlock: wX, ProposedAt: time.Now()}
	pY := &PendingBlock{ConsensusBlock: wY, ProposedAt: time.Now()}
	e.pendingBlocks[wX.id] = pX
	e.pendingBlocks[wY.id] = pY
	posX := e.blockPositionLocked(pX, wX.id)
	posY := e.blockPositionLocked(pY, wY.id)

	// Byzantine node 0 EQUIVOCATES: signs BOTH X and Y. Honest {1,2}→X, {3,4}→Y (a clean
	// 2-2 split, each canonical reaching count 3 with the Byzantine double-vote).
	e.recordCertVoteLocked(pX, Vote{BlockID: wX.id, NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, posX)})
	e.recordCertVoteLocked(pX, Vote{BlockID: wX.id, NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, posX)})
	e.recordCertVoteLocked(pX, Vote{BlockID: wX.id, NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, posX)})
	e.recordCertVoteLocked(pY, Vote{BlockID: wY.id, NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, posY)})
	e.recordCertVoteLocked(pY, Vote{BlockID: wY.id, NodeID: vs.nodeID(3), Accept: true, Signature: vs.sign(3, posY)})
	e.recordCertVoteLocked(pY, Vote{BlockID: wY.id, NodeID: vs.nodeID(4), Accept: true, Signature: vs.sign(4, posY)})

	certX := e.assembleCertLocked(pX, wX.id)
	certY := e.assembleCertLocked(pY, wY.id)
	e.mu.Unlock()

	// Each canonical reached the α=3 COUNT quorum, but 3/5 = 60% ≤ ⅔ — VerifyWeighted rejects
	// BOTH. No double-finalize; the equivocator cannot manufacture two conflicting certs.
	if certX != nil {
		t.Fatalf("EQUIVOCATION DOUBLE-CERT: X assembled a cert from its 3-vote group (60%% stake) — "+
			"the ⅔-stake gate or the per-canonical aggregation boundary FAILED (voters=%d)", len(certX.Votes))
	}
	if certY != nil {
		t.Fatalf("EQUIVOCATION DOUBLE-CERT: Y assembled a cert from its 3-vote group (60%% stake) (voters=%d)", len(certY.Votes))
	}
	t.Logf("wrapper equivocation X/Y with a shared Byzantine voter + 2-2 honest split: NEITHER canonical " +
		"finalized (each 60%% ≤ ⅔) — per-canonical aggregation + ⅔-stake gate hold; no double-finalize")
}

// TestRed_PartialEpochStake_SubQuorumSelfAccept_Probe answers "does FIX #1's floor CLOSE the
// subset-stake self-accept CLASS?". Verdict: NO — the floor closes the COUNT degeneracy
// (α can't drop to 1), but a shared PARTIAL epoch set of size 3 lets the {0,1,2} coalition
// reach α=3 AND clear ⅔ of the (partial) total=3, self-accepting a block that is only 3/5 =
// 60% of the TRUE set — below the ⅔ the full set requires. The consensus layer TRUSTS its
// StakeSource; completeness comes ENTIRELY from the node's atomic height-indexed
// GetValidatorSet (verified atomic-or-fail-closed), with NO consensus-level backstop.
func TestRed_PartialEpochStake_SubQuorumSelfAccept_Probe(t *testing.T) {
	params := config.LocalBFTParams() // K=4/α=3 — the floored committee

	// probe assembles a cert from node-0's own vote + votes {1,2} under the given stake +
	// verifier, and returns whether assembleCertLocked produced a cert (a non-nil cert has
	// ALREADY cleared VerifyWeighted — it WOULD finalize). Fresh vs per probe so the honest
	// vote-once map does not cross-contaminate.
	probe := func(mk func(vs *testValidatorSet) (StakeSource, VoteVerifier)) *QuorumCert {
		vs := newTestValidatorSet(5) // the TRUE set: 5 equal-weight validators
		stake, verifier := mk(vs)
		e, chainID := newQuorumEngineOpts(t, params, vs, 0, &recordingGossiper{}, WithStakeWeighting(stake))
		e.voteVerifier = verifier
		blk := newTestBlock(1, ids.Empty, "red-partial")
		pos := trackProposal(e, chainID, blk, 0) // records node 0's own signed accept
		e.mu.Lock()
		pb := e.pendingBlocks[blk.id]
		e.recordCertVoteLocked(pb, vs.signedVote(1, pos))
		e.recordCertVoteLocked(pb, vs.signedVote(2, pos))
		cert := e.assembleCertLocked(pb, blk.id) // nil ⇒ below α OR VerifyWeighted rejected
		e.mu.Unlock()
		return cert
	}

	// (1) CONTROL — COMPLETE stake feed: 3 verified votes reach α=3 but only 3/5 = 60% stake.
	//     assembleCertLocked runs VerifyWeighted (3 ≤ floor(2·5/3)=3) and returns nil → HALT.
	//     Blue's guarantee holds.
	if cert := probe(func(vs *testValidatorSet) (StakeSource, VoteVerifier) { return vs, vs }); cert != nil {
		t.Fatalf("CONTROL BROKE: 3-of-5 with the COMPLETE stake feed must NOT assemble a cert (60%% < ⅔), got %d voters", len(cert.Votes))
	}
	t.Logf("control: 3-of-5 with the COMPLETE stake feed → NO cert (60%% ≤ ⅔) — the floored committee is safe")

	// (2) PARTIAL epoch feed {0,1,2}: the SAME 3 votes now ASSEMBLE + clear VerifyWeighted —
	//     total=3, voted=3 > floor(2·3/3)=2. The floor's α=3 is MET by the coalition; the
	//     stake gate measures ⅔ of the PARTIAL total, not the TRUE set. A byte-consistent
	//     partial (verify + stake from ONE set@epoch, mirroring the node's validatorSetAtHeight).
	partialCert := probe(func(vs *testValidatorSet) (StakeSource, VoteVerifier) {
		p := newPartialEpochSet(vs, 0, 1, 2)
		return p, p
	})
	if partialCert == nil {
		t.Fatal("expected the partial-epoch coalition to self-accept at the consensus predicate; it did not — " +
			"re-derive the vector (a consensus-level completeness backstop would be a STRONGER posture than analyzed)")
	}
	if len(partialCert.Votes) != 3 {
		t.Fatalf("partial cert must carry the 3-coalition votes, got %d", len(partialCert.Votes))
	}
	t.Logf("VECTOR #1 CONFIRMED (consensus predicate): a shared PARTIAL epoch set {0,1,2} lets a 3-of-5 coalition")
	t.Logf("  SELF-ACCEPT (assembleCertLocked returns a VerifyWeighted-cleared cert) at 60%% of the TRUE set —")
	t.Logf("  the floor + VerifyWeighted do NOT close the subset-stake class. Production safety rests ENTIRELY on")
	t.Logf("  the node's atomic height-indexed GetValidatorSet (errUnfinalizedHeight fail-closed, verified in")
	t.Logf("  platformvm/validators/manager.go); there is NO consensus-level completeness backstop. LOAD-BEARING.")
}
