// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// bootstrapper_security_test.go — the C1 forged-chain gate at the LOOP layer.
//
// Frontier naming (a forged FRONTIER can never be named) is enforced node-side by the
// beacon + α-weighted-stake quorum on FrontierTip (see node chains/bootstrap_sync.go and
// its tests). This file proves the OTHER half: even under an HONEST, α-agreed frontier, a
// malicious Ancestors peer cannot inject a forged-but-Verify-passing sidechain during the
// descent. The content-addressed descent walks the parent chain DOWN from the agreed tip,
// buffering only blocks whose id is on that path — so a forged block (whose id is not on
// the agreed tip's parent chain) is ignored, and a peer that does not serve the requested
// block is abandoned. The executed chain therefore provably REACHES the α-agreed frontier.
package bootstrap

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// forgedChainOf builds a second VALID chain that shares `genesis` but diverges into
// distinct block ids at every height 1..n (the attacker's "forged but Verify-passing"
// chain from the same genesis). Every block is marked valid (it re-executes cleanly) and
// is contiguously parent-linked, so the ONLY thing that can stop it being finalized is the
// content-addressed descent — exactly the property under test.
func forgedChainOf(genesis *tBlock, n int) ([]*tBlock, map[string]*tBlock) {
	blocks := []*tBlock{genesis}
	reg := map[string]*tBlock{}
	parent := genesis.id
	for h := 1; h <= n; h++ {
		blk := &tBlock{
			id:     ids.GenerateTestID(),
			parent: parent,
			height: uint64(h),
			bytes:  []byte("FORGED@" + strconv.Itoa(h) + ":" + ids.GenerateTestID().String()),
			valid:  true,
		}
		reg[string(blk.bytes)] = blk
		blocks = append(blocks, blk)
		parent = blk.id
	}
	return blocks, reg
}

// maliciousAncestrySource advertises the HONEST (α-agreed) frontier tip but, when asked
// for ancestry, ALWAYS serves its forged chain (ignoring the requested id) — the strongest
// model of a peer trying to substitute a forged sidechain during the descent.
type maliciousAncestrySource struct {
	honestTip ids.ID
	forged    []*tBlock // oldest-first; forged[0] is the shared genesis
}

func (m *maliciousAncestrySource) FrontierTip(context.Context) (ids.ID, bool) {
	return m.honestTip, true
}

func (m *maliciousAncestrySource) Ancestors(_ context.Context, _ ids.ID, maxBlocks int) ([][]byte, error) {
	// Ignore the requested id — serve the top `maxBlocks` of the forged chain, oldest-first.
	n := len(m.forged)
	start := 0
	if n > maxBlocks {
		start = n - maxBlocks
	}
	out := make([][]byte, 0, n-start)
	for i := start; i < n; i++ {
		out = append(out, m.forged[i].bytes)
	}
	return out, nil
}

// mergeRegs lets the node PARSE both the honest and forged block bytes (a forged block is
// a well-formed block — it parses; what disqualifies it is being off the agreed tip's
// parent chain, which is precisely what we are testing).
func mergeRegs(regs ...map[string]*tBlock) map[string]*tBlock {
	out := map[string]*tBlock{}
	for _, r := range regs {
		for k, v := range r {
			out[k] = v
		}
	}
	return out
}

// TestRED_ForgedAncestryUnderHonestFrontierRejected is the loop-layer C1 proof.
//
// THE ATTACK (red, ported to the loop): a peer serves a SECOND valid chain from the same
// genesis during the descent. The pre-fix descent buffered every served block by height
// and executed from lastH+1, so the forged chain Verify-passed + was contiguous-to-genesis
// and got FINALIZED (red's PoC finalized 40 forged blocks). THE FIX: content-addressed
// descent. Asking for the honest tip, the forged batch does not contain it, so the walk
// finds nothing on-path and the pass is abandoned. With only the malicious peer reachable
// the loop stalls — and ZERO forged blocks are finalized. The node stays at genesis rather
// than bricking on a forged chain.
func TestRED_ForgedAncestryUnderHonestFrontierRejected(t *testing.T) {
	const N = 40
	honest, honestReg := chainOf(N, 0)
	genesis := honest[0]
	forged, forgedReg := forgedChainOf(genesis, N)

	// The node can parse BOTH chains and starts empty (only genesis accepted).
	node := newTestNode(mergeRegs(honestReg, forgedReg), genesis)

	src := &maliciousAncestrySource{honestTip: honest[N].id, forged: forged}
	bs := New(Config{Source: src, Chain: node, RetryInterval: time.Millisecond, Log: log.NewNoOpLogger()})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := bs.Run(ctx)

	// The loop cannot make progress against a peer that will only serve off-path (forged)
	// ancestry for the honest tip — it stalls instead of finalizing the forgery.
	if err != ErrStalled {
		t.Fatalf("expected ErrStalled (forged ancestry refused, no honest peer), got %v", err)
	}
	if node.accepts != 0 {
		t.Fatalf("C1: %d forged blocks were finalized — content-addressed descent FAILED (want 0)", node.accepts)
	}
	if node.height != 0 {
		t.Fatalf("C1: node advanced to height %d on a forged chain (want genesis, height 0)", node.height)
	}
	// And it certainly did not adopt the forged tip.
	if node.Has(ctx, forged[N].id) {
		t.Fatalf("C1: node finalized the forged tip — bricked against the canonical chain")
	}
}

