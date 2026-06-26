package bootstrap

import (
	"context"
	"fmt"
	"sync"

	"github.com/luxfi/consensus/engine/dag/state"
	"github.com/luxfi/ids"
)

// Bound constants for the fetch state machine. A malicious or faulty peer must
// not be able to OOM us or stall us forever, so every queue and retry path is
// capped. These mirror the avalanche bootstrapper's bounds, adapted for the DAG.
const (
	// maxOutstandingRequests caps in-flight GetAncestors requests. New requests
	// are only sent once an outstanding one is answered or fails.
	maxOutstandingRequests = 8

	// maxPendingVertices caps the number of fetched-but-not-yet-executed
	// vertices buffered in memory. Beyond this we stop buffering (anti-OOM).
	maxPendingVertices = 1 << 16

	// maxFetchAttempts caps per-vertex fetch retries across peers so a withheld
	// or non-existent vertex cannot drive a hot loop. Retries are spread across
	// peers; the cap is reset when a fresh peer connects so a newly-available
	// honest peer can still rescue the sync.
	maxFetchAttempts = 16
)

// BootstrapableEngine is an engine that can be bootstrapped.
type BootstrapableEngine interface {
	// Start starts the engine.
	Start(ctx context.Context, startReqID uint32) error

	// HealthCheck checks engine health.
	HealthCheck(ctx context.Context) (interface{}, error)

	// Shutdown shuts down the engine.
	Shutdown(ctx context.Context) error

	// IsBootstrapped returns whether bootstrapping is complete.
	IsBootstrapped() bool
}

// request correlates a GetAncestors request to the peer and request ID it was
// sent with, so a later Ancestors / GetFailed can be matched to the vertex we
// asked for.
type request struct {
	nodeID    ids.NodeID
	requestID uint32
}

// Bootstrapper syncs a DAG chain's vertices from peers.
//
// It drives a real fetch loop: seed the frontier (the stop vertex we must reach,
// plus any tips learned from peers), send GetAncestors for missing vertices,
// walk the parent edges of every received vertex, and keep fetching until the
// DAG from the frontier down to the locally-held last-accepted (or genesis) is
// complete. Only then are the buffered vertices executed in topological
// (parents-before-children) order, the VM linearized to the stop vertex, and
// bootstrapping declared finished.
type Bootstrapper struct {
	mu         sync.Mutex
	config     Config
	onFinished func(ctx context.Context, lastReqID uint32) error

	requestID    uint32
	started      bool
	bootstrapped bool

	// peers we are connected to and may fetch from.
	peers map[ids.NodeID]struct{}

	// frontier is the set of hard sync targets (the stop vertex). Bootstrapping
	// cannot complete until every frontier vertex is held. Tips merely learned
	// from peer gossip are NOT placed here (so a single bogus gossip cannot
	// block bootstrap forever) — they are fetched opportunistically instead.
	frontier map[ids.ID]struct{}

	// needToFetch holds vertex IDs we know we must fetch but have not yet
	// requested (no in-flight request, not yet held).
	needToFetch map[ids.ID]struct{}

	// outstanding correlates (peer,requestID) -> vertex requested; requestedVtx
	// is the reverse index so we never double-request the same vertex.
	outstanding  map[request]ids.ID
	requestedVtx map[ids.ID]request

	// attempts counts per-vertex fetch tries (bounded by maxFetchAttempts).
	attempts map[ids.ID]int

	// pending buffers fetched+parsed vertices awaiting topological execution.
	pending map[ids.ID]state.Vertex

	// processed marks vertices executed during this bootstrap run.
	processed map[ids.ID]struct{}

	// executedOrder records the order vertices were executed (parents-before-
	// children). It is the observable proof of topological execution.
	executedOrder []ids.ID
}

// New creates a new DAG bootstrapper.
func New(config Config, onFinished func(ctx context.Context, lastReqID uint32) error) (*Bootstrapper, error) {
	if config.Manager == nil {
		return nil, fmt.Errorf("manager is required")
	}
	if config.VM == nil {
		return nil, fmt.Errorf("VM is required")
	}

	return &Bootstrapper{
		config:       config,
		onFinished:   onFinished,
		peers:        make(map[ids.NodeID]struct{}),
		frontier:     make(map[ids.ID]struct{}),
		needToFetch:  make(map[ids.ID]struct{}),
		outstanding:  make(map[request]ids.ID),
		requestedVtx: make(map[ids.ID]request),
		attempts:     make(map[ids.ID]int),
		pending:      make(map[ids.ID]state.Vertex),
		processed:    make(map[ids.ID]struct{}),
	}, nil
}

