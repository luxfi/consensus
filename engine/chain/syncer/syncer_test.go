package syncer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// mockVM implements the VM interface for testing.
type mockVM struct {
	lastAcceptedID     ids.ID
	lastAcceptedErr    error
	blocks             map[ids.ID]*mockBlock
	getBlockErr        error
	setPreferenceID    ids.ID
	setPreferenceErr   error
	setPreferenceCalls int
}

func newMockVM() *mockVM {
	return &mockVM{
		blocks: make(map[ids.ID]*mockBlock),
	}
}

func (m *mockVM) LastAccepted(ctx context.Context) (ids.ID, error) {
	return m.lastAcceptedID, m.lastAcceptedErr
}

func (m *mockVM) GetBlock(ctx context.Context, id ids.ID) (block.Block, error) {
	if m.getBlockErr != nil {
		return nil, m.getBlockErr
	}
	blk, ok := m.blocks[id]
	if !ok {
		return nil, errors.New("block not found")
	}
	return blk, nil
}

func (m *mockVM) SetPreference(ctx context.Context, id ids.ID) error {
	m.setPreferenceCalls++
	m.setPreferenceID = id
	return m.setPreferenceErr
}

// mockBlock implements block.Block for testing.
type mockBlock struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp time.Time
	data      []byte
	status    uint8
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
func (b *mockBlock) Bytes() []byte                { return b.data }

// mockConsensus implements ConsensusState for testing.
type mockConsensus struct {
	syncCalled bool
	syncID     ids.ID
	syncHeight uint64
	syncErr    error
}

func (m *mockConsensus) SyncState(ctx context.Context, lastAcceptedID ids.ID, height uint64) error {
	m.syncCalled = true
	m.syncID = lastAcceptedID
	m.syncHeight = height
	return m.syncErr
}

// TestSyncerStart verifies the syncer correctly synchronizes consensus state.
func TestSyncerStart(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create a block at height 100 (simulating RLP import)
	blockID := ids.GenerateTestID()
	importedBlock := &mockBlock{
		id:        blockID,
		parentID:  ids.GenerateTestID(),
		height:    100,
		timestamp: time.Now(),
	}

	// Setup mock VM with imported state
	vm := newMockVM()
	vm.lastAcceptedID = blockID
	vm.blocks[blockID] = importedBlock

	// Setup mock consensus
	consensus := &mockConsensus{}

	// Track if onDone was called
	doneCalled := false
	var doneReqID uint32

	// Create and run syncer
	config := &Config{
		VM:        vm,
		Consensus: consensus,
	}
	syncer := New(config, func(ctx context.Context, reqID uint32) error {
		doneCalled = true
		doneReqID = reqID
		return nil
	})

	err := syncer.Start(ctx, 42)
	require.NoError(err)

	// Verify consensus was updated
	require.True(consensus.syncCalled, "consensus.SyncState should be called")
	require.Equal(blockID, consensus.syncID, "consensus should receive correct block ID")
	require.Equal(uint64(100), consensus.syncHeight, "consensus should receive correct height")

	// Verify VM preference was set
	require.Equal(1, vm.setPreferenceCalls, "SetPreference should be called once")
	require.Equal(blockID, vm.setPreferenceID, "SetPreference should receive correct block ID")

	// Verify done callback was invoked
	require.True(doneCalled, "onDone callback should be called")
	require.Equal(uint32(42), doneReqID, "onDone should receive correct request ID")
}

// TestSyncerNoVM verifies syncer handles nil VM gracefully.
func TestSyncerNoVM(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	doneCalled := false
	config := &Config{
		VM: nil,
	}
	syncer := New(config, func(ctx context.Context, reqID uint32) error {
		doneCalled = true
		return nil
	})

	err := syncer.Start(ctx, 0)
	require.NoError(err)
	require.True(doneCalled, "onDone should be called even with no VM")
}

// TestSyncerVMError verifies syncer handles VM errors gracefully.
func TestSyncerVMError(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	vm := newMockVM()
	vm.lastAcceptedErr = errors.New("database error")

	doneCalled := false
	config := &Config{
		VM: vm,
	}
	syncer := New(config, func(ctx context.Context, reqID uint32) error {
		doneCalled = true
		return nil
	})

	// Should not return error - should complete bootstrap anyway
	err := syncer.Start(ctx, 0)
	require.NoError(err)
	require.True(doneCalled, "onDone should be called even on VM error")
}

