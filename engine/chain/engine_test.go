package chain

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// mockBlock implements block.Block for testing
type mockBlock struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp time.Time
	status    uint8
	bytes     []byte
}

func (b *mockBlock) ID() ids.ID                   { return b.id }
func (b *mockBlock) Parent() ids.ID               { return b.parentID }
func (b *mockBlock) ParentID() ids.ID             { return b.parentID }
func (b *mockBlock) Height() uint64               { return b.height }
func (b *mockBlock) Timestamp() time.Time         { return b.timestamp }
func (b *mockBlock) Status() uint8                { return b.status }
func (b *mockBlock) Verify(context.Context) error { return nil }
func (b *mockBlock) Accept(context.Context) error { return nil }
func (b *mockBlock) Reject(context.Context) error { return nil }
func (b *mockBlock) Bytes() []byte                { return b.bytes }

func TestNew(t *testing.T) {
	engine := New()
	if engine == nil {
		t.Fatal("New() returned nil")
	}

	if engine.IsBootstrapped() {
		t.Error("Engine should not be bootstrapped initially")
	}
}

func TestStart(t *testing.T) {
	engine := New()
	ctx := context.Background()

	err := engine.Start(ctx, true)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !engine.IsBootstrapped() {
		t.Error("Engine should be bootstrapped after Start")
	}
}

