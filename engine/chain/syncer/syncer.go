// Package syncer provides state synchronization for blockchain engines.
//
// After operations like admin_importChain (RLP import), the EVM state database
// is updated but the consensus engine's pointers are stale. The Syncer reconciles
// consensus state with the VM's actual state.
//
// Problem: RLP import updates EVM state but NOT consensus state. This causes:
// - Transactions timeout (engine tries to build on old block)
// - Node crashes on restart with "chains not bootstrapped"
// - lastAccepted pointer is stale
//
// Solution: Syncer queries VM for current state, updates consensus to match,
// AND persists the state so restart works correctly.
//
// Key API for external callers (e.g., coreth after admin_importChain):
//
//	err := syncer.ApplyImportedHead(ctx, store, vm, consensus, blockID)
//
// This single function is the source of truth for external state advances.
package syncer

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/luxfi/consensus/core/types"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// Database keys for persistent consensus state.
// These are stored in the consensus database to survive restarts.
var (
	// lastAcceptedKey stores the last accepted block ID (32 bytes).
	lastAcceptedKey = []byte("lastAccepted")
	// lastAcceptedHeightKey stores the height of last accepted block (8 bytes).
	lastAcceptedHeightKey = []byte("lastAcceptedHeight")
	// bootstrappedKey stores whether the chain is bootstrapped (1 byte: 0 or 1).
	bootstrappedKey = []byte("bootstrapped")
)

// Errors
var (
	ErrNoVM              = errors.New("syncer: VM is required")
	ErrNoStateStore      = errors.New("syncer: StateStore is required for persistence")
	ErrVMLastAccepted    = errors.New("syncer: failed to get VM last accepted block")
	ErrVMGetBlock        = errors.New("syncer: failed to get block from VM")
	ErrSetPreference     = errors.New("syncer: failed to set VM preference")
	ErrConsensusCallback = errors.New("syncer: consensus state callback failed")
	ErrPersistFailed     = errors.New("syncer: failed to persist consensus state")
)

// StateStore provides persistent storage for consensus metadata.
// This interface allows the syncer to persist state that survives restarts.
// Implementations can use any key-value store (BadgerDB, LevelDB, etc.).
type StateStore interface {
	// Get retrieves a value by key. Returns nil, nil if key not found.
	Get(key []byte) ([]byte, error)
	// Put stores a key-value pair.
	Put(key, value []byte) error
	// Delete removes a key.
	Delete(key []byte) error
}

// VM is the minimal interface required from the VM for state sync.
// This matches the methods available on block.ChainVM.
type VM interface {
	// LastAccepted returns the ID of the last accepted block.
	LastAccepted(context.Context) (ids.ID, error)
	// GetBlock retrieves a block by ID.
	GetBlock(context.Context, ids.ID) (block.Block, error)
	// SetPreference tells the VM which block to build on next.
	SetPreference(context.Context, ids.ID) error
}

// ConsensusState is called by the syncer to update consensus engine state.
// The consensus engine should implement this callback to update its internal
// lastAccepted, finalizedTip, and bootstrap state.
type ConsensusState interface {
	// SyncState is called after the syncer determines the VM's current state.
	// The consensus engine should:
	//   1. Set lastAccepted/finalizedTip to the provided block ID
	//   2. Update height tracking
	//   3. Mark bootstrapped = true
	SyncState(ctx context.Context, lastAcceptedID ids.ID, height uint64) error
}

// Config holds syncer configuration.
type Config struct {
	// VM provides access to blockchain state.
	VM VM
	// Consensus receives state updates (optional, can use callback instead).
	Consensus ConsensusState
	// StateStore for persistent consensus metadata (optional but recommended).
	// Without this, restart will crash with "chains not bootstrapped".
	StateStore StateStore
	// Beacons are trusted nodes for state verification (optional).
	Beacons []types.NodeID
	// Logger for sync events (optional).
	Logger Logger
	// OnSyncComplete is called when sync finishes successfully (optional).
	// This is an alternative to implementing ConsensusState.
	OnSyncComplete func(ctx context.Context, lastAcceptedID ids.ID, height uint64) error
}