// TestSyncerGetBlockError verifies syncer handles GetBlock errors gracefully.
func TestSyncerGetBlockError(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	blockID := ids.GenerateTestID()
	vm := newMockVM()
	vm.lastAcceptedID = blockID
	vm.getBlockErr = errors.New("block not found")

	consensus := &mockConsensus{}
	doneCalled := false

	config := &Config{
		VM:        vm,
		Consensus: consensus,
	}
	syncer := New(config, func(ctx context.Context, reqID uint32) error {
		doneCalled = true
		return nil
	})

	err := syncer.Start(ctx, 0)
	require.NoError(err)

	// Consensus should still be called with height 0
	require.True(consensus.syncCalled)
	require.Equal(blockID, consensus.syncID)
	require.Equal(uint64(0), consensus.syncHeight, "height should be 0 when GetBlock fails")
	require.True(doneCalled)
}

// TestSyncerSetPreferenceError verifies syncer handles SetPreference errors gracefully.
func TestSyncerSetPreferenceError(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	blockID := ids.GenerateTestID()
	vm := newMockVM()
	vm.lastAcceptedID = blockID
	vm.blocks[blockID] = &mockBlock{id: blockID, height: 50}
	vm.setPreferenceErr = errors.New("preference error")

	consensus := &mockConsensus{}
	doneCalled := false

	config := &Config{
		VM:        vm,
		Consensus: consensus,
	}
	syncer := New(config, func(ctx context.Context, reqID uint32) error {
		doneCalled = true
		return nil
	})

	err := syncer.Start(ctx, 0)
	require.NoError(err)

	// Should still complete successfully
	require.True(consensus.syncCalled)
	require.True(doneCalled)
}

// TestSyncerOnSyncCompleteCallback verifies the OnSyncComplete callback works.
func TestSyncerOnSyncCompleteCallback(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	blockID := ids.GenerateTestID()
	vm := newMockVM()
	vm.lastAcceptedID = blockID
	vm.blocks[blockID] = &mockBlock{id: blockID, height: 200}

	var callbackID ids.ID
	var callbackHeight uint64
	callbackCalled := false

	config := &Config{
		VM: vm,
		OnSyncComplete: func(ctx context.Context, id ids.ID, height uint64) error {
			callbackCalled = true
			callbackID = id
			callbackHeight = height
			return nil
		},
	}
	syncer := New(config, nil)

	err := syncer.Start(ctx, 0)
	require.NoError(err)

	require.True(callbackCalled)
	require.Equal(blockID, callbackID)
	require.Equal(uint64(200), callbackHeight)
}

// TestSyncOnce verifies the one-shot sync function.
func TestSyncOnce(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	blockID := ids.GenerateTestID()
	vm := newMockVM()
	vm.lastAcceptedID = blockID
	vm.blocks[blockID] = &mockBlock{id: blockID, height: 500}

	result, err := SyncOnce(ctx, vm)
	require.NoError(err)
	require.NotNil(result)
	require.True(result.Synced)
	require.Equal(blockID, result.LastAcceptedID)
	require.Equal(uint64(500), result.Height)
	require.Nil(result.Error)
	require.Equal(1, vm.setPreferenceCalls)
}

// TestSyncOnceNoVM verifies SyncOnce handles nil VM.
func TestSyncOnceNoVM(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	result, err := SyncOnce(ctx, nil)
	require.Error(err)
	require.Equal(ErrNoVM, err)
	require.Nil(result)
}

// TestNewConfig validates NewConfig error handling.
func TestNewConfig(t *testing.T) {
	require := require.New(t)

	// Nil VM should error
	_, err := NewConfig(nil, nil, nil)
	require.Error(err)
	require.Equal(ErrNoVM, err)

	// Valid VM should succeed
	vm := newMockVM()
	config, err := NewConfig(vm, nil, nil)
	require.NoError(err)
	require.NotNil(config)
	require.Equal(vm, config.VM)
}

// TestHealthCheck verifies health check returns correct status.
func TestHealthCheck(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	blockID := ids.GenerateTestID()
	vm := newMockVM()
	vm.lastAcceptedID = blockID

	config := &Config{VM: vm}
	syncer := New(config, nil)

	health, err := syncer.HealthCheck(ctx)
	require.NoError(err)

	status := health.(map[string]interface{})
	require.Equal("ready", status["status"])
	require.Equal(blockID.String(), status["lastAccepted"])
}

