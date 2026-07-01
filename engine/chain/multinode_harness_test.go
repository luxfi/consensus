// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// multinode_harness_test.go — a REAL multi-node consensus simulation.
//
// Unlike the single-engine tests (which inject synthetic peer votes into ONE
// Transitive), this harness stands up N independent *Runtime engines and wires
// them to each other through an in-process gossip BUS that faithfully carries the
// production message topology:
//
//	proposer build → GossipPut / SendPushQuery ─▶ peer.HandleIncomingBlock
//	peer verify+vote → BroadcastVote            ─▶ peer.HandleIncomingVote
//	α-of-K reached → GossipCert                 ─▶ peer.HandleIncomingCert → finalize
//
// Nodes run CONCURRENTLY (one delivery goroutine per node), so finalization is
// EMERGENT: no test hand-feeds a quorum. A test triggers exactly one build and
// asserts that the honest majority independently converges on a SINGLE finalized
// block per height — the real property the down/wedged/forked-proposer fix must
// deliver. Run under -race.
//
// Faults are injected at the bus (a node can be made DOWN — dropped from delivery
// — or its outbound can be dropped to model a WEDGED-but-present proposer) and at
// the VM (a FORKED proposer emits a block whose execution state root diverges, so
// every honest node's execution Verify REJECTS it — the engine-boundary half of
// the forked-proposer matrix; the proposervm inner-block-Verify half is proven in
// the node package).
package chain

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// -----------------------------------------------------------------------------
// simBlock — a deterministic, execution-verified block.
//
// Bytes() is the canonical wire encoding; ID() = SHA-256(Bytes()), so EVERY node
// that ParseBlock's the same bytes reconstructs the byte-identical block with the
// same ID (no shared registry, no hidden coupling). stateRoot is the post-
// execution commitment: an honest block carries stateRoot == expectedStateRoot(
// parentStateRoot, payload); a FORKED block carries a tampered stateRoot, which a
// verifying node detects by RE-EXECUTING (recomputing the expected root) — exactly
// how a real EVM rejects a divergent-execution block. This is not a self-declared
// "valid" flag: validity is derived from the parent's committed state, which the
// verifier holds independently.
// -----------------------------------------------------------------------------

const simGenesisStateTag = "GENESIS-STATE"

func simGenesisRoot() ids.ID { return hashID("state", ids.Empty[:], []byte(simGenesisStateTag)) }

// expectedStateRoot is the deterministic "execution" function: the post-state of
// applying payload on top of parentStateRoot. Every honest node computes the same
// value, so a block whose claimed stateRoot differs is a divergent-execution fork.
func expectedStateRoot(parentStateRoot ids.ID, payload []byte) ids.ID {
	return hashID("state", parentStateRoot[:], payload)
}

func hashID(domain string, parts ...[]byte) ids.ID {
	h := sha256.New()
	h.Write([]byte(domain))
	for _, p := range parts {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(p)))
		h.Write(l[:])
		h.Write(p)
	}
	var id ids.ID
	copy(id[:], h.Sum(nil))
	return id
}

type simBlock struct {
	parentID  ids.ID
	height    uint64
	ts        int64
	stateRoot ids.ID // the block's CLAIMED post-execution state root
	payload   []byte

	// parentStateRoot is the state root this block was built on. It is NOT part of
	// the wire bytes (a real verifier reads it from its own accepted-state view);
	// the harness carries it so the sim VM can recompute the expected root without
	// a separate state store. Honest verification uses the VM's recorded parent
	// state, so a block cannot lie about its parent's state.
	parentStateRoot ids.ID
}

// simWire is the deterministic encoding. ID = SHA-256(simWire).
func (b *simBlock) Bytes() []byte {
	buf := make([]byte, 0, 32+8+8+32+len(b.payload))
	buf = append(buf, b.parentID[:]...)
	var u64 [8]byte
	binary.BigEndian.PutUint64(u64[:], b.height)
	buf = append(buf, u64[:]...)
	binary.BigEndian.PutUint64(u64[:], uint64(b.ts))
	buf = append(buf, u64[:]...)
	buf = append(buf, b.stateRoot[:]...)
	buf = append(buf, b.payload...)
	return buf
}

