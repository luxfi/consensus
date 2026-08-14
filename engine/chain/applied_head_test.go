// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// applied_head_test.go — catch-up has to aim at the VM, not at the ledger.
//
// The consensus ledger and the VM persist separately, so a node that stops uncleanly
// can come back with certified finality above the block its VM last committed. Finality
// then cannot advance at all: the fail-closed guard refuses to move past applied state,
// while catch-up steering by the ledger asks peers for the height above the gap and so
// never fetches the blocks the VM is missing. Every arriving cert is refused for naming
// a parent the VM does not hold, and the two heads never reconverge.
//
// Catch-up must therefore steer by the lower of the two heads: what the VM has applied.

package chain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// appliedVM holds exactly one accepted block, at a height of our choosing.
type appliedVM struct {
	parseVM
	acceptedID ids.ID
	blk        *verifyOnceBlock
}

func (v *appliedVM) LastAccepted(context.Context) (ids.ID, error) { return v.acceptedID, nil }
func (v *appliedVM) GetBlock(_ context.Context, id ids.ID) (block.Block, error) {
	if v.blk != nil && id == v.blk.id {
		return v.blk, nil
	}
	return nil, errors.New("not held")
}

// TestAppliedHeadReportsTheVMWhenTheLedgerIsAhead: a ledger certified well above the
// block the VM actually holds.
func TestAppliedHeadReportsTheVMWhenTheLedgerIsAhead(t *testing.T) {
	const (
		ledgerHeight  = 1_000_000 // certified by consensus
		appliedHeight = 998_500   // committed by the VM
	)

	blk := &verifyOnceBlock{
		id: ids.GenerateTestID(), parentID: ids.GenerateTestID(),
		height: appliedHeight, timestamp: time.Now(), bytes: []byte("applied"),
	}
	vm := &appliedVM{
		parseVM:    parseVM{byBytes: map[string]*verifyOnceBlock{"applied": blk}},
		acceptedID: blk.id,
		blk:        blk,
	}

	e := NewWithConfig(Config{Params: params5()}, WithVM(vm))
	rt := &Runtime{Transitive: e, config: NetworkConfig{VM: vm}}

	// Certified finality sits well above what the VM holds.
	tip := ids.GenerateTestID()
	e.consensus.mu.Lock()
	e.consensus.ledger = seedLedger(tip, tip, ledgerHeight)
	e.consensus.mu.Unlock()

	if fh, set := e.consensus.GetFinalizedHeight(); !set || fh != ledgerHeight {
		t.Fatalf("ledger seeded to %d/%v, want %d", fh, set, ledgerHeight)
	}

	_, applied, err := rt.AppliedHead(context.Background())
	if err != nil {
		t.Fatalf("AppliedHead: %v", err)
	}
	if applied == ledgerHeight {
		t.Fatal("AppliedHead returned the ledger's height — catch-up would ask for blocks above the gap and never fetch what the VM is missing")
	}
	if applied != appliedHeight {
		t.Fatalf("applied=%d, want the VM's %d", applied, appliedHeight)
	}
}

// TestAppliedHeadIsZeroForAnEmptyVM: a VM holding nothing reports 0, which is below
// any ledger height — so the lower-of-the-two rule sends catch-up to the VM, which is
// correct for a freshly wiped node.
func TestAppliedHeadIsZeroForAnEmptyVM(t *testing.T) {
	vm := &appliedVM{parseVM: parseVM{byBytes: map[string]*verifyOnceBlock{}}, acceptedID: ids.Empty}
	e := NewWithConfig(Config{Params: params5()}, WithVM(vm))
	rt := &Runtime{Transitive: e, config: NetworkConfig{VM: vm}}

	_, applied, err := rt.AppliedHead(context.Background())
	if err != nil {
		t.Fatalf("AppliedHead: %v", err)
	}
	if applied != 0 {
		t.Fatalf("applied=%d, want 0 for a VM holding nothing", applied)
	}
}