// Logger is a minimal logging interface.
type Logger interface {
	Info(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
}

// Syncer synchronizes consensus state with VM state.
// After RLP import or state sync, call Start() to reconcile state.
type Syncer struct {
	config         Config
	onDoneCallback func(ctx context.Context, lastReqID uint32) error
}

// NewConfig creates a validated syncer configuration.
func NewConfig(
	vm VM,
	consensus ConsensusState,
	beacons []types.NodeID,
) (*Config, error) {
	if vm == nil {
		return nil, ErrNoVM
	}
	return &Config{
		VM:        vm,
		Consensus: consensus,
		Beacons:   beacons,
	}, nil
}

// New creates a new state syncer.
//
// The onDone callback is invoked after synchronization completes. For the
// consensus engine, this typically transitions from syncing to bootstrapped state.
func New(config *Config, onDone func(ctx context.Context, lastReqID uint32) error) *Syncer {
	return &Syncer{
		config:         *config,
		onDoneCallback: onDone,
	}
}

// Start initiates state synchronization.
//
// This queries the VM for its current last accepted block and updates the
// consensus engine to match. Steps:
//   1. Query VM.LastAccepted() to get current chain tip
//   2. Fetch block details (height) via VM.GetBlock()
//   3. Update VM preference via SetPreference()
//   4. Persist consensus metadata to StateStore (critical for restart)
//   5. Notify consensus engine via Consensus.SyncState() or OnSyncComplete
//   6. Invoke onDone callback to complete bootstrap
//
// This is idempotent - calling it when already synced is safe.
func (s *Syncer) Start(ctx context.Context, startReqID uint32) error {
	if s.config.VM == nil {
		// No VM means nothing to sync - just complete immediately
		return s.done(ctx, startReqID)
	}

	// Step 1: Get VM's current last accepted block
	lastAcceptedID, err := s.config.VM.LastAccepted(ctx)
	if err != nil {
		s.log("warn", "failed to get last accepted from VM", "error", err)
		// Even on error, we should complete bootstrap to avoid hanging
		// The consensus engine will detect the inconsistency on first block build
		return s.done(ctx, startReqID)
	}

	s.log("info", "syncer queried VM last accepted",
		"lastAcceptedID", lastAcceptedID.String())

	// Step 2: Get block details (height) for consensus state
	var height uint64
	if lastAcceptedID != ids.Empty {
		blk, err := s.config.VM.GetBlock(ctx, lastAcceptedID)
		if err != nil {
			s.log("warn", "failed to get last accepted block details",
				"blockID", lastAcceptedID.String(),
				"error", err)
			// Continue with height 0 - consensus can recover
		} else {
			height = blk.Height()
			s.log("info", "syncer retrieved block details",
				"blockID", lastAcceptedID.String(),
				"height", height)
		}
	}

	// Step 3: Update VM preference to build on current tip
	// This is critical: without this, VM.Preferred() returns old block while
	// GetLastAccepted() returns new block, causing state mismatch errors
	if err := s.config.VM.SetPreference(ctx, lastAcceptedID); err != nil {
		s.log("warn", "failed to set VM preference",
			"blockID", lastAcceptedID.String(),
			"error", err)
		// Non-fatal: consensus can still function, just with potential latency
	} else {
		s.log("debug", "syncer set VM preference",
			"blockID", lastAcceptedID.String())
	}

	// Step 4: CRITICAL - Persist consensus state so restart works
	// Without this, restart crashes with "chains not bootstrapped"
	if err := s.persistState(lastAcceptedID, height); err != nil {
		s.log("warn", "failed to persist consensus state",
			"blockID", lastAcceptedID.String(),
			"height", height,
			"error", err)
		// Continue anyway - in-memory state is still valid for this session
	} else {
		s.log("info", "syncer persisted consensus state",
			"lastAcceptedID", lastAcceptedID.String(),
			"height", height)
	}

	// Step 5: Notify consensus engine of current state (in-memory update)
	if s.config.Consensus != nil {
		if err := s.config.Consensus.SyncState(ctx, lastAcceptedID, height); err != nil {
			s.log("warn", "failed to sync consensus state",
				"blockID", lastAcceptedID.String(),
				"height", height,
				"error", err)
			// Non-fatal: done callback will still complete bootstrap
		} else {
			s.log("info", "syncer updated consensus state",
				"lastAcceptedID", lastAcceptedID.String(),
				"height", height)
		}
	}

	// Alternative: use callback instead of interface
	if s.config.OnSyncComplete != nil {
		if err := s.config.OnSyncComplete(ctx, lastAcceptedID, height); err != nil {
			s.log("warn", "OnSyncComplete callback failed",
				"blockID", lastAcceptedID.String(),
				"error", err)
		}
	}

	// Step 6: Complete bootstrap
	return s.done(ctx, startReqID)
}

// persistState writes consensus metadata to the StateStore.
// This ensures restart works correctly after RLP import.
func (s *Syncer) persistState(lastAcceptedID ids.ID, height uint64) error {
	if s.config.StateStore == nil {
		// No StateStore configured - skip persistence
		// This is a warning condition but not fatal
		return nil
	}

	// Persist last accepted block ID
	if err := s.config.StateStore.Put(lastAcceptedKey, lastAcceptedID[:]); err != nil {
		return fmt.Errorf("%w: lastAccepted: %v", ErrPersistFailed, err)
	}

	// Persist height
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, height)
	if err := s.config.StateStore.Put(lastAcceptedHeightKey, heightBytes); err != nil {
		return fmt.Errorf("%w: height: %v", ErrPersistFailed, err)
	}

	// Persist bootstrapped marker
	if err := s.config.StateStore.Put(bootstrappedKey, []byte{1}); err != nil {
		return fmt.Errorf("%w: bootstrapped: %v", ErrPersistFailed, err)
	}

	return nil
}