func (b *simBlock) ID() ids.ID { return hashID("block", b.Bytes()) }
func (b *simBlock) Parent() ids.ID { return b.parentID }
func (b *simBlock) ParentID() ids.ID { return b.parentID }
func (b *simBlock) Height() uint64 { return b.height }
func (b *simBlock) Timestamp() time.Time { return time.Unix(b.ts, 0) }
func (b *simBlock) Status() uint8 { return 0 }

// Verify RE-EXECUTES: the block is valid iff its claimed stateRoot equals the root
// obtained by applying its payload to its parent's state root. A forked/divergent
// block (tampered stateRoot) fails here on EVERY honest node — no honest node will
// track or vote for it. Genesis-rooted blocks use the fixed genesis root.
func (b *simBlock) Verify(context.Context) error {
	want := expectedStateRoot(b.parentStateRoot, b.payload)
	if b.stateRoot != want {
		return fmt.Errorf("sim: divergent execution: claimed stateRoot %s != expected %s (forked block)", b.stateRoot, want)
	}
	return nil
}

func (b *simBlock) Accept(context.Context) error { return nil }
func (b *simBlock) Reject(context.Context) error { return nil }

var _ block.Block = (*simBlock)(nil)

// newHonestBlock builds a valid block on top of a parent with the given state.
func newHonestBlock(parentID ids.ID, parentStateRoot ids.ID, height uint64, payload string) *simBlock {
	pl := []byte(payload)
	return &simBlock{
		parentID:        parentID,
		height:          height,
		ts:              time.Now().Unix(),
		payload:         pl,
		parentStateRoot: parentStateRoot,
		stateRoot:       expectedStateRoot(parentStateRoot, pl),
	}
}

// newForkedBlock builds a divergent-execution block: same parent/height/payload as
// an honest block would, but a TAMPERED state root, so honest execution Verify
// rejects it. This models a forked proposer (e.g. mainnet luxd-3) that produces a
// well-formed wrapper over a divergent inner execution.
func newForkedBlock(parentID ids.ID, parentStateRoot ids.ID, height uint64, payload string) *simBlock {
	b := newHonestBlock(parentID, parentStateRoot, height, payload)
	b.stateRoot = hashID("TAMPERED", b.stateRoot[:]) // != expectedStateRoot → Verify fails
	return b
}

// -----------------------------------------------------------------------------
// simVM — a per-node BlockBuilder. It parses bytes deterministically (so peers
// reconstruct the identical block) and records accepted-block state roots so it
// can supply a parent's state root to ParseBlock for execution verification.
// -----------------------------------------------------------------------------

type simVM struct {
	mu         sync.Mutex
	toBuild    *simBlock          // the next block this node will build (nil = down/no build)
	stateByID  map[ids.ID]ids.ID  // blockID -> post-state root (accepted or seen)
	blockByID  map[ids.ID]*simBlock
	preferred  ids.ID
	lastAcc    ids.ID
	buildCount int
}

func newSimVM() *simVM {
	genesisID := ids.Empty
	vm := &simVM{
		stateByID: map[ids.ID]ids.ID{genesisID: simGenesisRoot()},
		blockByID: map[ids.ID]*simBlock{},
		lastAcc:   genesisID,
	}
	return vm
}

func (vm *simVM) setToBuild(b *simBlock) {
	vm.mu.Lock()
	vm.toBuild = b
	if b != nil {
		vm.blockByID[b.ID()] = b
		vm.stateByID[b.ID()] = b.stateRoot
	}
	vm.mu.Unlock()
}

func (vm *simVM) BuildBlock(context.Context) (block.Block, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	if vm.toBuild == nil {
		return nil, errors.New("sim: nothing to build")
	}
	vm.buildCount++
	return vm.toBuild, nil
}

