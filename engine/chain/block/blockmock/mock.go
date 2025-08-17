// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package blockmock

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// Ensure ChainVM implements block.ChainVM
var _ block.ChainVM = (*ChainVM)(nil)

// ChainVM is a mock implementation of block.ChainVM
type ChainVM struct {
	T                    *testing.T
	CantInitialize       bool
	CantSetState         bool
	CantShutdown         bool
	CantBuildBlock       bool
	CantParseBlock       bool
	CantGetBlock         bool
	CantSetPreference    bool
	CantLastAccepted     bool
	CantVerifyHeightIndex bool
	CantGetBlockIDAtHeight bool

	InitializeF func(context.Context, *block.ChainContext, block.DBManager, []byte, []byte, []byte, chan<- block.Message, []*block.Fx, block.AppSender) error
	SetStateF func(context.Context, uint8) error
	ShutdownF func(context.Context) error
	BuildBlockF func(context.Context) (block.Block, error)
	ParseBlockF func(context.Context, []byte) (block.Block, error)
	GetBlockF func(context.Context, ids.ID) (block.Block, error)
	SetPreferenceF func(context.Context, ids.ID) error
	LastAcceptedF func(context.Context) (ids.ID, error)
	VerifyHeightIndexF func(context.Context) error
	GetBlockIDAtHeightF func(context.Context, uint64) (ids.ID, error)
}

// NewChainVM creates a new ChainVM mock
// Note: ctrl parameter is for gomock compatibility but not used
func NewChainVM(ctrl interface{}) *ChainVM {
	return &ChainVM{}
}

// Initialize with correct signature for block.ChainVM interface
func (vm *ChainVM) Initialize(ctx context.Context, chainCtx *block.ChainContext, dbManager block.DBManager, genesisBytes []byte, upgradeBytes []byte, configBytes []byte, toEngine chan<- block.Message, fxs []*block.Fx, appSender block.AppSender) error {
	if vm.InitializeF != nil {
		return vm.InitializeF(ctx, chainCtx, dbManager, genesisBytes, upgradeBytes, configBytes, toEngine, fxs, appSender)
	}
	if vm.CantInitialize && vm.T != nil {
		vm.T.Fatal("unexpected InitializeChain")
	}
	return nil
}

func (vm *ChainVM) SetState(ctx context.Context, state uint8) error {
	if vm.SetStateF != nil {
		return vm.SetStateF(ctx, state)
	}
	if vm.CantSetState && vm.T != nil {
		vm.T.Fatal("unexpected SetState")
	}
	return nil
}

// Shutdown with no parameters for core.VM interface
func (vm *ChainVM) Shutdown() error {
	if vm.ShutdownF != nil {
		return vm.ShutdownF(nil)
	}
	if vm.CantShutdown && vm.T != nil {
		vm.T.Fatal("unexpected Shutdown")
	}
	return nil
}

// ShutdownContext with context parameter for block.ChainVM interface
func (vm *ChainVM) ShutdownContext(ctx context.Context) error {
	if vm.ShutdownF != nil {
		return vm.ShutdownF(ctx)
	}
	if vm.CantShutdown && vm.T != nil {
		vm.T.Fatal("unexpected ShutdownContext")
	}
	return nil
}

func (vm *ChainVM) BuildBlock(ctx context.Context) (block.Block, error) {
	if vm.BuildBlockF != nil {
		return vm.BuildBlockF(ctx)
	}
	if vm.CantBuildBlock && vm.T != nil {
		vm.T.Fatal("unexpected BuildBlock")
	}
	return nil, nil
}

func (vm *ChainVM) ParseBlock(ctx context.Context, bytes []byte) (block.Block, error) {
	if vm.ParseBlockF != nil {
		return vm.ParseBlockF(ctx, bytes)
	}
	if vm.CantParseBlock && vm.T != nil {
		vm.T.Fatal("unexpected ParseBlock")
	}
	return nil, nil
}

func (vm *ChainVM) GetBlock(ctx context.Context, id ids.ID) (block.Block, error) {
	if vm.GetBlockF != nil {
		return vm.GetBlockF(ctx, id)
	}
	if vm.CantGetBlock && vm.T != nil {
		vm.T.Fatal("unexpected GetBlock")
	}
	return nil, nil
}

