// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// catchup_settled_test.go — catch-up must steer by what this node has APPLIED, not
// only by what consensus has FINALIZED.
//
// These are not the same number. The ledger records what a quorum decided; the VM
// records what this node executed. Finalization can fold the ledger across a block the
// VM never applied, so the ledger legitimately runs ahead of the applied head. A
// catch-up gate reading only the ledger then discards every block in exactly the range
// it is fetching — the responder serves the gap, each entry is at or below the ledger,
// each is skipped as "already decided", and the node reports a full batch accepted
// while applying none of it. Nothing retries, because nothing recorded a failure.
//
// A node in that state stops forever at a gap of any size. It was observed live on a
// five-validator fleet with gaps of 58, 65 and 1,836 blocks.
package chain

import (
	"context"
	"fmt"
	"testing"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// advancingVM is catchupVM plus the one property a real EVM has and the plain mock does
// not: accepting a block advances the VM's last-accepted head. Without that, "applied"
// is frozen and no test can tell a node that is catching up from one that is stuck.
type advancingVM struct {
	*catchupVM
}

// applying wraps a block so Accept advances the VM head, as VM.Accept does in
// production. Identity is delegated, so the wrapper is invisible to every gate.
type applying struct {
	*verifyOnceBlock
	vm *advancingVM
}

func (a *applying) Accept(ctx context.Context) error {
	if err := a.verifyOnceBlock.Accept(ctx); err != nil {
		return err
	}
	a.vm.mu.Lock()
	a.vm.lastAcc = a.verifyOnceBlock.id
	a.vm.mu.Unlock()
	return nil
}

func (m *advancingVM) GetBlock(ctx context.Context, id ids.ID) (block.Block, error) {
	b, err := m.catchupVM.GetBlock(ctx, id)
	if err != nil {
		return nil, err
	}
	return &applying{verifyOnceBlock: b.(*verifyOnceBlock), vm: m}, nil
}

func (m *advancingVM) ParseBlock(ctx context.Context, bytes []byte) (block.Block, error) {
	b, err := m.catchupVM.ParseBlock(ctx, bytes)
	if err != nil {
		return nil, err
	}
	return &applying{verifyOnceBlock: b.(*verifyOnceBlock), vm: m}, nil
}

// TestCatchup_SteersByAppliedHeadNotLedger strands a node in the exact live shape: the
// ledger has folded to N+k while the VM has applied only N. Every block in N+1..N+k is
// therefore at or below the ledger, and a gate reading the ledger alone skips all of
// them.
//
// The assertion is VM.Accept, not the finalized height: the finalized height is already
// N+k before a single block is served, so it proves nothing. What must be true is that
// the blocks reach the VM.
func TestCatchup_SteersByAppliedHeadNotLedger(t *testing.T) {
	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(1_000_000)
	const k = 12

	// The VM's applied head is N.
	tip := newTestBlock(N, ids.Empty, "applied@N")
	base.register(tip)
	if err := vm.SetPreference(context.Background(), tip.id); err != nil {
		t.Fatalf("seed applied head: %v", err)
	}

	gap := buildGap(base, tip, k)

	// The ledger is ahead: fold it across N+1..N+k without the VM ever applying any of
	// them. This is what applyBranchFinalization does when pendingBlocks misses — it
	// writes a per-height entry with no VM block to accept, so the ledger records the
	// whole path while the applied head stays put.
	for _, blk := range gap {
		if _, err := rt.Transitive.consensus.FinalizeBranch(blk.id, blk.height, blk.parentID); err != nil {
			t.Fatalf("fold ledger over height %d: %v", blk.height, err)
		}
	}

	// Preconditions: the seam is real. Ledger at N+k, VM applied at N.
	fh, set := rt.Transitive.consensus.GetFinalizedHeight()
	if !set || fh != N+uint64(k) {
		t.Fatalf("precondition: ledger must be at %d, got (%d,%v)", N+uint64(k), fh, set)
	}
	if _, applied, err := rt.localLastAccepted(context.Background()); err != nil || applied != N {
		t.Fatalf("precondition: applied head must be %d, got (%d,%v)", N, applied, err)
	}

	// Serve the gap oldest-first, as the catch-up transport does.
	for i, blk := range gap {
		cert := catchupCertFor(t, vs, chainID, blk, []int{0, 1, 2, 3}, 3)
		// The contiguity rule still holds: only the block at applied+1 finalizes on the
		// spot; the rest are tracked so the fold can reach them. Neither outcome may be
		// a silent skip, which is what this test exists to catch.
		_ = rt.AcceptCatchupBlock(context.Background(), blk.bytes, cert)
		if i == 0 && blk.AcceptCalled() == 0 {
			t.Fatalf("block %d (applied+1) was never handed to the VM: catch-up skipped "+
				"it as already-decided because the ledger is at %d", blk.height, fh)
		}
	}

	// The load-bearing assertion: the applied head moved. A node whose ledger is ahead
	// must still ingest, or it is stopped forever at a gap of any size.
	_, applied, err := rt.localLastAccepted(context.Background())
	if err != nil {
		t.Fatalf("read applied head: %v", err)
	}
	if applied <= N {
		t.Fatalf("applied head did not move: still %d after serving %d gap blocks "+
			"(ledger %d). Catch-up steered by the ledger and discarded the whole gap.", applied, k, fh)
	}
}

// TestSettledHeight_TakesTheLowerOfLedgerAndApplied pins the predicate itself, so a
// later refactor cannot quietly restore the ledger-only reading that caused the wedge.
func TestSettledHeight_TakesTheLowerOfLedgerAndApplied(t *testing.T) {
	for _, tc := range []struct {
		name            string
		ledger, applied uint64
		want            uint64
	}{
		{"ledger ahead of the VM — the live failure", 1_000_012, 1_000_000, 1_000_000},
		{"in lock-step", 500, 500, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vs := newTestValidatorSet(5)
			base := newCatchupVM()
			vm := &advancingVM{catchupVM: base}
			rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

			head := newTestBlock(tc.applied, ids.Empty, fmt.Sprintf("applied@%d", tc.applied))
			base.register(head)
			if err := vm.SetPreference(context.Background(), head.id); err != nil {
				t.Fatalf("seed applied head: %v", err)
			}
			ledgerTip := newTestBlock(tc.ledger, head.id, fmt.Sprintf("ledger@%d", tc.ledger))
			base.register(ledgerTip)
			if _, err := rt.Transitive.consensus.FinalizeBranch(ledgerTip.id, tc.ledger, ids.Empty); err != nil {
				t.Fatalf("fold ledger: %v", err)
			}

			got, set := rt.settledHeight(context.Background())
			if !set || got != tc.want {
				t.Fatalf("settledHeight = (%d,%v), want (%d,true) for ledger=%d applied=%d",
					got, set, tc.want, tc.ledger, tc.applied)
			}
		})
	}
}