// TestDeprecatedAPI verifies backward compatibility with old API.
func TestDeprecatedAPI(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	blockID := ids.GenerateTestID()
	vm := newMockVM()
	vm.lastAcceptedID = blockID
	vm.blocks[blockID] = &mockBlock{id: blockID, height: 100}

	// Use deprecated config
	deprecatedConfig, err := NewDeprecatedConfig(nil, nil, nil, nil, nil, vm)
	require.NoError(err)

	doneCalled := false
	syncer := NewFromDeprecated(deprecatedConfig, func(ctx context.Context, reqID uint32) error {
		doneCalled = true
		return nil
	})

	err = syncer.Start(ctx, 0)
	require.NoError(err)
	require.True(doneCalled)
}

// -----------------------------------------------------------------------------
// Persistence Tests - Critical for restart resilience
// -----------------------------------------------------------------------------

// mockStateStore implements StateStore for testing.
type mockStateStore struct {
	data     map[string][]byte
	getErr   error
	putErr   error
	delErr   error
	putCalls int
	getCalls int
}

func newMockStateStore() *mockStateStore {
	return &mockStateStore{
		data: make(map[string][]byte),
	}
}

func (m *mockStateStore) Get(key []byte) ([]byte, error) {
	m.getCalls++
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.data[string(key)], nil
}

func (m *mockStateStore) Put(key, value []byte) error {
	m.putCalls++
	if m.putErr != nil {
		return m.putErr
	}
	m.data[string(key)] = value
	return nil
}

func (m *mockStateStore) Delete(key []byte) error {
	if m.delErr != nil {
		return m.delErr
	}
	delete(m.data, string(key))
	return nil
}

// TestSyncerPersistsState verifies syncer persists state to StateStore.
func TestSyncerPersistsState(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	blockID := ids.GenerateTestID()
	height := uint64(1000)

	vm := newMockVM()
	vm.lastAcceptedID = blockID
	vm.blocks[blockID] = &mockBlock{id: blockID, height: height}

	store := newMockStateStore()
	consensus := &mockConsensus{}

	config := &Config{
		VM:         vm,
		Consensus:  consensus,
		StateStore: store,
	}
	syncer := New(config, nil)

	err := syncer.Start(ctx, 0)
	require.NoError(err)

	// Verify state was persisted
	require.Equal(3, store.putCalls, "should persist 3 values: blockID, height, bootstrapped")

	// Verify we can load the state back
	loadedID, loadedHeight, bootstrapped, err := LoadPersistedState(store)
	require.NoError(err)
	require.Equal(blockID, loadedID, "persisted block ID should match")
	require.Equal(height, loadedHeight, "persisted height should match")
	require.True(bootstrapped, "should be marked as bootstrapped")
}

// TestApplyImportedHead verifies the single API for external state advances.
func TestApplyImportedHead(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	blockID := ids.GenerateTestID()
	height := uint64(5000)

	vm := newMockVM()
	vm.blocks[blockID] = &mockBlock{id: blockID, height: height}

	store := newMockStateStore()
	consensus := &mockConsensus{}

	// Call ApplyImportedHead - this is what coreth should call after RLP import
	err := ApplyImportedHead(ctx, store, vm, consensus, blockID)
	require.NoError(err)

	// Verify persistence
	loadedID, loadedHeight, bootstrapped, err := LoadPersistedState(store)
	require.NoError(err)
	require.Equal(blockID, loadedID)
	require.Equal(height, loadedHeight)
	require.True(bootstrapped)

	// Verify VM preference was set
	require.Equal(blockID, vm.setPreferenceID)
	require.Equal(1, vm.setPreferenceCalls)

	// Verify consensus was updated
	require.True(consensus.syncCalled)
	require.Equal(blockID, consensus.syncID)
	require.Equal(height, consensus.syncHeight)
}

// TestApplyImportedHeadRequiresStore verifies ApplyImportedHead requires StateStore.
func TestApplyImportedHeadRequiresStore(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	blockID := ids.GenerateTestID()
	vm := newMockVM()
	vm.blocks[blockID] = &mockBlock{id: blockID, height: 100}

	// Without StateStore, should fail
	err := ApplyImportedHead(ctx, nil, vm, nil, blockID)
	require.Error(err)
	require.ErrorIs(err, ErrNoStateStore)
}

