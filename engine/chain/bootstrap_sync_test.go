// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// bootstrap_sync_test.go — proves the BOOTSTRAP accept path (initial sync by
// fetch-from-frontier + re-execute, no vote, no cert):
//
//   - EMPTY node (genesis → tip): a fresh node with only genesis converges to a
//     peer's height N by re-executing fetched blocks 1..N oldest-first. It does NOT
//     stay stuck at 0.
//   - PARTIAL node (M → N): a node already at height M converges to N by executing
//     M+1..N; re-feeding a block it already holds is a clean no-op.
//   - SAFETY (reject invalid): a block that fails Verify (a corrupt/forged block
//     from a malicious peer) is REJECTED — VM.Accept never runs and finalized height
//     does not advance past it. The sync recovers when a VALID block arrives.
//   - PHASE BOUNDARY: once the node goes live (FinishBootstrap), the bootstrap accept
//     path is fail-closed — a fetched block can no longer finalize without an α-of-K
//     cert. This is exactly where bootstrap ends and the live cert-gate begins.
//   - ORDERING: a gapped / out-of-order block is refused by the per-height guard, so
//     oldest-first is ENFORCED, not assumed.
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
// EMPTY NODE — genesis → tip. The headline fix: a fresh/empty node FETCHES +
// EXECUTES + ACCEPTS blocks 1..N from a peer and reaches height N (not stuck at 0).
// -----------------------------------------------------------------------------

func TestBootstrap_EmptyNodeSyncsGenesisToTip(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, _, rec := newCatchupRuntime(t, vs, 0, vm)

	// An EMPTY node: it holds only genesis (height 0). This is the luxd-0 incident —
	// stuck at C-Chain height 0 because the bootstrapper fetched nothing.
	genesis := newTestBlock(0, ids.Empty, "genesis")
	seedBehindAt(t, rt, vm, genesis)

	// The peer's chain is genesis → N (= 50). The fetch loop delivers blocks 1..50
	// oldest-first; each is re-executed (Verify) and accepted on frontier-trust.
	const N = 50
	gap := buildGap(vm, genesis, N)
	feedBootstrap(t, rt, gap)

	// CONVERGENCE: the empty node reached the network tip purely by fetch+execute.
	if fh, set := rt.Transitive.consensus.GetFinalizedHeight(); !set || fh != uint64(N) {
		t.Fatalf("empty node did NOT sync to tip: finalized height (%d,%v), want %d", fh, set, N)
	}
	if tip := rt.Transitive.consensus.GetFinalizedTip(); tip != gap[N-1].id {
		t.Fatalf("finalized tip %s != block N %s", tip, gap[N-1].id)
	}

	// Bootstrap accepts WITHOUT voting or assembling certs — it re-executes
	// network-finalized blocks. No vote/cert may be emitted (re-voting a decided
	// height is spam the network drops).
	rec.mu.Lock()
	votes, certs := len(rec.votes), len(rec.certs)
	rec.mu.Unlock()
	if votes != 0 || certs != 0 {
		t.Fatalf("bootstrap must NOT vote (%d) or gossip certs (%d) — it re-executes finalized blocks", votes, certs)
	}
}

// -----------------------------------------------------------------------------
// PARTIAL NODE — M → N. The stranded-spare incident (luxd-2 at 1082780, tip
// 1082797): a node already at M converges to N by executing the gap.
// -----------------------------------------------------------------------------

func TestBootstrap_PartialNodeConvergesToTip(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	const M = uint64(1082780)
	const k = 17 // the incident delta: N = M+17 = 1082797
	tip := newTestBlock(M, ids.Empty, "tip@M")
	seedBehindAt(t, rt, vm, tip)
	gap := buildGap(vm, tip, k)

	feedBootstrap(t, rt, gap)

	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != M+uint64(k) {
		t.Fatalf("partial node did NOT converge: finalized height %d, want %d (M=%d + k=%d)", fh, M+uint64(k), M, k)
	}

	// IDEMPOTENT RESPONDER OVERLAP: re-feeding a block we already hold (height ≤
	// finalized) is a clean no-op — not a re-Accept, not an error. The frontier
	// responder always serves some blocks we already have; bootstrap must skip them.
	already := gap[0]
	if err := rt.AcceptBootstrapBlock(context.Background(), already.bytes); err != nil {
		t.Fatalf("re-feeding an already-synced block must be a no-op, got: %v", err)
	}
	if got := already.AcceptCalled(); got != 1 {
		t.Fatalf("already-synced block must NOT be re-Accepted, AcceptCalled=%d", got)
	}
}

// -----------------------------------------------------------------------------
// SAFETY — a block that fails Verify is REJECTED. A malicious peer cannot advance
// the sync with a corrupt/forged block; finalized height does not move. The sync
// then recovers when a VALID block at that height arrives.
// -----------------------------------------------------------------------------