func TestStop(t *testing.T) {
	engine := New()
	ctx := context.Background()

	_ = engine.Start(ctx, true)

	err := engine.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestHealthCheck(t *testing.T) {
	engine := New()
	ctx := context.Background()

	health, err := engine.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if health == nil {
		t.Error("HealthCheck returned nil")
	}

	// Check it returns a map with consensus stats
	if m, ok := health.(map[string]interface{}); ok {
		// Real consensus returns detailed stats
		if _, exists := m["total_blocks"]; !exists {
			t.Error("HealthCheck should include total_blocks stat")
		}
		if _, exists := m["bootstrapped"]; !exists {
			t.Error("HealthCheck should include bootstrapped stat")
		}
	} else {
		t.Error("HealthCheck should return a map[string]interface{}")
	}
}

func TestGetBlock(t *testing.T) {
	engine := New()
	ctx := context.Background()

	// GetBlock should return nil (no-op for now)
	// Using empty IDs for test
	nodeID := ids.EmptyNodeID
	blockID := ids.Empty

	err := engine.GetBlock(ctx, nodeID, 1, blockID)
	if err != nil {
		t.Errorf("GetBlock should not error: %v", err)
	}
}

func TestChainWorkflow(t *testing.T) {
	engine := New()
	ctx := context.Background()

	// Start engine
	err := engine.Start(ctx, true)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Check bootstrapped
	if !engine.IsBootstrapped() {
		t.Error("Should be bootstrapped")
	}

	// Health check
	health, err := engine.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	if health == nil {
		t.Error("Health should not be nil")
	}

	// Stop engine
	err = engine.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// mockVM implements BlockBuilder for testing
type mockVM struct {
	buildBlockCalls int
	buildBlockErr   error
	lastBuiltBlock  *mockBlock
	blocks          map[ids.ID]*mockBlock
	lastAcceptedID  ids.ID
}

func (m *mockVM) BuildBlock(ctx context.Context) (block.Block, error) {
	m.buildBlockCalls++
	if m.buildBlockErr != nil {
		return nil, m.buildBlockErr
	}
	blk := &mockBlock{
		id:     ids.GenerateTestID(),
		height: uint64(m.buildBlockCalls),
	}
	m.lastBuiltBlock = blk
	if m.blocks == nil {
		m.blocks = make(map[ids.ID]*mockBlock)
	}
	m.blocks[blk.id] = blk
	return blk, nil
}

func (m *mockVM) GetBlock(ctx context.Context, id ids.ID) (block.Block, error) {
	if m.blocks != nil {
		if blk, ok := m.blocks[id]; ok {
			return blk, nil
		}
	}
	return nil, errors.New("block not found")
}

func (m *mockVM) ParseBlock(ctx context.Context, bytes []byte) (block.Block, error) {
	return &mockBlock{bytes: bytes}, nil
}

func (m *mockVM) LastAccepted(ctx context.Context) (ids.ID, error) {
	return m.lastAcceptedID, nil
}

func (m *mockVM) SetPreference(ctx context.Context, id ids.ID) error {
	// Mock implementation - just record the preferred block
	m.lastAcceptedID = id
	return nil
}

// TestNotifyPendingTxsTriggersBuildBlock verifies that Notify(PendingTxs) triggers BuildBlock
func TestNotifyPendingTxsTriggersBuildBlock(t *testing.T) {
	engine := New()
	ctx := context.Background()

	// Start engine
	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Set up mock VM
	vm := &mockVM{}
	engine.SetVM(vm)

	// Initial state: no pending builds, no BuildBlock calls
	if engine.PendingBuildBlocks() != 0 {
		t.Error("Should have 0 pending blocks initially")
	}
	if vm.buildBlockCalls != 0 {
		t.Error("Should have 0 BuildBlock calls initially")
	}

	// Send PendingTxs notification
	err := engine.Notify(ctx, Message{Type: PendingTxs})
	if err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	// Verify BuildBlock was called
	if vm.buildBlockCalls != 1 {
		t.Errorf("Expected 1 BuildBlock call, got %d", vm.buildBlockCalls)
	}

	// Pending blocks should be consumed (0 remaining)
	if engine.PendingBuildBlocks() != 0 {
		t.Errorf("Expected 0 pending blocks after build, got %d", engine.PendingBuildBlocks())
	}

	// Send multiple notifications rapidly
	for i := 0; i < 5; i++ {
		if err := engine.Notify(ctx, Message{Type: PendingTxs}); err != nil {
			t.Fatalf("Notify %d failed: %v", i, err)
		}
	}

	// All 5 notifications should have triggered BuildBlock
	if vm.buildBlockCalls != 6 { // 1 + 5
		t.Errorf("Expected 6 total BuildBlock calls, got %d", vm.buildBlockCalls)
	}
}

// TestNotifyStateSyncDoneDoesNotBuild verifies that StateSyncDone doesn't trigger builds
func TestNotifyStateSyncDoneDoesNotBuild(t *testing.T) {
	engine := New()
	ctx := context.Background()

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	vm := &mockVM{}
	engine.SetVM(vm)

	// Send StateSyncDone notification
	err := engine.Notify(ctx, Message{Type: StateSyncDone})
	if err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	// BuildBlock should NOT be called
	if vm.buildBlockCalls != 0 {
		t.Errorf("Expected 0 BuildBlock calls for StateSyncDone, got %d", vm.buildBlockCalls)
	}
}

// TestNotifyWithNoVMDoesNotPanic verifies Notify handles nil VM gracefully
func TestNotifyWithNoVMDoesNotPanic(t *testing.T) {
	engine := New()
	ctx := context.Background()

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Don't set VM - it should be nil

	// Should not panic
	err := engine.Notify(ctx, Message{Type: PendingTxs})
	if err != nil {
		t.Fatalf("Notify with nil VM should not error: %v", err)
	}
}

// TestNotifyBuildBlockError verifies error handling when BuildBlock fails
func TestNotifyBuildBlockError(t *testing.T) {
	engine := New()
	ctx := context.Background()

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// VM that always errors
	vm := &mockVM{
		buildBlockErr: context.DeadlineExceeded,
	}
	engine.SetVM(vm)

	// Notify should not propagate the error (just log and continue)
	err := engine.Notify(ctx, Message{Type: PendingTxs})
	if err != nil {
		t.Fatalf("Notify should not error on BuildBlock failure: %v", err)
	}

	// BuildBlock was still called
	if vm.buildBlockCalls != 1 {
		t.Errorf("Expected 1 BuildBlock call, got %d", vm.buildBlockCalls)
	}
}

// -----------------------------------------------------------------------------
// Quorum-based acceptance tests
// -----------------------------------------------------------------------------

// trackingMockBlock tracks Accept/Reject calls for testing
type trackingMockBlock struct {
	id           ids.ID
	parentID     ids.ID
	height       uint64
	timestamp    time.Time
	bytes        []byte
	acceptCalled int64
	rejectCalled int64
}

func (b *trackingMockBlock) ID() ids.ID                   { return b.id }
func (b *trackingMockBlock) Parent() ids.ID               { return b.parentID }
func (b *trackingMockBlock) ParentID() ids.ID             { return b.parentID }
func (b *trackingMockBlock) Height() uint64               { return b.height }
func (b *trackingMockBlock) Timestamp() time.Time         { return b.timestamp }
func (b *trackingMockBlock) Status() uint8                { return 0 }
func (b *trackingMockBlock) Verify(context.Context) error { return nil }
func (b *trackingMockBlock) Accept(context.Context) error {
	atomic.AddInt64(&b.acceptCalled, 1)
	return nil
}
func (b *trackingMockBlock) Reject(context.Context) error {
	atomic.AddInt64(&b.rejectCalled, 1)
	return nil
}
func (b *trackingMockBlock) Bytes() []byte { return b.bytes }

func (b *trackingMockBlock) AcceptCalled() int64 { return atomic.LoadInt64(&b.acceptCalled) }
func (b *trackingMockBlock) RejectCalled() int64 { return atomic.LoadInt64(&b.rejectCalled) }

// trackingMockVM returns trackingMockBlocks for acceptance tracking
type trackingMockVM struct {
	blocks         []*trackingMockBlock
	buildBlockIdx  int
	lastAcceptedID ids.ID
}

func (m *trackingMockVM) BuildBlock(ctx context.Context) (block.Block, error) {
	if m.buildBlockIdx >= len(m.blocks) {
		return nil, errors.New("no more blocks")
	}
	blk := m.blocks[m.buildBlockIdx]
	m.buildBlockIdx++
	return blk, nil
}

func (m *trackingMockVM) GetBlock(ctx context.Context, id ids.ID) (block.Block, error) {
	for _, blk := range m.blocks {
		if blk.id == id {
			return blk, nil
		}
	}
	return nil, errors.New("block not found")
}

func (m *trackingMockVM) ParseBlock(ctx context.Context, bytes []byte) (block.Block, error) {
	return &trackingMockBlock{bytes: bytes}, nil
}

func (m *trackingMockVM) LastAccepted(ctx context.Context) (ids.ID, error) {
	return m.lastAcceptedID, nil
}

func (m *trackingMockVM) SetPreference(ctx context.Context, id ids.ID) error {
	m.lastAcceptedID = id
	return nil
}

// TestEngine_DoesNotAcceptWithoutQuorum verifies blocks are NOT accepted without sufficient votes
func TestEngine_DoesNotAcceptWithoutQuorum(t *testing.T) {
	engine := New()
	ctx := context.Background()

	// Create a tracking block
	blk := &trackingMockBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now(),
		bytes:     []byte("block-data"),
	}

	vm := &trackingMockVM{
		blocks: []*trackingMockBlock{blk},
	}
	engine.SetVM(vm)

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer engine.Stop(ctx)

	// Trigger block build
	if err := engine.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	// Verify block was added to pending
	engine.mu.RLock()
	pending, exists := engine.pendingBlocks[blk.id]
	engine.mu.RUnlock()

	if !exists {
		t.Fatal("Block should be in pending blocks")
	}
	if pending.VMBlock == nil {
		t.Fatal("VMBlock should be set")
	}

	// Process a single vote (below quorum - default K=20, Alpha=15)
	// One vote is not enough for quorum
	engine.ReceiveVote(Vote{
		BlockID:  blk.id,
		NodeID:   ids.GenerateTestNodeID(),
		Accept:   true,
		SignedAt: time.Now(),
	})

	// Give vote handler time to process
	time.Sleep(100 * time.Millisecond)

	// Block should NOT be accepted yet (insufficient votes)
	if blk.AcceptCalled() > 0 {
		t.Errorf("Block Accept() should NOT be called with insufficient votes, but was called %d times", blk.AcceptCalled())
	}

	// Check consensus state - should not be accepted
	if engine.IsAccepted(blk.id) {
		t.Error("Block should not be marked as accepted with insufficient votes")
	}
}

// TestEngine_AcceptsAfterQuorum verifies blocks ARE accepted after sufficient votes
func TestEngine_AcceptsAfterQuorum(t *testing.T) {
	// Use smaller parameters for testing (K=5, Alpha=3, Beta=2)
	// This means we need 3 out of 5 votes for quorum
	engine := NewWithParams(config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            2,
	})
	ctx := context.Background()

	// Create a tracking block
	blk := &trackingMockBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now(),
		bytes:     []byte("block-data"),
	}

	vm := &trackingMockVM{
		blocks: []*trackingMockBlock{blk},
	}
	engine.SetVM(vm)

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer engine.Stop(ctx)

	// Trigger block build
	if err := engine.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	// Verify block was added
	engine.mu.RLock()
	_, exists := engine.pendingBlocks[blk.id]
	engine.mu.RUnlock()

	if !exists {
		t.Fatal("Block should be in pending blocks")
	}

	// Send enough votes to reach quorum (3 votes for Alpha=3)
	for i := 0; i < 4; i++ {
		engine.ReceiveVote(Vote{
			BlockID:  blk.id,
			NodeID:   ids.GenerateTestNodeID(),
			Accept:   true,
			SignedAt: time.Now(),
		})
	}

	// Give poll loop time to process the acceptance
	// The poll loop runs every 50ms
	time.Sleep(300 * time.Millisecond)

	// Block SHOULD be accepted after quorum
	if blk.AcceptCalled() != 1 {
		t.Errorf("Block Accept() should be called exactly once after quorum, but was called %d times", blk.AcceptCalled())
	}

	// Block should no longer be in pending
	engine.mu.RLock()
	_, stillPending := engine.pendingBlocks[blk.id]
	engine.mu.RUnlock()

	if stillPending {
		t.Error("Block should be removed from pending after acceptance")
	}
}

