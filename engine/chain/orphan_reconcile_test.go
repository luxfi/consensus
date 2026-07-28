// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// orphan_reconcile_test.go — the build-ahead / diverged-VM-head reconcile.
//
// THE INCIDENT (live 5-node strict-PQ net under load). A node's inner EVM accepted head
// ran AHEAD of, or diverged from, the consensus finality ledger: when the engine steered
// the VM (SetPreference) onto the block it had just finalized, the EVM refused with "cannot
// orphan finalized block at height N to common block at height N-1" (core/blockchain.go:
// reorg guard). The engine responded with log.Crit → os.Exit(1): a healthy validator was
// killed even though its CONSENSUS finality was correct.
//
// THE FIX under test: reconcileVMToCertified replaces the unconditional fatal. It classifies
// the VM's diverged head against the finality LEDGER (the sole authority) and:
//   - drops an UNCERTIFIED provisional tip (above the finalized frontier, or a losing
//     sibling of a finalized block) by reconciling the VM to the certified block — no crash;
//   - HALTS fail-closed ONLY when the tip the VM would orphan is itself the consensus-
//     certified block at its height (a two-blocks-at-one-height double-finalization).
//
// SAFETY INVARIANT proven here: a tip is dropped ONLY when its canonical is provably NOT in
// byHeight, so NO certified block is ever orphaned to gain liveness.
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
// and whose LastAccepted returns a caller-controlled diverged head. It does NOT implement
// PreferenceReconciler (the no-live-reconcile case).
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

// TestReconcile_UncertifiedSibling_Reconciles: the VM's head is a LOSING SIBLING of the
// finalized block at the same height (its canonical is NOT the ledger's certified canonical
// there). It is uncertified ⇒ safe to drop ⇒ the VM is reconciled to the certified block and
// the node does NOT halt.
func TestReconcile_UncertifiedSibling_Reconciles(t *testing.T) {
	e := newTestEngine()

	certifiedOuter := ids.GenerateTestID() // the block consensus just finalized at height 7
	certifiedCanon := certifiedOuter
	e.consensus.ledger = seedLedger(certifiedOuter, certifiedCanon, 7) // byHeight[7] = certifiedCanon

	sibling := ids.GenerateTestID() // a DIFFERENT height-7 block the VM accepted locally
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
		t.Fatalf("VM must be reconciled TO the certified block %s, got %s", certifiedOuter, vm.reconciledTo)
	}
}

// TestReconcile_BuildAheadAboveFrontier_Reconciles: the VM's head is ABOVE the finalized
// frontier — the node built + accepted ahead of the quorum (the exact task scenario). Above
// the frontier there is no byHeight entry ⇒ uncertified ⇒ safe ⇒ reconcile, no halt.
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

// TestReconcile_CertifiedHead_HaltsFailClosed is the SAFETY assertion. The VM's head IS the
// consensus-certified block at its height, and the newly finalized block is a DIFFERENT
// canonical there — a genuine two-blocks-at-one-height double-finalization. The engine MUST
// refuse to reconcile (return false ⇒ caller halts fail-closed) and MUST NOT orphan the
// certified block.
func TestReconcile_CertifiedHead_HaltsFailClosed(t *testing.T) {
	e := newTestEngine()

	head := ids.GenerateTestID()                   // the VM's accepted head at height 7
	e.consensus.ledger = seedLedger(head, head, 7) // byHeight[7] = head (head IS certified)
	certifiedOther := ids.GenerateTestID()         // a DIFFERENT block claimed final at 7

	// BOTH blocks are readable, so the halt is decided by the LEDGER (byHeight[7] is head's
	// canonical, not certifiedOther's) and not by an unreadable id — this is the genuine
	// two-blocks-at-one-height shape, not an artefact of the mock.
	vm := &reconcilingOrphanVM{orphanVMBase: &orphanVMBase{
		head:   head,
		blocks: map[ids.ID]*mockBlock{head: mb(head, 7), certifiedOther: mb(certifiedOther, 7)},
	}}

	handled := e.reconcileVMToCertified(context.Background(), vm, certifiedOther, errOrphan)
	if handled {
		t.Fatal("SAFETY VIOLATION: engine reconciled away a CONSENSUS-CERTIFIED block (must return false / halt)")
	}
	if vm.reconcileCalls != 0 {
		t.Fatalf("SAFETY VIOLATION: ReconcilePreference was called for a certified head (%d times) — a certified block must NEVER be orphaned", vm.reconcileCalls)
	}
}

