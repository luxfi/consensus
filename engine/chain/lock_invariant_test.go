// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// lock_invariant_test.go — the UNLOCK-BEFORE-CALL-OUT invariant, enforced as a runtime test.
//
// Every call-out surface (VM.BuildBlock/GetBlock/LastAccepted/ParseBlock/SetPreference,
// block.Accept/Verify/Reject, and every Gossiper network send) is wrapped so that, WHEN it runs,
// it asserts the engine write lock is NOT held by the calling goroutine. The engine is then driven
// through its real build→propose→finalize flow (K==1 and K>1) via NewRuntime, so if any code path
// invokes a call-out while holding Transitive.mu the test fails immediately. This is the checked
// form of the invariant documented at Transitive.mu — the ban on the self-deadlock class, not just
// the one selfVoter instance.

type lockDetector struct {
	eng        *Transitive
	violations int32
	t          *testing.T
}

func (d *lockDetector) check(where string) {
	if d.eng != nil && d.eng.mu.heldForWriteByCurrentGoroutine() {
		atomic.AddInt32(&d.violations, 1)
		d.t.Errorf("CALL-OUT UNDER t.mu: %q ran while the engine WRITE lock was held by the same "+
			"goroutine — this is the reentrant-deadlock class (unlock before every call-out)", where)
	}
}

// instrBlock wraps a trackingMockBlock and checks the lock invariant on Accept/Verify/Reject.
type instrBlock struct {
	*trackingMockBlock
	d *lockDetector
}

func (b *instrBlock) Accept(ctx context.Context) error {
	b.d.check("block.Accept")
	return b.trackingMockBlock.Accept(ctx)
}
func (b *instrBlock) Verify(ctx context.Context) error {
	b.d.check("block.Verify")
	return b.trackingMockBlock.Verify(ctx)
}
func (b *instrBlock) Reject(ctx context.Context) error {
	b.d.check("block.Reject")
	return b.trackingMockBlock.Reject(ctx)
}

// instrVM wraps a trackingMockVM and checks the invariant on every VM call-out, returning
// instrumented blocks so block-level call-outs are covered too.
type instrVM struct {
	*trackingMockVM
	d *lockDetector
}

func (m *instrVM) BuildBlock(ctx context.Context) (block.Block, error) {
	m.d.check("VM.BuildBlock")
	blk, err := m.trackingMockVM.BuildBlock(ctx)
	if tb, ok := blk.(*trackingMockBlock); ok {
		return &instrBlock{trackingMockBlock: tb, d: m.d}, err
	}
	return blk, err
}
func (m *instrVM) GetBlock(ctx context.Context, id ids.ID) (block.Block, error) {
	m.d.check("VM.GetBlock")
	blk, err := m.trackingMockVM.GetBlock(ctx, id)
	if tb, ok := blk.(*trackingMockBlock); ok {
		return &instrBlock{trackingMockBlock: tb, d: m.d}, err
	}
	return blk, err
}
func (m *instrVM) ParseBlock(ctx context.Context, b []byte) (block.Block, error) {
	m.d.check("VM.ParseBlock")
	return m.trackingMockVM.ParseBlock(ctx, b)
}
func (m *instrVM) LastAccepted(ctx context.Context) (ids.ID, error) {
	m.d.check("VM.LastAccepted")
	return m.trackingMockVM.LastAccepted(ctx)
}
func (m *instrVM) SetPreference(ctx context.Context, id ids.ID) error {
	m.d.check("VM.SetPreference")
	return m.trackingMockVM.SetPreference(ctx, id)
}

// instrGossiper wraps recordingGossiper and checks the invariant on every network send.
type instrGossiper struct {
	*recordingGossiper
	d *lockDetector
}

