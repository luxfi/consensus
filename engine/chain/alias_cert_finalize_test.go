// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// alias_cert_finalize_test.go — the STORM-ALIAS finalize gap: the 2026-07 mainnet
// C-Chain freeze where consensus finalizes an outer envelope (topology.go logs
// "finalized block via α-of-K quorum cert voters=4") but the local EVM never
// Accepts, so the head stays frozen at the parent height (1085755) with the inner
// block for the next height built-and-verified-but-unaccepted.
//
// ROOT (engine terms): under pChainHeight=0 anyone-can-propose, every validator
// wraps the SAME inner execution block in its OWN outer proposervm envelope. Votes
// are canonical-keyed (they bind the inner commitment, not the envelope), so an
// α-of-K cert forms — but its Position.BlockID names ONE specific envelope. A node
// that holds a DIFFERENT alias envelope of that same inner block received the cert
// and HandleIncomingCert did a STRICT pendingBlocks[cert.Position.BlockID] lookup,
// MISSED, and requestCatchup'd for an envelope it does not need — instead of
// finalizing the local wrapper of the SAME inner block it already holds and
// verified. The α-of-K cert exists network-wide, yet the local VM.Accept is never
// called → the EVM head is frozen while consensus says the height is final.
//
// THE FIX (proven here): on the envelope-miss path, resolve the LOCAL wrapper of the
// cert's CANONICAL id (pendingByCanonicalLocked) and finalize THAT, rebasing the
// verified cert onto the local envelope. The outer ids are NOT in the signed message
// (CanonicalVoteMessage binds the canonical execution identity only), so the α votes
// still verify against the local wrapper's position — one certified inner block, any
// wrapper of it Accepts the identical execution. No fork; the cert is still required.

package chain

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/consensus/core/slashing"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// innerVM models the inner EXECUTION VM's accepted head — the luxfi/evm (coreth-lineage)
// lastAccepted the proposervm cascades into via postForkBlock.Accept → acceptInnerBlk →
// Tree.Accept(innerBlk). Its head advances ONLY when a wrapping outer block's Accept is
// called, which is exactly the propagation the EVM-accept-gap fix restores.
type innerVM struct {
	lastAcceptedHeight uint64
	lastAcceptedID     ids.ID
}

// innerHeadBlock is a VMBlock whose Accept advances the shared inner VM head to this
// block's inner id + height (mirroring the proposervm inner-accept cascade). Before Accept
// the inner block "exists" (verified) but the head lags at the parent height — the exact
// 1085755 symptom (block built+verified, not accepted, head frozen at the parent).
type innerHeadBlock struct {
	outerID  ids.ID
	parentID ids.ID
	innerID  ids.ID
	height   uint64
	vm       *innerVM
	accepts  int64
}

func (b *innerHeadBlock) ID() ids.ID           { return b.outerID }
func (b *innerHeadBlock) Parent() ids.ID       { return b.parentID }
func (b *innerHeadBlock) ParentID() ids.ID     { return b.parentID }
func (b *innerHeadBlock) Height() uint64       { return b.height }
func (b *innerHeadBlock) Timestamp() time.Time { return time.Unix(0, 0) }
func (b *innerHeadBlock) Status() uint8        { return 0 }
func (b *innerHeadBlock) Verify(context.Context) error { return nil }
func (b *innerHeadBlock) Accept(context.Context) error {
	atomic.AddInt64(&b.accepts, 1)
	b.vm.lastAcceptedHeight = b.height // proposervm acceptInnerBlk → inner EVM head advances
	b.vm.lastAcceptedID = b.innerID
	return nil
}
func (b *innerHeadBlock) Reject(context.Context) error { return nil }
func (b *innerHeadBlock) Bytes() []byte                { return b.outerID[:] }