// TestReplay_RefusesABlockTheLedgerDidNotFinalize is the safety half of replay.
//
// Replay applies a block without consuming a cert, so the ledger is the only thing
// standing between a peer and this node's state. A peer that answers a catch-up fetch
// with a well-formed block at a height in the gap — but not the block finalized there —
// must be refused, and must not reach the VM. Two ways to be wrong are covered: an
// impostor at a known height, and any block at a height the ledger cannot speak for.
func TestReplay_RefusesABlockTheLedgerDidNotFinalize(t *testing.T) {
	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(1_000_000)
	const k = 6

	tip := newTestBlock(N, ids.Empty, "applied@N")
	base.register(tip)
	if err := vm.SetPreference(context.Background(), tip.id); err != nil {
		t.Fatalf("seed applied head: %v", err)
	}
	gap := buildGap(base, tip, k)
	for _, blk := range gap {
		if _, err := rt.Transitive.consensus.FinalizeBranch(blk.id, blk.height, blk.parentID); err != nil {
			t.Fatalf("fold ledger over height %d: %v", blk.height, err)
		}
	}

	// An impostor at N+1: same height, same parent, different block. The ledger finalized
	// gap[0] there, so this one is not ours however plausible it looks.
	impostor := newTestBlock(N+1, tip.id, "impostor@N+1")
	base.register(impostor)
	certI := catchupCertFor(t, vs, chainID, impostor, []int{0, 1, 2, 3}, 3)
	if err := rt.AcceptCatchupBlock(context.Background(), impostor.bytes, certI); err == nil {
		t.Fatal("replay accepted a block the ledger did not finalize at that height")
	}
	if got := impostor.AcceptCalled(); got != 0 {
		t.Fatalf("impostor reached the VM: Accept called %d times", got)
	}

	// A height below anything the ledger recorded. It cannot vouch for it, so replay must
	// refuse rather than assume.
	stranger := newTestBlock(N-1, ids.GenerateTestID(), "stranger@N-1")
	base.register(stranger)
	certS := catchupCertFor(t, vs, chainID, stranger, []int{0, 1, 2, 3}, 3)
	_ = rt.AcceptCatchupBlock(context.Background(), stranger.bytes, certS)
	if got := stranger.AcceptCalled(); got != 0 {
		t.Fatalf("a block at a height the ledger cannot speak for reached the VM: Accept called %d times", got)
	}

	// The honest block at the same height is still accepted — the refusal above is about
	// identity, not about replay being switched off.
	certOK := catchupCertFor(t, vs, chainID, gap[0], []int{0, 1, 2, 3}, 3)
	if err := rt.AcceptCatchupBlock(context.Background(), gap[0].bytes, certOK); err != nil {
		t.Fatalf("the finalized block at N+1 was refused: %v", err)
	}
	if got := gap[0].AcceptCalled(); got != 1 {
		t.Fatalf("the finalized block at N+1 must be applied exactly once, got %d", got)
	}
}

