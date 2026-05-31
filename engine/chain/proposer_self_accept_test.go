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
)

// verifyOnceBlock implements block.Block such that Verify succeeds the first
// time and fails on subsequent calls. This models the production behavior of
// most luxd VMs (the PlatformVM CreateChainTx path in particular) where double
// Verify on the same block returns an error. The proposer's network handler
// (chains/manager.go applyQbit) re-derives vote.Accept from a fresh Verify call
// on incoming peer votes for its own block — without the IsOwnProposal trust
// path, the double-Verify failure flips peer Accepts to Rejects locally and
// drives the proposer's pendingBlock to rejected even though the cluster has
// committed it (the "proposer-self-accept gap").
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
func (b *verifyOnceBlock) Bytes() []byte           { return b.bytes }
func (b *verifyOnceBlock) AcceptCalled() int64     { return atomic.LoadInt64(&b.acceptCalled) }
func (b *verifyOnceBlock) RejectCalled() int64     { return atomic.LoadInt64(&b.rejectCalled) }
func (b *verifyOnceBlock) VerifyCalls() int64      { return atomic.LoadInt64(&b.verifyCount) }

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

// TestProposerSelfAccept_PeerVotesAcceptedDespiteLocalReVerifyFailure is the
// regression test for the lux-devnet "proposer-self-accept gap" bug.
//
// Scenario (5-validator network, alpha=3):
//  1. Proposer N builds block B; N's VM.Verify(B) succeeds (verifyCount=1)
//  2. N self-votes (acceptVotes=1); B enters pendingBlocks with IsOwnProposal=true
//  3. N broadcasts; the other 4 followers accept B via fast-follow and each
//     sends a Chits message back to N
//  4. On N, each incoming Chits is converted to a Vote by manager.applyQbit
//     which calls blk.Verify(ctx) AGAIN to derive vote.Accept. With this VM
//     that returns an error, accept=false (the bug surface).
//  5. With the fix, handleVote treats incoming votes for OwnProposal blocks
//     as effectiveAccept=true regardless of vote.Accept, advancing acceptVotes
//     to alpha and triggering tryFinalizeBlock → VMBlock.Accept(ctx).
//
// Pre-fix: vote.Accept=false flips the count into rejectVotes, B is rejected
// on N even though cluster committed. AcceptCalled==0.
// Post-fix: AcceptCalled==1.
func TestProposerSelfAccept_PeerVotesAcceptedDespiteLocalReVerifyFailure(t *testing.T) {
	// 5-validator parameters: K=5, alpha=3
	engine := NewWithParams(config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            2,
	})
	ctx := context.Background()

	blk := &verifyOnceBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now(),
		bytes:     []byte("proposer-block"),
	}
	vm := &verifyOnceVM{blocks: []*verifyOnceBlock{blk}}
	engine.SetVM(vm)

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer engine.Stop(ctx)

	// Proposer path: Notify triggers buildBlocksLocked, which calls Verify
	// (verifyCount=1), self-votes, and inserts pendingBlocks with IsOwnProposal=true.
	if err := engine.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	engine.mu.RLock()
	pending, exists := engine.pendingBlocks[blk.id]
	engine.mu.RUnlock()
	if !exists {
		t.Fatal("block must be tracked in pendingBlocks after Notify")
	}
	if !pending.IsOwnProposal {
		t.Fatal("proposer's own block must carry IsOwnProposal=true")
	}
	if got := blk.VerifyCalls(); got != 1 {
		t.Fatalf("Verify should be called exactly once at proposal time, got %d", got)
	}

	// Simulate peer votes arriving with vote.Accept=false — modelling
	// manager.applyQbit's re-Verify failure on the proposer for its own block.
	// We send 4 votes (>= alpha=3) — pre-fix this would push rejectVotes to alpha
	// and reject the block; post-fix all four count as effective Accepts and
	// the block is accepted.
	for i := 0; i < 4; i++ {
		engine.ReceiveVote(Vote{
			BlockID:  blk.id,
			NodeID:   ids.GenerateTestNodeID(),
			Accept:   false, // <-- the bug surface: applyQbit's failed re-Verify
			SignedAt: time.Now(),
		})
	}

	// Allow the voteHandler + tryFinalizeBlock to drain.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && blk.AcceptCalled() == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("VMBlock.Accept must be called exactly once after quorum, got %d", got)
	}
	if got := blk.RejectCalled(); got != 0 {
		t.Fatalf("VMBlock.Reject must NOT be called for accepted block, got %d", got)
	}
	if !engine.IsAccepted(blk.id) {
		t.Fatal("engine must mark block accepted after quorum")
	}
	engine.mu.RLock()
	_, stillPending := engine.pendingBlocks[blk.id]
	engine.mu.RUnlock()
	if stillPending {
		t.Fatal("accepted block must be removed from pendingBlocks")
	}
}