// TestReconcile_HeadEqualsCertified_NoOp: a transient SetPreference refusal where the VM's
// head already equals the certified block. No divergence ⇒ handled, no reconcile.
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

// TestReconcile_NoLiveReconcile_DefersNonFatal: a VM WITHOUT the optional PreferenceReconciler
// and an uncertified diverged head. The engine must NOT crash: it keeps its correct consensus
// finality and defers the VM head reconcile to offline recovery (handled=true, no reconcile).
func TestReconcile_NoLiveReconcile_DefersNonFatal(t *testing.T) {
	e := newTestEngine()
	certified := ids.GenerateTestID()
	e.consensus.ledger = seedLedger(certified, certified, 7)

	sibling := ids.GenerateTestID()
	vm := &orphanVMBase{ // does NOT implement PreferenceReconciler
		head:   sibling,
		blocks: map[ids.ID]*mockBlock{sibling: mb(sibling, 7)},
	}
	if _, ok := interface{}(vm).(PreferenceReconciler); ok {
		t.Fatal("test precondition: orphanVMBase must NOT implement PreferenceReconciler")
	}
	if !e.reconcileVMToCertified(context.Background(), vm, certified, errOrphan) {
		t.Fatal("a VM without live reconcile must be handled non-fatally (no halt), got false")
	}
}

// TestReconcile_UnreadableHead_DefersNonFatal: LastAccepted returns a head the VM cannot
// GetBlock. Cannot classify ⇒ do NOT crash, do NOT touch the VM (nothing orphaned), defer.
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
		t.Fatalf("must NOT reconcile an unclassifiable head, got %d calls", vm.reconcileCalls)
	}
}

// --- end-to-end through the sole finalizer -------------------------------------------

// TestAcceptWithCert_OrphanRefusal_ReconcilesInsteadOfHalting drives the FULL finalize path
// (acceptWithCertCore → ApplyCert → applyBranchFinalization(VM.Accept) → SetPreference) with
// a VM that refuses SetPreference with the orphan error and whose accepted head diverged from
// the just-finalized block. Pre-fix this path hit log.Crit → os.Exit(1); the test asserts the
// engine now finalizes cleanly (returns nil) AND reconciled the VM — the node did not halt.
func TestAcceptWithCert_OrphanRefusal_ReconcilesInsteadOfHalting(t *testing.T) {
	e := newTestEngine()
	ctx := context.Background()

	// The block we finalize at height 1 (fresh ledger ⇒ first finalize seeds byHeight[1]).
	finalID := ids.GenerateTestID()
	cb := &Block{id: finalID, parentID: ids.Empty, height: 1}
	if err := e.consensus.AddBlock(ctx, cb); err != nil {
		t.Fatalf("AddBlock: %v", err)
	}

	// A DIVERGED VM head: a different height-1 block the VM accepted ahead/locally. SetPreference
	// to finalID would orphan it, so the VM refuses; the head is uncertified (byHeight[1] will be
	// finalID's canonical), so the engine must reconcile — not crash.
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

	// SOLE finalizer. Pre-fix: os.Exit(1) inside here. Post-fix: clean finalize + reconcile.
	if err := e.acceptWithCertCore(ctx, finalID, cert, false); err != nil {
		t.Fatalf("acceptWithCertCore returned error (expected clean finalize+reconcile): %v", err)
	}

	// The engine reconciled the VM to the finalized block instead of halting.
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
