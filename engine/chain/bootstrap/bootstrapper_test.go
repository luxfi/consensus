// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// bootstrapper_test.go — drives the fetch+execute loop with an IN-MEMORY peer and
// node, proving:
//   - an EMPTY node converges to a peer's tip in ONE batch (gap ≤ window) and across
//     MANY batches (the descent: gap > window, fetched top-down, executed bottom-up);
//   - a PARTIAL node converges from M to N;
//   - an INVALID block from the peer HALTS the sync (the node never advances past it);
//   - a node with NO peer ahead finishes immediately (nothing to sync to);
//   - a peer that serves NOTHING stalls (a real failure is surfaced, not masked).
package bootstrap

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// tBlock is a minimal identity-preserving test block. Its bytes encode its id, so a
// ParseBlock round-trip recovers the same block from a shared registry.
type tBlock struct {
	id, parent ids.ID
	height     uint64
	bytes      []byte
	valid      bool
}

func (b *tBlock) ID() ids.ID                    { return b.id }
func (b *tBlock) Parent() ids.ID                { return b.parent }
func (b *tBlock) ParentID() ids.ID              { return b.parent }
func (b *tBlock) Height() uint64                { return b.height }
func (b *tBlock) Timestamp() time.Time          { return time.Unix(int64(b.height), 0) }
func (b *tBlock) Status() uint8                 { return 0 }
func (b *tBlock) Verify(context.Context) error  { return nil }
func (b *tBlock) Accept(context.Context) error  { return nil }
func (b *tBlock) Reject(context.Context) error  { return nil }
func (b *tBlock) Bytes() []byte                 { return b.bytes }

// chainOf builds a genesis→N chain. blocks[0] is genesis (height 0, parent Empty).
// invalidAt (if > 0) marks that height's block invalid (a corrupt/forged block).
func chainOf(n int, invalidAt uint64) ([]*tBlock, map[string]*tBlock) {
	blocks := make([]*tBlock, 0, n+1)
	reg := map[string]*tBlock{}
	var parent ids.ID
	for h := 0; h <= n; h++ {
		blk := &tBlock{
			id:     ids.GenerateTestID(),
			parent: parent,
			height: uint64(h),
			bytes:  []byte("blk@" + strconv.Itoa(h) + ":" + ids.GenerateTestID().String()),
			valid:  uint64(h) != invalidAt,
		}
		reg[string(blk.bytes)] = blk
		blocks = append(blocks, blk)
		parent = blk.id
	}
	return blocks, reg
}

// testPeer is an in-memory BlockSource: a node holding the full chain genesis→tip. It
// reports FrontierNamed by default; `status` overrides that to model the no-beacon /
// connecting / no-quorum cases, and `connectAfter` models the CANARY boot race (the beacon
// set is still connecting for the first N frontier queries, then names the tip).
type testPeer struct {
	byHeight     []*tBlock
	byID         map[ids.ID]*tBlock
	serveNothing bool           // model a dead/withholding peer
	status       FrontierStatus // status to report once "connected" (default FrontierNamed)
	connectAfter int            // report FrontierConnecting for the first N queries, then `status`
	noQuorumFor  int            // after connecting, report FrontierNoQuorum for the next N queries (a TRANSIENT split that settles), then `status`
	frontierCalls int           // observable: how many times the loop asked for the frontier
}

func newTestPeer(chain []*tBlock) *testPeer {
	p := &testPeer{byHeight: chain, byID: map[ids.ID]*tBlock{}, status: FrontierNamed}
	for _, b := range chain {
		p.byID[b.id] = b
	}
	return p
}

func (p *testPeer) FrontierTip(context.Context) (ids.ID, FrontierStatus) {
	p.frontierCalls++
	// CANARY: beacons not connected yet for the first connectAfter queries.
	if p.connectAfter > 0 && p.frontierCalls <= p.connectAfter {
		return ids.Empty, FrontierConnecting
	}
	// TRANSIENT SPLIT (F2): connected enough to ASK, but the honest beacons are momentarily
	// split across adjacent finalized tips for the next noQuorumFor queries (the live frontier
	// is a moving target). A single one-shot tally here would mis-fire; the loop must RETRY and
	// converge once the split settles.
	if p.noQuorumFor > 0 && p.frontierCalls <= p.connectAfter+p.noQuorumFor {
		return ids.Empty, FrontierNoQuorum
	}
	switch p.status {
	case FrontierNamed:
		if len(p.byHeight) == 0 {
			return ids.Empty, FrontierNoBeacons
		}
		return p.byHeight[len(p.byHeight)-1].id, FrontierNamed
	default: // FrontierNoBeacons / FrontierConnecting / FrontierNoQuorum
		return ids.Empty, p.status
	}
}