func TestBootstrap_RejectsInvalidBlockThenRecovers(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	const M = uint64(100)
	tip := newTestBlock(M, ids.Empty, "tip@M")
	seedBehindAt(t, rt, vm, tip)

	// A corrupt block at the contiguous next height M+1 whose Verify FAILS. We model
	// the failure faithfully with the verify-once block: pre-exhausting its single
	// successful Verify makes the bootstrap path's Verify (the next call) fail, exactly
	// as a real VM's Verify would reject a block with a bad state root / invalid txs.
	bad := newTestBlock(M+1, tip.id, "corrupt@M+1")
	vm.register(bad)
	_ = bad.Verify(context.Background()) // exhaust the one good Verify → next call errors

	err := rt.AcceptBootstrapBlock(context.Background(), bad.bytes)
	if err == nil {
		t.Fatal("SAFETY VIOLATION: a block that fails Verify was accepted via bootstrap")
	}
	if got := bad.AcceptCalled(); got != 0 {
		t.Fatalf("SAFETY VIOLATION: invalid block ran VM.Accept %d×", got)
	}
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != M {
		t.Fatalf("SAFETY VIOLATION: finalized height moved off M=%d to %d on an invalid block", M, fh)
	}

	// RECOVERY: a VALID block at the same height M+1 finalizes — the rejected invalid
	// block did not poison the height (it never committed to the ledger).
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
// PHASE BOUNDARY — once the node goes live (FinishBootstrap), the bootstrap accept
// path is FAIL-CLOSED. This is where bootstrap ends and the cert-gated live path
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

	// Now a fetched block — even a perfectly valid, contiguous one — is REFUSED by the
	// bootstrap path. Once live, only the α-of-K cert path finalizes.
	next := newTestBlock(M+1, tip.id, "post-live@M+1")
	vm.register(next)
	if err := rt.AcceptBootstrapBlock(context.Background(), next.bytes); err == nil {
		t.Fatal("PHASE VIOLATION: bootstrap accept succeeded after the node went live (cert-gate bypass)")
	}
	if got := next.AcceptCalled(); got != 0 {
		t.Fatalf("PHASE VIOLATION: post-live bootstrap accept ran VM.Accept %d×", got)
	}
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != M {
		t.Fatalf("PHASE VIOLATION: finalized height moved off M=%d to %d via post-live bootstrap", M, fh)
	}
}

// -----------------------------------------------------------------------------
// ORDERING — an out-of-order / gapped block is refused by the per-height guard, so
// the oldest-first invariant is ENFORCED. After the gap is filled in order, the
// same height finalizes.
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
	// (height M+2 != finalized+1 == M+1) WITHOUT verifying/accepting.
	if err := rt.AcceptBootstrapBlock(context.Background(), gap[1].bytes); err == nil {
		t.Fatal("ORDERING VIOLATION: a height-M+2 block accepted while finalized at M (gap guard bypassed)")
	}
	if got := gap[1].AcceptCalled(); got != 0 {
		t.Fatalf("ORDERING VIOLATION: out-of-order block ran VM.Accept %d×", got)
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
// M2 — FIRST-BLOCK ANCHOR. When the consensus finalized-height tracker is UNSET
// (the un-seeded / empty-genesis path — SyncState only sets it when the VM has a
// non-empty last-accepted), the per-height guard alone would record WHATEVER
// (height, parent) the first fetched block claims. AcceptBootstrapBlock instead
// binds the FIRST block to the VM's ACTUAL last-accepted: a peer cannot seed
// finality at an arbitrary height/parent.
// -----------------------------------------------------------------------------

func TestBootstrap_M2_FirstBlockAnchorsToLocalLastAccepted(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	// Empty node holding only genesis (height 0). Crucially we DO NOT seedBehindAt —
	// so the consensus finalized-height tracker stays UNSET (set==false). The VM's
	// last-accepted is genesis.
	genesis := newTestBlock(0, ids.Empty, "genesis")
	vm.register(genesis)
	_ = vm.SetPreference(context.Background(), genesis.id)
	if _, set := rt.Transitive.consensus.GetFinalizedHeight(); set {
		t.Fatal("precondition: tracker must be UNSET for the M2 path")
	}

	// ATTACK A — a peer's first block seeds finality far ahead (height 500) on an
	// arbitrary parent. Without the anchor, markFinalizedLocked would record it. The
	// anchor refuses it: height 500 != localLastH+1 (== 1).
	ahead := newTestBlock(500, ids.GenerateTestID(), "seed-ahead@500")
	vm.register(ahead)
	if err := rt.AcceptBootstrapBlock(context.Background(), ahead.bytes); err == nil {
		t.Fatal("M2 VIOLATION: first block at height 500 seeded finality off an unset tracker")
	}
	if got := ahead.AcceptCalled(); got != 0 {
		t.Fatalf("M2 VIOLATION: ahead block ran VM.Accept %d×", got)
	}
	if _, set := rt.Transitive.consensus.GetFinalizedHeight(); set {
		t.Fatal("M2 VIOLATION: tracker became set off an unanchored first block")
	}

	// ATTACK B — the right height (1) but the WRONG parent (not genesis). Refused: the
	// first block must extend the VM's actual last-accepted.
	wrongParent := newTestBlock(1, ids.GenerateTestID(), "wrong-parent@1")
	vm.register(wrongParent)
	if err := rt.AcceptBootstrapBlock(context.Background(), wrongParent.bytes); err == nil {
		t.Fatal("M2 VIOLATION: first block at height 1 with a non-genesis parent was accepted")
	}
	if _, set := rt.Transitive.consensus.GetFinalizedHeight(); set {
		t.Fatal("M2 VIOLATION: tracker became set off a wrong-parent first block")
	}

	// HONEST — height 1, parent == genesis. Anchored, Verify passes, finalizes; the
	// tracker is now seeded at height 1 and the normal contiguity guard takes over.
	first := newTestBlock(1, genesis.id, "first@1")
	vm.register(first)
	if err := rt.AcceptBootstrapBlock(context.Background(), first.bytes); err != nil {
		t.Fatalf("honest contiguous first block must finalize, got: %v", err)
	}
	if fh, set := rt.Transitive.consensus.GetFinalizedHeight(); !set || fh != 1 {
		t.Fatalf("M2: tracker must be seeded at height 1 after the anchored first block, got (%d,%v)", fh, set)
	}
}
