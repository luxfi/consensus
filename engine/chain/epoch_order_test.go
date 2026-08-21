// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// epoch_order_test.go — the far-past epoch bound has a THIRD door, and it is not
// another function. It is ARRIVAL ORDER through the two doors already guarded.
//
// epochRegresses compares a block's stamped epoch against its PARENT'S RECORDED
// epoch, and admits when the parent is not tracked — justified by "an orphan
// cannot extend finalized history anyway". The justification holds for exactly as
// long as the block stays an orphan, and the peer decides how long that is. Send
// the CHILD first and the comparison has no parent to make; send the parent a
// moment later and the child is no longer an orphan, but nothing re-reads its
// epoch. The stale value is now the recorded epoch of a tracked block, and every
// later reader — blockPositionLocked's set-root, VerifyVote's pubkey resolution,
// verifyQuasarSupermajority's stake tally — resolves against it.
//
// The invariant these pin is the one the bound is FOR, stated over state rather
// than over a call: no TRACKED block sits at an epoch below its TRACKED parent's.
package chain

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// deliver drives one block through the gossip door exactly as HandleIncomingBlock
// does after a successful Verify.
func deliver(rt *Runtime, blk *pChainBlock) {
	rt.fastFollowMu.Lock()
	rt.followVerifiedBlock(context.Background(), blk, ids.GenerateTestNodeID())
	rt.fastFollowMu.Unlock()
}

// TestEpochOrder_ChildBeforeParent_KeepsStaleEpoch states the bound over the
// tracked set instead of over one call. Parent-first is the case the existing
// suite drives and it refuses; child-first admits the identical block, and the
// stale epoch survives the parent's arrival.
func TestEpochOrder_ChildBeforeParent_KeepsStaleEpoch(t *testing.T) {
	vm := newEpochGateVM()

	// CONTROL — parent first. This is the ordering every existing test of the
	// bound uses, and it refuses. Running it here makes the refusal in the attack
	// case attributable to ORDER and nothing else: same blocks, same epochs, same
	// door.
	control := newReceiveGateRuntime(vm)
	cparent := newEpochBlock(1_000, 100, ids.GenerateTestID(), "order-control-parent")
	cchild := newEpochBlock(1_001, 5, cparent.id, "order-control-child")
	vm.register(cparent)
	vm.register(cchild)
	deliver(control, cparent)
	deliver(control, cchild)
	if isTracked(control, cchild.id) {
		t.Fatal("control broke: parent-first must refuse a child whose epoch regresses — " +
			"without that refusal the attack case below proves nothing")
	}

	// THE ATTACK — the same two blocks, reversed.
	rt := newReceiveGateRuntime(vm)
	parent := newEpochBlock(1_000, 100, ids.GenerateTestID(), "order-attack-parent")
	child := newEpochBlock(1_001, 5, parent.id, "order-attack-child")
	vm.register(parent)
	vm.register(child)

	deliver(rt, child)  // parent untracked ⇒ nothing to regress against ⇒ admitted
	deliver(rt, parent) // the child is no longer an orphan

	if !isTracked(rt, parent.id) {
		t.Fatal("parent must track — the attack needs it to, and a refusal here would make the assertion vacuous")
	}
	if !isTracked(rt, child.id) {
		return // the bound closed this door; nothing left to prove
	}

	childEpoch, _ := rt.Transitive.consensus.EpochHeightOf(child.id)
	parentEpoch, _ := rt.Transitive.consensus.EpochHeightOf(parent.id)
	if childEpoch < parentEpoch {
		t.Fatalf("SAFETY BREAK: a TRACKED block sits at epoch %d below its TRACKED parent's %d. "+
			"The bound is evaluated once, at arrival, against a parent the sender chooses not to "+
			"have sent yet; the parent-first delivery of these exact blocks is refused. Every later "+
			"reader of the child — set-root, pubkey resolution, stake tally — now resolves at epoch %d.",
			childEpoch, parentEpoch, childEpoch)
	}
}

