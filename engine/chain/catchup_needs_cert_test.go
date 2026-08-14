// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// catchup_needs_cert_test.go — tracking is not finalizing.
//
// The catch-up path verifies and TRACKS blocks that arrive above the node's next
// contiguous height, so that a later verified quorum cert on a descendant can
// finalize the run behind it. That is only sound if a tracked block can never
// become final on its own. These tests pin that: no cert, no finality, no matter
// how many blocks a peer hands us or how well-formed they are.

package chain

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// parseVM hands back the block a peer's bytes name, so the catch-up path can be
// driven with blocks at chosen heights.
type parseVM struct{ byBytes map[string]*verifyOnceBlock }

func (v *parseVM) BuildBlock(context.Context) (block.Block, error) {
	return nil, errors.New("parseVM does not build")
}

func (v *parseVM) GetBlock(_ context.Context, id ids.ID) (block.Block, error) {
	for _, b := range v.byBytes {
		if b.id == id {
			return b, nil
		}
	}
	return nil, errors.New("not held")
}

func (v *parseVM) ParseBlock(_ context.Context, bytes []byte) (block.Block, error) {
	if b, ok := v.byBytes[string(bytes)]; ok {
		return b, nil
	}
	return nil, errors.New("unknown bytes")
}

func (v *parseVM) LastAccepted(context.Context) (ids.ID, error) { return ids.Empty, nil }
func (v *parseVM) SetPreference(context.Context, ids.ID) error  { return nil }

// TestTrackedCatchupBlockIsNotFinal is the security property the tracking change
// rests on: a block delivered WITHOUT a cert is verified and kept, and is still
// not accepted afterwards. If this ever passes finality to an untracked-cert
// block, a peer can decide our chain by sending us bytes.
func TestTrackedCatchupBlockIsNotFinal(t *testing.T) {
	vm := &parseVM{byBytes: map[string]*verifyOnceBlock{}}
	e := NewWithConfig(Config{Params: params5()}, WithVM(vm))
	rt := &Runtime{Transitive: e, config: NetworkConfig{
		ChainID: ids.GenerateTestID(), Logger: log.Noop(), VM: vm,
	}}

	tip := ids.GenerateTestID()
	e.consensus.mu.Lock()
	e.consensus.ledger = seedLedger(tip, tip, 8)
	e.consensus.mu.Unlock()

	// A run of well-formed blocks well above our next height (9), none certified.
	var ids20 []ids.ID
	parent := tip
	for h := 20; h < 26; h++ {
		raw := fmt.Sprintf("uncertified-%d", h)
		blk := &verifyOnceBlock{
			id: ids.GenerateTestID(), parentID: parent, height: uint64(h),
			timestamp: time.Now(), bytes: []byte(raw),
		}
		vm.byBytes[raw] = blk
		ids20 = append(ids20, blk.id)
		parent = blk.id

		err := rt.AcceptCatchupBlock(context.Background(), []byte(raw), nil)
		if err == nil {
			t.Fatalf("height %d: AcceptCatchupBlock reported SUCCESS with no cert — a peer could finalize our chain by sending bytes", h)
		}
		if !errors.Is(err, ErrCatchupCertRejected) {
			t.Fatalf("height %d: want ErrCatchupCertRejected, got %v", h, err)
		}
	}

	// The whole run is held, and not one block of it is final.
	for i, id := range ids20 {
		if e.IsAccepted(id) {
			t.Fatalf("block %d of the uncertified run is ACCEPTED — tracking leaked into finality", i)
		}
	}
	if fh, _ := e.consensus.GetFinalizedHeight(); fh != 8 {
		t.Fatalf("finalized height moved to %d on uncertified blocks; must still be 8", fh)
	}
}

// TestUncertifiedRunLeavesFinalityWhereItWas: repeated delivery of the same
// uncertified blocks must not accumulate into anything. A peer that re-sends
// forever gets no further than one that sends once.
func TestUncertifiedRunLeavesFinalityWhereItWas(t *testing.T) {
	vm := &parseVM{byBytes: map[string]*verifyOnceBlock{}}
	e := NewWithConfig(Config{Params: params5()}, WithVM(vm))
	rt := &Runtime{Transitive: e, config: NetworkConfig{
		ChainID: ids.GenerateTestID(), Logger: log.Noop(), VM: vm,
	}}

	tip := ids.GenerateTestID()
	e.consensus.mu.Lock()
	e.consensus.ledger = seedLedger(tip, tip, 8)
	e.consensus.mu.Unlock()

	blk := &verifyOnceBlock{
		id: ids.GenerateTestID(), parentID: tip, height: 40,
		timestamp: time.Now(), bytes: []byte("resent"),
	}
	vm.byBytes["resent"] = blk

	for i := 0; i < 50; i++ {
		if err := rt.AcceptCatchupBlock(context.Background(), []byte("resent"), nil); err == nil {
			t.Fatalf("delivery %d finalized an uncertified block", i)
		}
	}
	if e.IsAccepted(blk.id) {
		t.Fatal("50 uncertified deliveries accepted the block — repetition must not substitute for a cert")
	}
	if fh, _ := e.consensus.GetFinalizedHeight(); fh != 8 {
		t.Fatalf("finalized height moved to %d; must still be 8", fh)
	}
}

