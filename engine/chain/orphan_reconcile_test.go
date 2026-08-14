// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// orphan_reconcile_test.go — reconciling a VM head that built ahead of, or diverged from,
// the consensus finality ledger.
//
// The two components hold different heads, and only one of them is the authority. A node's
// inner EVM accepts blocks locally, so its accepted head can run ahead of the finality
// ledger, or sit on a sibling the ledger did not certify. When the engine then steers the VM
// onto the block consensus just finalized, the EVM's reorg guard refuses the SetPreference
// with "cannot orphan finalized block at height N to common block at height N-1". That
// refusal says the VM would lose a block it locally accepted; it says nothing about whether
// consensus finality is correct, and treating it as fatal kills a node whose finality is
// correct.
//
// reconcileVMToCertified therefore classifies the VM's diverged head against the finality
// ledger, which is the sole authority on what is final, and:
//   - drops an uncertified provisional tip — one above the finalized frontier, or a losing
//     sibling of a finalized block — by reconciling the VM to the certified block;
//   - halts fail-closed when the tip the VM would orphan is itself the consensus-certified
//     block at its height, since that is two blocks certified at one height.
//
// The safety invariant proven here: a tip is dropped only when its canonical is provably
// absent from byHeight, so no certified block is ever orphaned to buy liveness.
package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// errOrphan is the EVM "cannot orphan finalized block" refusal, verbatim in shape.
var errOrphan = errors.New("cannot orphan finalized block at height: 7 to common block at height: 6")

// orphanVMBase is a BlockBuilder whose SetPreference always refuses with the orphan error
// and whose LastAccepted returns a caller-controlled diverged head. It does not implement
// PreferenceReconciler — the case where no live reconcile is available.
type orphanVMBase struct {
	head   ids.ID
	blocks map[ids.ID]*mockBlock
}

func (m *orphanVMBase) BuildBlock(context.Context) (block.Block, error) {
	return nil, errors.New("no build in this mock")
}
func (m *orphanVMBase) GetBlock(_ context.Context, id ids.ID) (block.Block, error) {
	if b, ok := m.blocks[id]; ok {
		return b, nil
	}
	return nil, errors.New("block not found")
}
func (m *orphanVMBase) ParseBlock(context.Context, []byte) (block.Block, error) {
	return nil, errors.New("no parse in this mock")
}
func (m *orphanVMBase) LastAccepted(context.Context) (ids.ID, error) { return m.head, nil }
func (m *orphanVMBase) SetPreference(context.Context, ids.ID) error  { return errOrphan }

// reconcilingOrphanVM adds the optional live reconcile primitive (what the EVM provides via
// its accepted-tip rewind). It records the reconcile so the test can assert it fired with
// the certified id — and never fired for a certified head.
type reconcilingOrphanVM struct {
	*orphanVMBase
	reconcileErr   error
	reconcileCalls int
	reconciledTo   ids.ID
}

func (m *reconcilingOrphanVM) ReconcilePreference(_ context.Context, certified ids.ID) error {
	m.reconcileCalls++
	if m.reconcileErr != nil {
		return m.reconcileErr
	}
	m.reconciledTo = certified
	m.head = certified // the EVM aligns its accepted head to the certified block
	return nil
}

func mb(id ids.ID, height uint64) *mockBlock { return &mockBlock{id: id, height: height} }

// --- direct classification tests: the BFT-safety core --------------------------------

// TestReconcile_UncertifiedSibling_Reconciles: the VM's head is a losing sibling of the
// finalized block at the same height — its canonical is not the ledger's certified canonical
// there. Uncertified, so safe to drop: the VM is reconciled to the certified block and the
// node does not halt.
func TestReconcile_UncertifiedSibling_Reconciles(t *testing.T) {
	e := newTestEngine()

	certifiedOuter := ids.GenerateTestID() // the block consensus just finalized at height 7
	certifiedCanon := certifiedOuter
	e.consensus.ledger = seedLedger(certifiedOuter, certifiedCanon, 7) // byHeight[7] = certifiedCanon

	sibling := ids.GenerateTestID() // a different height-7 block the VM accepted locally
	vm := &reconcilingOrphanVM{orphanVMBase: &orphanVMBase{
		head:   sibling,
		blocks: map[ids.ID]*mockBlock{sibling: mb(sibling, 7)},
	}}

	handled := e.reconcileVMToCertified(context.Background(), vm, certifiedOuter, errOrphan)
	if !handled {
		t.Fatal("uncertified sibling head must be handled safely (no halt), got handled=false")
	}
	if vm.reconcileCalls != 1 {
		t.Fatalf("expected exactly one ReconcilePreference call, got %d", vm.reconcileCalls)
	}
	if vm.reconciledTo != certifiedOuter {
		t.Fatalf("VM must be reconciled to the certified block %s, got %s", certifiedOuter, vm.reconciledTo)
	}
}