// TestReplay_ClosesAGapDeeperThanTheLedgerWindow is the ceiling fix (red RED-1). The
// ledger prunes its own byHeight to a near-tip window, so a gap deeper than that window
// used to be unnameable and replay refused every block of it. The recovery index names
// heights far below the tip, so a deep gap closes.
//
// It reproduces the post-wedge state directly: the ledger folded to N+k while the VM
// stayed at N, and the recovery index holds N+1..N+k — exactly what applyBranchFinalization
// leaves behind when it folds across a pendingBlocks miss (records the height, skips
// VM.Accept). Then the gap is served for replay; the bottom heights are pruned from
// byHeight and reachable only through recovery.
func TestReplay_ClosesAGapDeeperThanTheLedgerWindow(t *testing.T) {
	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(1_000_000)
	const k = 1100 // > the ledger's equivocation window (1024), < recoveryDepth

	tip := newTestBlock(N, ids.Empty, "applied@N")
	base.register(tip)
	if err := vm.SetPreference(context.Background(), tip.id); err != nil {
		t.Fatalf("seed applied head: %v", err)
	}
	gap := buildGap(base, tip, k)

	// Fold the ledger across the whole gap AND record each height in recovery — the pair
	// applyBranchFinalization writes when the VM misses a block. The VM stays at N.
	for _, blk := range gap {
		if _, err := rt.Transitive.consensus.FinalizeBranch(blk.id, blk.height, blk.parentID); err != nil {
			t.Fatalf("fold ledger over %d: %v", blk.height, err)
		}
		rt.Transitive.mu.Lock()
		rt.Transitive.recordRecoveredLocked(blk.height, blk.id)
		rt.Transitive.mu.Unlock()
	}

	// Preconditions: ledger at N+k, VM at N, and the bottom of the gap PRUNED from the
	// ledger's own index (so it can only be named through recovery).
	if fh, set := rt.Transitive.consensus.GetFinalizedHeight(); !set || fh != N+uint64(k) {
		t.Fatalf("precondition: ledger at (%d,%v), want %d", fh, set, N+uint64(k))
	}
	if _, applied, _ := rt.localLastAccepted(context.Background()); applied != N {
		t.Fatalf("precondition: applied head %d, want %d", applied, N)
	}
	if _, _, known := rt.Transitive.consensus.FinalizedAt(gap[0].height); known {
		t.Fatalf("precondition: height %d must be PRUNED from byHeight for this test to exercise recovery", gap[0].height)
	}

	// Serve the whole gap oldest-first for replay (no cert — replay takes none).
	for _, blk := range gap {
		_ = rt.AcceptCatchupBlock(context.Background(), blk.bytes, nil)
	}

	// The load-bearing assertion: the applied head closed the entire deep gap, including
	// the heights only the recovery index could name.
	_, applied, err := rt.localLastAccepted(context.Background())
	if err != nil {
		t.Fatalf("read applied head: %v", err)
	}
	if applied != N+uint64(k) {
		t.Fatalf("applied head = %d, want %d — a gap of %d (deeper than the ledger window) did not close",
			applied, N+uint64(k), k)
	}
}