func (vm *ChainVM) SetPreference(ctx context.Context, id ids.ID) error {
	if vm.SetPreferenceF != nil {
		return vm.SetPreferenceF(ctx, id)
	}
	if vm.CantSetPreference && vm.T != nil {
		vm.T.Fatal("unexpected SetPreference")
	}
	return nil
}

func (vm *ChainVM) LastAccepted(ctx context.Context) (ids.ID, error) {
	if vm.LastAcceptedF != nil {
		return vm.LastAcceptedF(ctx)
	}
	if vm.CantLastAccepted && vm.T != nil {
		vm.T.Fatal("unexpected LastAccepted")
	}
	return ids.Empty, nil
}

func (vm *ChainVM) VerifyHeightIndex(ctx context.Context) error {
	if vm.VerifyHeightIndexF != nil {
		return vm.VerifyHeightIndexF(ctx)
	}
	if vm.CantVerifyHeightIndex && vm.T != nil {
		vm.T.Fatal("unexpected VerifyHeightIndex")
	}
	return nil
}

func (vm *ChainVM) GetBlockIDAtHeight(ctx context.Context, height uint64) (ids.ID, error) {
	if vm.GetBlockIDAtHeightF != nil {
		return vm.GetBlockIDAtHeightF(ctx, height)
	}
	if vm.CantGetBlockIDAtHeight && vm.T != nil {
		vm.T.Fatal("unexpected GetBlockIDAtHeight")
	}
	return ids.Empty, nil
}

func (vm *ChainVM) CreateHandlers(ctx context.Context) (map[string]http.Handler, error) {
	// Return empty handlers map for mock
	return make(map[string]http.Handler), nil
}

// StateSyncableVM is a mock implementation of block.StateSyncableVM
type StateSyncableVM struct {
	ChainVM
	CantStateSyncEnabled bool
	CantStateSyncGetOngoingSyncStateSummary bool
	CantGetLastStateSummary bool
	CantParseStateSummary bool
	CantGetStateSummary bool

	StateSyncEnabledF func(context.Context) (bool, error)
	StateSyncGetOngoingSyncStateSummaryF func(context.Context) (block.StateSummary, error)
	GetLastStateSummaryF func(context.Context) (block.StateSummary, error)
	ParseStateSummaryF func(context.Context, []byte) (block.StateSummary, error)
	GetStateSummaryF func(context.Context, uint64) (block.StateSummary, error)
}

func (vm *StateSyncableVM) StateSyncEnabled(ctx context.Context) (bool, error) {
	if vm.StateSyncEnabledF != nil {
		return vm.StateSyncEnabledF(ctx)
	}
	if vm.CantStateSyncEnabled && vm.T != nil {
		vm.T.Fatal("unexpected StateSyncEnabled")
	}
	return false, nil
}

func (vm *StateSyncableVM) StateSyncGetOngoingSyncStateSummary(ctx context.Context) (block.StateSummary, error) {
	if vm.StateSyncGetOngoingSyncStateSummaryF != nil {
		return vm.StateSyncGetOngoingSyncStateSummaryF(ctx)
	}
	if vm.CantStateSyncGetOngoingSyncStateSummary && vm.T != nil {
		vm.T.Fatal("unexpected StateSyncGetOngoingSyncStateSummary")
	}
	return nil, nil
}

func (vm *StateSyncableVM) GetLastStateSummary(ctx context.Context) (block.StateSummary, error) {
	if vm.GetLastStateSummaryF != nil {
		return vm.GetLastStateSummaryF(ctx)
	}
	if vm.CantGetLastStateSummary && vm.T != nil {
		vm.T.Fatal("unexpected GetLastStateSummary")
	}
	return nil, nil
}

func (vm *StateSyncableVM) ParseStateSummary(ctx context.Context, bytes []byte) (block.StateSummary, error) {
	if vm.ParseStateSummaryF != nil {
		return vm.ParseStateSummaryF(ctx, bytes)
	}
	if vm.CantParseStateSummary && vm.T != nil {
		vm.T.Fatal("unexpected ParseStateSummary")
	}
	return nil, nil
}

func (vm *StateSyncableVM) GetStateSummary(ctx context.Context, height uint64) (block.StateSummary, error) {
	if vm.GetStateSummaryF != nil {
		return vm.GetStateSummaryF(ctx, height)
	}
	if vm.CantGetStateSummary && vm.T != nil {
		vm.T.Fatal("unexpected GetStateSummary")
	}
	return nil, nil
}

