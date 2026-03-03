// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/slashing"
	"github.com/luxfi/ids"
)

// --- Config and option coverage ---

func TestConfigValidate(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config should be valid: %v", err)
	}
}

func TestWithProposer(t *testing.T) {
	proposer := &mockProposer{}
	engine := New(WithProposer(proposer))
	if engine.proposer == nil {
		t.Error("WithProposer should set proposer")
	}
}

func TestWithEmitter(t *testing.T) {
	proposer := &mockProposer{}
	engine := New(WithEmitter(proposer))
	if engine.proposer == nil {
		t.Error("WithEmitter should set proposer (alias)")
	}
}

func TestWithLogger(t *testing.T) {
	engine := New(WithLogger(nil))
	// When nil is passed, the constructor should set log.Noop()
	if engine.log == nil {
		t.Error("log should not be nil even when WithLogger(nil)")
	}
}

func TestWithVoteBuffers(t *testing.T) {
	engine := New(WithVoteBuffers(50, 200))
	if cap(engine.voteRequests) != 50 {
		t.Errorf("expected voteRequests cap 50, got %d", cap(engine.voteRequests))
	}
	if cap(engine.votes) != 200 {
		t.Errorf("expected votes cap 200, got %d", cap(engine.votes))
	}
}

func TestWithVoteBuffersZero(t *testing.T) {
	// Zero values should not create new channels (keep defaults)
	engine := New(WithVoteBuffers(0, 0))
	if engine.voteRequests == nil {
		t.Error("voteRequests should not be nil")
	}
	if engine.votes == nil {
		t.Error("votes should not be nil")
	}
}

func TestWithParams(t *testing.T) {
	params := config.DefaultParams()
	params.K = 42
	engine := New(WithParams(params))
	if engine.params.K != 42 {
		t.Errorf("expected K=42, got %d", engine.params.K)
	}
}

func TestWithSlashing(t *testing.T) {
	det := slashing.NewDetector(64, 0.5)
	db := slashing.NewDB(10 * time.Minute)
	engine := New(WithSlashing(det, db))
	if engine.slashingDetector == nil {
		t.Error("slashing detector should be set")
	}
	if engine.slashingDB == nil {
		t.Error("slashing DB should be set")
	}
}

// --- StartWithID / StopWithError / Context ---

func TestStartWithID(t *testing.T) {
	engine := New()
	ctx := context.Background()
	if err := engine.StartWithID(ctx, 1); err != nil {
		t.Fatalf("StartWithID failed: %v", err)
	}
	if !engine.IsBootstrapped() {
		t.Error("should be bootstrapped")
	}
	engine.Stop(ctx)
}

func TestStopWithError(t *testing.T) {
	engine := New()
	ctx := context.Background()
	engine.Start(ctx, true)

	err := engine.StopWithError(ctx, errors.New("test error"))
	if err != nil {
		t.Fatalf("StopWithError failed: %v", err)
	}
	if engine.IsBootstrapped() {
		t.Error("should not be bootstrapped after stop")
	}
}

func TestContextBeforeStart(t *testing.T) {
	engine := New()
	ctx := engine.Context()
	if ctx == nil {
		t.Error("Context should return non-nil before Start")
	}
}

func TestContextAfterStart(t *testing.T) {
	engine := New()
	engine.Start(context.Background(), true)
	ctx := engine.Context()
	if ctx == nil {
		t.Error("Context should return non-nil after Start")
	}
	engine.Stop(context.Background())
}

// --- SetEmitter ---

func TestSetEmitter(t *testing.T) {
	engine := New()
	proposer := &mockProposer{}
	engine.SetEmitter(proposer)
	if engine.proposer == nil {
		t.Error("SetEmitter should set proposer")
	}
}

// --- HasPendingBlock / GetPendingBlock ---

func TestHasPendingBlockEmpty(t *testing.T) {
	engine := New()
	if engine.HasPendingBlock(ids.Empty) {
		t.Error("should have no pending blocks initially")
	}
}

func TestGetPendingBlockEmpty(t *testing.T) {
	engine := New()
	blk, ok := engine.GetPendingBlock(ids.Empty)
	if ok {
		t.Error("should not find pending block")
	}
	if blk != nil {
		t.Error("block should be nil")
	}
}

func TestGetPendingBlockNoVMBlock(t *testing.T) {
	engine := New()
	blockID := ids.GenerateTestID()
	engine.pendingBlocks[blockID] = &PendingBlock{
		ConsensusBlock: &Block{id: blockID},
		VMBlock:        nil, // no VM block
	}
	blk, ok := engine.GetPendingBlock(blockID)
	if ok {
		t.Error("should return false when VMBlock is nil")
	}
	if blk != nil {
		t.Error("block should be nil when VMBlock is nil")
	}
}