func (g *instrGossiper) GossipPut(c, n ids.ID, b []byte) int {
	g.d.check("Gossiper.GossipPut")
	return g.recordingGossiper.GossipPut(c, n, b)
}
func (g *instrGossiper) SendPushQuery(c, n ids.ID, b []byte, v []ids.NodeID) int {
	g.d.check("Gossiper.SendPushQuery")
	return g.recordingGossiper.SendPushQuery(c, n, b, v)
}
func (g *instrGossiper) SendPullQuery(c, n, blk ids.ID, v []ids.NodeID) int {
	g.d.check("Gossiper.SendPullQuery")
	return g.recordingGossiper.SendPullQuery(c, n, blk, v)
}
func (g *instrGossiper) GossipCert(c ids.ID, blk ids.ID, cert []byte) error {
	g.d.check("Gossiper.GossipCert")
	return g.recordingGossiper.GossipCert(c, blk, cert)
}

// TestBlue_NoCallOutUnderEngineLock drives the real NewRuntime build/propose/finalize flow with
// every call-out surface instrumented, and asserts NONE runs while the engine write lock is held —
// for both a single-validator (K==1, inline finalize) and a multi-validator (K>1, Propose +
// RequestVotes + SetPreference) engine.
func TestBlue_NoCallOutUnderEngineLock(t *testing.T) {
	lockInstrumentEnabled.Store(true)
	t.Cleanup(func() { lockInstrumentEnabled.Store(false) })

	run := func(name string, k int, sampler ValidatorSampler, verifier VoteVerifier, signer VoteSigner, nBlocks int) {
		t.Run(name, func(t *testing.T) {
			self := ids.GenerateTestNodeID()
			d := &lockDetector{t: t}
			blocks := make([]*trackingMockBlock, nBlocks)
			var parent ids.ID
			for i := 0; i < nBlocks; i++ {
				blocks[i] = &trackingMockBlock{id: ids.GenerateTestID(), parentID: parent, height: uint64(i + 1), timestamp: time.Now(), bytes: []byte{byte(i)}}
				parent = blocks[i].id
			}
			p := config.SingleValidatorParams()
			p.K, p.AlphaPreference, p.AlphaConfidence = k, maxInt(1, 2*k/3+1), maxInt(1, 2*k/3+1)
			cfg := NetworkConfig{
				ChainID: ids.GenerateTestID(), NetworkID: ids.GenerateTestID(), NodeID: self,
				Validators: sampler, Logger: log.Noop(),
				Gossiper: &instrGossiper{recordingGossiper: &recordingGossiper{}, d: d},
				VM:       &instrVM{trackingMockVM: &trackingMockVM{blocks: blocks}, d: d},
				Params:   &p, VoteVerifier: verifier, VoteSigner: signer,
			}
			rt := NewRuntime(cfg)
			d.eng = rt.Transitive
			ctx := context.Background()
			if err := rt.Start(ctx, true); err != nil {
				t.Fatalf("start: %v", err)
			}
			t.Cleanup(func() { _ = rt.Stop(context.Background()) })
			for i := 0; i < nBlocks; i++ {
				if err := rt.Notify(ctx, Message{Type: PendingTxs}); err != nil {
					t.Fatalf("notify: %v", err)
				}
				time.Sleep(120 * time.Millisecond)
			}
			if v := atomic.LoadInt32(&d.violations); v != 0 {
				t.Fatalf("%d call-out(s) ran under the engine write lock", v)
			}
		})
	}

	self1 := ids.GenerateTestNodeID()
	// K==1: single-validator inline finalize (Propose + block.Accept + VM reads).
	run("single-validator", 1, singleSelfSampler{self: self1}, rejectingVerifier{}, testAuth.signerFor(self1), 2)
	// K>1: exercises Propose + RequestVotes + SetPreference + finalizeOwnProposal call-outs.
	selfM := ids.GenerateTestNodeID()
	run("multi-validator", 3, &multiValidatorSampler{nodeID: selfM, peers: []ids.NodeID{ids.GenerateTestNodeID(), ids.GenerateTestNodeID()}}, testAuth, testAuth.signerFor(selfM), 2)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