// -----------------------------------------------------------------------------
// Lifecycle invariant tests
// -----------------------------------------------------------------------------

// TestReceiveVote_DropsWhenNotStarted verifies votes are dropped when engine not started
func TestReceiveVote_DropsWhenNotStarted(t *testing.T) {
	engine := New()
	// DO NOT start the engine

	vote := Vote{
		BlockID:  ids.GenerateTestID(),
		NodeID:   ids.GenerateTestNodeID(),
		Accept:   true,
		SignedAt: time.Now(),
	}

	// ReceiveVote should return false (dropped) when not started
	accepted := engine.ReceiveVote(vote)
	if accepted {
		t.Error("ReceiveVote should return false when engine not started")
	}
}

// TestReceiveVote_AcceptsWhenStarted verifies votes are queued when engine is started
func TestReceiveVote_AcceptsWhenStarted(t *testing.T) {
	engine := New()
	ctx := context.Background()

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer engine.Stop(ctx)

	vote := Vote{
		BlockID:  ids.GenerateTestID(),
		NodeID:   ids.GenerateTestNodeID(),
		Accept:   true,
		SignedAt: time.Now(),
	}

	// ReceiveVote should return true (queued) when started
	accepted := engine.ReceiveVote(vote)
	if !accepted {
		t.Error("ReceiveVote should return true when engine is started")
	}
}