// Start begins bootstrapping by seeding the frontier and driving the fetch loop.
//
// Unlike the previous stub, this does NOT declare the node bootstrapped at
// whatever local vertices it happens to hold. It marks bootstrapped only after
// the DAG has actually been synced to the frontier (see checkFinish).
func (b *Bootstrapper) Start(ctx context.Context, startReqID uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.requestID = startReqID
	b.started = true

	// The well-known stop vertex is the tip we must sync up to. It is the hard
	// target: bootstrap is not complete until we hold it and its history.
	if b.config.StopVertexID != ids.Empty {
		b.frontier[b.config.StopVertexID] = struct{}{}
	}

	return b.drive(ctx)
}

// drive (re)seeds the frontier and any known-missing parents into the fetch
// queue, then runs the fetch loop. It is invoked from Start and whenever a peer
// connects (a fresh peer may be able to serve vertices a previous peer withheld).
// Caller must hold b.mu.
func (b *Bootstrapper) drive(ctx context.Context) error {
	if b.bootstrapped {
		return nil
	}
	// Respect the startup gate: don't begin fetching until enough validating
	// stake is connected. Connected() re-drives once the gate opens.
	if b.config.StartupTracker != nil && !b.config.StartupTracker.ShouldStart() {
		return nil
	}

	// (Re)seed hard frontier targets we still lack.
	for id := range b.frontier {
		if !b.haveVertex(ctx, id) && !b.inflight(id) {
			b.needToFetch[id] = struct{}{}
		}
	}
	// (Re)seed any missing parents of buffered vertices — this lets a newly
	// connected peer rescue a sync that stalled on a previously-withheld parent.
	for _, vtx := range b.pending {
		b.enqueueMissingParents(ctx, vtx)
	}

	return b.fetch(ctx)
}

// fetch drains needToFetch into outstanding GetAncestors requests (bounded by
// maxOutstandingRequests), then checks whether the sync is complete.
// Caller must hold b.mu.
func (b *Bootstrapper) fetch(ctx context.Context) error {
	if b.config.Sender == nil {
		// No transport: we cannot fetch. Only completes if nothing is needed.
		return b.checkFinish(ctx)
	}

	for len(b.outstanding) < maxOutstandingRequests {
		vtxID, ok := b.popNeedToFetch()
		if !ok {
			break
		}
		if b.inflight(vtxID) || b.haveVertex(ctx, vtxID) {
			continue // already requested or already held
		}
		if b.attempts[vtxID] >= maxFetchAttempts {
			continue // give up on this vertex (bounded retry) — do not hot-loop
		}
		peer, ok := b.selectPeer(ids.EmptyNodeID)
		if !ok {
			// No peer available right now; requeue and wait for a connection.
			b.needToFetch[vtxID] = struct{}{}
			break
		}
		b.sendGetAncestors(ctx, peer, vtxID)
	}

	return b.checkFinish(ctx)
}

// sendGetAncestors issues a real GetAncestors request and records it for
// response correlation. Caller must hold b.mu.
func (b *Bootstrapper) sendGetAncestors(ctx context.Context, peer ids.NodeID, vtxID ids.ID) {
	b.requestID++
	req := request{nodeID: peer, requestID: b.requestID}
	b.outstanding[req] = vtxID
	b.requestedVtx[vtxID] = req
	b.attempts[vtxID]++
	// The transport is asynchronous: the response arrives later via Ancestors,
	// or a timeout arrives via GetFailed. We never block on it here.
	_ = b.config.Sender.SendGetAncestors(ctx, peer, b.requestID, vtxID)
}