// BuildBlockWithContextChainVM is a mock for block.BuildBlockWithContextChainVM
type BuildBlockWithContextChainVM struct {
	ChainVM
	CantBuildBlockWithContext bool

	BuildBlockWithContextF func(context.Context, *block.Context) (block.Block, error)
}

func (vm *BuildBlockWithContextChainVM) BuildBlockWithContext(ctx context.Context, blockCtx *block.Context) (block.Block, error) {
	if vm.BuildBlockWithContextF != nil {
		return vm.BuildBlockWithContextF(ctx, blockCtx)
	}
	if vm.CantBuildBlockWithContext && vm.T != nil {
		vm.T.Fatal("unexpected BuildBlockWithContext")
	}
	return nil, nil
}

// WithVerifyContext is a mock for block.WithVerifyContext
type WithVerifyContext struct {
	CantShouldVerifyWithContext bool
	CantVerifyWithContext bool

	ShouldVerifyWithContextF func(context.Context) (bool, error)
	VerifyWithContextF func(context.Context, *block.Context) error
}

func (b *WithVerifyContext) ShouldVerifyWithContext(ctx context.Context) (bool, error) {
	if b.ShouldVerifyWithContextF != nil {
		return b.ShouldVerifyWithContextF(ctx)
	}
	return false, nil
}

func (b *WithVerifyContext) VerifyWithContext(ctx context.Context, blockCtx *block.Context) error {
	if b.VerifyWithContextF != nil {
		return b.VerifyWithContextF(ctx, blockCtx)
	}
	return nil
}

// Block is a mock implementation of block.Block
type Block struct {
	T                   *testing.T
	CantID              bool
	CantAccept          bool
	CantReject          bool
	CantStatus          bool
	CantParent          bool
	CantHeight          bool
	CantTimestamp       bool
	CantVerify          bool
	CantBytes           bool

	IDF       func() ids.ID
	AcceptF   func(context.Context) error
	RejectF   func(context.Context) error
	StatusF   func() choices.Status
	ParentF   func() ids.ID
	HeightF   func() uint64
	TimestampF func() time.Time
	VerifyF   func(context.Context) error
	BytesF    func() []byte
}

func (b *Block) ID() ids.ID {
	if b.IDF != nil {
		return b.IDF()
	}
	if b.CantID && b.T != nil {
		b.T.Fatal("unexpected ID")
	}
	return ids.Empty
}

func (b *Block) Accept(ctx context.Context) error {
	if b.AcceptF != nil {
		return b.AcceptF(ctx)
	}
	if b.CantAccept && b.T != nil {
		b.T.Fatal("unexpected Accept")
	}
	return nil
}

func (b *Block) Reject(ctx context.Context) error {
	if b.RejectF != nil {
		return b.RejectF(ctx)
	}
	if b.CantReject && b.T != nil {
		b.T.Fatal("unexpected Reject")
	}
	return nil
}

func (b *Block) Status() choices.Status {
	if b.StatusF != nil {
		return b.StatusF()
	}
	if b.CantStatus && b.T != nil {
		b.T.Fatal("unexpected Status")
	}
	return choices.Unknown
}

func (b *Block) Parent() ids.ID {
	if b.ParentF != nil {
		return b.ParentF()
	}
	if b.CantParent && b.T != nil {
		b.T.Fatal("unexpected Parent")
	}
	return ids.Empty
}

func (b *Block) Height() uint64 {
	if b.HeightF != nil {
		return b.HeightF()
	}
	if b.CantHeight && b.T != nil {
		b.T.Fatal("unexpected Height")
	}
	return 0
}

func (b *Block) Timestamp() time.Time {
	if b.TimestampF != nil {
		return b.TimestampF()
	}
	if b.CantTimestamp && b.T != nil {
		b.T.Fatal("unexpected Timestamp")
	}
	return time.Time{}
}

func (b *Block) Verify(ctx context.Context) error {
	if b.VerifyF != nil {
		return b.VerifyF(ctx)
	}
	if b.CantVerify && b.T != nil {
		b.T.Fatal("unexpected Verify")
	}
	return nil
}

func (b *Block) Bytes() []byte {
	if b.BytesF != nil {
		return b.BytesF()
	}
	if b.CantBytes && b.T != nil {
		b.T.Fatal("unexpected Bytes")
	}
	return nil
}