// TestApplyImportedHeadRequiresVM verifies ApplyImportedHead requires VM.
func TestApplyImportedHeadRequiresVM(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	store := newMockStateStore()
	blockID := ids.GenerateTestID()

	// Without VM, should fail
	err := ApplyImportedHead(ctx, store, nil, nil, blockID)
	require.Error(err)
	require.ErrorIs(err, ErrNoVM)
}

// TestLoadPersistedStateEmpty verifies LoadPersistedState handles empty store.
func TestLoadPersistedStateEmpty(t *testing.T) {
	require := require.New(t)

	store := newMockStateStore()

	blockID, height, bootstrapped, err := LoadPersistedState(store)
	require.NoError(err)
	require.Equal(ids.Empty, blockID)
	require.Equal(uint64(0), height)
	require.False(bootstrapped)
}

// TestLoadPersistedStateNilStore verifies LoadPersistedState handles nil store.
func TestLoadPersistedStateNilStore(t *testing.T) {
	require := require.New(t)

	blockID, height, bootstrapped, err := LoadPersistedState(nil)
	require.NoError(err)
	require.Equal(ids.Empty, blockID)
	require.Equal(uint64(0), height)
	require.False(bootstrapped)
}

// TestClearPersistedState verifies ClearPersistedState removes all state.
func TestClearPersistedState(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	blockID := ids.GenerateTestID()
	vm := newMockVM()
	vm.blocks[blockID] = &mockBlock{id: blockID, height: 1000}

	store := newMockStateStore()

	// First, persist some state
	err := ApplyImportedHead(ctx, store, vm, nil, blockID)
	require.NoError(err)

	// Verify state exists
	loadedID, _, _, err := LoadPersistedState(store)
	require.NoError(err)
	require.Equal(blockID, loadedID)

	// Clear state
	err = ClearPersistedState(store)
	require.NoError(err)

	// Verify state is gone
	loadedID, height, bootstrapped, err := LoadPersistedState(store)
	require.NoError(err)
	require.Equal(ids.Empty, loadedID)
	require.Equal(uint64(0), height)
	require.False(bootstrapped)
}

// TestRestartResilience simulates restart scenario.
// This is the critical test: after RLP import + restart, chain should work.
func TestRestartResilience(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Simulate: RLP import creates a chain at height 10000
	importedBlockID := ids.GenerateTestID()
	importedHeight := uint64(10000)

	// Session 1: Import chain and apply head
	vm1 := newMockVM()
	vm1.blocks[importedBlockID] = &mockBlock{id: importedBlockID, height: importedHeight}

	store := newMockStateStore() // Simulates persistent DB

	err := ApplyImportedHead(ctx, store, vm1, nil, importedBlockID)
	require.NoError(err)

	// Session 2: Simulate restart - new VM, new consensus, but SAME store
	vm2 := newMockVM()
	vm2.blocks[importedBlockID] = &mockBlock{id: importedBlockID, height: importedHeight}
	consensus2 := &mockConsensus{}

	// Load persisted state (this is what startup should do)
	loadedID, loadedHeight, bootstrapped, err := LoadPersistedState(store)
	require.NoError(err)

	// CRITICAL: These assertions prove restart works
	require.Equal(importedBlockID, loadedID, "restart should recover block ID")
	require.Equal(importedHeight, loadedHeight, "restart should recover height")
	require.True(bootstrapped, "restart should know chain is bootstrapped")

	// Initialize consensus with persisted state
	err = consensus2.SyncState(ctx, loadedID, loadedHeight)
	require.NoError(err)

	// Verify consensus is correctly initialized
	require.Equal(importedBlockID, consensus2.syncID)
	require.Equal(importedHeight, consensus2.syncHeight)
}

// TestSyncerWithStateStorePersistError verifies syncer continues on persist error.
func TestSyncerWithStateStorePersistError(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	blockID := ids.GenerateTestID()
	vm := newMockVM()
	vm.lastAcceptedID = blockID
	vm.blocks[blockID] = &mockBlock{id: blockID, height: 100}

	store := newMockStateStore()
	store.putErr = errors.New("disk full")

	consensus := &mockConsensus{}
	doneCalled := false

	config := &Config{
		VM:         vm,
		Consensus:  consensus,
		StateStore: store,
	}
	syncer := New(config, func(ctx context.Context, reqID uint32) error {
		doneCalled = true
		return nil
	})

	// Should NOT fail - should continue with in-memory state
	err := syncer.Start(ctx, 0)
	require.NoError(err)

	// Consensus should still be updated (in-memory)
	require.True(consensus.syncCalled)
	require.True(doneCalled)
}
