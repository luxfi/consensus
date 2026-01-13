// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// mockImportVM simulates a VM that has had blocks imported via RLP.
// It tracks the last accepted block and supports SetPreference.
type mockImportVM struct {
	lastAcceptedID    ids.ID
	lastAcceptedErr   error
	blocks            map[ids.ID]*mockBlock
	getBlockErr       error
	setPreferenceID   ids.ID
	setPreferenceErr  error
	setPreferenceCalls int
	buildBlockErr     error
	parseBlockErr     error
}

func newMockImportVM() *mockImportVM {
	return &mockImportVM{
		blocks: make(map[ids.ID]*mockBlock),
	}
}

func (m *mockImportVM) LastAccepted(ctx context.Context) (ids.ID, error) {
	return m.lastAcceptedID, m.lastAcceptedErr
}

func (m *mockImportVM) GetBlock(ctx context.Context, id ids.ID) (block.Block, error) {
	if m.getBlockErr != nil {
		return nil, m.getBlockErr
	}
	blk, ok := m.blocks[id]
	if !ok {
		return nil, errors.New("block not found")
	}
	return blk, nil
}

func (m *mockImportVM) SetPreference(ctx context.Context, id ids.ID) error {
	m.setPreferenceCalls++
	m.setPreferenceID = id
	return m.setPreferenceErr
}

func (m *mockImportVM) BuildBlock(ctx context.Context) (block.Block, error) {
	return nil, m.buildBlockErr
}

func (m *mockImportVM) ParseBlock(ctx context.Context, data []byte) (block.Block, error) {
	return nil, m.parseBlockErr
}

// TestOnImportComplete_BasicSync verifies that OnImportComplete correctly
// syncs consensus state after an RLP import.
func TestOnImportComplete_BasicSync(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create a block at height 1000 (simulating RLP import result)
	importedBlockID := ids.GenerateTestID()
	importedBlock := &mockBlock{
		id:        importedBlockID,
		parentID:  ids.GenerateTestID(),
		height:    1000,
		timestamp: time.Now(),
	}

	// Setup mock VM with imported state
	vm := newMockImportVM()
	vm.lastAcceptedID = importedBlockID
	vm.blocks[importedBlockID] = importedBlock

	// Create runtime with mock VM
	rt := NewRuntime(NetworkConfig{
		ChainID:   ids.GenerateTestID(),
		NetworkID: ids.GenerateTestID(),
		VM:        vm,
	})

	// Before sync: engine not bootstrapped (not started)
	require.False(rt.IsBootstrapped(), "Engine should not be bootstrapped before sync")

	// Call OnImportComplete
	err := rt.OnImportComplete(ctx)
	require.NoError(err, "OnImportComplete should succeed")

	// Verify consensus is now bootstrapped
	require.True(rt.IsBootstrapped(), "Engine should be bootstrapped after OnImportComplete")

	// Verify VM preference was set
	require.Equal(1, vm.setPreferenceCalls, "SetPreference should be called once")
	require.Equal(importedBlockID, vm.setPreferenceID, "SetPreference should use imported block ID")

	// Verify consensus state was updated
	require.Equal(importedBlockID, rt.consensus.GetFinalizedTip(),
		"Consensus finalizedTip should match imported block")
}

// TestOnImportComplete_HighHeight tests sync with a very high block height
// (simulating importing a mature chain).
func TestOnImportComplete_HighHeight(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create a block at height 10 million
	importedBlockID := ids.GenerateTestID()
	importedBlock := &mockBlock{
		id:        importedBlockID,
		parentID:  ids.GenerateTestID(),
		height:    10_000_000,
		timestamp: time.Now(),
	}

	vm := newMockImportVM()
	vm.lastAcceptedID = importedBlockID
	vm.blocks[importedBlockID] = importedBlock

	rt := NewRuntime(NetworkConfig{
		ChainID:   ids.GenerateTestID(),
		NetworkID: ids.GenerateTestID(),
		VM:        vm,
	})

	err := rt.OnImportComplete(ctx)
	require.NoError(err)
	require.True(rt.IsBootstrapped())
	require.Equal(importedBlockID, rt.consensus.GetFinalizedTip())
}

// TestOnImportComplete_EmptyBlock tests sync when VM returns empty block ID
// (e.g., genesis-only chain).
func TestOnImportComplete_EmptyBlock(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	vm := newMockImportVM()
	vm.lastAcceptedID = ids.Empty // No blocks imported yet

	rt := NewRuntime(NetworkConfig{
		ChainID:   ids.GenerateTestID(),
		NetworkID: ids.GenerateTestID(),
		VM:        vm,
	})

	err := rt.OnImportComplete(ctx)
	require.NoError(err)
	require.True(rt.IsBootstrapped(), "Should be bootstrapped even with empty last accepted")
}

