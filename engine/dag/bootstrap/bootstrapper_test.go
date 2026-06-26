package bootstrap

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"testing"

	"github.com/luxfi/consensus/engine/dag/state"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test doubles: a minimal but real DAG codec + Manager + VM + network.
// ---------------------------------------------------------------------------

// testVtx is a concrete state.Vertex with a deterministic byte encoding.
type testVtx struct {
	id      ids.ID
	parents []ids.ID
	height  uint64
}

func (v *testVtx) ID() ids.ID          { return v.id }
func (v *testVtx) ParentIDs() []ids.ID { return v.parents }
func (v *testVtx) Height() uint64      { return v.height }
func (v *testVtx) Bytes() []byte       { return encodeVtx(v) }

// encodeVtx serializes a vertex: id[32] | nParents[1] | parents... | height[8].
func encodeVtx(v *testVtx) []byte {
	out := make([]byte, 0, 32+1+len(v.parents)*32+8)
	out = append(out, v.id[:]...)
	out = append(out, byte(len(v.parents)))
	for _, p := range v.parents {
		out = append(out, p[:]...)
	}
	var h [8]byte
	binary.BigEndian.PutUint64(h[:], v.height)
	return append(out, h[:]...)
}

// decodeVtx is the inverse of encodeVtx. It rejects malformed/garbage input
// (anti-spoof: an unparseable vertex must be rejected, not silently accepted).
func decodeVtx(b []byte) (*testVtx, error) {
	if len(b) < 32+1+8 {
		return nil, errors.New("too short")
	}
	var id ids.ID
	copy(id[:], b[:32])
	if id == ids.Empty {
		return nil, errors.New("empty id")
	}
	n := int(b[32])
	off := 33
	if len(b) != off+n*32+8 {
		return nil, errors.New("bad length")
	}
	parents := make([]ids.ID, n)
	for i := 0; i < n; i++ {
		copy(parents[i][:], b[off:off+32])
		off += 32
	}
	return &testVtx{id: id, parents: parents, height: binary.BigEndian.Uint64(b[off : off+8])}, nil
}

func vtxID(i int) ids.ID {
	var id ids.ID
	binary.BigEndian.PutUint64(id[:8], uint64(i))
	return id
}

func newVtx(i int, parents ...ids.ID) *testVtx {
	return &testVtx{id: vtxID(i), parents: parents, height: uint64(i)}
}

// buildChain returns a linear DAG genesis<-...<-tip of n vertices.
// order[0] is genesis (no parents); order[n-1] is the tip.
func buildChain(n int) (dag map[ids.ID]*testVtx, order []*testVtx) {
	dag = map[ids.ID]*testVtx{}
	var parents []ids.ID
	for i := 1; i <= n; i++ {
		v := newVtx(i, parents...)
		dag[v.id] = v
		order = append(order, v)
		parents = []ids.ID{v.id}
	}
	return dag, order
}

var errNotFound = errors.New("vertex not found")

// fakeManager is an in-memory state.Manager that records AddVertex order.
type fakeManager struct {
	mu     sync.Mutex
	store  map[ids.ID]state.Vertex
	addLog []ids.ID
}

func newManager() *fakeManager { return &fakeManager{store: map[ids.ID]state.Vertex{}} }

func (m *fakeManager) preload(vs ...*testVtx) {
	for _, v := range vs {
		m.store[v.id] = v
	}
}

func (m *fakeManager) GetVertex(id ids.ID) (state.Vertex, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.store[id]; ok {
		return v, nil
	}
	return nil, errNotFound
}

func (m *fakeManager) AddVertex(v state.Vertex) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.store[v.ID()]; !ok {
		m.addLog = append(m.addLog, v.ID())
	}
	m.store[v.ID()] = v
	return nil
}

func (m *fakeManager) VertexIssued(v state.Vertex) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.store[v.ID()]
	return ok
}

func (m *fakeManager) IsProcessing(ids.ID) bool { return false }

func (m *fakeManager) ParseVtx(_ context.Context, b []byte) (state.Vertex, error) {
	v, err := decodeVtx(b)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (m *fakeManager) GetVtx(_ context.Context, id ids.ID) (state.Vertex, error) {
	return m.GetVertex(id)
}

func (m *fakeManager) Edge(context.Context) []ids.ID { return nil }
func (m *fakeManager) StopVertexAccepted()           {}

// fakeVM records linearization calls.
type fakeVM struct {
	mu         sync.Mutex
	linearized []ids.ID
}

func (vm *fakeVM) Linearize(_ context.Context, stop ids.ID) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	vm.linearized = append(vm.linearized, stop)
	return nil
}