// TestReconcile_BuildAheadAboveFrontier_Reconciles: the VM's head is above the finalized
// frontier, because the node built and accepted ahead of the quorum. Above the frontier there
// is no byHeight entry, so the head is uncertified and safe to drop: reconcile, no halt.
func TestReconcile_BuildAheadAboveFrontier_Reconciles(t *testing.T) {
	e := newTestEngine()

	certifiedOuter := ids.GenerateTestID()
	e.consensus.ledger = seedLedger(certifiedOuter, certifiedOuter, 7) // frontier at height 7

	aheadTip := ids.GenerateTestID() // accepted ahead at height 9
	vm := &reconcilingOrphanVM{orphanVMBase: &orphanVMBase{
		head:   aheadTip,
		blocks: map[ids.ID]*mockBlock{aheadTip: mb(aheadTip, 9)},
	}}

	handled := e.reconcileVMToCertified(context.Background(), vm, certifiedOuter, errOrphan)
	if !handled {
		t.Fatal("build-ahead tip above the finalized frontier must be handled safely (no halt)")
	}
	if vm.reconcileCalls != 1 || vm.reconciledTo != certifiedOuter {
		t.Fatalf("expected reconcile to %s, got calls=%d to=%s", certifiedOuter, vm.reconcileCalls, vm.reconciledTo)
	}
}

// TestReconcile_CertifiedHead_HaltsFailClosed is the safety assertion. The VM's head is itself
// the consensus-certified block at its height, and the newly finalized block is a different
// canonical there — two blocks certified at one height. The engine has to refuse to reconcile
// (return false, so the caller halts fail-closed) rather than orphan the certified block.
func TestReconcile_CertifiedHead_HaltsFailClosed(t *testing.T) {
	e := newTestEngine()

	head := ids.GenerateTestID()                   // the VM's accepted head at height 7
	e.consensus.ledger = seedLedger(head, head, 7) // byHeight[7] = head (head IS certified)
	certifiedOther := ids.GenerateTestID()         // a different block claimed final at 7

	// Both blocks are readable, so the halt is decided by the ledger — byHeight[7] holds head's
	// canonical, not certifiedOther's — and not by an unreadable id. That keeps this the genuine
	// two-blocks-at-one-height shape rather than an artefact of the mock.
	vm := &reconcilingOrphanVM{orphanVMBase: &orphanVMBase{
		head:   head,
		blocks: map[ids.ID]*mockBlock{head: mb(head, 7), certifiedOther: mb(certifiedOther, 7)},
	}}

	handled := e.reconcileVMToCertified(context.Background(), vm, certifiedOther, errOrphan)
	if handled {
		t.Fatal("safety: the engine reconciled away a consensus-certified block (it must return false and halt)")
	}
	if vm.reconcileCalls != 0 {
		t.Fatalf("safety: ReconcilePreference was called for a certified head (%d times) — a certified block is never orphaned", vm.reconcileCalls)
	}
}

// TestReconcile_HeadEqualsCertified_NoOp: a transient SetPreference refusal where the VM's
// head already equals the certified block. Nothing has diverged, so it is handled with no
// reconcile.
func TestReconcile_HeadEqualsCertified_NoOp(t *testing.T) {
	e := newTestEngine()
	certified := ids.GenerateTestID()
	e.consensus.ledger = seedLedger(certified, certified, 7)
	vm := &reconcilingOrphanVM{orphanVMBase: &orphanVMBase{
		head:   certified,
		blocks: map[ids.ID]*mockBlock{certified: mb(certified, 7)},
	}}
	if !e.reconcileVMToCertified(context.Background(), vm, certified, errOrphan) {
		t.Fatal("head==certified must be handled (no halt)")
	}
	if vm.reconcileCalls != 0 {
		t.Fatalf("no reconcile needed when head==certified, got %d calls", vm.reconcileCalls)
	}
}

// TestReconcile_NoLiveReconcile_DefersNonFatal: a VM without the optional PreferenceReconciler
// and an uncertified diverged head. The engine keeps its correct consensus finality and defers
// the VM head reconcile to offline recovery (handled=true, no reconcile) rather than crashing.
func TestReconcile_NoLiveReconcile_DefersNonFatal(t *testing.T) {
	e := newTestEngine()
	certified := ids.GenerateTestID()
	e.consensus.ledger = seedLedger(certified, certified, 7)

	sibling := ids.GenerateTestID()
	vm := &orphanVMBase{ // deliberately does not implement PreferenceReconciler
		head:   sibling,
		blocks: map[ids.ID]*mockBlock{sibling: mb(sibling, 7)},
	}
	if _, ok := interface{}(vm).(PreferenceReconciler); ok {
		t.Fatal("test precondition: orphanVMBase must not implement PreferenceReconciler")
	}
	if !e.reconcileVMToCertified(context.Background(), vm, certified, errOrphan) {
		t.Fatal("a VM without live reconcile must be handled non-fatally (no halt), got false")
	}
}