// TestSettledHeight_LowersToGenuineZero is red RED-3: a VM genuinely at height 0 under a
// ledger above it is a wedge, and the floor must drop to 0 so replay can execute height 1.
// The unreadable-head case (which also reads as 0) must NOT lower the floor.
func TestSettledHeight_LowersToGenuineZero(t *testing.T) {
	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, _, _ := newCatchupRuntime(t, vs, 0, vm)

	// VM genuinely at genesis (height 0), readable.
	genesis := newTestBlock(0, ids.Empty, "genesis")
	base.register(genesis)
	if err := vm.SetPreference(context.Background(), genesis.id); err != nil {
		t.Fatalf("seed genesis: %v", err)
	}
	// Ledger folded above it.
	ledgerTip := newTestBlock(6, genesis.id, "ledger@6")
	base.register(ledgerTip)
	if _, err := rt.Transitive.consensus.FinalizeBranch(ledgerTip.id, 6, genesis.id); err != nil {
		t.Fatalf("fold ledger: %v", err)
	}

	got, set := rt.settledHeight(context.Background())
	if !set || got != 0 {
		t.Fatalf("settledHeight = (%d,%v), want (0,true) — a genuine-zero VM under a ledger is a wedge the floor must expose", got, set)
	}
}

// TestRecoveryIndex_RecordsTheRealHeightThroughTheFold guards the population half of the
// recovery index (red pass-4 Q2): a block finalized through the real cert fold must land
// in recoveredAt at its TRUE height, not 0. The height a gap block carries into the accept
// loop comes from the Plan (AcceptHeights), because a gap block is no longer tracked and
// deriving its height from pendingBlocks yields 0 — which would file it under the wrong
// height and make the whole index inert for exactly the blocks it exists to serve.
func TestRecoveryIndex_RecordsTheRealHeightThroughTheFold(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(500_000)
	const k = 20
	tip := newTestBlock(N, ids.Empty, "tip@N")
	seedBehindAt(t, rt, vm, tip)
	gap := buildGap(vm, tip, k)

	// Finalize the gap through the real cert path — applyBranchFinalization runs, and its
	// accept loop is what records recovery.
	for _, blk := range gap {
		cert := catchupCertFor(t, vs, chainID, blk, []int{0, 1, 2, 3}, 3)
		if err := rt.AcceptCatchupBlock(context.Background(), blk.bytes, cert); err != nil {
			t.Fatalf("finalize height %d: %v", blk.height, err)
		}
	}

	// Every finalized height must be in the recovery index under its OWN height, mapping
	// to its OWN id — never height 0.
	if id, ok := rt.Transitive.recoveredOuterAt(0); ok {
		t.Fatalf("recovery index has a bogus height-0 entry (%s) — a gap block was filed under 0", id)
	}
	for _, blk := range gap {
		id, ok := rt.Transitive.recoveredOuterAt(blk.height)
		if !ok {
			t.Fatalf("recovery index missing height %d entirely", blk.height)
		}
		if id != blk.id {
			t.Fatalf("recovery index height %d → %s, want %s", blk.height, id, blk.id)
		}
	}
}