// ParseBlock reconstructs a simBlock from wire bytes. The parent's state root is
// resolved from this node's own recorded state (genesis, an accepted block, or a
// previously-seen block), so execution Verify checks the claimed root against the
// node's independent view — a forked block cannot smuggle a false parent state.
func (vm *simVM) ParseBlock(_ context.Context, data []byte) (block.Block, error) {
	if len(data) < 32+8+8+32 {
		return nil, errors.New("sim: short block bytes")
	}
	var parentID, stateRoot ids.ID
	copy(parentID[:], data[:32])
	height := binary.BigEndian.Uint64(data[32:40])
	ts := int64(binary.BigEndian.Uint64(data[40:48]))
	copy(stateRoot[:], data[48:80])
	payload := append([]byte(nil), data[80:]...)

	vm.mu.Lock()
	parentState, ok := vm.stateByID[parentID]
	vm.mu.Unlock()
	if !ok {
		// Unknown parent: fall back to genesis root for height-1 blocks (the common
		// case in these tests). A real node would fetch the parent via catch-up.
		parentState = simGenesisRoot()
	}
	b := &simBlock{
		parentID:        parentID,
		height:          height,
		ts:              ts,
		stateRoot:       stateRoot,
		payload:         payload,
		parentStateRoot: parentState,
	}
	vm.mu.Lock()
	vm.blockByID[b.ID()] = b
	vm.stateByID[b.ID()] = b.stateRoot
	vm.mu.Unlock()
	return b, nil
}

func (vm *simVM) GetBlock(_ context.Context, id ids.ID) (block.Block, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	if b, ok := vm.blockByID[id]; ok {
		return b, nil
	}
	return nil, errors.New("sim: block not found")
}

func (vm *simVM) LastAccepted(context.Context) (ids.ID, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	return vm.lastAcc, nil
}

func (vm *simVM) SetPreference(_ context.Context, id ids.ID) error {
	vm.mu.Lock()
	vm.preferred = id
	vm.mu.Unlock()
	return nil
}

var _ BlockBuilder = (*simVM)(nil)

// -----------------------------------------------------------------------------
// busGossiper — routes a node's outbound gossip to the OTHER nodes' inbound
// queues. Implements the network-level QuorumGossiper. Delivery is asynchronous
// (per-node inbound goroutine) so the concurrent engines finalize emergently and
// no synchronous A→B→A reentrancy can deadlock the engine locks.
// -----------------------------------------------------------------------------

type busMsgKind int

const (
	msgBlock busMsgKind = iota
	msgVote
	msgCert
)

type busMsg struct {
	kind    busMsgKind
	from    ids.NodeID
	blockID ids.ID
	payload []byte
}

type simBus struct {
	mu    sync.Mutex
	nodes map[ids.NodeID]*simNode
}

func newSimBus() *simBus { return &simBus{nodes: map[ids.NodeID]*simNode{}} }

func (bus *simBus) register(n *simNode) {
	bus.mu.Lock()
	bus.nodes[n.nodeID] = n
	bus.mu.Unlock()
}

// deliver enqueues msg to every node except `from` that is currently reachable.
func (bus *simBus) deliver(from ids.NodeID, m busMsg) int {
	bus.mu.Lock()
	targets := make([]*simNode, 0, len(bus.nodes))
	for id, n := range bus.nodes {
		if id == from {
			continue
		}
		targets = append(targets, n)
	}
	bus.mu.Unlock()
	sent := 0
	for _, n := range targets {
		if n.reachable() {
			n.enqueue(m)
			sent++
		}
	}
	return sent
}

// busGossiper is the per-node handle the engine calls. It tags outbound with the
// node's own id and can be silenced (a wedged/partitioned proposer whose outbound
// is dropped).
type busGossiper struct {
	bus    *simBus
	self   ids.NodeID
	silent func() bool // if true, outbound is dropped (wedged/partitioned)
}

func (g *busGossiper) dropOut() bool { return g.silent != nil && g.silent() }

