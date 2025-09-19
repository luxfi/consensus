// Package blockmock provides mock implementations for testing
package blockmock

import (
	"context"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/engine/chain/block"
)

// Block interface for mocking - copy of the parent Block interface
type Block interface {
	ID() ids.ID
	Parent() ids.ID
	ParentID() ids.ID
	Height() uint64
	Timestamp() time.Time
	Status() uint8
	Verify(context.Context) error
	Accept(context.Context) error
	Reject(context.Context) error
	Bytes() []byte
}

// Context provides context for building blocks
type Context struct {
	PChainHeight uint64
}

// MockBlock provides a mock implementation for testing
type MockBlock struct {
	id       ids.ID
	height   uint64
	parent   ids.ID
	accepted bool
}

// NewMockBlock creates a new mock block
func NewMockBlock(id []byte, height uint64, parent []byte) block.Block {
	var idArray, parentArray ids.ID
	copy(idArray[:], id)
	copy(parentArray[:], parent)
	return &MockBlock{
		id:     idArray,
		height: height,
		parent: parentArray,
	}
}

// ID returns the block ID
func (m *MockBlock) ID() ids.ID {
	return m.id
}

// Height returns the block height
func (m *MockBlock) Height() uint64 {
	return m.height
}

// Parent returns the parent block ID (alias for ParentID)
func (m *MockBlock) Parent() ids.ID {
	return m.parent
}

// ParentID returns the parent block ID
func (m *MockBlock) ParentID() ids.ID {
	return m.parent
}

// Accept marks the block as accepted
func (m *MockBlock) Accept(ctx context.Context) error {
	m.accepted = true
	return nil
}

// Reject marks the block as rejected
func (m *MockBlock) Reject(ctx context.Context) error {
	return nil
}

// ChainVM is a mock implementation of a chain VM with configurable functions
type ChainVM struct {
	// Function fields for configurable behavior
	InitializeF       func(context.Context, interface{}, interface{}, []byte, []byte, []byte, interface{}, []interface{}, interface{}) error
	BuildBlockF       func(context.Context) (block.Block, error)
	ParseBlockF       func(context.Context, []byte) (block.Block, error)
	GetBlockF         func(context.Context, ids.ID) (block.Block, error)
	GetBlockIDAtHeightF func(context.Context, uint64) (ids.ID, error)
	SetPreferenceF    func(context.Context, ids.ID) error
	LastAcceptedF     func(context.Context) (ids.ID, error)
}

// NewChainVM creates a new mock chain VM
func NewChainVM() *ChainVM {
	return &ChainVM{}
}

// Status returns the block status
func (m *MockBlock) Status() uint8 {
	if m.accepted {
		return 2 // Accepted
	}
	return 0 // Processing
}

// Timestamp returns the block timestamp
func (m *MockBlock) Timestamp() time.Time {
	return time.Now()
}

// Verify verifies the block
func (m *MockBlock) Verify(ctx context.Context) error {
	return nil
}

// Bytes returns the block bytes
func (m *MockBlock) Bytes() []byte {
	return m.id[:]
}

// Initialize initializes the mock VM
func (vm *ChainVM) Initialize(ctx context.Context, chainCtx interface{}, db interface{}, genesisBytes []byte, upgradeBytes []byte, configBytes []byte, msgChan interface{}, fxs []interface{}, appSender interface{}) error {
	if vm.InitializeF != nil {
		return vm.InitializeF(ctx, chainCtx, db, genesisBytes, upgradeBytes, configBytes, msgChan, fxs, appSender)
	}
	return nil
}

// BuildBlock builds a mock block
func (vm *ChainVM) BuildBlock(ctx context.Context) (block.Block, error) {
	if vm.BuildBlockF != nil {
		return vm.BuildBlockF(ctx)
	}
	return NewMockBlock([]byte{1}, 1, []byte{0}), nil
}

// ParseBlock parses a mock block
func (vm *ChainVM) ParseBlock(ctx context.Context, bytes []byte) (block.Block, error) {
	if vm.ParseBlockF != nil {
		return vm.ParseBlockF(ctx, bytes)
	}
	return NewMockBlock(bytes, 1, []byte{0}), nil
}