func (vm *fakeVM) ParseVtx(_ context.Context, b []byte) (state.Vertex, error) {
	return decodeVtx(b)
}

func (vm *fakeVM) didLinearize(id ids.ID) bool {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	for _, l := range vm.linearized {
		if l == id {
			return true
		}
	}
	return false
}

// sentReq records a GetAncestors we actually put on the wire.
type sentReq struct {
	node  ids.NodeID
	reqID uint32
	vtx   ids.ID
}

// fakeNet is the Config.Sender: it records GetAncestors requests so the test can
// deliver (or withhold) responses, mirroring an asynchronous transport.
type fakeNet struct {
	mu   sync.Mutex
	sent []sentReq // undrained queue, consumed by runSync
	all  []sentReq // full history, for countFor assertions
}

func (n *fakeNet) SendGetAncestors(_ context.Context, node ids.NodeID, reqID uint32, vtx ids.ID) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	r := sentReq{node: node, reqID: reqID, vtx: vtx}
	n.sent = append(n.sent, r)
	n.all = append(n.all, r)
	return nil
}

// take drains and returns all requests recorded since the last call.
func (n *fakeNet) take() []sentReq {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := n.sent
	n.sent = nil
	return out
}

// count returns the number of currently-undrained requests (non-destructive).
func (n *fakeNet) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.sent)
}

// countFor returns how many requests (ever) targeted a given vertex.
func (n *fakeNet) countFor(id ids.ID) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	c := 0
	for _, r := range n.all {
		if r.vtx == id {
			c++
		}
	}
	return c
}

// serveAncestors builds a GetAncestors response from a peer's DAG: the requested
// vertex first, then its ancestors in BFS order, bounded by max (0 = unbounded).
func serveAncestors(dag map[ids.ID]*testVtx, root ids.ID, max int) [][]byte {
	if max <= 0 {
		max = len(dag) + 1
	}
	out := make([][]byte, 0, max)
	seen := map[ids.ID]bool{}
	queue := []ids.ID{root}
	for len(queue) > 0 && len(out) < max {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		v, ok := dag[id]
		if !ok {
			continue
		}
		seen[id] = true
		out = append(out, v.Bytes())
		queue = append(queue, v.parents...)
	}
	return out
}

// runSync drives the asynchronous request/response loop until the network goes
// quiet. For each sent request, serve decides whether to answer (returning the
// response containers) or to drop it (ok=false -> GetFailed, i.e. a timeout).
func runSync(t *testing.T, b *Bootstrapper, net *fakeNet, serve func(r sentReq) ([][]byte, bool)) {
	t.Helper()
	ctx := context.Background()
	for round := 0; round < 10_000; round++ {
		reqs := net.take()
		if len(reqs) == 0 {
			return
		}
		for _, r := range reqs {
			if resp, ok := serve(r); ok {
				require.NoError(t, b.Ancestors(ctx, r.node, r.reqID, resp))
			} else {
				require.NoError(t, b.GetFailed(ctx, r.node, r.reqID))
			}
		}
	}
	t.Fatal("sync did not converge within round budget")
}

// assertTopoOrder verifies executedOrder lists every parent before its child.
func assertTopoOrder(t *testing.T, order []ids.ID, dag map[ids.ID]*testVtx) {
	t.Helper()
	pos := map[ids.ID]int{}
	for i, id := range order {
		pos[id] = i
	}
	for id, v := range dag {
		ci, ok := pos[id]
		if !ok {
			continue // not executed this run (e.g. pre-existing) — invariant trivially holds
		}
		for _, p := range v.parents {
			if pi, ok := pos[p]; ok {
				require.Less(t, pi, ci, "parent %v must be executed before child %v", p, id)
			}
		}
	}
}

func newBootstrapper(t *testing.T, m *fakeManager, vm *fakeVM, net *fakeNet, stop ids.ID) (*Bootstrapper, *bool) {
	t.Helper()
	finished := false
	b, err := New(Config{
		Manager:                        m,
		VM:                             vm,
		Sender:                         net,
		StopVertexID:                   stop,
		AncestorsMaxContainersReceived: 2, // small cap -> forces multi-round parent walks
	}, func(context.Context, uint32) error {
		finished = true
		return nil
	})
	require.NoError(t, err)
	return b, &finished
}