// TestReceiveVote_DropsAfterStop verifies votes are dropped after engine stops
func TestReceiveVote_DropsAfterStop(t *testing.T) {
	engine := New()
	ctx := context.Background()

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Stop the engine
	if err := engine.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	vote := Vote{
		BlockID:  ids.GenerateTestID(),
		NodeID:   ids.GenerateTestNodeID(),
		Accept:   true,
		SignedAt: time.Now(),
	}

	// ReceiveVote should return false after stop
	accepted := engine.ReceiveVote(vote)
	if accepted {
		t.Error("ReceiveVote should return false after engine stops")
	}
}

// -----------------------------------------------------------------------------
// Vote correlation tests (pendingBlocks tracking)
// -----------------------------------------------------------------------------

// TestHandleVote_IgnoresUnknownBlocks verifies votes for untracked blocks are ignored
func TestHandleVote_IgnoresUnknownBlocks(t *testing.T) {
	engine := New()
	ctx := context.Background()

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer engine.Stop(ctx)

	// Get initial vote count
	initialStats := engine.Stats()
	initialReceived := initialStats["votes_received"].(uint64)

	// Send vote for block that doesn't exist in pendingBlocks
	unknownBlockID := ids.GenerateTestID()
	engine.ReceiveVote(Vote{
		BlockID:  unknownBlockID,
		NodeID:   ids.GenerateTestNodeID(),
		Accept:   true,
		SignedAt: time.Now(),
	})

	// Give handler time to process
	time.Sleep(100 * time.Millisecond)

	// votes_received should increment (we received it)
	stats := engine.Stats()
	received := stats["votes_received"].(uint64)
	if received != initialReceived+1 {
		t.Errorf("Expected votes_received to increment, got %d", received)
	}

	// But block should NOT appear in consensus (unknown block ignored)
	if engine.IsAccepted(unknownBlockID) {
		t.Error("Unknown block should not be marked as accepted")
	}
}