// Ancestors handles a response to one of our GetAncestors requests. It validates
// the response against the request (anti-spoof), buffers the requested vertex
// and any correctly-chained ancestors, walks their parent edges to discover
// further missing vertices, and continues the fetch loop.
func (b *Bootstrapper) Ancestors(ctx context.Context, nodeID ids.NodeID, requestID uint32, containers [][]byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	req := request{nodeID: nodeID, requestID: requestID}
	requestedVtxID, ok := b.outstanding[req]
	if !ok {
		// Unsolicited or stale: not a response to a request we made. Ignore it
		// so a peer cannot inject vertices we never asked for.
		return nil
	}
	delete(b.outstanding, req)
	delete(b.requestedVtx, requestedVtxID)

	if len(containers) == 0 {
		// Peer doesn't have it (or withheld it): retry on a different peer.
		return b.refetch(ctx, requestedVtxID, nodeID)
	}
	if max := b.config.AncestorsMaxContainersReceived; max > 0 && len(containers) > max {
		containers = containers[:max] // bound how many vertices we accept
	}

	// The first container must be exactly the vertex we requested.
	head, err := b.config.Manager.ParseVtx(ctx, containers[0])
	if err != nil || head == nil || head.ID() != requestedVtxID {
		// Unparseable or wrong vertex from a faulty/malicious peer: reject the
		// whole response and retry elsewhere. Nothing is buffered, so the sync
		// state is not advanced or corrupted.
		return b.refetch(ctx, requestedVtxID, nodeID)
	}

	// Accept the head, then accept each subsequent container only if it was
	// declared as a parent of an already-accepted vertex (a valid ancestor
	// chain). This rejects unrelated vertices stuffed into the response.
	batch := make([]state.Vertex, 0, len(containers))
	eligible := make(map[ids.ID]struct{}, len(containers))
	if v := b.buffer(head); v != nil {
		batch = append(batch, v)
	}
	for _, p := range head.ParentIDs() {
		eligible[p] = struct{}{}
	}

	for _, raw := range containers[1:] {
		vtx, err := b.config.Manager.ParseVtx(ctx, raw)
		if err != nil || vtx == nil {
			break // stop at the first bad vertex; don't trust the remainder
		}
		id := vtx.ID()
		if _, want := eligible[id]; !want {
			break // not a declared ancestor — reject the rest
		}
		delete(eligible, id)
		if v := b.buffer(vtx); v != nil {
			batch = append(batch, v)
		}
		for _, p := range vtx.ParentIDs() {
			eligible[p] = struct{}{}
		}
	}

	return b.process(ctx, batch...)
}

// refetch retries fetching a vertex on a peer other than the one that just
// failed, so a single withholding or malicious peer cannot permanently stall
// the sync. Caller must hold b.mu.
func (b *Bootstrapper) refetch(ctx context.Context, vtxID ids.ID, failedPeer ids.NodeID) error {
	if b.haveVertex(ctx, vtxID) || b.inflight(vtxID) {
		return b.process(ctx)
	}
	if b.attempts[vtxID] < maxFetchAttempts {
		if peer, ok := b.selectPeer(failedPeer); ok {
			b.sendGetAncestors(ctx, peer, vtxID)
		} else {
			b.needToFetch[vtxID] = struct{}{} // no other peer now; retry later
		}
	}
	return b.process(ctx)
}

// process records the missing parents of newly-buffered vertices, then continues
// the fetch loop. Caller must hold b.mu.
func (b *Bootstrapper) process(ctx context.Context, accepted ...state.Vertex) error {
	for _, vtx := range accepted {
		b.enqueueMissingParents(ctx, vtx)
	}
	return b.fetch(ctx)
}

// checkFinish completes bootstrapping iff the DAG is fully synced: no requests
// are queued or in flight, every buffered vertex has been executed in
// topological order, and every hard frontier target is held. Otherwise it
// returns without declaring bootstrapped (fail-secure). Caller must hold b.mu.
func (b *Bootstrapper) checkFinish(ctx context.Context) error {
	if b.bootstrapped {
		return nil
	}
	if len(b.needToFetch) > 0 || len(b.outstanding) > 0 {
		return nil // still fetching
	}

	// Quiescent: execute everything we can in topological order. If executing
	// surfaced a fetchable missing ancestor, go fetch it (one more round).
	if b.execute(ctx) {
		return b.fetch(ctx)
	}
	if len(b.pending) > 0 {
		return nil // leftover vertices with an unfetchable ancestor — incomplete
	}
	// Every hard frontier target must actually be held now.
	for id := range b.frontier {
		if !b.haveVertex(ctx, id) {
			return nil // target not reached — stay bootstrapping
		}
	}

	// DAG synced. Linearize to the stop vertex (DAG -> linear chain), if set.
	if b.config.StopVertexID != ids.Empty {
		if err := b.config.VM.Linearize(ctx, b.config.StopVertexID); err != nil {
			return fmt.Errorf("failed to linearize: %w", err)
		}
	}

	b.bootstrapped = true
	if b.onFinished != nil {
		return b.onFinished(ctx, b.requestID)
	}
	return nil
}