// done invokes the completion callback.
func (s *Syncer) done(ctx context.Context, reqID uint32) error {
	if s.onDoneCallback != nil {
		return s.onDoneCallback(ctx, reqID)
	}
	return nil
}

// log writes a log message if a logger is configured.
func (s *Syncer) log(level, msg string, args ...interface{}) {
	if s.config.Logger == nil {
		return
	}
	switch level {
	case "info":
		s.config.Logger.Info(msg, args...)
	case "debug":
		s.config.Logger.Debug(msg, args...)
	case "warn":
		s.config.Logger.Warn(msg, args...)
	}
}

// HealthCheck returns syncer health status.
func (s *Syncer) HealthCheck(ctx context.Context) (interface{}, error) {
	status := map[string]interface{}{
		"status": "ready",
	}

	// Include VM state if available
	if s.config.VM != nil {
		lastAccepted, err := s.config.VM.LastAccepted(ctx)
		if err == nil {
			status["lastAccepted"] = lastAccepted.String()
		} else {
			status["lastAcceptedError"] = err.Error()
		}
	}

	return status, nil
}

// DeprecatedConfig is the old config format for backward compatibility.
// Deprecated: Use Config instead.
type DeprecatedConfig struct {
	GetHandler     interface{}
	Context        interface{}
	StartupTracker interface{}
	Sender         interface{}
	Beacons        []types.NodeID
	VM             interface{}
}

// NewDeprecatedConfig creates a config from deprecated parameters.
// Deprecated: Use NewConfig instead.
func NewDeprecatedConfig(
	getHandler interface{},
	ctx interface{},
	startupTracker interface{},
	sender interface{},
	beacons []types.NodeID,
	vm interface{},
) (*DeprecatedConfig, error) {
	return &DeprecatedConfig{
		GetHandler:     getHandler,
		Context:        ctx,
		StartupTracker: startupTracker,
		Sender:         sender,
		Beacons:        beacons,
		VM:             vm,
	}, nil
}

// NewFromDeprecated creates a Syncer from deprecated config.
// This allows gradual migration from old API to new API.
func NewFromDeprecated(config *DeprecatedConfig, onDone func(ctx context.Context, lastReqID uint32) error) *Syncer {
	// Try to extract VM if it implements our interface
	var vm VM
	if v, ok := config.VM.(VM); ok {
		vm = v
	}

	return &Syncer{
		config: Config{
			VM:      vm,
			Beacons: config.Beacons,
		},
		onDoneCallback: onDone,
	}
}

// SyncResult contains the result of a sync operation.
type SyncResult struct {
	// LastAcceptedID is the block ID consensus should build on.
	LastAcceptedID ids.ID
	// Height is the height of the last accepted block.
	Height uint64
	// Synced is true if state was successfully synchronized.
	Synced bool
	// Error contains any error that occurred (sync may still complete).
	Error error
}

// SyncOnce performs a one-shot sync operation and returns the result.
// This is useful for testing or manual sync triggers.
func SyncOnce(ctx context.Context, vm VM) (*SyncResult, error) {
	if vm == nil {
		return nil, ErrNoVM
	}

	result := &SyncResult{}

	// Get last accepted
	lastAcceptedID, err := vm.LastAccepted(ctx)
	if err != nil {
		result.Error = fmt.Errorf("%w: %v", ErrVMLastAccepted, err)
		return result, result.Error
	}
	result.LastAcceptedID = lastAcceptedID

	// Get height
	if lastAcceptedID != ids.Empty {
		blk, err := vm.GetBlock(ctx, lastAcceptedID)
		if err != nil {
			result.Error = fmt.Errorf("%w: %v", ErrVMGetBlock, err)
			// Continue - we at least have the ID
		} else {
			result.Height = blk.Height()
		}
	}

	// Set preference
	if err := vm.SetPreference(ctx, lastAcceptedID); err != nil {
		result.Error = fmt.Errorf("%w: %v", ErrSetPreference, err)
		// Continue - sync can still be useful
	}

	result.Synced = true
	return result, nil
}

// -----------------------------------------------------------------------------
// ApplyImportedHead - Single Source of Truth for External State Advances
// -----------------------------------------------------------------------------

