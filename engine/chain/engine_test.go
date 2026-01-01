package chain

import (
	"context"
	"errors"
	"testing"
	"time"

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

func (b *mockBlock) ID() ids.ID            { return b.id }
func (b *mockBlock) Parent() ids.ID        { return b.parentID }
func (b *mockBlock) ParentID() ids.ID      { return b.parentID }
func (b *mockBlock) Height() uint64        { return b.height }
func (b *mockBlock) Timestamp() time.Time  { return b.timestamp }
func (b *mockBlock) Status() uint8         { return b.status }
func (b *mockBlock) Verify(context.Context) error { return nil }
func (b *mockBlock) Accept(context.Context) error { return nil }
func (b *mockBlock) Reject(context.Context) error { return nil }
func (b *mockBlock) Bytes() []byte         { return b.bytes }

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