// TestOnImportComplete_VMError tests that VM errors are handled gracefully.
func TestOnImportComplete_VMError(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	vm := newMockImportVM()
	vm.lastAcceptedErr = errors.New("database error")

	rt := NewRuntime(NetworkConfig{
		ChainID:   ids.GenerateTestID(),
		NetworkID: ids.GenerateTestID(),
		VM:        vm,
	})

	err := rt.OnImportComplete(ctx)
	require.Error(err, "Should return error when VM fails")
}

// TestOnImportComplete_GetBlockError tests that GetBlock errors are non-fatal.
func TestOnImportComplete_GetBlockError(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	importedBlockID := ids.GenerateTestID()
	vm := newMockImportVM()
	vm.lastAcceptedID = importedBlockID
	vm.getBlockErr = errors.New("block not found") // GetBlock fails

	rt := NewRuntime(NetworkConfig{
		ChainID:   ids.GenerateTestID(),
		NetworkID: ids.GenerateTestID(),
		VM:        vm,
	})

	// Should still succeed - GetBlock failure is non-fatal
	err := rt.OnImportComplete(ctx)
	require.NoError(err)
	require.True(rt.IsBootstrapped())
	require.Equal(importedBlockID, vm.setPreferenceID, "SetPreference should still be called")
}

// TestOnImportComplete_SetPreferenceError tests that SetPreference errors are non-fatal.
func TestOnImportComplete_SetPreferenceError(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	importedBlockID := ids.GenerateTestID()
	importedBlock := &mockBlock{
		id:     importedBlockID,
		height: 500,
	}

	vm := newMockImportVM()
	vm.lastAcceptedID = importedBlockID
	vm.blocks[importedBlockID] = importedBlock
	vm.setPreferenceErr = errors.New("preference error") // SetPreference fails

	rt := NewRuntime(NetworkConfig{
		ChainID:   ids.GenerateTestID(),
		NetworkID: ids.GenerateTestID(),
		VM:        vm,
	})

	// Should still succeed - SetPreference failure is non-fatal
	err := rt.OnImportComplete(ctx)
	require.NoError(err)
	require.True(rt.IsBootstrapped())
}

// TestOnImportComplete_Idempotent tests that calling OnImportComplete multiple
// times is safe and idempotent.
func TestOnImportComplete_Idempotent(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	importedBlockID := ids.GenerateTestID()
	importedBlock := &mockBlock{
		id:     importedBlockID,
		height: 100,
	}

	vm := newMockImportVM()
	vm.lastAcceptedID = importedBlockID
	vm.blocks[importedBlockID] = importedBlock

	rt := NewRuntime(NetworkConfig{
		ChainID:   ids.GenerateTestID(),
		NetworkID: ids.GenerateTestID(),
		VM:        vm,
	})

	// Call multiple times
	for i := 0; i < 5; i++ {
		err := rt.OnImportComplete(ctx)
		require.NoError(err)
		require.True(rt.IsBootstrapped())
		require.Equal(importedBlockID, rt.consensus.GetFinalizedTip())
	}

	// SetPreference called each time
	require.Equal(5, vm.setPreferenceCalls)
}

// TestOnImportComplete_NilVM tests that nil VM is handled gracefully.
func TestOnImportComplete_NilVM(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	rt := NewRuntime(NetworkConfig{
		ChainID:   ids.GenerateTestID(),
		NetworkID: ids.GenerateTestID(),
		VM:        nil, // No VM
	})

	err := rt.OnImportComplete(ctx)
	require.NoError(err, "Should succeed with nil VM")
}

