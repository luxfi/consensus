// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// bootstrap_sync_test.go — the bootstrap accept path: initial sync by
// fetch-from-frontier and re-execute, with no vote and no cert.
//
//   - Empty node (genesis → tip): a fresh node holding only genesis converges to a
//     peer's height N by re-executing fetched blocks 1..N oldest-first, rather than
//     staying at 0.
//   - Partial node (M → N): a node already at height M converges to N by executing
//     M+1..N; re-feeding a block it already holds is a clean no-op.
//   - Safety: a block that fails Verify — corrupt or forged, from a peer that need
//     not be honest — is rejected. VM.Accept never runs and finalized height does not
//     advance past it. The sync recovers when a valid block arrives.
//   - Phase boundary: once the node goes live (FinishBootstrap), the bootstrap accept
//     path is fail-closed — a fetched block can no longer finalize without an α-of-K
//     cert. That is where bootstrap ends and the live cert-gate begins.
//   - Ordering: a gapped or out-of-order block is refused by the per-height guard, so
//     oldest-first is enforced by the engine rather than assumed of the transport.
package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

// feedBootstrap feeds gap blocks oldest-first through AcceptBootstrapBlock, exactly
// as the node-side fetch loop delivers fetched ancestors during initial sync.
func feedBootstrap(t *testing.T, rt *Runtime, gap []*verifyOnceBlock) {
	t.Helper()
	for i, blk := range gap {
		if err := rt.AcceptBootstrapBlock(context.Background(), blk.bytes); err != nil {
			t.Fatalf("gap[%d] (height %d) bootstrap-accept failed: %v", i, blk.height, err)
		}
		if got := blk.AcceptCalled(); got != 1 {
			t.Fatalf("gap[%d] (height %d) must VM.Accept exactly once, got %d", i, blk.height, got)
		}
	}
}

// -----------------------------------------------------------------------------
// Empty node — genesis → tip. A node holding only genesis fetches, executes and
// accepts blocks 1..N from a peer, reaching height N.
// -----------------------------------------------------------------------------

func TestBootstrap_EmptyNodeSyncsGenesisToTip(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, _, rec := newCatchupRuntime(t, vs, 0, vm)

	// An empty node: it holds only genesis (height 0). A bootstrapper that fetches
	// nothing leaves such a node at height 0 for as long as it runs — nothing else in
	// the engine will advance it.
	genesis := newTestBlock(0, ids.Empty, "genesis")
	seedBehindAt(t, rt, vm, genesis)

	// The peer's chain is genesis → N (= 50). The fetch loop delivers blocks 1..50
	// oldest-first; each is re-executed (Verify) and accepted on frontier-trust.
	const N = 50
	gap := buildGap(vm, genesis, N)
	feedBootstrap(t, rt, gap)

	// Convergence: the empty node reached the network tip purely by fetch+execute.
	if fh, set := rt.Transitive.consensus.GetFinalizedHeight(); !set || fh != uint64(N) {
		t.Fatalf("empty node did not sync to tip: finalized height (%d,%v), want %d", fh, set, N)
	}
	if tip := rt.Transitive.consensus.GetFinalizedTip(); tip != gap[N-1].id {
		t.Fatalf("finalized tip %s != block N %s", tip, gap[N-1].id)
	}

	// Bootstrap accepts without voting or assembling certs — it re-executes blocks the
	// network already finalized. No vote or cert may be emitted: re-voting a decided
	// height is spam the network drops.
	rec.mu.Lock()
	votes, certs := len(rec.votes), len(rec.certs)
	rec.mu.Unlock()
	if votes != 0 || certs != 0 {
		t.Fatalf("bootstrap must not vote (%d) or gossip certs (%d) — it re-executes finalized blocks", votes, certs)
	}
}

// -----------------------------------------------------------------------------
// Partial node — M → N. A node already at M converges to N by executing the gap.
// -----------------------------------------------------------------------------