// TestEpochOrder_DepartedCoalitionFinalizes carries the same reordering all the
// way to VM.Accept.
//
// The set at epoch 5 is a coalition that has since left; the live set at epoch
// 100 shares no member with it. A fresh block at value height 1001, whose parent
// is a live epoch-100 block, is stamped at epoch 5 and delivered before that
// parent. The engine records epoch 5 for it, stamps the epoch-5 set-root into the
// position, resolves the voters' pubkeys in the epoch-5 set and tallies their
// stake against the epoch-5 total — so the departed coalition's signatures clear
// the ⅔ gate and the block finalizes. This is the outcome the bound exists to
// prevent, reached without regressing an epoch against any parent the engine was
// holding at the time.
func TestEpochOrder_DepartedCoalitionFinalizes(t *testing.T) {
	const (
		oldEpoch = uint64(5)
		newEpoch = uint64(100)
	)
	departed := newEpochSigners(4)
	live := newEpochSigners(5)

	src := &epochValidatorSet{
		byEpoch: map[uint64]map[ids.NodeID]ed25519.PublicKey{
			oldEpoch: {
				departed.ids[0]: departed.pub(0), departed.ids[1]: departed.pub(1),
				departed.ids[2]: departed.pub(2), departed.ids[3]: departed.pub(3),
			},
			newEpoch: {
				live.ids[0]: live.pub(0), live.ids[1]: live.pub(1), live.ids[2]: live.pub(2),
				live.ids[3]: live.pub(3), live.ids[4]: live.pub(4),
			},
		},
		stake: map[uint64]map[ids.NodeID]uint64{
			oldEpoch: {departed.ids[0]: 25, departed.ids[1]: 25, departed.ids[2]: 25, departed.ids[3]: 25},
			newEpoch: {live.ids[0]: 20, live.ids[1]: 20, live.ids[2]: 20, live.ids[3]: 20, live.ids[4]: 20},
		},
	}

	chainID := ids.GenerateTestID()
	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, live.ids[0], src, &recordingGossiper{}, live.signerFor(0)),
		WithStakeWeighting(src),
		WithValidatorSetRoot(src))
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	rt := &Runtime{Transitive: e, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	parent := &pChainBlock{
		id: ids.GenerateTestID(), parentID: ids.Empty, height: 1000,
		pChainHeight: newEpoch, timestamp: time.Now(), bytes: []byte("epoch-order:live-parent"),
	}
	child := &pChainBlock{
		id: ids.GenerateTestID(), parentID: parent.id, height: 1001,
		pChainHeight: oldEpoch, timestamp: time.Now(), bytes: []byte("epoch-order:far-past-child"),
	}

	deliver(rt, child)
	deliver(rt, parent)

	e.mu.RLock()
	pending := e.pendingBlocks[child.id]
	e.mu.RUnlock()
	if pending == nil {
		return // the bound closed this door before the block was ever tracked
	}
	pos := e.blockPositionLockedForTest(pending, child.id)
	if pos.ValidatorSetRoot != src.ValidatorSetRoot(oldEpoch) {
		t.Fatalf("the engine stamped set-root %s; expected the stale epoch-%d root %s — "+
			"if this ever fails the epoch is no longer being read off the child",
			pos.ValidatorSetRoot, oldEpoch, src.ValidatorSetRoot(oldEpoch))
	}

	votes := make([]SignedVote, 0, 4)
	for i := 0; i < 4; i++ {
		votes = append(votes, SignedVote{
			NodeID:    departed.ids[i],
			Accept:    true,
			Signature: ed25519.Sign(departed.keys[departed.ids[i]], CanonicalVoteMessage(pos)),
		})
	}
	cert, err := AssembleQuorumCert(pos, Quasar, 4, votes)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	certBytes, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	finalized := rt.HandleIncomingCert(certBytes)
	accepts := child.AcceptCalled()
	height, set := e.consensus.GetFinalizedHeight()
	if finalized || accepts != 0 || set {
		t.Fatalf("SAFETY BREAK: a coalition holding no stake in the live set finalized a fresh block. "+
			"finalized=%v VM.Accept=%d finalizedHeight=%d(set=%v). The child's parent is a tracked "+
			"epoch-%d block; the child is stamped at epoch %d and was merely delivered first, so the "+
			"bound admitted it and the whole verification — set-root, pubkeys, ⅔ stake tally — ran "+
			"against the set the attacker named.",
			finalized, accepts, height, set, newEpoch, oldEpoch)
	}
}
