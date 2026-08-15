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

// TestReplay_CertVouchesWhereTheLedgerCannot covers the blind spot that stranded a live
// fleet. A ledger built from a boot seed holds one height and grows upward, so a node
// that restarted below its own finalized height cannot name the range it must replay.
// Refusing there leaves it stopped — observed as an applied head that moved a handful of
// blocks and then stalled with the tip hundreds ahead.
//
// A verified quorum cert speaks for those heights. It is not a weaker authority than the
// ledger; it is what writes the ledger. The forged case is the other half: without a real
// ⅔ witness, nothing is applied.
func TestReplay_CertVouchesWhereTheLedgerCannot(t *testing.T) {
	const N = uint64(1_000_000)
	const k = 5

	newFleet := func(t *testing.T) (*testValidatorSet, *catchupVM, *Runtime, ids.ID, *verifyOnceBlock, []*verifyOnceBlock) {
		t.Helper()
		vs := newTestValidatorSet(5)
		base := newCatchupVM()
		vm := &advancingVM{catchupVM: base}
		rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)

		tip := newTestBlock(N, ids.Empty, "applied@N")
		base.register(tip)
		if err := vm.SetPreference(context.Background(), tip.id); err != nil {
			t.Fatalf("seed applied head: %v", err)
		}
		gap := buildGap(base, tip, k)

		// Seed the ledger at the TOP of the gap only, exactly as a boot seed does: one
		// entry, nothing below it. The blocks in between are finalized as far as the
		// network is concerned and unnameable as far as this ledger is concerned.
		top := gap[k-1]
		if _, err := rt.Transitive.consensus.FinalizeBranch(top.id, top.height, top.parentID); err != nil {
			t.Fatalf("seed ledger at the top of the gap: %v", err)
		}
		if _, _, known := rt.Transitive.consensus.FinalizedAt(gap[0].height); known {
			t.Fatalf("precondition: the ledger must NOT know height %d", gap[0].height)
		}
		return vs, base, rt, chainID, tip, gap
	}

	t.Run("a real quorum cert is applied", func(t *testing.T) {
		vs, _, rt, chainID, _, gap := newFleet(t)
		cert := catchupCertFor(t, vs, chainID, gap[0], []int{0, 1, 2, 3}, 3)
		if err := rt.AcceptCatchupBlock(context.Background(), gap[0].bytes, cert); err != nil {
			t.Fatalf("a block at a height the ledger cannot name, carrying a real ⅔ cert, was refused: %v", err)
		}
		if got := gap[0].AcceptCalled(); got != 1 {
			t.Fatalf("VM.Accept called %d times, want 1 — the node is still stopped", got)
		}
		_, applied, err := rt.localLastAccepted(context.Background())
		if err != nil || applied != gap[0].height {
			t.Fatalf("applied head = %d (err %v), want %d", applied, err, gap[0].height)
		}
	})

	t.Run("a sub-quorum cert is refused", func(t *testing.T) {
		vs, _, rt, chainID, _, gap := newFleet(t)
		// Two of five signers, and the cert says so: a real signature set that declares a
		// threshold below the ⅔ this chain requires. Claiming three would fail to assemble.
		cert := catchupCertFor(t, vs, chainID, gap[0], []int{0, 1}, 2)
		if err := rt.AcceptCatchupBlock(context.Background(), gap[0].bytes, cert); err == nil {
			t.Fatal("a sub-quorum cert vouched for a block the ledger cannot name")
		}
		if got := gap[0].AcceptCalled(); got != 0 {
			t.Fatalf("a sub-quorum cert reached the VM: Accept called %d times", got)
		}
	})

	t.Run("a cert for a different block is refused", func(t *testing.T) {
		vs, base, rt, chainID, tip, gap := newFleet(t)
		other := newTestBlock(gap[0].height, tip.id, "other@N+1")
		base.register(other)
		// A perfectly valid cert — for a different block. It must not license this one.
		cert := catchupCertFor(t, vs, chainID, other, []int{0, 1, 2, 3}, 3)
		if err := rt.AcceptCatchupBlock(context.Background(), gap[0].bytes, cert); err == nil {
			t.Fatal("a cert naming another block licensed applying this one")
		}
		if got := gap[0].AcceptCalled(); got != 0 {
			t.Fatalf("Accept called %d times on a cert that names a different block", got)
		}
	})
}

// TestReplay_RefusesAReboundCertOnAContiguousSibling isolates the signed-identity
// binding from every other guard. The attacker's block is a STRUCTURALLY VALID child of
// the applied head — right height, right parent — so contiguity passes, the epoch is the
// same, and the set-root source is nil. The ONLY thing standing between a rebound honest
// cert and VM.Accept is that certVouchesFor binds on the signed canonical id, not the
// unsigned outer BlockID. Revert that one check and this test applies an attacker block.
func TestReplay_RefusesAReboundCertOnAContiguousSibling(t *testing.T) {
	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(1_000_000)
	tip := newTestBlock(N, ids.Empty, "applied@N")
	base.register(tip)
	if err := vm.SetPreference(context.Background(), tip.id); err != nil {
		t.Fatalf("seed applied head: %v", err)
	}

	// The honest block at N+1 and an attacker SIBLING at N+1 — both valid children of the
	// applied head, so both clear contiguity. Only their identity differs.
	honestBlk := newTestBlock(N+1, tip.id, "honest@N+1")
	evil := newTestBlock(N+1, tip.id, "EVIL@N+1")
	base.register(honestBlk)
	base.register(evil)

	// Ledger seeded only ABOVE the gap, so height N+1 falls to the cert branch.
	top := newTestBlock(N+5, honestBlk.id, "ledger@N+5")
	base.register(top)
	if _, err := rt.Transitive.consensus.FinalizeBranch(top.id, top.height, top.parentID); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}

	// An honest 4-of-5 cert for the honest block, rebound onto the attacker sibling:
	// promote the honest id into the signed canonical slot, rewrite the unsigned outer id.
	honest := catchupCertFor(t, vs, chainID, honestBlk, []int{0, 1, 2, 3}, 3)
	qc, err := UnmarshalQuorumCert(honest)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	qc.Position.CanonicalID = honestBlk.id
	qc.Position.ParentCanonicalID = honestBlk.parentID
	qc.Position.BlockID = evil.id
	qc.Position.ParentID = evil.parentID
	forged, err := qc.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if verr := rt.Transitive.verifyCert(qc, 0); verr != nil {
		t.Fatalf("rebinding broke signatures (attack does not apply): %v", verr)
	}

	err = rt.AcceptCatchupBlock(context.Background(), evil.bytes, forged)
	if got := evil.AcceptCalled(); got != 0 {
		t.Fatalf("EXPLOITED: a rebound cert applied an attacker sibling (Accept=%d, err=%v)", got, err)
	}
	// And the honest block the cert actually signs IS applied — the binding refuses the
	// impostor without breaking the legitimate path.
	if err := rt.AcceptCatchupBlock(context.Background(), honestBlk.bytes, honest); err != nil {
		t.Fatalf("the honestly-certified block was refused: %v", err)
	}
	if got := honestBlk.AcceptCalled(); got != 1 {
		t.Fatalf("honest block Accept=%d, want 1", got)
	}
}