// honestThenForgedSource serves the HONEST ancestry for ids on the honest chain (so an
// honest descent succeeds) but ALSO would serve forged blocks if asked for forged ids.
// Used to prove that once the frontier is honest, the whole synced chain is honest.
type dualSource struct {
	honestTip ids.ID
	byID      map[ids.ID]*tBlock // honest ∪ forged, for honest-style ancestry serving
}

func (d *dualSource) FrontierTip(context.Context) (ids.ID, bool) { return d.honestTip, true }
func (d *dualSource) Ancestors(_ context.Context, blockID ids.ID, maxBlocks int) ([][]byte, error) {
	tip, ok := d.byID[blockID]
	if !ok {
		return nil, nil
	}
	var rev [][]byte
	cur := tip
	for i := 0; i < maxBlocks; i++ {
		rev = append(rev, cur.bytes)
		if cur.parent == ids.Empty {
			break
		}
		cur = d.byID[cur.parent]
		if cur == nil {
			break
		}
	}
	out := make([][]byte, len(rev))
	for i := range rev {
		out[len(rev)-1-i] = rev[i]
	}
	return out, nil
}

// TestRED_HonestFrontierSyncsOnlyCanonicalChain: with an HONEST α-agreed frontier and a
// content-addressed source, the node syncs the canonical chain end to end (the fix does
// not regress the honest path), and never touches any forged block even though it could
// parse one.
func TestRED_HonestFrontierSyncsOnlyCanonicalChain(t *testing.T) {
	const N = 60
	honest, honestReg := chainOf(N, 0)
	genesis := honest[0]
	forged, forgedReg := forgedChainOf(genesis, N)

	byID := map[ids.ID]*tBlock{}
	for _, b := range honest {
		byID[b.id] = b
	}
	for _, b := range forged {
		byID[b.id] = b
	}
	node := newTestNode(mergeRegs(honestReg, forgedReg), genesis)
	src := &dualSource{honestTip: honest[N].id, byID: byID}

	bs := New(Config{Source: src, Chain: node, RetryInterval: time.Millisecond, Log: log.NewNoOpLogger()})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bs.Run(ctx); err != nil {
		t.Fatalf("honest content-addressed sync should converge, got %v", err)
	}
	if node.height != N || node.tipID != honest[N].id {
		t.Fatalf("node must reach the canonical tip (height %d), got height %d", N, node.height)
	}
	for _, f := range forged[1:] {
		if node.Has(ctx, f.id) {
			t.Fatalf("node finalized a forged block %s — must only adopt the canonical chain", f.id)
		}
	}
}