// TestReconcile_UnreadableHead_DefersNonFatal: LastAccepted returns a head the VM cannot
// GetBlock. An unclassifiable head is left alone — nothing is orphaned, nothing crashes, the
// reconcile defers.
func TestReconcile_UnreadableHead_DefersNonFatal(t *testing.T) {
	e := newTestEngine()
	certified := ids.GenerateTestID()
	e.consensus.ledger = seedLedger(certified, certified, 7)

	vm := &reconcilingOrphanVM{orphanVMBase: &orphanVMBase{
		head:   ids.GenerateTestID(), // not in blocks ⇒ GetBlock fails
		blocks: map[ids.ID]*mockBlock{},
	}}
	if !e.reconcileVMToCertified(context.Background(), vm, certified, errOrphan) {
		t.Fatal("unreadable head must be handled non-fatally (no halt)")
	}
	if vm.reconcileCalls != 0 {
		t.Fatalf("an unclassifiable head must not be reconciled, got %d calls", vm.reconcileCalls)
	}
}

// --- end-to-end through the sole finalizer -------------------------------------------

// TestAcceptWithCert_OrphanRefusal_ReconcilesInsteadOfHalting drives the whole finalize path
// (acceptWithCertCore → ApplyCert → applyBranchFinalization(VM.Accept) → SetPreference) with a
// VM that refuses SetPreference with the orphan error and whose accepted head diverged from the
// just-finalized block. The classification above is only worth anything if it is reached from
// the real finalizer, so this asserts the path finalizes cleanly (returns nil) and reconciles
// the VM, rather than treating the refusal as fatal.
func TestAcceptWithCert_OrphanRefusal_ReconcilesInsteadOfHalting(t *testing.T) {
	e := newTestEngine()
	ctx := context.Background()

	// The block we finalize at height 1 (fresh ledger ⇒ first finalize seeds byHeight[1]).
	finalID := ids.GenerateTestID()
	cb := &Block{id: finalID, parentID: ids.Empty, height: 1}
	if err := e.consensus.AddBlock(ctx, cb); err != nil {
		t.Fatalf("AddBlock: %v", err)
	}

	// A diverged VM head: a different height-1 block the VM accepted locally. SetPreference to
	// finalID would orphan it, so the VM refuses; the head is uncertified, since byHeight[1] will
	// be finalID's canonical, so the engine reconciles rather than crashing.
	sibling := ids.GenerateTestID()
	vm := &reconcilingOrphanVM{orphanVMBase: &orphanVMBase{
		head:   sibling,
		blocks: map[ids.ID]*mockBlock{sibling: mb(sibling, 1), finalID: mb(finalID, 1)},
	}}
	e.SetVM(vm)

	// Track the block as an accept-able pending block with a VM block whose Accept succeeds.
	e.mu.Lock()
	e.pendingBlocks[finalID] = &PendingBlock{ConsensusBlock: cb, VMBlock: mb(finalID, 1)}
	e.mu.Unlock()

	// Build the finality authority token (the K==1 degenerate 1-of-1 cert shape) naming finalID.
	cert := VerifiedQuorumCert{qc: &QuorumCert{
		Version:   QuorumCertVersion,
		Type:      QCFinality,
		Tier:      Nova,
		Position:  VotePosition{Height: 1, Round: 0, BlockID: finalID, ParentID: ids.Empty, CanonicalID: finalID},
		Threshold: 1,
	}}

	// The sole finalizer: a clean finalize plus a reconcile, with the orphan refusal handled
	// inside rather than escalated.
	if err := e.acceptWithCertCore(ctx, finalID, cert, false); err != nil {
		t.Fatalf("acceptWithCertCore returned error (expected clean finalize+reconcile): %v", err)
	}

	// The engine reconciled the VM to the finalized block instead of halting the node.
	if vm.reconcileCalls != 1 {
		t.Fatalf("expected the engine to reconcile the VM exactly once, got %d", vm.reconcileCalls)
	}
	if vm.reconciledTo != finalID {
		t.Fatalf("VM must be reconciled to the finalized block %s, got %s", finalID, vm.reconciledTo)
	}
	// And consensus finality is correct: height 1 is finalized to finalID's canonical.
	if canon, ok := e.consensus.FinalizedBlockAtHeight(1); !ok || canon != finalID {
		t.Fatalf("consensus must have finalized height 1 to %s, got (%s, ok=%v)", finalID, canon, ok)
	}
}