// TestFollower_HonorsVoteAcceptValue verifies the IsOwnProposal trust path
// does NOT apply to follower-tracked blocks. For non-own pending entries
// (e.g., blocks tracked via HandleIncomingBlock's slow path), vote.Accept
// is honored faithfully — a vote.Accept=false counts as rejection.
//
// This ensures the proposer-self-accept fix is precisely scoped and does
// not weaken consensus for follower-side block tracking, where the engine
// did not perform the initial Verify and therefore cannot vouch for the
// block independently of peer signals.
func TestFollower_HonorsVoteAcceptValue(t *testing.T) {
	engine := NewWithParams(config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            2,
	})
	ctx := context.Background()

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer engine.Stop(ctx)

	blkID := ids.GenerateTestID()
	cb := &Block{
		id:        blkID,
		parentID:  ids.Empty,
		height:    7,
		timestamp: time.Now().Unix(),
	}
	if err := engine.AddBlock(ctx, cb); err != nil {
		t.Fatalf("AddBlock: %v", err)
	}

	// Inject a follower-style pending entry: NOT an own proposal,
	// no VMBlock attached (the follower's HandleIncomingBlock slow path).
	engine.mu.Lock()
	engine.pendingBlocks[blkID] = &PendingBlock{
		ConsensusBlock: cb,
		VMBlock:        nil,
		ProposedAt:     time.Now(),
		VoteCount:      1,
		Decided:        false,
		IsOwnProposal:  false,
	}
	engine.mu.Unlock()

	// Send 4 negative votes — these MUST count as rejections.
	for i := 0; i < 4; i++ {
		engine.ReceiveVote(Vote{
			BlockID:  blkID,
			NodeID:   ids.GenerateTestNodeID(),
			Accept:   false,
			SignedAt: time.Now(),
		})
	}

	// Allow vote handler to drain.
	time.Sleep(200 * time.Millisecond)

	if engine.IsAccepted(blkID) {
		t.Fatal("follower-tracked block must NOT be accepted when peers vote reject")
	}
}

// TestProposer_AcceptsOnceUnderConcurrentVotes verifies the Accept path is
// called exactly once even under burst-arrival of peer votes — guards against
// the "double-accept" failure mode the bug report explicitly forbade.
func TestProposer_AcceptsOnceUnderConcurrentVotes(t *testing.T) {
	engine := NewWithParams(config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            2,
	})
	ctx := context.Background()

	blk := &verifyOnceBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now(),
		bytes:     []byte("burst-block"),
	}
	vm := &verifyOnceVM{blocks: []*verifyOnceBlock{blk}}
	engine.SetVM(vm)

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer engine.Stop(ctx)

	if err := engine.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	// 8 concurrent peer votes (overshoot alpha=3) — Accept must still be exactly once.
	for i := 0; i < 8; i++ {
		go engine.ReceiveVote(Vote{
			BlockID:  blk.id,
			NodeID:   ids.GenerateTestNodeID(),
			Accept:   false, // bug-surface signal
			SignedAt: time.Now(),
		})
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && blk.AcceptCalled() == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	// give late goroutines a chance to try double-accepting
	time.Sleep(200 * time.Millisecond)

	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("VMBlock.Accept must be called exactly once, got %d", got)
	}
}

// recordingProposer captures Propose/RequestVotes calls without going through
// any real network layer, so tests can model the production wiring (proposer
// is non-nil → finalizeOwnProposal fires) without producing actual gossip
// traffic. Mirrors the production BlockProposer surface.
type recordingProposer struct {
	proposeCalled  int64
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

func (p *recordingProposer) ProposeCalls() int64 {
	return atomic.LoadInt64(&p.proposeCalled)
}

// TestProposerSelfAccept_FinalizesWithoutPeerChits is the regression test for
// the v1.27.26 lux-devnet stall: the v1.25.1 IsOwnProposal trust path closes
// the gap only when peer Chits actually arrive at the proposer. When Chits do
// not arrive (network condition empirically observed on K=3 alpha=2 devnet),
// the proposer's pending block stays decided=false indefinitely; VM.Accept
// is never called; the node freezes at its proposal height.
//
// The v1.25.2 fix routes the proposer's commitment through
// finalizeOwnProposal at the end of buildBlocksLocked: once the proposer
// gossips the block and requests votes, it locally commits to its own
// (already-verified) block without waiting on peer signals. This is safe
// because the proposer ran Verify successfully — under honest-validator
// assumption, peers running the same Verify reach the same decision, and
// cluster authoritative finality (BLS+Pulsar witness) is unaffected.
//
// Test models the production wiring (recordingProposer non-nil) and sends
// ZERO peer votes; the proposer must still call VMBlock.Accept exactly once.
func TestProposerSelfAccept_FinalizesWithoutPeerChits(t *testing.T) {
	// 5-validator alpha=3 cluster — without the fix, lone self-vote (count=1)
	// stays below alpha (3) forever and the block never finalizes.
	engine := NewWithParams(config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            2,
	})
	ctx := context.Background()

	blk := &verifyOnceBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now(),
		bytes:     []byte("self-accept-no-chits"),
	}
	vm := &verifyOnceVM{blocks: []*verifyOnceBlock{blk}}
	engine.SetVM(vm)
	proposer := &recordingProposer{}
	engine.SetProposer(proposer)

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer engine.Stop(ctx)

	if err := engine.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	// Notify is synchronous through buildBlocksLocked which now ends in
	// finalizeOwnProposal — Accept must already have fired before Notify
	// returns. Allow a short grace for async signal propagation.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && blk.AcceptCalled() == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	if got := proposer.ProposeCalls(); got != 1 {
		t.Fatalf("proposer.Propose must fire once at proposal time, got %d", got)
	}
	if got := blk.VerifyCalls(); got != 1 {
		t.Fatalf("Verify must be called exactly once at proposal time, got %d", got)
	}
	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("VMBlock.Accept must fire on proposer without peer Chits, got %d", got)
	}
	if got := blk.RejectCalled(); got != 0 {
		t.Fatalf("VMBlock.Reject must NOT be called for self-finalized block, got %d", got)
	}
	if !engine.IsAccepted(blk.id) {
		t.Fatal("engine must mark block accepted after self-finalize")
	}
	engine.mu.RLock()
	_, stillPending := engine.pendingBlocks[blk.id]
	engine.mu.RUnlock()
	if stillPending {
		t.Fatal("self-finalized block must be removed from pendingBlocks")
	}
}