// trackEnvelopeInnerVM tracks a verified outer wrapper whose VMBlock advances a shared
// inner VM head on Accept — so a test can observe the inner lastAccepted move (or not).
func trackEnvelopeInnerVM(rt *Runtime, vm *innerVM, outerID, parentOuter, innerID, parentInner ids.ID, height uint64, round uint32) *innerHeadBlock {
	blk := &innerHeadBlock{outerID: outerID, parentID: parentOuter, innerID: innerID, height: height, vm: vm}
	cb := &Block{
		id:                outerID,
		parentID:          parentOuter,
		height:            height,
		canonicalID:       innerID,
		parentCanonicalID: parentInner,
	}
	_ = rt.Transitive.consensus.AddBlock(context.Background(), cb)
	rt.Transitive.mu.Lock()
	rt.Transitive.pendingBlocks[outerID] = &PendingBlock{ConsensusBlock: cb, VMBlock: blk, ProposedAt: time.Now(), Round: round}
	rt.Transitive.mu.Unlock()
	return blk
}

// TestEVMAcceptGap_InnerHeadAdvancesOnSiblingCert is the explicit #3 regression: the outer
// layer is network-final (a valid α-of-K cert exists for a SIBLING wrapper of the inner
// block), the inner block is built+verified-but-unaccepted, and the inner VM head LAGS. The
// fix must propagate Accept so the inner head ADVANCES — otherwise the EVM is frozen while
// the height is final (the mainnet 1085755→1085756 freeze). Fails on old code (head stays
// lagging); passes on the fix.
func TestEVMAcceptGap_InnerHeadAdvancesOnSiblingCert(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	follower := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(4), vs, &recordingGossiper{}, vs.signerFor(4)))
	if err := follower.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = follower.Stop(context.Background()) })
	rt := &Runtime{Transitive: follower, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	// The inner VM head lags at the parent height (the "1085755" analog): the inner block for
	// the next height exists (verified) but is NOT yet accepted.
	const H = uint64(1)
	vm := &innerVM{lastAcceptedHeight: H - 1, lastAcceptedID: ids.Empty}

	innerC := ids.GenerateTestID()     // inner execution block at height H (built+verified, unaccepted)
	localOuter := ids.GenerateTestID() // our proposervm wrapper of innerC
	peerOuter := ids.GenerateTestID()  // a sibling wrapper — the cert names THIS

	local := trackEnvelopeInnerVM(rt, vm, localOuter, ids.Empty, innerC, ids.Empty, H, 0)

	if vm.lastAcceptedHeight != H-1 {
		t.Fatalf("precondition: inner head must lag at %d, got %d", H-1, vm.lastAcceptedHeight)
	}

	// A valid 4-of-5 cert forms over the SIBLING wrapper — canonical innerC at height H.
	certPeer := canonicalCert(t, vs, chainID, peerOuter, ids.Empty, innerC, ids.Empty, H, 0, 4)
	if !rt.HandleIncomingCert(certPeer) {
		t.Fatal("EVM-ACCEPT GAP: a sibling-wrapper cert did not finalize the local wrapper")
	}

	// THE PROOF: the fix propagated Accept, so the inner VM head ADVANCED H-1 → H.
	if vm.lastAcceptedHeight != H {
		t.Fatalf("inner VM head must advance to %d after finalize, still at %d (the EVM freeze)",
			H, vm.lastAcceptedHeight)
	}
	if vm.lastAcceptedID != innerC {
		t.Fatalf("inner VM lastAccepted must be the certified inner block %s, got %s", innerC, vm.lastAcceptedID)
	}
	if got := atomic.LoadInt64(&local.accepts); got != 1 {
		t.Fatalf("local wrapper Accept must be called exactly once, got %d", got)
	}
}