// GetBlock gets a mock block
func (vm *ChainVM) GetBlock(ctx context.Context, id ids.ID) (block.Block, error) {
	if vm.GetBlockF != nil {
		return vm.GetBlockF(ctx, id)
	}
	return NewMockBlock(id[:], 1, []byte{0}), nil
}

// GetBlockIDAtHeight gets block ID at height
func (vm *ChainVM) GetBlockIDAtHeight(ctx context.Context, height uint64) (ids.ID, error) {
	if vm.GetBlockIDAtHeightF != nil {
		return vm.GetBlockIDAtHeightF(ctx, height)
	}
	return ids.ID{1}, nil
}

// SetPreference sets the preferred block
func (vm *ChainVM) SetPreference(ctx context.Context, id ids.ID) error {
	if vm.SetPreferenceF != nil {
		return vm.SetPreferenceF(ctx, id)
	}
	return nil
}

// LastAccepted returns the last accepted block
func (vm *ChainVM) LastAccepted(ctx context.Context) (ids.ID, error) {
	if vm.LastAcceptedF != nil {
		return vm.LastAcceptedF(ctx)
	}
	return ids.ID{1}, nil
}

// BuildBlockWithContextChainVM provides a mock implementation
type BuildBlockWithContextChainVM struct {
	ChainVM
}

// BuildBlockWithContext builds a block with context
func (vm *BuildBlockWithContextChainVM) BuildBlockWithContext(ctx context.Context, blockCtx *Context) (Block, error) {
	return NewMockBlock([]byte{1}, 1, []byte{0}), nil
}

// WithVerifyContext provides mock verify context support
type WithVerifyContext struct{}

// VerifyWithContext verifies with context
func (w *WithVerifyContext) VerifyWithContext(ctx context.Context, blockCtx *Context) error {
	return nil
}

// ShouldVerifyWithContext returns whether to verify with context
func (w *WithVerifyContext) ShouldVerifyWithContext(ctx context.Context) (bool, error) {
	return true, nil
}

// StateSyncableVM provides a mock state syncable VM
type StateSyncableVM struct {
	ChainVM
	// Additional function fields for state sync functionality
	StateSyncEnabledF                    func(context.Context) (bool, error)
	StateSyncGetOngoingSyncStateSummaryF func(context.Context) (block.StateSummary, error)
	GetLastStateSummaryF                 func(context.Context) (block.StateSummary, error)
	ParseStateSummaryF                   func(context.Context, []byte) (block.StateSummary, error)
	GetStateSummaryF                     func(context.Context, uint64) (block.StateSummary, error)
	SetStateF                            func(context.Context, uint8) error
}

// StateSyncEnabled returns whether state sync is enabled
func (vm *StateSyncableVM) StateSyncEnabled(ctx context.Context) (bool, error) {
	if vm.StateSyncEnabledF != nil {
		return vm.StateSyncEnabledF(ctx)
	}
	return false, nil
}

// GetOngoingSyncStateSummary returns the ongoing sync state summary
func (vm *StateSyncableVM) GetOngoingSyncStateSummary(ctx context.Context) (block.StateSummary, error) {
	if vm.StateSyncGetOngoingSyncStateSummaryF != nil {
		return vm.StateSyncGetOngoingSyncStateSummaryF(ctx)
	}
	return nil, nil
}

// GetLastStateSummary returns the last state summary
func (vm *StateSyncableVM) GetLastStateSummary(ctx context.Context) (block.StateSummary, error) {
	if vm.GetLastStateSummaryF != nil {
		return vm.GetLastStateSummaryF(ctx)
	}
	return nil, nil
}

// ParseStateSummary parses a state summary
func (vm *StateSyncableVM) ParseStateSummary(ctx context.Context, bytes []byte) (block.StateSummary, error) {
	if vm.ParseStateSummaryF != nil {
		return vm.ParseStateSummaryF(ctx, bytes)
	}
	return nil, nil
}

// GetStateSummary gets a state summary by height
func (vm *StateSyncableVM) GetStateSummary(ctx context.Context, height uint64) (block.StateSummary, error) {
	if vm.GetStateSummaryF != nil {
		return vm.GetStateSummaryF(ctx, height)
	}
	return nil, nil
}

// SetState sets the VM state
func (vm *StateSyncableVM) SetState(ctx context.Context, state uint8) error {
	if vm.SetStateF != nil {
		return vm.SetStateF(ctx, state)
	}
	return nil
}