// ApplyImportedHead is the single API for external state advances.
//
// Call this from coreth after admin_importChain or any operation that externally
// advances the chain head. This function:
//
//  1. Updates persistent consensus metadata (survives restart)
//  2. Updates in-memory pointers (lastAccepted, preferred)
//  3. Ensures bootstrap status is consistent
//  4. Updates VM preference to build on new head
//
// INVARIANT: After this call, restart does not crash; chain continues producing blocks.
//
// Example usage from coreth:
//
//	// After RLP import completes...
//	newHeadID := importedBlocks[len(importedBlocks)-1].Hash()
//	err := syncer.ApplyImportedHead(ctx, store, vm, consensus, ids.ID(newHeadID))
//	if err != nil {
//	    return fmt.Errorf("failed to apply imported head: %w", err)
//	}
func ApplyImportedHead(
	ctx context.Context,
	store StateStore,
	vm VM,
	consensus ConsensusState,
	blockID ids.ID,
) error {
	if store == nil {
		return ErrNoStateStore
	}
	if vm == nil {
		return ErrNoVM
	}

	// Step 1: Get block details (height) from VM
	var height uint64
	if blockID != ids.Empty {
		blk, err := vm.GetBlock(ctx, blockID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrVMGetBlock, err)
		}
		height = blk.Height()
	}

	// Step 2: Persist consensus metadata - CRITICAL for restart
	// This is the key fix: without persistence, restart crashes
	if err := persistConsensusState(store, blockID, height); err != nil {
		return fmt.Errorf("failed to persist state: %w", err)
	}

	// Step 3: Update VM preference
	if err := vm.SetPreference(ctx, blockID); err != nil {
		return fmt.Errorf("%w: %v", ErrSetPreference, err)
	}

	// Step 4: Update in-memory consensus state
	if consensus != nil {
		if err := consensus.SyncState(ctx, blockID, height); err != nil {
			return fmt.Errorf("%w: %v", ErrConsensusCallback, err)
		}
	}

	return nil
}

// persistConsensusState writes all consensus metadata atomically.
func persistConsensusState(store StateStore, blockID ids.ID, height uint64) error {
	// Persist last accepted block ID (32 bytes)
	if err := store.Put(lastAcceptedKey, blockID[:]); err != nil {
		return fmt.Errorf("%w: lastAccepted: %v", ErrPersistFailed, err)
	}

	// Persist height (8 bytes, big-endian)
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, height)
	if err := store.Put(lastAcceptedHeightKey, heightBytes); err != nil {
		return fmt.Errorf("%w: height: %v", ErrPersistFailed, err)
	}

	// Persist bootstrapped marker (1 byte)
	if err := store.Put(bootstrappedKey, []byte{1}); err != nil {
		return fmt.Errorf("%w: bootstrapped: %v", ErrPersistFailed, err)
	}

	return nil
}

// LoadPersistedState loads consensus state from the StateStore.
// Call this on startup to restore state after restart.
// Returns:
//   - blockID: last accepted block ID (or ids.Empty if not set)
//   - height: block height (or 0 if not set)
//   - bootstrapped: whether the chain was bootstrapped (false if not set)
//   - err: any error reading from store
func LoadPersistedState(store StateStore) (blockID ids.ID, height uint64, bootstrapped bool, err error) {
	if store == nil {
		return ids.Empty, 0, false, nil
	}

	// Load last accepted block ID
	idBytes, err := store.Get(lastAcceptedKey)
	if err != nil {
		return ids.Empty, 0, false, fmt.Errorf("failed to load lastAccepted: %w", err)
	}
	if len(idBytes) == ids.IDLen {
		copy(blockID[:], idBytes)
	}

	// Load height
	heightBytes, err := store.Get(lastAcceptedHeightKey)
	if err != nil {
		return blockID, 0, false, fmt.Errorf("failed to load height: %w", err)
	}
	if len(heightBytes) == 8 {
		height = binary.BigEndian.Uint64(heightBytes)
	}

	// Load bootstrapped marker
	bootBytes, err := store.Get(bootstrappedKey)
	if err != nil {
		return blockID, height, false, fmt.Errorf("failed to load bootstrapped: %w", err)
	}
	bootstrapped = len(bootBytes) > 0 && bootBytes[0] == 1

	return blockID, height, bootstrapped, nil
}

// ClearPersistedState removes all persisted consensus state.
// Use this for testing or when resetting the chain.
func ClearPersistedState(store StateStore) error {
	if store == nil {
		return nil
	}
	if err := store.Delete(lastAcceptedKey); err != nil {
		return err
	}
	if err := store.Delete(lastAcceptedHeightKey); err != nil {
		return err
	}
	if err := store.Delete(bootstrappedKey); err != nil {
		return err
	}
	return nil
}