// Ancestors serves up to maxBlocks blocks ending at blockID, OLDEST-FIRST, walking
// down toward genesis — exactly the node's GetContext semantics.
func (p *testPeer) Ancestors(_ context.Context, blockID ids.ID, maxBlocks int) ([][]byte, error) {
	if p.serveNothing {
		return nil, nil
	}
	tip, ok := p.byID[blockID]
	if !ok {
		return nil, nil
	}
	var rev [][]byte
	cur := tip
	for i := 0; i < maxBlocks; i++ {
		rev = append(rev, cur.bytes)
		if cur.parent == ids.Empty {
			break
		}
		cur = p.byID[cur.parent]
		if cur == nil {
			break
		}
	}
	// reverse → oldest-first
	out := make([][]byte, len(rev))
	for i := range rev {
		out[len(rev)-1-i] = rev[i]
	}
	return out, nil
}

// testNode is an in-memory Chain: it starts at some last-accepted height and accepts
// fetched blocks with the SAME contract as chain.Runtime.AcceptBootstrapBlock —
// contiguity (height == last+1, parent == tip) + re-execute (valid) — so the loop is
// exercised faithfully.
type testNode struct {
	reg     map[string]*tBlock
	have    map[ids.ID]bool
	tipID   ids.ID
	height  uint64
	accepts int
}

func newTestNode(reg map[string]*tBlock, seed *tBlock) *testNode {
	n := &testNode{reg: reg, have: map[ids.ID]bool{}}
	n.tipID = seed.id
	n.height = seed.height
	n.have[seed.id] = true
	return n
}

func (n *testNode) ParseBlock(_ context.Context, b []byte) (block.Block, error) {
	if blk, ok := n.reg[string(b)]; ok {
		return blk, nil
	}
	return nil, errUnknownBytes
}
func (n *testNode) LastAccepted(context.Context) (ids.ID, uint64, error) {
	return n.tipID, n.height, nil
}
func (n *testNode) Has(_ context.Context, id ids.ID) bool { return n.have[id] }
func (n *testNode) AcceptBootstrapBlock(_ context.Context, b []byte) error {
	blk, ok := n.reg[string(b)]
	if !ok {
		return errUnknownBytes
	}
	if blk.height <= n.height {
		return nil // already have — responder overlap
	}
	if blk.height != n.height+1 || blk.parent != n.tipID {
		return errOutOfOrder
	}
	if !blk.valid {
		return errInvalidBlock // re-execute (Verify) failed
	}
	n.height = blk.height
	n.tipID = blk.id
	n.have[blk.id] = true
	n.accepts++
	return nil
}

type bootErr string

func (e bootErr) Error() string { return string(e) }

const (
	errUnknownBytes = bootErr("unknown bytes")
	errOutOfOrder   = bootErr("out of order")
	errInvalidBlock = bootErr("invalid block")
)