// TestHandleVote_ProcessesKnownBlocks verifies votes for tracked blocks are processed
func TestHandleVote_ProcessesKnownBlocks(t *testing.T) {
	engine := NewWithParams(config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            2,
	})
	ctx := context.Background()

	blk := &trackingMockBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now(),
		bytes:     []byte("test-block"),
	}

	vm := &trackingMockVM{blocks: []*trackingMockBlock{blk}}
	engine.SetVM(vm)

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer engine.Stop(ctx)

	// Build block to add it to pendingBlocks
	if err := engine.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	// Verify block is in pendingBlocks
	engine.mu.RLock()
	pending, exists := engine.pendingBlocks[blk.id]
	engine.mu.RUnlock()
	if !exists {
		t.Fatal("Block should be in pendingBlocks")
	}
	initialVoteCount := pending.VoteCount

	// Send vote for known block
	engine.ReceiveVote(Vote{
		BlockID:  blk.id,
		NodeID:   ids.GenerateTestNodeID(),
		Accept:   true,
		SignedAt: time.Now(),
	})

	// Give handler time to process
	time.Sleep(100 * time.Millisecond)

	// VoteCount should increment (or block may have been accepted already)
	engine.mu.RLock()
	pending, stillExists := engine.pendingBlocks[blk.id]
	var newVoteCount int
	if stillExists {
		newVoteCount = pending.VoteCount
	}
	engine.mu.RUnlock()

	// Block might have been accepted and removed from pendingBlocks
	// which is also valid - votes were processed and led to acceptance
	if stillExists && newVoteCount <= initialVoteCount {
		t.Errorf("Expected VoteCount to increment from %d, got %d", initialVoteCount, newVoteCount)
	}
	// If !stillExists, the block was accepted which means votes were processed successfully
}

// -----------------------------------------------------------------------------
// Accept/Reject vote handling tests
// -----------------------------------------------------------------------------