// execute drains buffered vertices into committed state in topological order: a
// vertex is executed only once all of its parents are either genesis, held
// locally before this run, or already executed this run. It returns true if it
// discovered (and queued) a fetchable missing ancestor. Caller must hold b.mu.
func (b *Bootstrapper) execute(ctx context.Context) (queuedMissing bool) {
	for {
		progressed := false
		for id, vtx := range b.pending {
			if !b.parentsSatisfied(ctx, vtx) {
				continue
			}
			// Commit to state in topological order; AddVertex is the persist.
			if err := b.config.Manager.AddVertex(vtx); err == nil {
				b.processed[id] = struct{}{}
				b.executedOrder = append(b.executedOrder, id)
			}
			delete(b.pending, id)
			progressed = true
		}
		if !progressed {
			break
		}
	}
	// Anything still pending is blocked on a missing ancestor. Queue the ones we
	// can still fetch so a subsequent round (or a freshly connected peer) can
	// make progress.
	for _, vtx := range b.pending {
		if b.enqueueMissingParents(ctx, vtx) {
			queuedMissing = true
		}
	}
	return queuedMissing
}

// --- helpers (all assume b.mu held) ---

// buffer stores a newly-seen vertex for later topological execution, returning
// the vertex if it was newly buffered (nil if duplicate or over the cap).
func (b *Bootstrapper) buffer(vtx state.Vertex) state.Vertex {
	id := vtx.ID()
	delete(b.needToFetch, id)
	delete(b.attempts, id)
	if _, ok := b.processed[id]; ok {
		return nil
	}
	if _, ok := b.pending[id]; ok {
		return nil
	}
	if len(b.pending) >= maxPendingVertices {
		return nil // anti-OOM: refuse to buffer beyond the cap
	}
	b.pending[id] = vtx
	return vtx
}

// enqueueMissingParents queues for fetch any parent of vtx that we don't hold,
// isn't already in flight, and hasn't exhausted its retry budget.
func (b *Bootstrapper) enqueueMissingParents(ctx context.Context, vtx state.Vertex) (queued bool) {
	for _, p := range vtx.ParentIDs() {
		if p == ids.Empty || b.haveVertex(ctx, p) || b.inflight(p) {
			continue
		}
		if b.attempts[p] >= maxFetchAttempts {
			continue
		}
		b.needToFetch[p] = struct{}{}
		queued = true
	}
	return queued
}

// haveVertex reports whether we already have a vertex — buffered, executed this
// run, or held locally (pre-existing / persisted). Used to decide what to fetch.
func (b *Bootstrapper) haveVertex(ctx context.Context, id ids.ID) bool {
	if id == ids.Empty {
		return true // genesis sentinel: nothing above it to fetch
	}
	if _, ok := b.pending[id]; ok {
		return true
	}
	if _, ok := b.processed[id]; ok {
		return true
	}
	return b.locallyHave(ctx, id)
}

// parentsSatisfied reports whether every parent of vtx has already been executed
// or was held locally before this run — the precondition for executing vtx.
// Note this is stricter than haveVertex: a parent that is only buffered (not yet
// executed) does NOT satisfy, enforcing parents-before-children execution.
func (b *Bootstrapper) parentsSatisfied(ctx context.Context, vtx state.Vertex) bool {
	for _, p := range vtx.ParentIDs() {
		if p == ids.Empty {
			continue
		}
		if _, ok := b.processed[p]; ok {
			continue
		}
		if b.locallyHave(ctx, p) {
			continue
		}
		return false
	}
	return true
}

// locallyHave reports whether the vertex is held by the manager (persisted
// before this run, or committed by execute this run).
func (b *Bootstrapper) locallyHave(ctx context.Context, id ids.ID) bool {
	_, err := b.config.Manager.GetVtx(ctx, id)
	return err == nil
}

func (b *Bootstrapper) inflight(id ids.ID) bool {
	_, ok := b.requestedVtx[id]
	return ok
}

// popNeedToFetch removes and returns an arbitrary queued vertex ID.
func (b *Bootstrapper) popNeedToFetch() (ids.ID, bool) {
	for id := range b.needToFetch {
		delete(b.needToFetch, id)
		return id, true
	}
	return ids.Empty, false
}

// selectPeer returns a connected peer, preferring one other than exclude so a
// retry rotates away from a failing peer.
func (b *Bootstrapper) selectPeer(exclude ids.NodeID) (ids.NodeID, bool) {
	var fallback ids.NodeID
	haveFallback := false
	for p := range b.peers {
		if p == exclude {
			fallback = p
			haveFallback = true
			continue
		}
		return p, true
	}
	if haveFallback {
		return fallback, true
	}
	return ids.EmptyNodeID, false
}