func runBootstrap(t *testing.T, peer BlockSource, node Chain) error {
	t.Helper()
	bs := New(Config{
		Source:            peer,
		Chain:             node,
		MaxBlocksPerFetch: 256,
		RetryInterval:     time.Millisecond,      // keep the stall/connect paths fast in tests
		ConnectDeadline:   200 * time.Millisecond, // bound the beacon-connectivity wait in tests
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return bs.Run(ctx)
}

func TestLoop_EmptyNodeSyncsOneBatch(t *testing.T) {
	const N = 50 // < window: one Ancestors batch reaches genesis
	chain, reg := chainOf(N, 0)
	peer := newTestPeer(chain)
	node := newTestNode(reg, chain[0]) // empty: only genesis

	if err := runBootstrap(t, peer, node); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if node.height != N {
		t.Fatalf("empty node did not reach tip: height %d, want %d", node.height, N)
	}
	if node.accepts != N {
		t.Fatalf("expected %d accepts (1..%d), got %d", N, N, node.accepts)
	}
}

func TestLoop_EmptyNodeSyncsMultiBatchDescent(t *testing.T) {
	const N = 600 // > window (256): forces the top-down descent + bottom-up execute
	chain, reg := chainOf(N, 0)
	peer := newTestPeer(chain)
	node := newTestNode(reg, chain[0])

	if err := runBootstrap(t, peer, node); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if node.height != N {
		t.Fatalf("descent did not reach tip: height %d, want %d", node.height, N)
	}
}

func TestLoop_PartialNodeConverges(t *testing.T) {
	const N = 300
	const M = 280
	chain, reg := chainOf(N, 0)
	peer := newTestPeer(chain)
	node := newTestNode(reg, chain[M]) // partial: starts at height M

	if err := runBootstrap(t, peer, node); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if node.height != N {
		t.Fatalf("partial node did not converge: height %d, want %d", node.height, N)
	}
	if node.accepts != N-M {
		t.Fatalf("expected %d accepts (M+1..N), got %d", N-M, node.accepts)
	}
}

func TestLoop_InvalidBlockHaltsSync(t *testing.T) {
	const N = 50
	const bad = uint64(30) // the peer serves a corrupt block at height 30
	chain, reg := chainOf(N, bad)
	peer := newTestPeer(chain)
	node := newTestNode(reg, chain[0])

	// The invalid block at height 30 fails re-execution; the node accepts 1..29 then
	// STOPS — it never advances past the unverifiable block, and the run surfaces a
	// stall rather than reaching N.
	err := runBootstrap(t, peer, node)
	if err != ErrStalled {
		t.Fatalf("expected ErrStalled (sync cannot pass the invalid block), got %v", err)
	}
	if node.height != bad-1 {
		t.Fatalf("sync must halt at the block BELOW the invalid one (%d), got %d", bad-1, node.height)
	}
}

// TestLoop_NoBeaconSet_SingleNodeFinishesImmediately: a node with NO beacon set configured
// (single-node / dev / --skip-bootstrap) has no network frontier to reach, so it completes
// at once — it must NOT hang waiting for a quorum that can never form. This is the
// single-node case the connectivity-wait must preserve.
func TestLoop_NoBeaconSet_SingleNodeFinishesImmediately(t *testing.T) {
	const N = 10
	chain, reg := chainOf(N, 0)
	peer := newTestPeer(chain)
	peer.status = FrontierNoBeacons // no beacon set — nothing to sync to
	node := newTestNode(reg, chain[N])

	if err := runBootstrap(t, peer, node); err != nil {
		t.Fatalf("single-node (no beacons) bootstrap should finish cleanly, got %v", err)
	}
	if node.accepts != 0 {
		t.Fatalf("no beacon set → nothing to accept, got %d accepts", node.accepts)
	}
}

// TestLoop_StaleNodeConvergesAfterDelayedBeaconConnect REPRODUCES THE MAINNET CANARY and
// proves the fix. A STALE node sits at height M; the producers (beacons) are at N (gap
// within the window). At boot the beacons have NOT connected yet, so the FIRST few frontier
// queries return FrontierConnecting. THE BUG was: the loop read that as "nothing to sync to"
// and declared caughtUp at the stale height M (then entered live consensus, never to
// converge). THE FIX: the loop WAITS through the connecting passes — declaring caughtUp at M
// is impossible — and only once the beacons connect and a ⅔ quorum names tip N does it
// descend, execute the gap, and converge to N.
func TestLoop_StaleNodeConvergesAfterDelayedBeaconConnect(t *testing.T) {
	const N = 40 // producers' (beacon-named) frontier height
	const M = 23 // our STALE local height (gap N-M = 17 — the canary's gap-17, within window)
	chain, reg := chainOf(N, 0)
	peer := newTestPeer(chain)
	peer.connectAfter = 5 // beacons connect only after the 5th frontier query (boot race)
	node := newTestNode(reg, chain[M])

	if err := runBootstrap(t, peer, node); err != nil {
		t.Fatalf("stale node must converge once beacons connect, got %v", err)
	}
	// It must NOT have false-completed at the stale height M: it reached the beacon frontier N.
	if node.height != N {
		t.Fatalf("CANARY: node did not converge to the beacon frontier — height %d, want %d (false-complete at M=%d?)", node.height, N, M)
	}
	if node.accepts != N-M {
		t.Fatalf("expected exactly the gap accepted (M+1..N = %d), got %d", N-M, node.accepts)
	}
	// And it provably WAITED for connectivity rather than concluding caught-up immediately:
	// at minimum it polled through the connecting passes before naming the frontier.
	if peer.frontierCalls <= peer.connectAfter {
		t.Fatalf("loop must have WAITED through the connecting passes (got %d frontier calls, want > %d)", peer.frontierCalls, peer.connectAfter)
	}
}

// TestLoop_GenuinelyCaughtUp_CompletesPromptly: beacons ARE connected and name a tip equal
// to our height (no peer ahead). The node completes at once with nothing fetched — the
// connectivity-wait must not delay a node that is actually synced.
func TestLoop_GenuinelyCaughtUp_CompletesPromptly(t *testing.T) {
	const N = 25
	chain, reg := chainOf(N, 0)
	peer := newTestPeer(chain)               // FrontierNamed, tip = N
	node := newTestNode(reg, chain[N])       // already at N — holds the frontier tip

	if err := runBootstrap(t, peer, node); err != nil {
		t.Fatalf("a genuinely caught-up node must complete promptly, got %v", err)
	}
	if node.accepts != 0 {
		t.Fatalf("caught-up node fetches nothing, got %d accepts", node.accepts)
	}
}

// TestLoop_BeaconsNeverConnect_FailsSafe (red's LOW, folded in): a node with beacons
// configured but that NEVER reach the connectivity to form a ⅔ quorum must FAIL SAFE — it
// stays un-bootstrapped (ErrBeaconsUnreachable), NEVER false-completing at its stale height.
func TestLoop_BeaconsNeverConnect_FailsSafe(t *testing.T) {
	const N = 30
	const M = 10
	chain, reg := chainOf(N, 0)
	peer := newTestPeer(chain)
	peer.status = FrontierConnecting // beacons configured but never enough connected
	node := newTestNode(reg, chain[M])

	err := runBootstrap(t, peer, node)
	if err != ErrBeaconsUnreachable {
		t.Fatalf("eclipsed-at-boot node must fail safe with ErrBeaconsUnreachable, got %v", err)
	}
	if node.height != M || node.accepts != 0 {
		t.Fatalf("FALSE-COMPLETE: node advanced past its stale height (height %d, %d accepts) — must finalize nothing", node.height, node.accepts)
	}
}

// TestLoop_BeaconsConnectedNoQuorum_FailsSafe: beacons ARE connected (enough to ask) but no
// ⅔-by-stake supermajority agrees on a single frontier (eclipse/partition/disagreement).
// The node must FAIL SAFE — not conclude caught-up at the stale height.
func TestLoop_BeaconsConnectedNoQuorum_FailsSafe(t *testing.T) {
	const N = 30
	const M = 10
	chain, reg := chainOf(N, 0)
	peer := newTestPeer(chain)
	peer.status = FrontierNoQuorum // connected, but no ⅔ agreement
	node := newTestNode(reg, chain[M])

	err := runBootstrap(t, peer, node)
	if err != ErrNoBeaconQuorum {
		t.Fatalf("no-⅔-quorum node must fail safe with ErrNoBeaconQuorum, got %v", err)
	}
	if node.height != M || node.accepts != 0 {
		t.Fatalf("FALSE-COMPLETE: node advanced past its stale height (height %d, %d accepts) — must finalize nothing", node.height, node.accepts)
	}
}

// TestRED_TransientNoQuorumOnHealthyNodeMustConverge is red's F2 PoC — the fix that makes the
// mainnet roll deterministic. A HEALTHY STALE node (height M, gap within the window) boots into
// a LIVE chain whose frontier is a MOVING TARGET. The beacons connect (a few FrontierConnecting
// passes), then on the very first tally — at the instant connectivity crosses the ⅔ floor, with
// the freshest least-settled beacons — they are momentarily SPLIT across adjacent finalized tips
// (FrontierNoQuorum). One more poll clears it (the split settles → FrontierNamed). The PRE-FIX
// loop returned ErrNoBeaconQuorum on that single transient NoQuorum and the node stuck at M,
// never reaching N (then the watchdog → restart). THE FIX: the loop RETRIES the transient
// no-quorum, the next tally names tip N, and the node descends + converges to N.
func TestRED_TransientNoQuorumOnHealthyNodeMustConverge(t *testing.T) {
	const N = 40 // producers' (beacon-named) frontier height
	const M = 23 // our STALE local height (gap N-M = 17 — the canary's gap-17, within window)
	chain, reg := chainOf(N, 0)
	peer := newTestPeer(chain)
	peer.connectAfter = 2 // boot race: 2 FrontierConnecting passes
	peer.noQuorumFor = 1  // then exactly ONE transient split (FrontierNoQuorum) before it settles
	node := newTestNode(reg, chain[M])

	if err := runBootstrap(t, peer, node); err != nil {
		t.Fatalf("a healthy node hitting a TRANSIENT no-quorum split must converge, not fail terminally, got %v", err)
	}
	// It must NOT have failed terminally at M on the transient: it reached the beacon frontier N.
	if node.height != N {
		t.Fatalf("F2: node did not converge to the beacon frontier — height %d, want %d (terminal-NoQuorum at M=%d?)", node.height, N, M)
	}
	if node.accepts != N-M {
		t.Fatalf("expected exactly the gap accepted (M+1..N = %d), got %d", N-M, node.accepts)
	}
	// And it provably RETRIED past the connecting AND the transient no-quorum passes.
	if peer.frontierCalls <= peer.connectAfter+peer.noQuorumFor {
		t.Fatalf("loop must have RETRIED through the connecting + transient no-quorum passes (got %d frontier calls, want > %d)",
			peer.frontierCalls, peer.connectAfter+peer.noQuorumFor)
	}
}

// TestRED_PersistentNoQuorumStillFailsSafe is the F2 control: a PERSISTENT no-quorum (genuine
// eclipse / partition / ≥⅓ Byzantine beacon stake) must STILL fail safe after the bounded retry
// — the retry must not turn a real liveness stall into an infinite spin or, worse, a
// false-complete. The node finalizes NOTHING and surfaces ErrNoBeaconQuorum.
func TestRED_PersistentNoQuorumStillFailsSafe(t *testing.T) {
	const N = 30
	const M = 10
	chain, reg := chainOf(N, 0)
	peer := newTestPeer(chain)
	peer.status = FrontierNoQuorum // connected, but no ⅔ agreement EVER (persistent)
	node := newTestNode(reg, chain[M])

	err := runBootstrap(t, peer, node)
	if err != ErrNoBeaconQuorum {
		t.Fatalf("a PERSISTENT no-quorum must fail safe with ErrNoBeaconQuorum after the bound, got %v", err)
	}
	if node.height != M || node.accepts != 0 {
		t.Fatalf("FALSE-COMPLETE: persistent no-quorum advanced the node (height %d, %d accepts) — must finalize nothing", node.height, node.accepts)
	}
	// It must have actually RETRIED (polled the frontier more than once) before giving up — a
	// persistent no-quorum is bounded, not immediate (default MaxNoQuorumRounds = 10).
	if peer.frontierCalls != defaultMaxNoQuorumRounds {
		t.Fatalf("persistent no-quorum must fail at exactly the bound (%d rounds), got %d frontier calls", defaultMaxNoQuorumRounds, peer.frontierCalls)
	}
}

// TestRED_NoQuorumBoundedRetryCount pins the bounded-retry COUNT precisely: with the bound set
// to N, a no-quorum that CLEARS in N-1 rounds CONVERGES, while one that PERSISTS to round N
// fails safe with ErrNoBeaconQuorum at exactly that bound (it never retries unboundedly and
// never false-completes). This is the transient/persistent boundary the whole F2 fix turns on.
func TestRED_NoQuorumBoundedRetryCount(t *testing.T) {
	const N = 40
	const M = 20
	const bound = 5

	runWithBound := func(peer *testPeer, node *testNode) error {
		bs := New(Config{
			Source: peer, Chain: node,
			RetryInterval:     time.Millisecond,
			ConnectDeadline:   time.Second,
			MaxNoQuorumRounds: bound,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return bs.Run(ctx)
	}

	// CLEARS within the bound (bound-1 transient rounds) → converges.
	chainOK, regOK := chainOf(N, 0)
	peerOK := newTestPeer(chainOK)
	peerOK.noQuorumFor = bound - 1 // 4 transient rounds, then named — under the bound of 5
	nodeOK := newTestNode(regOK, chainOK[M])
	if err := runWithBound(peerOK, nodeOK); err != nil {
		t.Fatalf("a no-quorum clearing in %d rounds (< bound %d) must converge, got %v", bound-1, bound, err)
	}
	if nodeOK.height != N {
		t.Fatalf("transient (cleared in %d rounds) must converge to N=%d, got %d", bound-1, N, nodeOK.height)
	}

	// PERSISTS past the bound → fails safe at exactly `bound` no-quorum rounds.
	chainBad, regBad := chainOf(N, 0)
	peerBad := newTestPeer(chainBad)
	peerBad.noQuorumFor = bound + 3 // still NoQuorum when the bound is hit
	nodeBad := newTestNode(regBad, chainBad[M])
	if err := runWithBound(peerBad, nodeBad); err != ErrNoBeaconQuorum {
		t.Fatalf("a no-quorum persisting past the bound (%d rounds) must fail safe with ErrNoBeaconQuorum, got %v", bound, err)
	}
	if nodeBad.height != M || nodeBad.accepts != 0 {
		t.Fatalf("FALSE-COMPLETE: persistent no-quorum advanced the node (height %d, %d accepts) — must finalize nothing", nodeBad.height, nodeBad.accepts)
	}
	if peerBad.frontierCalls != bound {
		t.Fatalf("bounded retry must fail at EXACTLY %d no-quorum rounds, got %d frontier calls", bound, peerBad.frontierCalls)
	}
}

// invalidStatusSource returns an UNDEFINED FrontierStatus (the zero value / out-of-range) every
// call — modeling a future FrontierTip bug. The loop must FAIL SAFE (never read it as caught-up).
type invalidStatusSource struct{ raw FrontierStatus }

func (s invalidStatusSource) FrontierTip(context.Context) (ids.ID, FrontierStatus) {
	return ids.Empty, s.raw
}
func (invalidStatusSource) Ancestors(context.Context, ids.ID, int) ([][]byte, error) {
	return nil, nil
}

// TestRED_UndefinedFrontierStatusFailsSafe is the F4 default-case proof: an out-of-range or
// zero-value FrontierStatus (FrontierInvalid) — which the type now defines as meaningless — must
// fall through syncOnce's fail-safe default to a bounded-retry-then-ErrNoBeaconQuorum, NOT to a
// false-complete at the stale height. Defense-in-depth: even a status the type does not define
// can never finalize a node short of the frontier.
func TestRED_UndefinedFrontierStatusFailsSafe(t *testing.T) {
	const N = 30
	const M = 10
	for _, raw := range []FrontierStatus{FrontierInvalid, FrontierStatus(99)} {
		chain, reg := chainOf(N, 0)
		node := newTestNode(reg, chain[M])
		bs := New(Config{
			Source: invalidStatusSource{raw: raw}, Chain: node,
			RetryInterval:     time.Millisecond,
			ConnectDeadline:   100 * time.Millisecond,
			MaxNoQuorumRounds: 3,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := bs.Run(ctx)
		cancel()
		if err != ErrNoBeaconQuorum {
			t.Fatalf("undefined status %d must fail safe (bounded retry → ErrNoBeaconQuorum), got %v", raw, err)
		}
		if node.height != M || node.accepts != 0 {
			t.Fatalf("FALSE-COMPLETE on undefined status %d: node advanced (height %d, %d accepts) — must finalize nothing", raw, node.height, node.accepts)
		}
	}
}

func TestLoop_AlreadyAtFrontier_FinishesImmediately(t *testing.T) {
	const N = 10
	chain, reg := chainOf(N, 0)
	peer := newTestPeer(chain)
	node := newTestNode(reg, chain[N]) // already holds the frontier tip

	if err := runBootstrap(t, peer, node); err != nil {
		t.Fatalf("already-synced bootstrap should finish cleanly, got %v", err)
	}
	if node.accepts != 0 {
		t.Fatalf("already at frontier → nothing to accept, got %d accepts", node.accepts)
	}
}

func TestLoop_DeadPeerStalls(t *testing.T) {
	const N = 50
	chain, reg := chainOf(N, 0)
	peer := newTestPeer(chain)
	peer.serveNothing = true // advertises a frontier but never serves blocks
	node := newTestNode(reg, chain[0])

	if err := runBootstrap(t, peer, node); err != ErrStalled {
		t.Fatalf("a peer that serves nothing must stall, got %v", err)
	}
	if node.height != 0 {
		t.Fatalf("no block should have been accepted from a dead peer, height %d", node.height)
	}
}