// TestStormAlias_PeerCertFinalizesLocalWrapper_EVMAccepts is the fail-without /
// pass-with. A 4-of-5 cert forms over a PEER's wrapper of inner block C; this node
// holds only its OWN (different) wrapper of C. The cert MUST finalize the local
// wrapper AND call its VM.Accept — otherwise the local EVM is frozen while the
// height is network-final (the exact 1085755 symptom).
func TestStormAlias_PeerCertFinalizesLocalWrapper_EVMAccepts(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	db := slashing.NewDB(0)

	follower := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(4), vs, &recordingGossiper{}, vs.signerFor(4)),
		WithSlashing(slashing.NewDetector(64, 0.5), db))
	if err := follower.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = follower.Stop(context.Background()) })
	rt := &Runtime{Transitive: follower, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	innerC := ids.GenerateTestID()      // the shared inner execution block at height 1
	localOuter := ids.GenerateTestID()  // THIS node's proposervm wrapper of innerC
	peerOuter := ids.GenerateTestID()   // a DIFFERENT validator's wrapper of innerC
	const H = uint64(1)

	// This node tracks ONLY its own wrapper. It has verified innerC (blkLocal) but
	// never received the peer's envelope — exactly the storm arrangement.
	blkLocal := trackEnvelope(rt, localOuter, ids.Empty, innerC, ids.Empty, H, 0)

	// Sanity: the peer's envelope is genuinely NOT tracked locally.
	rt.Transitive.mu.RLock()
	_, tracksPeer := rt.Transitive.pendingBlocks[peerOuter]
	rt.Transitive.mu.RUnlock()
	if tracksPeer {
		t.Fatal("precondition: this node must NOT track the peer envelope")
	}

	// A valid α-of-K (4-of-5) finality cert over the PEER's wrapper — canonical innerC.
	certPeer := canonicalCert(t, vs, chainID, peerOuter, ids.Empty, innerC, ids.Empty, H, 0, 4)

	finalized := rt.HandleIncomingCert(certPeer)

	if !finalized {
		t.Fatal("STORM-ALIAS GAP: a valid 4-of-5 cert for a sibling wrapper of the SAME inner " +
			"block did not finalize the locally-held wrapper — the local EVM stays frozen at the " +
			"parent height while the height is network-final (the 1085755 freeze)")
	}
	// Finality is keyed on the CANONICAL inner commitment.
	if got, ok := follower.consensus.FinalizedBlockAtHeight(H); !ok || got != innerC {
		t.Fatalf("height %d must finalize to canonical %s, got (%s, ok=%v)", H, innerC, got, ok)
	}
	// THE EVM-ACCEPT PROOF: the local wrapper's VM.Accept must have been called exactly
	// once, which cascades to the inner EVM block's Accept (proposervm acceptInnerBlk).
	if got := blkLocal.AcceptCalled(); got != 1 {
		t.Fatalf("local wrapper VM.Accept must be called exactly once so the EVM advances, got %d", got)
	}
}

// TestStormAlias_NoLocalWrapper_FallsBackToCatchup proves the fix does NOT
// over-reach: a node that holds NO wrapper of the certified inner block (genuinely
// behind) must NOT fabricate a finalize — it defers to catch-up (returns false, no
// Accept), exactly as before.
func TestStormAlias_NoLocalWrapper_FallsBackToCatchup(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()

	follower := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(4), vs, &recordingGossiper{}, vs.signerFor(4)))
	if err := follower.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = follower.Stop(context.Background()) })
	rt := &Runtime{Transitive: follower, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	innerC := ids.GenerateTestID()
	peerOuter := ids.GenerateTestID()
	const H = uint64(1)

	// Track NOTHING at height 1. The cert's inner block is unknown locally.
	certPeer := canonicalCert(t, vs, chainID, peerOuter, ids.Empty, innerC, ids.Empty, H, 0, 4)

	if finalized := rt.HandleIncomingCert(certPeer); finalized {
		t.Fatal("a node holding NO wrapper of the certified inner block must NOT finalize " +
			"(it has verified nothing at this height) — it must defer to catch-up")
	}
	if _, ok := follower.consensus.FinalizedBlockAtHeight(H); ok {
		t.Fatal("nothing must be finalized when no local wrapper of the canonical exists")
	}
}