func (g *busGossiper) GossipPut(_ ids.ID, _ ids.ID, blockData []byte) int {
	if g.dropOut() {
		return 0
	}
	return g.bus.deliver(g.self, busMsg{kind: msgBlock, from: g.self, payload: append([]byte(nil), blockData...)})
}

func (g *busGossiper) SendPushQuery(_ ids.ID, _ ids.ID, blockData []byte, _ []ids.NodeID) int {
	if g.dropOut() {
		return 0
	}
	return g.bus.deliver(g.self, busMsg{kind: msgBlock, from: g.self, payload: append([]byte(nil), blockData...)})
}

func (g *busGossiper) BroadcastVote(_ ids.ID, _ ids.ID, blockID ids.ID, voteBytes []byte) int {
	if g.dropOut() {
		return 0
	}
	return g.bus.deliver(g.self, busMsg{kind: msgVote, from: g.self, blockID: blockID, payload: append([]byte(nil), voteBytes...)})
}

func (g *busGossiper) GossipCert(_ ids.ID, _ ids.ID, blockID ids.ID, certBytes []byte) int {
	if g.dropOut() {
		return 0
	}
	return g.bus.deliver(g.self, busMsg{kind: msgCert, from: g.self, blockID: blockID, payload: append([]byte(nil), certBytes...)})
}

// Unused legacy transport methods (single-recipient pull/vote) — no-ops in the
// broadcast topology under test.
func (g *busGossiper) SendPullQuery(ids.ID, ids.ID, ids.ID, []ids.NodeID) int { return 0 }
func (g *busGossiper) SendVote(ids.ID, ids.NodeID, ids.ID) error              { return nil }

var _ QuorumGossiper = (*busGossiper)(nil)

// -----------------------------------------------------------------------------
// simNode — one validator: a Runtime engine + its VM + inbound delivery loop.
// -----------------------------------------------------------------------------

type simNode struct {
	nodeID ids.NodeID
	rt     *Runtime
	vm     *simVM

	inbox  chan busMsg
	stop   chan struct{}
	wg     sync.WaitGroup

	upMu sync.RWMutex
	up   bool // false ⇒ DOWN: drops all inbound (models a crashed/partitioned node)
}

func (n *simNode) reachable() bool {
	n.upMu.RLock()
	defer n.upMu.RUnlock()
	return n.up
}

func (n *simNode) setUp(up bool) {
	n.upMu.Lock()
	n.up = up
	n.upMu.Unlock()
}

func (n *simNode) enqueue(m busMsg) {
	// Non-blocking best-effort with a large buffer; a full inbox drops (models a
	// lossy link). Sized generously so healthy runs never drop.
	select {
	case n.inbox <- m:
	default:
	}
}

func (n *simNode) run() {
	defer n.wg.Done()
	ctx := context.Background()
	for {
		select {
		case <-n.stop:
			return
		case m := <-n.inbox:
			if !n.reachable() {
				continue
			}
			switch m.kind {
			case msgBlock:
				_, _ = n.rt.HandleIncomingBlock(ctx, m.payload, m.from)
			case msgVote:
				n.rt.HandleIncomingVote(m.blockID, m.payload)
			case msgCert:
				n.rt.HandleIncomingCert(m.payload)
			}
		}
	}
}

// finalizedTip returns this node's finalized tip id + height.
func (n *simNode) finalizedTip() (ids.ID, uint64, bool) {
	return n.rt.FinalizedLedger()
}

// -----------------------------------------------------------------------------
// simNet — a set of N validators sharing one bus and one test validator set.
// -----------------------------------------------------------------------------

type simNet struct {
	t     *testing.T
	vs    *testValidatorSet
	bus   *simBus
	nodes []*simNode
	chain ids.ID
}