// TestWeakSubjectivity_RefusesFrontierNotDescendingFromCheckpoint: an operator pins a
// recent finalized checkpoint; a frontier on a chain that does NOT pass through the
// checkpoint at its height is refused (defense-in-depth for the empty-genesis case).
func TestWeakSubjectivity_RefusesFrontierNotDescendingFromCheckpoint(t *testing.T) {
	const N = 80
	honest, honestReg := chainOf(N, 0)
	genesis := honest[0]
	// A DIFFERENT valid chain from the same genesis (the "wrong" fork the operator wants
	// to exclude). Serve it as the frontier; pin a checkpoint on the HONEST chain.
	forged, forgedReg := forgedChainOf(genesis, N)

	node := newTestNode(mergeRegs(honestReg, forgedReg), genesis)
	byID := map[ids.ID]*tBlock{}
	for _, b := range forged {
		byID[b.id] = b
	}
	src := &dualSource{honestTip: forged[N].id, byID: byID}

	// Pin a checkpoint at height 50 on the HONEST chain. The forged frontier does not pass
	// through it, so the descent must refuse the frontier.
	ckpt := honest[50]
	bs := New(Config{
		Source: src, Chain: node, RetryInterval: time.Millisecond, Log: log.NewNoOpLogger(),
		WeakSubjectivityID: ckpt.id, WeakSubjectivityHeight: 50,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := bs.Run(ctx)
	if err != ErrFrontierNotDescendedFromCheckpoint {
		t.Fatalf("expected ErrFrontierNotDescendedFromCheckpoint, got %v", err)
	}
	if node.accepts != 0 {
		t.Fatalf("weak-subjectivity: %d blocks finalized off a non-descending frontier (want 0)", node.accepts)
	}
}

// TestWeakSubjectivity_AcceptsFrontierDescendingFromCheckpoint: the same checkpoint, but
// now the frontier IS on the honest chain that passes through the checkpoint — sync
// proceeds normally.
func TestWeakSubjectivity_AcceptsFrontierDescendingFromCheckpoint(t *testing.T) {
	const N = 80
	honest, honestReg := chainOf(N, 0)
	genesis := honest[0]
	byID := map[ids.ID]*tBlock{}
	for _, b := range honest {
		byID[b.id] = b
	}
	node := newTestNode(honestReg, genesis)
	src := &dualSource{honestTip: honest[N].id, byID: byID}

	ckpt := honest[50]
	bs := New(Config{
		Source: src, Chain: node, RetryInterval: time.Millisecond, Log: log.NewNoOpLogger(),
		WeakSubjectivityID: ckpt.id, WeakSubjectivityHeight: 50,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bs.Run(ctx); err != nil {
		t.Fatalf("frontier descending from the checkpoint must sync, got %v", err)
	}
	if node.height != N {
		t.Fatalf("node must reach the canonical tip (height %d), got %d", N, node.height)
	}
}

// TestMaxBufferBytes_BoundsDescentMemory (M3): a peer streaming oversized blocks during a
// deep descent hits the BYTE budget (not just the block-count budget) and the pass fails
// ErrGapTooLarge rather than exhausting memory.
func TestMaxBufferBytes_BoundsDescentMemory(t *testing.T) {
	const N = 4000 // > one fetch window, forces a multi-round descent
	chain, reg := chainOf(N, 0)
	// Make every block ~64 KiB so a few hundred buffered blocks exceed a tight byte budget
	// well before the 64Ki block-count budget.
	big := make([]byte, 64*1024)
	for i := range chain {
		chain[i].bytes = append(append([]byte("big@"+strconv.Itoa(i)+":"), big...), chain[i].id[:]...)
		reg[string(chain[i].bytes)] = chain[i]
	}
	peer := newTestPeer(chain)
	node := newTestNode(reg, chain[0])

	bs := New(Config{
		Source: peer, Chain: node, RetryInterval: time.Millisecond, Log: log.NewNoOpLogger(),
		MaxBufferBytes: 8 * 1024 * 1024, // 8 MiB budget — far below N*64KiB
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := bs.Run(ctx)
	if err != ErrGapTooLarge {
		t.Fatalf("expected ErrGapTooLarge from the byte budget, got %v", err)
	}
}

// TestRED_DeepGapFailsSafe_NoFalseComplete: an EMPTY node whose gap to the frontier
// exceeds the in-memory window does NOT converge via this path — it returns
// ErrGapTooLarge and finalizes NOTHING, so the caller leaves it un-bootstrapped (it must
// state-sync to within the window first). The safety-critical property is "no
// FALSE-complete": the node never declares itself synced at a partial height. This is the
// genesis→1.08M mainnet case (gap >> 64Ki); the realistic first canary is the STALE node
// (small gap), proven by TestLoop_PartialNodeConverges.
func TestRED_DeepGapFailsSafe_NoFalseComplete(t *testing.T) {
	const N = 600
	chain, reg := chainOf(N, 0)
	peer := newTestPeer(chain)
	node := newTestNode(reg, chain[0]) // empty: only genesis

	// A tight window (20 blocks/fetch, 50-block buffer) models a gap far larger than the
	// node can hold in memory.
	bs := New(Config{
		Source: peer, Chain: node, RetryInterval: time.Millisecond, Log: log.NewNoOpLogger(),
		MaxBlocksPerFetch: 20, MaxBuffer: 50,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := bs.Run(ctx)
	if err != ErrGapTooLarge {
		t.Fatalf("deep gap must fail safe with ErrGapTooLarge, got %v", err)
	}
	if node.accepts != 0 || node.height != 0 {
		t.Fatalf("FALSE-COMPLETE: a too-deep gap advanced the node (height %d, %d accepts) — must finalize nothing", node.height, node.accepts)
	}
}
