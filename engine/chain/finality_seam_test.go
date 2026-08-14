// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// finality_seam_test.go — the finality walk must cross the seam a restart leaves.
//
// The finalized tip is durable; the block tree above it is not. So a node that comes
// back up knows it finalized through height H and remembers nothing above H, while its
// VM still holds every block it ever accepted. A cert for a block above H then walks
// down looking for ancestors the node HAS but the engine has forgotten, and the walk
// fails closed on the first one — permanently, because the gap only widens.

package chain

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
)

func seamBlock(n int) ids.ID {
	var id ids.ID
	id[0], id[1] = byte(n), byte(n>>8)
	return id
}

// vmChain builds a parent-linked run of blocks the way a VM holds them after accepting.
func vmChain(from, to int) map[ids.ID]*mockBlock {
	out := map[ids.ID]*mockBlock{}
	for h := from; h <= to; h++ {
		out[seamBlock(h)] = &mockBlock{
			id:        seamBlock(h),
			parentID:  seamBlock(h - 1),
			height:    uint64(h),
			timestamp: time.Unix(int64(h), 0),
			bytes:     []byte{byte(h)},
		}
	}
	return out
}

// TestFinalityWalkCrossesTheRestartSeam: the engine remembers nothing above the
// finalized tip, the VM holds the whole run, and the walk must resolve through it.
func TestFinalityWalkCrossesTheRestartSeam(t *testing.T) {
	vm := &mockVM{blocks: vmChain(16137, 16423)}
	engine := New(WithVM(vm))

	// Exactly the post-restart state: durable tip below, empty tree above.
	if len(engine.consensus.blocks) != 0 {
		t.Fatalf("a fresh engine must start with an empty tree, has %d", len(engine.consensus.blocks))
	}

	a := engine.consensus.ancestry()
	for _, h := range []int{16137, 16200, 16422, 16423} {
		parent, height, _, _, ok := a.Parent(seamBlock(h))
		if !ok {
			t.Fatalf("height %d: walk could not resolve a block the VM holds — the cert for it would be refused for a missing ancestor", h)
		}
		if height != uint64(h) || parent != seamBlock(h-1) {
			t.Fatalf("height %d resolved to (parent=%v height=%d)", h, parent, height)
		}
	}
}

// TestFinalityWalkStillFailsClosed: reading back from the VM must not invent a block.
// One the VM does not hold still misses, so the behind-node defer is preserved.
func TestFinalityWalkStillFailsClosed(t *testing.T) {
	vm := &mockVM{blocks: vmChain(16137, 16423)}
	a := New(WithVM(vm)).consensus.ancestry()

	if _, _, _, _, ok := a.Parent(seamBlock(99999)); ok {
		t.Fatal("resolved a block the VM does not hold — the fail-closed defer is gone")
	}
}

// TestFinalityWalkWithoutAVMSeesOnlyItsOwnTree pins the pre-fix behaviour, so the
// seam-crossing above is read as the deliberate change it is rather than as always
// having worked.
func TestFinalityWalkWithoutAVMSeesOnlyItsOwnTree(t *testing.T) {
	engine := New() // no VM wired: nothing to read back from
	if _, _, _, _, ok := engine.consensus.ancestry().Parent(seamBlock(16200)); ok {
		t.Fatal("an engine with no VM must not resolve a block it never saw")
	}

	// A block this process DID see is still resolved from the live tree.
	engine.consensus.blocks[seamBlock(5)] = &Block{
		id: seamBlock(5), parentID: seamBlock(4), height: 5,
	}
	if _, height, _, _, ok := engine.consensus.ancestry().Parent(seamBlock(5)); !ok || height != 5 {
		t.Fatal("the live tree must still answer for blocks this process has seen")
	}
}

// TestLiveTreeWinsOverTheVM: the in-memory entry carries this process's own view
// (votes, wrapper aliases), so it must not be shadowed by a read-back.
func TestLiveTreeWinsOverTheVM(t *testing.T) {
	vm := &mockVM{blocks: vmChain(10, 12)}
	engine := New(WithVM(vm))
	engine.consensus.blocks[seamBlock(11)] = &Block{
		id: seamBlock(11), parentID: seamBlock(7), height: 11, // deliberately different parent
	}
	parent, _, _, _, ok := engine.consensus.ancestry().Parent(seamBlock(11))
	if !ok || parent != seamBlock(7) {
		t.Fatalf("live tree must win; got parent=%v ok=%v", parent, ok)
	}
}