// TestProcessVote_AcceptTrueIncrementsSupport verifies Accept=true votes count toward acceptance
func TestProcessVote_AcceptTrueIncrementsSupport(t *testing.T) {
	engine := NewWithParams(config.Parameters{
		K:               3,
		AlphaPreference: 2,
		AlphaConfidence: 2,
		Beta:            1,
	})
	ctx := context.Background()

	blk := &trackingMockBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now(),
		bytes:     []byte("test"),
	}

	vm := &trackingMockVM{blocks: []*trackingMockBlock{blk}}
	engine.SetVM(vm)

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer engine.Stop(ctx)

	// Build block
	if err := engine.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	// Send Accept=true votes to reach quorum
	for i := 0; i < 3; i++ {
		engine.ReceiveVote(Vote{
			BlockID:  blk.id,
			NodeID:   ids.GenerateTestNodeID(),
			Accept:   true,
			SignedAt: time.Now(),
		})
	}

	// Wait for acceptance
	time.Sleep(200 * time.Millisecond)

	// Block should be accepted
	if blk.AcceptCalled() != 1 {
		t.Errorf("Expected Accept() to be called once, got %d", blk.AcceptCalled())
	}
}

// TestProcessVote_AcceptFalseDoesNotAccept verifies Accept=false votes don't trigger acceptance
func TestProcessVote_AcceptFalseDoesNotAccept(t *testing.T) {

	engine := NewWithParams(config.Parameters{
		K:               3,
		AlphaPreference: 2,
		AlphaConfidence: 2,
		Beta:            1,
	})
	ctx := context.Background()

	blk := &trackingMockBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now(),
		bytes:     []byte("test"),
	}

	vm := &trackingMockVM{blocks: []*trackingMockBlock{blk}}
	engine.SetVM(vm)

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer engine.Stop(ctx)

	// Build block
	if err := engine.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	// Send Accept=false votes (rejections)
	for i := 0; i < 5; i++ {
		engine.ReceiveVote(Vote{
			BlockID:  blk.id,
			NodeID:   ids.GenerateTestNodeID(),
			Accept:   false, // Reject votes
			SignedAt: time.Now(),
		})
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Block should NOT be accepted (only reject votes received)
	if blk.AcceptCalled() > 0 {
		t.Errorf("Accept() should NOT be called with only reject votes, got %d calls", blk.AcceptCalled())
	}
}

// TestEngine_RejectsWithInsufficientSupport verifies blocks can be rejected
func TestEngine_RejectsWithInsufficientSupport(t *testing.T) {

	// Use smaller parameters (K=5, Alpha=3, Beta=2)
	engine := NewWithParams(config.Parameters{
		K:               5,
		AlphaPreference: 3,
		AlphaConfidence: 3,
		Beta:            2,
	})
	ctx := context.Background()

	blk := &trackingMockBlock{
		id:        ids.GenerateTestID(),
		parentID:  ids.Empty,
		height:    1,
		timestamp: time.Now(),
		bytes:     []byte("block-data"),
	}

	vm := &trackingMockVM{
		blocks: []*trackingMockBlock{blk},
	}
	engine.SetVM(vm)

	if err := engine.Start(ctx, true); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer engine.Stop(ctx)

	// Trigger block build
	if err := engine.Notify(ctx, Message{Type: PendingTxs}); err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	// Send reject votes (Accept=false)
	for i := 0; i < 4; i++ {
		engine.ReceiveVote(Vote{
			BlockID:  blk.id,
			NodeID:   ids.GenerateTestNodeID(),
			Accept:   false, // Reject votes
			SignedAt: time.Now(),
		})
	}

	// Give poll loop time to process
	time.Sleep(300 * time.Millisecond)

	// Block should NOT have Accept called
	if blk.AcceptCalled() > 0 {
		t.Errorf("Block Accept() should NOT be called for rejected block, but was called %d times", blk.AcceptCalled())
	}
}

// -----------------------------------------------------------------------------
// SyncState tests (for RLP import recovery)
// -----------------------------------------------------------------------------