// TestProposerSelfAccept_IdempotentUnderLateChits verifies that late-arriving
// peer Chits after the proposer has already self-finalized cannot drive a
// second VMBlock.Accept call. This guards the production scenario where peers
// fast-follow accept and then Chits arrive at the proposer AFTER its
// self-finalize already fired.
func TestProposerSelfAccept_IdempotentUnderLateChits(t *testing.T) {
	engine := NewWithParams(config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            2,
	})
	ctx := context.Background()

	blk := &verifyOnceBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now(),
		bytes:     []byte("late-chits"),
	}
	vm := &verifyOnceVM{blocks: []*verifyOnceBlock{blk}}
	engine.SetVM(vm)
	engine.SetProposer(&recordingProposer{})

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer engine.Stop(ctx)

	if err := engine.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	// Wait for self-finalize.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && blk.AcceptCalled() == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("self-finalize must fire once, got %d Accept calls", got)
	}

	// Now simulate late-arriving peer Chits (would normally drive natural
	// finalization but the block is already decided + removed from pending).
	for i := 0; i < 5; i++ {
		engine.ReceiveVote(Vote{
			BlockID:  blk.id,
			NodeID:   ids.GenerateTestNodeID(),
			Accept:   false, // bug-surface signal from applyQbit re-Verify
			SignedAt: time.Now(),
		})
	}

	// Drain time for handleVote to process the late votes.
	time.Sleep(200 * time.Millisecond)

	if got := blk.AcceptCalled(); got != 1 {
		t.Fatalf("VMBlock.Accept must still be exactly 1 after late Chits, got %d", got)
	}
}

// TestFollower_StillRequiresAlphaQuorum_AfterSelfAcceptFix verifies the
// finalizeOwnProposal path is precisely scoped: IsOwnProposal=false entries
// (followers tracking peer-proposed blocks via HandleIncomingBlock slow path)
// must still require the full alpha-of-K quorum signal. The self-accept
// shortcut applies only to the node that built the block.
func TestFollower_StillRequiresAlphaQuorum_AfterSelfAcceptFix(t *testing.T) {
	engine := NewWithParams(config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            2,
	})
	engine.SetProposer(&recordingProposer{})
	ctx := context.Background()

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer engine.Stop(ctx)

	blkID := ids.GenerateTestID()
	cb := &Block{
		id:        blkID,
		parentID:  ids.Empty,
		height:    7,
		timestamp: time.Now().Unix(),
	}
	if err := engine.AddBlock(ctx, cb); err != nil {
		t.Fatalf("AddBlock: %v", err)
	}

	// Inject follower-style entry (NOT IsOwnProposal). finalizeOwnProposal
	// MUST refuse to fire on this entry even though a proposer is wired.
	engine.mu.Lock()
	engine.pendingBlocks[blkID] = &PendingBlock{
		ConsensusBlock: cb,
		VMBlock:        nil,
		ProposedAt:     time.Now(),
		VoteCount:      1,
		Decided:        false,
		IsOwnProposal:  false,
	}
	engine.mu.Unlock()

	// Direct call: must be a no-op since IsOwnProposal=false.
	engine.finalizeOwnProposal(ctx, blkID)

	engine.mu.RLock()
	pending, exists := engine.pendingBlocks[blkID]
	engine.mu.RUnlock()
	if !exists {
		t.Fatal("follower-tracked block must NOT be removed by finalizeOwnProposal")
	}
	if pending.Decided {
		t.Fatal("follower-tracked block must NOT be marked Decided by finalizeOwnProposal")
	}
	if engine.IsAccepted(blkID) {
		t.Fatal("follower-tracked block must NOT be marked accepted by finalizeOwnProposal")
	}
}