// HealthCheck returns bootstrap progress.
func (b *Bootstrapper) HealthCheck(ctx context.Context) (interface{}, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return map[string]interface{}{
		"bootstrapped": b.bootstrapped,
		"needToFetch":  len(b.needToFetch),
		"outstanding":  len(b.outstanding),
		"pending":      len(b.pending),
		"executed":     len(b.processed),
		"peers":        len(b.peers),
	}, nil
}

// Shutdown shuts down the bootstrapper.
func (b *Bootstrapper) Shutdown(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.needToFetch = make(map[ids.ID]struct{})
	b.outstanding = make(map[request]ids.ID)
	b.requestedVtx = make(map[ids.ID]request)
	b.pending = make(map[ids.ID]state.Vertex)

	if b.config.VtxBlocked != nil {
		b.config.VtxBlocked.Clear()
	}
	if b.config.TxBlocked != nil {
		b.config.TxBlocked.Clear()
	}

	return nil
}

// IsBootstrapped returns whether bootstrapping is complete.
func (b *Bootstrapper) IsBootstrapped() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.bootstrapped
}

// GetAncestors serves an ancestor request from a peer (the fetch counterpart of
// Ancestors). It delegates to the AllGetsServer.
func (b *Bootstrapper) GetAncestors(ctx context.Context, nodeID ids.NodeID, requestID uint32, vtxID ids.ID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.config.AllGetsServer != nil {
		return b.config.AllGetsServer.GetAncestors(ctx, nodeID, requestID, vtxID, b.config.AncestorsMaxContainersReceived)
	}
	return nil
}

// GetFailed handles a timed-out / failed GetAncestors by retrying the vertex on
// a different peer. This is the canonical "request timeout" path: the network
// timeout manager calls this when a request goes unanswered.
func (b *Bootstrapper) GetFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	req := request{nodeID: nodeID, requestID: requestID}
	vtxID, ok := b.outstanding[req]
	if !ok {
		return nil // not a request we are tracking
	}
	delete(b.outstanding, req)
	delete(b.requestedVtx, vtxID)

	return b.refetch(ctx, vtxID, nodeID)
}

// Connected tracks a peer and (re)drives the fetch loop — a fresh peer may be
// able to serve vertices a previous peer withheld, so per-vertex retry budgets
// are reset to let it rescue a stalled sync.
func (b *Bootstrapper) Connected(ctx context.Context, nodeID ids.NodeID, nodeVersion string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.peers[nodeID] = struct{}{}
	for id := range b.attempts {
		delete(b.attempts, id)
	}

	if b.started && !b.bootstrapped {
		return b.drive(ctx)
	}
	return nil
}

// Disconnected stops tracking a peer.
func (b *Bootstrapper) Disconnected(ctx context.Context, nodeID ids.NodeID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.peers, nodeID)
	return nil
}

// Put is dropped during bootstrap: vertices are pulled deterministically from
// the frontier via GetAncestors, not pushed. The live engine handles Puts once
// bootstrapping is done.
func (b *Bootstrapper) Put(ctx context.Context, nodeID ids.NodeID, requestID uint32, vtxBytes []byte) error {
	return nil
}

// PushQuery is dropped during bootstrap (see Put).
func (b *Bootstrapper) PushQuery(ctx context.Context, nodeID ids.NodeID, requestID uint32, vtxBytes []byte) error {
	return nil
}

// PullQuery is not used during bootstrap.
func (b *Bootstrapper) PullQuery(ctx context.Context, nodeID ids.NodeID, requestID uint32, vtxID ids.ID) error {
	return nil
}

// Vote is not used during bootstrap.
func (b *Bootstrapper) Vote(ctx context.Context, nodeID ids.NodeID, requestID uint32, preferredID ids.ID, preferredIDAtHeight ids.ID, acceptedID ids.ID) error {
	return nil
}

// QueryFailed is not used during bootstrap.
func (b *Bootstrapper) QueryFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32) error {
	return nil
}

// Trace wraps a bootstrapper with tracing.
func Trace(b *Bootstrapper, tracer interface{}) *Bootstrapper {
	return b
}

// Ensure Bootstrapper implements BootstrapableEngine.
var _ BootstrapableEngine = (*Bootstrapper)(nil)