func TestBootstrap_PartialNodeConvergesToTip(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	const M = uint64(1_000_000) // a node deep in a long chain, not near genesis
	const k = 17                // gap width: N = M+k
	tip := newTestBlock(M, ids.Empty, "tip@M")
	seedBehindAt(t, rt, vm, tip)
	gap := buildGap(vm, tip, k)

	feedBootstrap(t, rt, gap)

	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != M+uint64(k) {
		t.Fatalf("partial node did not converge: finalized height %d, want %d (M=%d + k=%d)", fh, M+uint64(k), M, k)
	}

	// Idempotent responder overlap: re-feeding a block we already hold (height ≤
	// finalized) is a clean no-op — not a re-Accept, not an error. The frontier
	// responder always serves some blocks we already have; bootstrap must skip them.
	already := gap[0]
	if err := rt.AcceptBootstrapBlock(context.Background(), already.bytes); err != nil {
		t.Fatalf("re-feeding an already-synced block must be a no-op, got: %v", err)
	}
	if got := already.AcceptCalled(); got != 1 {
		t.Fatalf("already-synced block must not be re-Accepted, AcceptCalled=%d", got)
	}
}

// -----------------------------------------------------------------------------
// Safety — a block that fails Verify is rejected. A peer cannot advance the sync
// with a corrupt or forged block; finalized height does not move. The sync then
// recovers when a valid block at that height arrives.
// -----------------------------------------------------------------------------

func TestBootstrap_RejectsInvalidBlockThenRecovers(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	const M = uint64(100)
	tip := newTestBlock(M, ids.Empty, "tip@M")
	seedBehindAt(t, rt, vm, tip)

	// A corrupt block at the contiguous next height M+1 whose Verify fails. The
	// verify-once block models that faithfully: pre-exhausting its single successful
	// Verify makes the bootstrap path's Verify (the next call) fail, as a real VM's
	// Verify would reject a block with a bad state root or invalid txs.
	bad := newTestBlock(M+1, tip.id, "corrupt@M+1")
	vm.register(bad)
	_ = bad.Verify(context.Background()) // exhaust the one good Verify → next call errors

	err := rt.AcceptBootstrapBlock(context.Background(), bad.bytes)
	if err == nil {
		t.Fatal("a block that fails Verify was accepted via bootstrap")
	}
	if got := bad.AcceptCalled(); got != 0 {
		t.Fatalf("invalid block ran VM.Accept %d×", got)
	}
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != M {
		t.Fatalf("finalized height moved off M=%d to %d on an invalid block", M, fh)
	}

	// Recovery: a valid block at the same height M+1 finalizes — the rejected block
	// did not poison the height, because it never committed to the ledger.
	good := newTestBlock(M+1, tip.id, "valid@M+1")
	vm.register(good)
	if err := rt.AcceptBootstrapBlock(context.Background(), good.bytes); err != nil {
		t.Fatalf("valid block after a rejected one must finalize, got: %v", err)
	}
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != M+1 {
		t.Fatalf("sync did not recover to M+1, got %d", fh)
	}
}

// -----------------------------------------------------------------------------
// Phase boundary — once the node goes live (FinishBootstrap), the bootstrap accept
// path is fail-closed. That is where bootstrap ends and the cert-gated live path
// begins: a fetched block can no longer finalize without an α-of-K cert.
// -----------------------------------------------------------------------------

func TestBootstrap_FailClosedOnceLive(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	const M = uint64(100)
	tip := newTestBlock(M, ids.Empty, "tip@M")
	seedBehindAt(t, rt, vm, tip)

	// Sanity: while bootstrapping, the contiguous next block accepts.
	if !rt.Transitive.InBootstrapPhase() {
		t.Fatal("a fresh engine must start in the bootstrap phase")
	}

	// The node reaches the frontier and goes live.
	rt.Transitive.FinishBootstrap()
	if rt.Transitive.InBootstrapPhase() {
		t.Fatal("FinishBootstrap must end the bootstrap phase")
	}

	// Now a fetched block — even a perfectly valid, contiguous one — is refused by the
	// bootstrap path. Once live, only the α-of-K cert path finalizes.
	next := newTestBlock(M+1, tip.id, "post-live@M+1")
	vm.register(next)
	if err := rt.AcceptBootstrapBlock(context.Background(), next.bytes); err == nil {
		t.Fatal("bootstrap accept succeeded after the node went live, bypassing the cert-gate")
	}
	if got := next.AcceptCalled(); got != 0 {
		t.Fatalf("post-live bootstrap accept ran VM.Accept %d×", got)
	}
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != M {
		t.Fatalf("finalized height moved off M=%d to %d via post-live bootstrap", M, fh)
	}
}

// -----------------------------------------------------------------------------
// Ordering — an out-of-order or gapped block is refused by the per-height guard, so
// the oldest-first invariant is enforced. After the gap is filled in order, the same
// height finalizes.
// -----------------------------------------------------------------------------

