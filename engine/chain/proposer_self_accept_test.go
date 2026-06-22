// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// proposer_self_accept_test.go — proposer finalization semantics under the
// α-of-K quorum-cert model.
//
// HISTORY: this file used to assert the OPPOSITE of correctness. It encoded the
// "proposer-self-accept" workaround as desired behavior: a proposer finalizing
// its own block on its LONE self-vote with ZERO peer Chits
// (TestProposerSelfAccept_FinalizesWithoutPeerChits), and peer REJECT votes for
// the proposer's own block being laundered into ACCEPTs
// (TestProposerSelfAccept_PeerVotesAcceptedDespiteLocalReVerifyFailure). Those
// were the self-finality fork hole. They are REPLACED here with tests that
// assert the proposer does NOT finalize without an α-of-K quorum, and that the
// freeze the workaround "fixed" is instead solved by the cert path (see
// quorum_finality_test.go TestLiveness_*). The shared VM/block helpers remain.
package chain

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// verifyOnceBlock implements block.Block such that Verify succeeds the first
// time and fails on subsequent calls — modelling production VMs (PlatformVM's
// CreateChainTx path) that reject a double Verify. Under the OLD design this
// double-Verify-failure was the reason the proposer flipped peer accepts to
// rejects (and why the IsOwnProposal flip was bolted on). Under the cert model
// the proposer NEVER re-Verifies a peer's vote — it verifies the peer's
// SIGNATURE over the peer's ACCEPT decision — so this VM no longer poisons
// finalization.
type verifyOnceBlock struct {
	id           ids.ID
	parentID     ids.ID
	height       uint64
	timestamp    time.Time
	bytes        []byte
	verifyCount  int64
	acceptCalled int64
	rejectCalled int64
}

func (b *verifyOnceBlock) ID() ids.ID           { return b.id }
func (b *verifyOnceBlock) Parent() ids.ID       { return b.parentID }
func (b *verifyOnceBlock) ParentID() ids.ID     { return b.parentID }
func (b *verifyOnceBlock) Height() uint64       { return b.height }
func (b *verifyOnceBlock) Timestamp() time.Time { return b.timestamp }
func (b *verifyOnceBlock) Status() uint8        { return 0 }
func (b *verifyOnceBlock) Verify(context.Context) error {
	n := atomic.AddInt64(&b.verifyCount, 1)
	if n > 1 {
		return errVerifiedAlready
	}
	return nil
}
func (b *verifyOnceBlock) Accept(context.Context) error {
	atomic.AddInt64(&b.acceptCalled, 1)
	return nil
}
func (b *verifyOnceBlock) Reject(context.Context) error {
	atomic.AddInt64(&b.rejectCalled, 1)
	return nil
}
func (b *verifyOnceBlock) Bytes() []byte       { return b.bytes }
func (b *verifyOnceBlock) AcceptCalled() int64 { return atomic.LoadInt64(&b.acceptCalled) }
func (b *verifyOnceBlock) RejectCalled() int64 { return atomic.LoadInt64(&b.rejectCalled) }
func (b *verifyOnceBlock) VerifyCalls() int64  { return atomic.LoadInt64(&b.verifyCount) }

var errVerifiedAlready = errVerifiedAlreadyT{}

type errVerifiedAlreadyT struct{}

func (errVerifiedAlreadyT) Error() string { return "block already verified" }

type verifyOnceVM struct {
	blocks         []*verifyOnceBlock
	buildBlockIdx  int
	lastAcceptedID ids.ID
}

func (m *verifyOnceVM) BuildBlock(ctx context.Context) (block.Block, error) {
	if m.buildBlockIdx >= len(m.blocks) {
		return nil, errVerifiedAlready
	}
	blk := m.blocks[m.buildBlockIdx]
	m.buildBlockIdx++
	return blk, nil
}

func (m *verifyOnceVM) GetBlock(_ context.Context, id ids.ID) (block.Block, error) {
	for _, blk := range m.blocks {
		if blk.id == id {
			return blk, nil
		}
	}
	return nil, errVerifiedAlready
}

func (m *verifyOnceVM) ParseBlock(_ context.Context, bytes []byte) (block.Block, error) {
	return &verifyOnceBlock{bytes: bytes}, nil
}

func (m *verifyOnceVM) LastAccepted(_ context.Context) (ids.ID, error) {
	return m.lastAcceptedID, nil
}

func (m *verifyOnceVM) SetPreference(_ context.Context, id ids.ID) error {
	m.lastAcceptedID = id
	return nil
}

// recordingProposer captures Propose/RequestVotes without a real network layer.
type recordingProposer struct {
	proposeCalled      int64
	requestVotesCalled int64
}

func (p *recordingProposer) Propose(_ context.Context, _ BlockProposal) error {
	atomic.AddInt64(&p.proposeCalled, 1)
	return nil
}

func (p *recordingProposer) RequestVotes(_ context.Context, _ VoteRequest) error {
	atomic.AddInt64(&p.requestVotesCalled, 1)
	return nil
}

func (p *recordingProposer) ProposeCalls() int64 { return atomic.LoadInt64(&p.proposeCalled) }