// newSimNet builds n validators wired to a shared bus, each running the REAL
// quorum-cert engine (verifier+signer+stake from the shared set). params.K/α come
// from the caller. The engines are STARTED and their delivery loops running.
func newSimNet(t *testing.T, n int, params config.Parameters) *simNet {
	t.Helper()
	vs := newTestValidatorSet(n)
	bus := newSimBus()
	chainID := ids.GenerateTestID()
	net := &simNet{t: t, vs: vs, bus: bus, chain: chainID}

	for i := 0; i < n; i++ {
		vm := newSimVM()
		node := &simNode{
			nodeID: vs.nodeID(i),
			vm:     vm,
			inbox:  make(chan busMsg, 4096),
			stop:   make(chan struct{}),
			up:     true,
		}
		gossip := &busGossiper{bus: bus, self: vs.nodeID(i)}
		cfg := NetworkConfig{
			ChainID:          chainID,
			NetworkID:        ids.Empty,
			NodeID:           vs.nodeID(i),
			Logger:           log.Noop(),
			Gossiper:         gossip,
			VM:               vm,
			Params:           &params,
			VoteVerifier:     vs,
			VoteSigner:       vs.signerFor(i),
			StakeSource:      vs,
			ValidatorSetRoot: nil,
		}
		rt := NewRuntime(cfg)
		if err := rt.Start(context.Background(), true); err != nil {
			t.Fatalf("node %d Start: %v", i, err)
		}
		node.rt = rt
		bus.register(node)
		net.nodes = append(net.nodes, node)
	}
	for _, node := range net.nodes {
		node.wg.Add(1)
		go node.run()
	}
	t.Cleanup(net.shutdown)
	return net
}

func (net *simNet) shutdown() {
	for _, n := range net.nodes {
		close(n.stop)
	}
	for _, n := range net.nodes {
		n.wg.Wait()
		_ = n.rt.Stop(context.Background())
	}
}

// build makes node i the builder of blk and triggers exactly one build pass. The
// engine gossips + solicits; the emergent vote/cert flow does the rest.
func (net *simNet) build(i int, blk *simBlock) {
	net.t.Helper()
	net.nodes[i].vm.setToBuild(blk)
	if err := net.nodes[i].rt.Transitive.Notify(context.Background(), Message{Type: PendingTxs}); err != nil {
		net.t.Fatalf("node %d Notify: %v", i, err)
	}
}

// down marks node i DOWN: it stops sending (bus won't route to it) and drops all
// inbound. Models a crashed / partitioned validator.
func (net *simNet) down(i int) { net.nodes[i].setUp(false) }

// finalizedEverywhere reports whether EVERY currently-up node has finalized blk at
// its height (emergent agreement), and whether any up node finalized a DIFFERENT
// block at that height (a fork).
func (net *simNet) finalizedEverywhere(blk *simBlock) (all bool, fork bool) {
	all = true
	for _, n := range net.nodes {
		if !n.reachable() {
			continue
		}
		got, ok := n.rt.FinalizedBlockAtHeight(blk.height)
		if !ok {
			all = false
			continue
		}
		if got != blk.ID() {
			fork = true
		}
	}
	return all, fork
}

// anyFinalizedAtHeight reports whether any up node finalized ANY block at height h,
// and the distinct set of finalized ids (to detect divergent heads).
func (net *simNet) headsAtHeight(h uint64) map[ids.ID]int {
	heads := map[ids.ID]int{}
	for _, n := range net.nodes {
		if !n.reachable() {
			continue
		}
		if got, ok := n.rt.FinalizedBlockAtHeight(h); ok {
			heads[got]++
		}
	}
	return heads
}

// upCount returns how many nodes are currently up.
func (net *simNet) upCount() int {
	c := 0
	for _, n := range net.nodes {
		if n.reachable() {
			c++
		}
	}
	return c
}

// prodParams5 is the production-shaped 5-validator BFT param set: K=5, α=4
// (⌊2·5/3⌋+1). With one validator down/wedged/forked the remaining 4 are the
// EXACT quorum (zero margin) — the mainnet condition. RoundTO is parked long so
// the background re-poll ticker does not interfere with the emergent assertion
// (finalization is driven by real gossip, not the ticker).
func prodParams5() config.Parameters {
	p := params5()
	p.K = 5
	p.AlphaPreference = 4
	p.AlphaConfidence = 4
	p.RoundTO = 30 * time.Second
	return p
}