// TestGarbageCatchupBytesAreRefused: bytes that do not parse never reach the
// tracker at all.
func TestGarbageCatchupBytesAreRefused(t *testing.T) {
	vm := &parseVM{byBytes: map[string]*verifyOnceBlock{}}
	e := NewWithConfig(Config{Params: params5()}, WithVM(vm))
	rt := &Runtime{Transitive: e, config: NetworkConfig{
		ChainID: ids.GenerateTestID(), Logger: log.Noop(), VM: vm,
	}}

	if err := rt.AcceptCatchupBlock(context.Background(), []byte("not a block"), nil); err == nil {
		t.Fatal("unparseable bytes were accepted")
	}
	if err := rt.AcceptCatchupBlock(context.Background(), []byte("not a block"), []byte("forged cert")); err == nil {
		t.Fatal("unparseable bytes with a forged cert were accepted")
	}
}

// TestHeldBlockIsNotReVerified: when our VM already holds the block, catch-up must
// use that copy instead of asking the VM to verify it again. Re-verification is an
// insert of an old canonical block, which a VM correctly refuses — and a node whose
// finality has fallen behind its own applied head is served nothing BUT blocks it
// already has, so that refusal used to reject the entire response.
func TestHeldBlockIsNotReVerified(t *testing.T) {
	vm := &parseVM{byBytes: map[string]*verifyOnceBlock{}}
	e := NewWithConfig(Config{Params: params5()}, WithVM(vm))
	rt := &Runtime{Transitive: e, config: NetworkConfig{
		ChainID: ids.GenerateTestID(), Logger: log.Noop(), VM: vm,
	}}

	tip := ids.GenerateTestID()
	e.consensus.mu.Lock()
	e.consensus.ledger = seedLedger(tip, tip, 8)
	e.consensus.mu.Unlock()

	blk := &verifyOnceBlock{
		id: ids.GenerateTestID(), parentID: tip, height: 30,
		timestamp: time.Now(), bytes: []byte("already-held"),
	}
	vm.byBytes["already-held"] = blk

	// The block was verified when we first accepted it; a second Verify now fails,
	// exactly as a VM refusing to re-insert its own canonical block does.
	if err := blk.Verify(context.Background()); err != nil {
		t.Fatalf("first verify should succeed: %v", err)
	}

	err := rt.AcceptCatchupBlock(context.Background(), []byte("already-held"), nil)
	if err == nil {
		t.Fatal("a block with no cert must not finalize")
	}
	if errors.Is(err, errVerifiedAlready) {
		t.Fatal("catch-up re-verified a block the VM already holds — the whole response would be rejected")
	}
	if got := blk.VerifyCalls(); got != 1 {
		t.Fatalf("Verify called %d times; the held copy must be used instead of re-verifying", got)
	}
	if e.IsAccepted(blk.id) {
		t.Fatal("held block was finalized without a cert")
	}
}

// TestHeldBlockAtNextHeightIsNotReVerified: the height that MOVES the ledger is
// finalized+1, and it took a different code path from every other height. A node
// behind on certs holds its whole gap, so re-verification refused the one block
// whose tracking the fold depends on — and pathFromTip then defers every cert above
// that hole.
func TestHeldBlockAtNextHeightIsNotReVerified(t *testing.T) {
	vm := &parseVM{byBytes: map[string]*verifyOnceBlock{}}
	e := NewWithConfig(Config{Params: params5()}, WithVM(vm))
	rt := &Runtime{Transitive: e, config: NetworkConfig{
		ChainID: ids.GenerateTestID(), Logger: log.Noop(), VM: vm,
	}}

	tip := ids.GenerateTestID()
	e.consensus.mu.Lock()
	e.consensus.ledger = seedLedger(tip, tip, 8)
	e.consensus.mu.Unlock()

	// Height 9 == finalized+1: the contiguous next block.
	blk := &verifyOnceBlock{
		id: ids.GenerateTestID(), parentID: tip, height: 9,
		timestamp: time.Now(), bytes: []byte("held-next"),
	}
	vm.byBytes["held-next"] = blk
	if err := blk.Verify(context.Background()); err != nil { // accepted earlier
		t.Fatalf("first verify: %v", err)
	}

	err := rt.AcceptCatchupBlock(context.Background(), []byte("held-next"), nil)
	if errors.Is(err, errVerifiedAlready) {
		t.Fatal("finalized+1 was re-verified — the one height the fold needs tracked is the one we refuse")
	}
	if got := blk.VerifyCalls(); got != 1 {
		t.Fatalf("Verify called %d times at finalized+1; the held copy must be used", got)
	}
	if e.IsAccepted(blk.id) {
		t.Fatal("finalized+1 was finalized without a cert")
	}
}

// TestRecallIsWiredWhenTheVMArrivesAfterConstruction: the node builds the engine
// with no VM and attaches one later, so wiring the VM read-back only at construction
// left it off in production while every test that passed WithVM saw it on.
func TestRecallIsWiredWhenTheVMArrivesAfterConstruction(t *testing.T) {
	vm := &parseVM{byBytes: map[string]*verifyOnceBlock{}}
	blk := &verifyOnceBlock{
		id: ids.GenerateTestID(), parentID: ids.GenerateTestID(), height: 77,
		timestamp: time.Now(), bytes: []byte("late-vm"),
	}
	vm.byBytes["late-vm"] = blk

	// Exactly the node's order: construct without a VM, attach afterwards.
	e := NewWithParams(params5())
	if _, _, _, _, ok := e.consensus.ancestry().Parent(blk.id); ok {
		t.Fatal("resolved a block before any VM was attached")
	}
	e.SetVM(vm)

	_, height, _, _, ok := e.consensus.ancestry().Parent(blk.id)
	if !ok {
		t.Fatal("after SetVM the walk still cannot read the VM — recall is unwired on the path the node actually uses")
	}
	if height != 77 {
		t.Fatalf("resolved height %d, want 77", height)
	}
}