func TestBootstrap_OutOfOrderRefusedThenInOrderConverges(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	const M = uint64(100)
	tip := newTestBlock(M, ids.Empty, "tip@M")
	seedBehindAt(t, rt, vm, tip)
	gap := buildGap(vm, tip, 3) // M+1, M+2, M+3

	// Skip ahead: feed M+2 while still finalized at M. The contiguity guard refuses it
	// (height M+2 != finalized+1 == M+1) without verifying or accepting.
	if err := rt.AcceptBootstrapBlock(context.Background(), gap[1].bytes); err == nil {
		t.Fatal("a height-M+2 block was accepted while finalized at M, bypassing the gap guard")
	}
	if got := gap[1].AcceptCalled(); got != 0 {
		t.Fatalf("out-of-order block ran VM.Accept %d×", got)
	}
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != M {
		t.Fatalf("out-of-order accept moved finalized height off M=%d to %d", M, fh)
	}

	// In order → all finalize. The earlier refusal was the guard, not a stuck path.
	feedBootstrap(t, rt, gap)
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != M+3 {
		t.Fatalf("did not converge to M+3 after in-order feed, got %d", fh)
	}
}

// -----------------------------------------------------------------------------
// First-block anchor. When the consensus finalized-height tracker is unset — the
// un-seeded, empty-genesis path, since SyncState only sets it when the VM has a
// non-empty last-accepted — the per-height guard alone would record whatever
// (height, parent) the first fetched block claims. AcceptBootstrapBlock instead
// binds the first block to the VM's actual last-accepted, so a peer cannot seed
// finality at an arbitrary height or parent.
// -----------------------------------------------------------------------------

func TestBootstrap_FirstBlockAnchorsToLocalLastAccepted(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	// Empty node holding only genesis (height 0). We deliberately skip seedBehindAt, so
	// the consensus finalized-height tracker stays unset (set==false). The VM's
	// last-accepted is genesis.
	genesis := newTestBlock(0, ids.Empty, "genesis")
	vm.register(genesis)
	_ = vm.SetPreference(context.Background(), genesis.id)
	if _, set := rt.Transitive.consensus.GetFinalizedHeight(); set {
		t.Fatal("precondition: tracker must be unset for the unanchored path")
	}

	// A peer's first block seeds finality far ahead (height 500) on an arbitrary
	// parent. Without the anchor, markFinalizedLocked would record it. The anchor
	// refuses it: height 500 != localLastH+1 (== 1).
	ahead := newTestBlock(500, ids.GenerateTestID(), "seed-ahead@500")
	vm.register(ahead)
	if err := rt.AcceptBootstrapBlock(context.Background(), ahead.bytes); err == nil {
		t.Fatal("first block at height 500 seeded finality off an unset tracker")
	}
	if got := ahead.AcceptCalled(); got != 0 {
		t.Fatalf("ahead block ran VM.Accept %d×", got)
	}
	if _, set := rt.Transitive.consensus.GetFinalizedHeight(); set {
		t.Fatal("tracker became set off an unanchored first block")
	}

	// The right height (1) but the wrong parent (not genesis). Refused: the first
	// block must extend the VM's actual last-accepted.
	wrongParent := newTestBlock(1, ids.GenerateTestID(), "wrong-parent@1")
	vm.register(wrongParent)
	if err := rt.AcceptBootstrapBlock(context.Background(), wrongParent.bytes); err == nil {
		t.Fatal("first block at height 1 with a non-genesis parent was accepted")
	}
	if _, set := rt.Transitive.consensus.GetFinalizedHeight(); set {
		t.Fatal("tracker became set off a wrong-parent first block")
	}

	// Height 1, parent == genesis. Anchored, Verify passes, finalizes; the tracker is
	// now seeded at height 1 and the normal contiguity guard takes over.
	first := newTestBlock(1, genesis.id, "first@1")
	vm.register(first)
	if err := rt.AcceptBootstrapBlock(context.Background(), first.bytes); err != nil {
		t.Fatalf("contiguous first block must finalize, got: %v", err)
	}
	if fh, set := rt.Transitive.consensus.GetFinalizedHeight(); !set || fh != 1 {
		t.Fatalf("tracker must be seeded at height 1 after the anchored first block, got (%d,%v)", fh, set)
	}
}
