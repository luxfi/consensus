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