const peerA = byte(0xA1)

func nodeID(b byte) ids.NodeID {
	var n ids.NodeID
	n[0] = b
	return n
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestEmptyNodeFetchesAndSyncsToFrontier is the core proof: a fresh/empty node,
// told only the frontier (stop vertex), actually SENDS GetAncestors, walks the
// parent edges across multiple rounds, executes in topological order, and
// reaches the peer's frontier — instead of declaring itself bootstrapped at its
// (empty) local state.
func TestEmptyNodeFetchesAndSyncsToFrontier(t *testing.T) {
	ctx := context.Background()
	dag, order := buildChain(5) // genesis(1) <- 2 <- 3 <- 4 <- tip(5)
	tip := order[len(order)-1].id

	m, vm, net := newManager(), &fakeVM{}, &fakeNet{}
	b, finished := newBootstrapper(t, m, vm, net, tip)

	require.NoError(t, b.Connected(ctx, nodeID(peerA), "v1"))
	require.NoError(t, b.Start(ctx, 1))

	// Empty node MUST NOT be bootstrapped before fetching, and MUST have put a
	// real GetAncestors on the wire (this is exactly the bug being fixed).
	require.False(t, b.IsBootstrapped(), "must not false-complete before syncing")
	require.Equal(t, 1, net.count(), "Start must send a GetAncestors for the frontier")

	var totalRequests int
	runSync(t, b, net, func(r sentReq) ([][]byte, bool) {
		totalRequests++
		return serveAncestors(dag, r.vtx, 2), true
	})

	require.True(t, b.IsBootstrapped(), "node must be bootstrapped after syncing the DAG")
	require.True(t, *finished, "onFinished must be called exactly when synced")
	require.True(t, vm.didLinearize(tip), "must linearize to the stop vertex")

	// Real fetching happened across multiple rounds (cap=2 forces >1 request).
	require.Greater(t, totalRequests, 1, "must fetch parents over multiple GetAncestors rounds")

	// Reached the frontier: every vertex from genesis..tip is now held.
	for id := range dag {
		_, err := m.GetVertex(id)
		require.NoError(t, err, "vertex %v must be synced into state", id)
	}

	// Executed strictly parents-before-children.
	require.Len(t, b.executedOrder, 5)
	assertTopoOrder(t, b.executedOrder, dag)
	require.Equal(t, b.executedOrder, m.addLog, "state commit order == topological execution order")
}

// TestBranchingDAGTopologicalOrder proves topological execution holds for a true
// DAG (a vertex with two parents), not just a chain.
func TestBranchingDAGTopologicalOrder(t *testing.T) {
	ctx := context.Background()
	// genesis(1) ; A=2(<-1) ; B=3(<-2) ; C=4(<-2) ; D=5(<-3,<-4)
	g := newVtx(1)
	a := newVtx(2, g.id)
	bb := newVtx(3, a.id)
	c := newVtx(4, a.id)
	d := newVtx(5, bb.id, c.id)
	dag := map[ids.ID]*testVtx{g.id: g, a.id: a, bb.id: bb, c.id: c, d.id: d}

	m, vm, net := newManager(), &fakeVM{}, &fakeNet{}
	b, finished := newBootstrapper(t, m, vm, net, d.id)
	require.NoError(t, b.Connected(ctx, nodeID(peerA), "v1"))
	require.NoError(t, b.Start(ctx, 7))

	runSync(t, b, net, func(r sentReq) ([][]byte, bool) {
		return serveAncestors(dag, r.vtx, 2), true
	})

	require.True(t, b.IsBootstrapped())
	require.True(t, *finished)
	require.Len(t, b.executedOrder, 5)
	assertTopoOrder(t, b.executedOrder, dag)
}

// TestBehindNodeConverges proves a node that already holds the lower DAG fetches
// only the missing tip region and stops at its local boundary (does not
// re-fetch history it already has).
func TestBehindNodeConverges(t *testing.T) {
	ctx := context.Background()
	dag, order := buildChain(5) // 1..5
	tip := order[4].id

	m, vm, net := newManager(), &fakeVM{}, &fakeNet{}
	// Preload genesis..V3 (already accepted locally). Missing: V4, tip(V5).
	m.preload(order[0], order[1], order[2])

	b, finished := newBootstrapper(t, m, vm, net, tip)
	require.NoError(t, b.Connected(ctx, nodeID(peerA), "v1"))
	require.NoError(t, b.Start(ctx, 3))

	runSync(t, b, net, func(r sentReq) ([][]byte, bool) {
		return serveAncestors(dag, r.vtx, 2), true
	})

	require.True(t, b.IsBootstrapped())
	require.True(t, *finished)

	// Converged: only the missing tip region was executed.
	require.Equal(t, []ids.ID{order[3].id, order[4].id}, b.executedOrder)
	assertTopoOrder(t, b.executedOrder, dag)

	// Boundary respected: never re-fetched vertices it already held.
	require.Zero(t, net.countFor(order[0].id), "genesis already held — must not fetch")
	require.Zero(t, net.countFor(order[1].id), "V2 already held — must not fetch")
	require.Zero(t, net.countFor(order[2].id), "V3 already held — must not fetch")
}

// TestMaliciousVertexRejectedRetriesAnotherPeer proves a wrong vertex from a
// faulty peer is rejected (not buffered, sync not advanced) and the request is
// retried on a DIFFERENT peer, after which the node converges. The bad peer is
// the only peer connected at Start, so it is deterministically queried first;
// the good peer is then available for the rejection-driven retry to rotate to.
func TestMaliciousVertexRejectedRetriesAnotherPeer(t *testing.T) {
	ctx := context.Background()
	dag, order := buildChain(4)
	tip := order[3].id
	bad, good := nodeID(0xBA), nodeID(0x60)

	m, vm, net := newManager(), &fakeVM{}, &fakeNet{}
	b, finished := newBootstrapper(t, m, vm, net, tip)
	require.NoError(t, b.Connected(ctx, bad, "v1")) // only peer at Start
	require.NoError(t, b.Start(ctx, 9))
	require.NoError(t, b.Connected(ctx, good, "v1")) // available for the retry

	poisonID := vtxID(999) // a vertex that exists nowhere in the real DAG
	servedPoison := false
	retriedOnGood := false

	runSync(t, b, net, func(r sentReq) ([][]byte, bool) {
		if r.node == good {
			retriedOnGood = true
			return serveAncestors(dag, r.vtx, 2), true
		}
		// The bad peer replies with a vertex whose ID is not what we asked for.
		servedPoison = true
		return [][]byte{newVtx(999).Bytes()}, true
	})

	require.True(t, servedPoison, "bad peer must have been exercised first")
	require.True(t, retriedOnGood, "must retry on a different peer after rejection")
	require.True(t, b.IsBootstrapped(), "must still converge despite the malicious peer")
	require.True(t, *finished)

	// The poison was never accepted into state, buffered, or executed.
	_, err := m.GetVertex(poisonID)
	require.ErrorIs(t, err, errNotFound, "poison vertex must never enter state")
	require.NotContains(t, b.executedOrder, poisonID)
	for _, id := range b.executedOrder {
		require.Contains(t, dag, id, "only real DAG vertices may be executed")
	}
}

// TestEmptyAndUnparseableAncestorsRejected proves the two response-validation
// branches reject bad input without advancing the sync: an empty response and
// unparseable garbage both trigger a retry, and nothing is buffered or executed
// until a well-formed response arrives. Single peer -> fully deterministic.
func TestEmptyAndUnparseableAncestorsRejected(t *testing.T) {
	ctx := context.Background()
	dag, order := buildChain(3)
	tip := order[2].id

	m, vm, net := newManager(), &fakeVM{}, &fakeNet{}
	b, finished := newBootstrapper(t, m, vm, net, tip)
	require.NoError(t, b.Connected(ctx, nodeID(peerA), "v1"))
	require.NoError(t, b.Start(ctx, 5))

	step := 0
	runSync(t, b, net, func(r sentReq) ([][]byte, bool) {
		// Only the first request for the tip is abused; deeper fetches are honest.
		if r.vtx == tip {
			switch step {
			case 0:
				step++
				return [][]byte{}, true // empty response -> reject + retry
			case 1:
				step++
				// nothing has been buffered or executed by the rejected responses
				require.Empty(t, b.executedOrder, "no vertex may be executed from rejected responses")
				require.Zero(t, len(b.pending), "no vertex may be buffered from rejected responses")
				return [][]byte{{0xDE, 0xAD}}, true // unparseable -> reject + retry
			}
		}
		return serveAncestors(dag, r.vtx, 2), true // honest from here on
	})

	require.GreaterOrEqual(t, step, 2, "both the empty and unparseable branches must be exercised")
	require.True(t, b.IsBootstrapped(), "must converge after the honest response")
	require.True(t, *finished)
	require.GreaterOrEqual(t, net.countFor(tip), 3, "tip must be retried after each bad response")
	assertTopoOrder(t, b.executedOrder, dag)
}

// TestWithheldResponseRetriesAnotherPeer proves a timed-out (withheld) response
// is retried on another peer — no permanent stall.
func TestWithheldResponseRetriesAnotherPeer(t *testing.T) {
	ctx := context.Background()
	dag, order := buildChain(4)
	tip := order[3].id
	silent, good := nodeID(0x51), nodeID(0x60)

	m, vm, net := newManager(), &fakeVM{}, &fakeNet{}
	b, finished := newBootstrapper(t, m, vm, net, tip)
	require.NoError(t, b.Connected(ctx, silent, "v1"))
	require.NoError(t, b.Connected(ctx, good, "v1"))
	require.NoError(t, b.Start(ctx, 11))

	usedGood := false
	runSync(t, b, net, func(r sentReq) ([][]byte, bool) {
		if r.node == good {
			usedGood = true
			return serveAncestors(dag, r.vtx, 2), true
		}
		return nil, false // silent peer: timeout
	})

	require.True(t, usedGood, "must fall back to the responsive peer")
	require.True(t, b.IsBootstrapped(), "must converge despite a withholding peer")
	require.True(t, *finished)
	assertTopoOrder(t, b.executedOrder, dag)
}

// TestWithheldFrontierFailsSecure proves the fix is fail-secure: if the frontier
// vertex is withheld by the only peer, the node does NOT falsely declare itself
// bootstrapped, retries are bounded (no hot loop / OOM), and once an honest peer
// connects it converges.
func TestWithheldFrontierFailsSecure(t *testing.T) {
	ctx := context.Background()
	dag, order := buildChain(3)
	tip := order[2].id
	silent, good := nodeID(0x51), nodeID(0x60)

	m, vm, net := newManager(), &fakeVM{}, &fakeNet{}
	b, finished := newBootstrapper(t, m, vm, net, tip)
	require.NoError(t, b.Connected(ctx, silent, "v1"))
	require.NoError(t, b.Start(ctx, 13))

	// Phase 1: only a withholding peer. Drive every request to timeout.
	runSync(t, b, net, func(r sentReq) ([][]byte, bool) {
		return nil, false
	})

	require.False(t, b.IsBootstrapped(), "must NOT false-complete when the frontier is unreachable")
	require.False(t, *finished, "onFinished must not fire on an incomplete sync")
	require.False(t, vm.didLinearize(tip), "must not linearize an unsynced DAG")
	// Bounded retries: never exceeds the per-vertex attempt cap (anti hot-loop).
	require.LessOrEqual(t, net.countFor(tip), maxFetchAttempts, "retries must be bounded")

	// Phase 2: an honest peer connects -> attempt budget resets -> converge.
	require.NoError(t, b.Connected(ctx, good, "v1"))
	runSync(t, b, net, func(r sentReq) ([][]byte, bool) {
		if r.node == good {
			return serveAncestors(dag, r.vtx, 2), true
		}
		return nil, false
	})

	require.True(t, b.IsBootstrapped(), "must converge once an honest peer is available")
	require.True(t, *finished)
	require.True(t, vm.didLinearize(tip))
	assertTopoOrder(t, b.executedOrder, dag)
}

// TestGenesisNoFrontierCompletes proves the legitimate fast path is preserved: a
// node with no stop vertex and nothing to fetch completes immediately (this is
// NOT the bug — the bug was false-completing a node that DID have a frontier).
func TestGenesisNoFrontierCompletes(t *testing.T) {
	ctx := context.Background()
	m, vm, net := newManager(), &fakeVM{}, &fakeNet{}
	b, finished := newBootstrapper(t, m, vm, net, ids.Empty) // no frontier

	require.NoError(t, b.Start(ctx, 1))
	require.True(t, b.IsBootstrapped(), "no frontier, nothing to sync -> complete")
	require.True(t, *finished)
	require.Empty(t, net.take(), "must not send fetch requests when there is nothing to fetch")
}