// TestSyncState_UpdatesConsensusState verifies SyncState correctly updates consensus.
// This is critical for recovering from admin_importChain RLP imports.
func TestSyncState_UpdatesConsensusState(t *testing.T) {
	engine := New()
	ctx := context.Background()

	// Before sync, bootstrapped is false (engine not started)
	if engine.IsBootstrapped() {
		t.Error("Engine should not be bootstrapped before Start or SyncState")
	}

	// Simulate RLP import by syncing to a block at height 1000
	importedBlockID := ids.GenerateTestID()
	err := engine.SyncState(ctx, importedBlockID, 1000)
	if err != nil {
		t.Fatalf("SyncState failed: %v", err)
	}

	// After sync, engine should be bootstrapped
	if !engine.IsBootstrapped() {
		t.Error("Engine should be bootstrapped after SyncState")
	}

	// Consensus should have updated finalized tip
	if engine.consensus.GetFinalizedTip() != importedBlockID {
		t.Errorf("Expected finalizedTip=%s, got %s",
			importedBlockID, engine.consensus.GetFinalizedTip())
	}
}

// TestSyncState_ClearsStalePendingBlocks verifies SyncState removes stale blocks.
func TestSyncState_ClearsStalePendingBlocks(t *testing.T) {
	engine := New()
	ctx := context.Background()

	// Add some pending blocks at various heights
	block1 := &Block{id: ids.GenerateTestID(), height: 100}
	block2 := &Block{id: ids.GenerateTestID(), height: 500}
	block3 := &Block{id: ids.GenerateTestID(), height: 1500}

	engine.mu.Lock()
	engine.pendingBlocks[block1.id] = &PendingBlock{ConsensusBlock: block1}
	engine.pendingBlocks[block2.id] = &PendingBlock{ConsensusBlock: block2}
	engine.pendingBlocks[block3.id] = &PendingBlock{ConsensusBlock: block3}
	engine.mu.Unlock()

	// Sync to height 1000 - blocks at 100 and 500 should be cleared
	importedBlockID := ids.GenerateTestID()
	err := engine.SyncState(ctx, importedBlockID, 1000)
	if err != nil {
		t.Fatalf("SyncState failed: %v", err)
	}

	// Verify stale blocks were removed
	engine.mu.RLock()
	_, has1 := engine.pendingBlocks[block1.id]
	_, has2 := engine.pendingBlocks[block2.id]
	_, has3 := engine.pendingBlocks[block3.id]
	engine.mu.RUnlock()

	if has1 {
		t.Error("Block at height 100 should be cleared (below sync height 1000)")
	}
	if has2 {
		t.Error("Block at height 500 should be cleared (below sync height 1000)")
	}
	if !has3 {
		t.Error("Block at height 1500 should NOT be cleared (above sync height 1000)")
	}
}

// TestSyncState_Idempotent verifies calling SyncState multiple times is safe.
func TestSyncState_Idempotent(t *testing.T) {
	engine := New()
	ctx := context.Background()

	blockID1 := ids.GenerateTestID()
	blockID2 := ids.GenerateTestID()

	// First sync
	if err := engine.SyncState(ctx, blockID1, 100); err != nil {
		t.Fatalf("First SyncState failed: %v", err)
	}

	if engine.consensus.GetFinalizedTip() != blockID1 {
		t.Error("First sync should update finalizedTip")
	}

	// Second sync (higher block)
	if err := engine.SyncState(ctx, blockID2, 200); err != nil {
		t.Fatalf("Second SyncState failed: %v", err)
	}

	if engine.consensus.GetFinalizedTip() != blockID2 {
		t.Error("Second sync should update finalizedTip to new block")
	}

	// Should still be bootstrapped
	if !engine.IsBootstrapped() {
		t.Error("Should remain bootstrapped after multiple SyncState calls")
	}
}

// TestSyncState_WithEmptyID verifies SyncState handles empty block ID.
func TestSyncState_WithEmptyID(t *testing.T) {
	engine := New()
	ctx := context.Background()

	// Sync with empty ID (genesis state)
	err := engine.SyncState(ctx, ids.Empty, 0)
	if err != nil {
		t.Fatalf("SyncState with empty ID failed: %v", err)
	}

	// Should still bootstrap
	if !engine.IsBootstrapped() {
		t.Error("Should be bootstrapped even with empty block ID")
	}
}