func TestGetPendingBlockWithVMBlock(t *testing.T) {
	engine := New()
	blockID := ids.GenerateTestID()
	mb := &mockBlock{id: blockID, height: 1}
	engine.pendingBlocks[blockID] = &PendingBlock{
		ConsensusBlock: &Block{id: blockID},
		VMBlock:        mb,
	}
	blk, ok := engine.GetPendingBlock(blockID)
	if !ok {
		t.Error("should find pending block")
	}
	if blk == nil {
		t.Error("block should not be nil")
	}
}

// --- ReceiveVote ---

func TestReceiveVoteNotStarted(t *testing.T) {
	engine := New()
	vote := Vote{BlockID: ids.GenerateTestID(), Accept: true}
	if engine.ReceiveVote(vote) {
		t.Error("should return false when not started")
	}
}

func TestReceiveVoteStarted(t *testing.T) {
	engine := New()
	engine.Start(context.Background(), true)
	defer engine.Stop(context.Background())

	vote := Vote{BlockID: ids.GenerateTestID(), Accept: true}
	if !engine.ReceiveVote(vote) {
		t.Error("should return true when started")
	}
}

func TestReceiveVoteBufferFull(t *testing.T) {
	engine := New(WithVoteBuffers(1, 1))
	engine.Start(context.Background(), true)
	defer engine.Stop(context.Background())

	// Fill the buffer
	engine.ReceiveVote(Vote{BlockID: ids.GenerateTestID(), Accept: true})
	// This should return false (buffer full)
	if engine.ReceiveVote(Vote{BlockID: ids.GenerateTestID(), Accept: true}) {
		t.Error("should return false when buffer is full")
	}
}

// --- DrainAccepted ---

func TestDrainAcceptedNoBlocks(t *testing.T) {
	engine := New()
	engine.Start(context.Background(), true)
	defer engine.Stop(context.Background())

	// Should not panic
	engine.DrainAccepted(context.Background())
}

func TestDrainAcceptedWithAcceptedBlock(t *testing.T) {
	engine := New()
	ctx := context.Background()
	engine.Start(ctx, true)
	defer engine.Stop(ctx)

	vm := &mockVM{}
	engine.SetVM(vm)

	blockID := ids.GenerateTestID()
	mb := &mockBlock{id: blockID, height: 1}
	cBlock := &Block{id: blockID, height: 1}

	// Add block to consensus and accept it
	engine.consensus.AddBlock(ctx, cBlock)
	engine.consensus.ProcessVote(ctx, blockID, true)
	engine.consensus.Poll(ctx, map[ids.ID]int{blockID: 1})

	if !engine.consensus.IsAccepted(blockID) {
		// In K=1 mode, single vote should accept
		t.Skip("consensus did not accept block (K>1)")
	}

	engine.pendingBlocks[blockID] = &PendingBlock{
		ConsensusBlock: cBlock,
		VMBlock:        mb,
		Decided:        false,
	}

	engine.DrainAccepted(ctx)

	if _, exists := engine.pendingBlocks[blockID]; exists {
		t.Error("accepted block should be removed from pending")
	}
}

// --- SyncState ---

func TestSyncState(t *testing.T) {
	engine := New()
	ctx := context.Background()
	engine.Start(ctx, true)
	defer engine.Stop(ctx)

	blockID := ids.GenerateTestID()
	staleBlock := &PendingBlock{
		ConsensusBlock: &Block{id: ids.GenerateTestID(), height: 5},
	}
	futureBlock := &PendingBlock{
		ConsensusBlock: &Block{id: ids.GenerateTestID(), height: 20},
	}
	engine.pendingBlocks[ids.GenerateTestID()] = staleBlock
	futureID := ids.GenerateTestID()
	engine.pendingBlocks[futureID] = futureBlock

	err := engine.SyncState(ctx, blockID, 10)
	if err != nil {
		t.Fatalf("SyncState failed: %v", err)
	}

	// Stale block (height 5 <= 10) should be removed
	// Future block (height 20 > 10) should remain
	if len(engine.pendingBlocks) != 1 {
		t.Errorf("expected 1 remaining pending block, got %d", len(engine.pendingBlocks))
	}
	if _, exists := engine.pendingBlocks[futureID]; !exists {
		t.Error("future block should remain")
	}
	if !engine.IsBootstrapped() {
		t.Error("should be bootstrapped after SyncState")
	}
}

// --- Stats ---

func TestStats(t *testing.T) {
	engine := New()
	stats := engine.Stats()

	expectedKeys := []string{"blocks_built", "blocks_accepted", "blocks_rejected",
		"votes_sent", "votes_received", "pending_blocks", "bootstrapped"}
	for _, key := range expectedKeys {
		if _, ok := stats[key]; !ok {
			t.Errorf("missing stat key: %s", key)
		}
	}
}