// TestOnImportComplete_AfterStart tests sync after engine has already started.
func TestOnImportComplete_AfterStart(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Start with genesis
	genesisID := ids.GenerateTestID()
	genesisBlock := &mockBlock{
		id:     genesisID,
		height: 0,
	}

	vm := newMockImportVM()
	vm.lastAcceptedID = genesisID
	vm.blocks[genesisID] = genesisBlock

	rt := NewRuntime(NetworkConfig{
		ChainID:   ids.GenerateTestID(),
		NetworkID: ids.GenerateTestID(),
		VM:        vm,
	})

	// Start the engine
	err := rt.Start(ctx, true)
	require.NoError(err)
	require.True(rt.IsBootstrapped())

	// Now simulate RLP import - update VM state to new block
	importedBlockID := ids.GenerateTestID()
	importedBlock := &mockBlock{
		id:       importedBlockID,
		parentID: genesisID,
		height:   5000,
	}
	vm.lastAcceptedID = importedBlockID
	vm.blocks[importedBlockID] = importedBlock

	// Sync after import
	vm.setPreferenceCalls = 0 // Reset counter
	err = rt.OnImportComplete(ctx)
	require.NoError(err)
	require.True(rt.IsBootstrapped())
	require.Equal(importedBlockID, rt.consensus.GetFinalizedTip())
	require.Equal(importedBlockID, vm.setPreferenceID)
}

// TestSyncStateFromVM tests the standalone sync function.
func TestSyncStateFromVM(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	importedBlockID := ids.GenerateTestID()
	importedBlock := &mockBlock{
		id:     importedBlockID,
		height: 750,
	}

	vm := newMockImportVM()
	vm.lastAcceptedID = importedBlockID
	vm.blocks[importedBlockID] = importedBlock

	engine := New()

	blockID, height, err := SyncStateFromVM(ctx, vm, engine)
	require.NoError(err)
	require.Equal(importedBlockID, blockID)
	require.Equal(uint64(750), height)
	require.True(engine.IsBootstrapped())
}

// TestSyncStateFromVM_NilConsensus tests sync with nil consensus engine.
func TestSyncStateFromVM_NilConsensus(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	importedBlockID := ids.GenerateTestID()
	importedBlock := &mockBlock{
		id:     importedBlockID,
		height: 100,
	}

	vm := newMockImportVM()
	vm.lastAcceptedID = importedBlockID
	vm.blocks[importedBlockID] = importedBlock

	blockID, height, err := SyncStateFromVM(ctx, vm, nil)
	require.NoError(err)
	require.Equal(importedBlockID, blockID)
	require.Equal(uint64(100), height)
}

// TestOnImportComplete_ClearsPendingBlocks tests that pending blocks below
// the synced height are cleared.
func TestOnImportComplete_ClearsPendingBlocks(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Setup blocks
	oldBlockID := ids.GenerateTestID()
	oldBlock := &mockBlock{id: oldBlockID, height: 50}

	importedBlockID := ids.GenerateTestID()
	importedBlock := &mockBlock{id: importedBlockID, height: 1000}

	vm := newMockImportVM()
	vm.lastAcceptedID = importedBlockID
	vm.blocks[importedBlockID] = importedBlock

	rt := NewRuntime(NetworkConfig{
		ChainID:   ids.GenerateTestID(),
		NetworkID: ids.GenerateTestID(),
		VM:        vm,
	})

	// Add a pending block at old height
	rt.Transitive.pendingBlocks[oldBlockID] = &PendingBlock{
		ConsensusBlock: &Block{id: oldBlockID, height: 50},
		VMBlock:        oldBlock,
	}

	// Sync should clear old pending blocks
	err := rt.OnImportComplete(ctx)
	require.NoError(err)

	// Old pending block should be cleared
	_, exists := rt.Transitive.pendingBlocks[oldBlockID]
	require.False(exists, "Old pending blocks should be cleared after sync")
}

// TestOnImportComplete_PreservesNewPendingBlocks tests that pending blocks
// above the synced height are preserved.
func TestOnImportComplete_PreservesNewPendingBlocks(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Setup blocks
	newBlockID := ids.GenerateTestID()
	newBlock := &mockBlock{id: newBlockID, height: 1050}

	importedBlockID := ids.GenerateTestID()
	importedBlock := &mockBlock{id: importedBlockID, height: 1000}

	vm := newMockImportVM()
	vm.lastAcceptedID = importedBlockID
	vm.blocks[importedBlockID] = importedBlock

	rt := NewRuntime(NetworkConfig{
		ChainID:   ids.GenerateTestID(),
		NetworkID: ids.GenerateTestID(),
		VM:        vm,
	})

	// Add a pending block ABOVE synced height
	rt.Transitive.pendingBlocks[newBlockID] = &PendingBlock{
		ConsensusBlock: &Block{id: newBlockID, height: 1050},
		VMBlock:        newBlock,
	}

	// Sync should preserve new pending blocks
	err := rt.OnImportComplete(ctx)
	require.NoError(err)

	// New pending block should be preserved
	_, exists := rt.Transitive.pendingBlocks[newBlockID]
	require.True(exists, "New pending blocks should be preserved after sync")
}