// TestProposer_DoesNotFinalizeOwnProposalWithoutQuorum is the INVERSE of the old
// TestProposerSelfAccept_FinalizesWithoutPeerChits: a proposer with ZERO peer
// votes must NOT finalize its own block. It stays pending (recoverable), never
// force-accepted. This is the core self-finality fix.
func TestProposer_DoesNotFinalizeOwnProposalWithoutQuorum(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngine(t, params5(), vs, 0, &recordingGossiper{})

	blk := newTestBlock(1, ids.Empty, "no-chits")
	_ = trackProposal(e, chainID, blk, 0)

	// Proposer attempts to self-finalize with only its own vote — refused.
	e.finalizeOwnProposal(context.Background(), blk.id)

	if e.IsAccepted(blk.id) {
		t.Fatal("SAFETY: proposer finalized own block without an alpha-of-K quorum")
	}
	if blk.AcceptCalled() != 0 {
		t.Fatalf("VM.Accept must not run without quorum, got %d", blk.AcceptCalled())
	}
	e.mu.RLock()
	_, stillPending := e.pendingBlocks[blk.id]
	e.mu.RUnlock()
	if !stillPending {
		t.Fatal("LIVENESS: a sub-quorum own block must remain pending, not be dropped")
	}
}

// TestProposer_RejectVotesNeverFinalizeOwnProposal is the INVERSE of the old
// TestProposerSelfAccept_PeerVotesAcceptedDespiteLocalReVerifyFailure: peers
// voting REJECT on the proposer's own block must NOT finalize it (the
// effectiveAccept = ... || IsOwnProposal flip is deleted).
func TestProposer_RejectVotesNeverFinalizeOwnProposal(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngine(t, params5(), vs, 0, &recordingGossiper{})

	blk := newTestBlock(1, ids.Empty, "own-rejected")
	_ = trackProposal(e, chainID, blk, 0)

	for i := 1; i < 5; i++ {
		e.ReceiveVote(Vote{BlockID: blk.id, NodeID: vs.nodeID(i), Accept: false, SignedAt: time.Now()})
	}
	if waitFor(300*time.Millisecond, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("SAFETY: own block finalized by REJECT votes (flip not removed)")
	}
	if blk.AcceptCalled() != 0 {
		t.Fatalf("VM.Accept must not run for a rejected block, got %d", blk.AcceptCalled())
	}
}

// TestProposer_FinalizesOwnProposalAtQuorum proves the proposer DOES finalize
// once a genuine α-of-K of signed accepts arrives — finality is reached the
// right way (cert), and exactly once.
func TestProposer_FinalizesOwnProposalAtQuorum(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngine(t, params5(), vs, 0, &recordingGossiper{})

	blk := newTestBlock(1, ids.Empty, "own-quorum")
	pos := trackProposal(e, chainID, blk, 0)

	e.ReceiveVote(vs.signedVote(1, pos))
	e.ReceiveVote(vs.signedVote(2, pos))

	if !waitFor(2*time.Second, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("LIVENESS: own block did not finalize after alpha-of-K signed accepts")
	}
	if blk.AcceptCalled() != 1 {
		t.Fatalf("VM.Accept exactly once at quorum, got %d", blk.AcceptCalled())
	}
	e.mu.RLock()
	_, stillPending := e.pendingBlocks[blk.id]
	e.mu.RUnlock()
	if stillPending {
		t.Fatal("finalized block must be removed from pendingBlocks")
	}
}

// TestProposer_AcceptsOnceUnderConcurrentVotes guards against double-accept
// under burst arrival of (valid, signed) peer votes overshooting alpha.
func TestProposer_AcceptsOnceUnderConcurrentVotes(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, chainID := newQuorumEngine(t, params5(), vs, 0, &recordingGossiper{})

	blk := newTestBlock(1, ids.Empty, "concurrent")
	pos := trackProposal(e, chainID, blk, 0)

	// All 4 other validators sign accept concurrently (overshoot alpha=3).
	for i := 1; i < 5; i++ {
		v := vs.signedVote(i, pos)
		go e.ReceiveVote(v)
	}
	if !waitFor(2*time.Second, func() bool { return blk.AcceptCalled() == 1 }) {
		t.Fatalf("VM.Accept must reach exactly 1, got %d", blk.AcceptCalled())
	}
	time.Sleep(150 * time.Millisecond) // let stragglers try to double-accept
	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("VM.Accept must be exactly once, got %d", got)
	}
}

// TestFinalizeOwnProposal_NoOpForFollowerEntry keeps the precise-scoping
// property: finalizeOwnProposal does nothing for an entry that is NOT this
// node's own proposal.
func TestFinalizeOwnProposal_NoOpForFollowerEntry(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, _ := newQuorumEngine(t, params5(), vs, 0, &recordingGossiper{})
	ctx := context.Background()

	blkID := ids.GenerateTestID()
	cb := &Block{id: blkID, parentID: ids.Empty, height: 7, timestamp: time.Now().Unix()}
	if err := e.AddBlock(ctx, cb); err != nil {
		t.Fatalf("AddBlock: %v", err)
	}
	e.mu.Lock()
	e.pendingBlocks[blkID] = &PendingBlock{ConsensusBlock: cb, VMBlock: nil, ProposedAt: time.Now(), IsOwnProposal: false}
	e.mu.Unlock()

	e.finalizeOwnProposal(ctx, blkID)

	if e.IsAccepted(blkID) {
		t.Fatal("follower-tracked block must NOT be accepted by finalizeOwnProposal")
	}
	e.mu.RLock()
	pending, exists := e.pendingBlocks[blkID]
	e.mu.RUnlock()
	if !exists || pending.Decided {
		t.Fatal("follower-tracked block must remain pending+undecided")
	}
}