// --- Burst mode config ---

func TestNewWithConfigBurstMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Params.BlockTime = 1 * time.Millisecond // <= 1ms = burst mode
	cfg.VoteRequestBuffer = 0
	cfg.VoteBuffer = 0

	engine := NewWithConfig(cfg)
	if cap(engine.voteRequests) != 4096 {
		t.Errorf("burst mode should set voteRequests to 4096, got %d", cap(engine.voteRequests))
	}
	if cap(engine.votes) != 16384 {
		t.Errorf("burst mode should set votes to 16384, got %d", cap(engine.votes))
	}
}

// --- handleVote with slashing ---

func TestHandleVoteWithSlashing(t *testing.T) {
	det := slashing.NewDetector(64, 0.5)
	db := slashing.NewDB(10 * time.Minute)
	engine := New(WithSlashing(det, db))
	ctx := context.Background()
	engine.Start(ctx, true)
	defer engine.Stop(ctx)

	blockID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()
	cBlock := &Block{id: blockID, height: 1}

	engine.pendingBlocks[blockID] = &PendingBlock{
		ConsensusBlock: cBlock,
		VMBlock:        &mockBlock{id: blockID, height: 1},
	}

	// First vote should succeed
	engine.handleVote(Vote{BlockID: blockID, NodeID: nodeID, Accept: true})

	// Stats should show vote received
	if engine.votesReceived == 0 {
		t.Error("should have received votes")
	}
}

func TestHandleVoteUnknownBlock(t *testing.T) {
	engine := New()
	ctx := context.Background()
	engine.Start(ctx, true)
	defer engine.Stop(ctx)

	// Vote for unknown block should be silently dropped
	engine.handleVote(Vote{BlockID: ids.GenerateTestID(), Accept: true})
	if engine.votesReceived != 1 {
		t.Errorf("should count vote even if block unknown, got %d", engine.votesReceived)
	}
}

func TestHandleVoteReject(t *testing.T) {
	engine := New()
	ctx := context.Background()
	engine.Start(ctx, true)
	defer engine.Stop(ctx)

	blockID := ids.GenerateTestID()
	cBlock := &Block{id: blockID, height: 1}
	engine.consensus.AddBlock(ctx, cBlock)

	engine.pendingBlocks[blockID] = &PendingBlock{
		ConsensusBlock: cBlock,
		VMBlock:        &mockBlock{id: blockID},
	}

	engine.handleVote(Vote{BlockID: blockID, Accept: false})
	// RejectCount should be incremented
	engine.mu.RLock()
	p := engine.pendingBlocks[blockID]
	engine.mu.RUnlock()
	if p != nil && p.RejectCount != 1 {
		t.Errorf("expected RejectCount=1, got %d", p.RejectCount)
	}
}

// --- CheckBlockProposal ---

func TestCheckBlockProposalNoSlashing(t *testing.T) {
	engine := New()
	ev := engine.CheckBlockProposal(ids.GenerateTestNodeID(), 1, ids.GenerateTestID(), nil)
	if ev != nil {
		t.Error("should return nil when slashing not configured")
	}
}

func TestCheckBlockProposalJailed(t *testing.T) {
	det := slashing.NewDetector(64, 0.5)
	db := slashing.NewDB(10 * time.Minute)
	engine := New(WithSlashing(det, db))

	nodeID := ids.GenerateTestNodeID()
	db.RecordEvidence(slashing.Evidence{
		Type:        slashing.DoubleSign,
		ValidatorID: nodeID,
		Height:      1,
	})

	ev := engine.CheckBlockProposal(nodeID, 2, ids.GenerateTestID(), nil)
	if ev == nil {
		t.Error("should return evidence for jailed validator")
	}
}

func TestSlashingDB(t *testing.T) {
	engine := New()
	if engine.SlashingDB() != nil {
		t.Error("should return nil when not configured")
	}

	db := slashing.NewDB(10 * time.Minute)
	engine2 := New(WithSlashing(slashing.NewDetector(64, 0.5), db))
	if engine2.SlashingDB() == nil {
		t.Error("should return DB when configured")
	}
}

// --- Double start ---

func TestDoubleStart(t *testing.T) {
	engine := New()
	ctx := context.Background()
	engine.Start(ctx, true)
	defer engine.Stop(ctx)

	err := engine.Start(ctx, true)
	if err != ErrAlreadyStarted {
		t.Errorf("expected ErrAlreadyStarted, got %v", err)
	}
}

// --- mockProposer ---

type mockProposer struct{}

func (m *mockProposer) Propose(ctx context.Context, proposal BlockProposal) error {
	return nil
}

func (m *mockProposer) RequestVotes(ctx context.Context, req VoteRequest) error {
	return nil
}
