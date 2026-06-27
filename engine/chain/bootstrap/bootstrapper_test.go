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
