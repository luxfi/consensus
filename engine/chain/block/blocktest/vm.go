// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package blocktest

import (
	"context"
	"errors"
	"time"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/consensus/protocol/chain"
	"github.com/luxfi/ids"
)

var (
	errBuildBlock  = errors.New("unexpectedly called BuildBlock")
	errParseBlock  = errors.New("unexpectedly called ParseBlock")
	errGetBlock    = errors.New("unexpectedly called GetBlock")
	errSetState    = errors.New("unexpectedly called SetState")
	errLastAccepted = errors.New("unexpectedly called LastAccepted")
)

// VM is a test VM
type VM struct {
	block.ChainVM

	CantBuildBlock,
	CantParseBlock,
	CantGetBlock,
	CantSetState,
	CantSetPreference,
	CantLastAccepted,
	CantVerifyHeightIndex,
	CantGetBlockIDAtHeight bool

	BuildBlockF            func(context.Context) (chain.Block, error)
	ParseBlockF            func(context.Context, []byte) (chain.Block, error)
	GetBlockF              func(context.Context, ids.ID) (chain.Block, error)
	SetStateF              func(context.Context, interfaces.State) error
	SetPreferenceF         func(context.Context, ids.ID) error
	LastAcceptedF          func(context.Context) (ids.ID, error)
	VerifyHeightIndexF     func(context.Context) error
	GetBlockIDAtHeightF    func(context.Context, uint64) (ids.ID, error)
}

func (vm *VM) BuildBlock(ctx context.Context) (chain.Block, error) {
	if vm.BuildBlockF != nil {
		return vm.BuildBlockF(ctx)
	}
	if vm.CantBuildBlock {
		return nil, errBuildBlock
	}
	return nil, nil
}

func (vm *VM) ParseBlock(ctx context.Context, b []byte) (chain.Block, error) {
	if vm.ParseBlockF != nil {
		return vm.ParseBlockF(ctx, b)
	}
	if vm.CantParseBlock {
		return nil, errParseBlock
	}
	return nil, nil
}

func (vm *VM) GetBlock(ctx context.Context, blkID ids.ID) (chain.Block, error) {
	if vm.GetBlockF != nil {
		return vm.GetBlockF(ctx, blkID)
	}
	if vm.CantGetBlock {
		return nil, errGetBlock
	}
	return nil, nil
}

func (vm *VM) SetState(ctx context.Context, state interfaces.State) error {
	if vm.SetStateF != nil {
		return vm.SetStateF(ctx, state)
	}
	if vm.CantSetState {
		return errSetState
	}
	return nil
}

func (vm *VM) SetPreference(ctx context.Context, blkID ids.ID) error {
	if vm.SetPreferenceF != nil {
		return vm.SetPreferenceF(ctx, blkID)
	}
	if vm.CantSetPreference {
		return errors.New("unexpectedly called SetPreference")
	}
	return nil
}

func (vm *VM) LastAccepted(ctx context.Context) (ids.ID, error) {
	if vm.LastAcceptedF != nil {
		return vm.LastAcceptedF(ctx)
	}
	if vm.CantLastAccepted {
		return ids.Empty, errLastAccepted
	}
	return ids.Empty, nil
}

func (vm *VM) VerifyHeightIndex(ctx context.Context) error {
	if vm.VerifyHeightIndexF != nil {
		return vm.VerifyHeightIndexF(ctx)
	}
	if vm.CantVerifyHeightIndex {
		return errors.New("unexpectedly called VerifyHeightIndex")
	}
	return nil
}

func (vm *VM) GetBlockIDAtHeight(ctx context.Context, height uint64) (ids.ID, error) {
	if vm.GetBlockIDAtHeightF != nil {
		return vm.GetBlockIDAtHeightF(ctx, height)
	}
	if vm.CantGetBlockIDAtHeight {
		return ids.Empty, errors.New("unexpectedly called GetBlockIDAtHeight")
	}
	return ids.Empty, nil
}

// BatchedVM is a test implementation
type BatchedVM struct {
	VM

	CantGetAncestors,
	CantBatchParseBlock bool

	GetAncestorsF    func(context.Context, ids.ID, int, int, int) ([][]byte, error)
	BatchParseBlockF func(context.Context, [][]byte) ([]chain.Block, error)
}

func (vm *BatchedVM) GetAncestors(
	ctx context.Context,
	blkID ids.ID,
	maxBlocksNum int,
	maxBlocksSize int,
	maxBlocksRetrievalTime int,
) ([][]byte, error) {
	if vm.GetAncestorsF != nil {
		return vm.GetAncestorsF(ctx, blkID, maxBlocksNum, maxBlocksSize, maxBlocksRetrievalTime)
	}
	if vm.CantGetAncestors {
		return nil, errors.New("unexpectedly called GetAncestors")
	}
	return nil, nil
}

func (vm *BatchedVM) BatchParseBlock(ctx context.Context, blks [][]byte) ([]chain.Block, error) {
	if vm.BatchParseBlockF != nil {
		return vm.BatchParseBlockF(ctx, blks)
	}
	if vm.CantBatchParseBlock {
		return nil, errors.New("unexpectedly called BatchParseBlock")
	}
	return nil, nil
}

// StateSyncableVM is a test implementation
type StateSyncableVM struct {
	VM

	CantStateSyncEnabled,
	CantStateSyncGetOngoingSyncStateSummary,
	CantGetLastStateSummary,
	CantParseStateSummary,
	CantGetStateSummary bool

	StateSyncEnabledF                     func(context.Context) (bool, error)
	StateSyncGetOngoingSyncStateSummaryF func(context.Context) (StateSummary, error)
	GetLastStateSummaryF                  func(context.Context) (StateSummary, error)
	ParseStateSummaryF                    func(context.Context, []byte) (StateSummary, error)
	GetStateSummaryF                      func(context.Context, uint64) (StateSummary, error)
}

func (vm *StateSyncableVM) StateSyncEnabled(ctx context.Context) (bool, error) {
	if vm.StateSyncEnabledF != nil {
		return vm.StateSyncEnabledF(ctx)
	}
	if vm.CantStateSyncEnabled {
		return false, errors.New("unexpectedly called StateSyncEnabled")
	}
	return false, nil
}

func (vm *StateSyncableVM) StateSyncGetOngoingSyncStateSummary(ctx context.Context) (StateSummary, error) {
	if vm.StateSyncGetOngoingSyncStateSummaryF != nil {
		return vm.StateSyncGetOngoingSyncStateSummaryF(ctx)
	}
	if vm.CantStateSyncGetOngoingSyncStateSummary {
		return nil, errors.New("unexpectedly called StateSyncGetOngoingSyncStateSummary")
	}
	return nil, nil
}

func (vm *StateSyncableVM) GetLastStateSummary(ctx context.Context) (StateSummary, error) {
	if vm.GetLastStateSummaryF != nil {
		return vm.GetLastStateSummaryF(ctx)
	}
	if vm.CantGetLastStateSummary {
		return nil, errors.New("unexpectedly called GetLastStateSummary")
	}
	return nil, nil
}

func (vm *StateSyncableVM) ParseStateSummary(ctx context.Context, summaryBytes []byte) (StateSummary, error) {
	if vm.ParseStateSummaryF != nil {
		return vm.ParseStateSummaryF(ctx, summaryBytes)
	}
	if vm.CantParseStateSummary {
		return nil, errors.New("unexpectedly called ParseStateSummary")
	}
	return nil, nil
}

func (vm *StateSyncableVM) GetStateSummary(ctx context.Context, height uint64) (StateSummary, error) {
	if vm.GetStateSummaryF != nil {
		return vm.GetStateSummaryF(ctx, height)
	}
	if vm.CantGetStateSummary {
		return nil, errors.New("unexpectedly called GetStateSummary")
	}
	return nil, nil
}

// StateSummary is a test state summary
type StateSummary interface {
	block.StateSummary
}

// TestStateSummary is a test implementation
type TestStateSummary struct {
	IDV     ids.ID
	HeightV uint64
	BytesV  []byte
	AcceptF func(context.Context) (block.StateSyncMode, error)
}

func (s *TestStateSummary) ID() ids.ID     { return s.IDV }
func (s *TestStateSummary) Height() uint64 { return s.HeightV }
func (s *TestStateSummary) Bytes() []byte  { return s.BytesV }

func (s *TestStateSummary) Accept(ctx context.Context) (block.StateSyncMode, error) {
	if s.AcceptF != nil {
		return s.AcceptF(ctx)
	}
	return block.StateSyncSkipped, nil
}

// Block is a test implementation of a block
type Block struct {
	id       ids.ID
	parentID ids.ID
	height   uint64
	status   choices.Status
	bytes    []byte
	ts       time.Time
}

// NewBlock creates a new test block
func NewBlock(id ids.ID, parentID ids.ID, height uint64, ts time.Time) *Block {
	return &Block{
		id:       id,
		parentID: parentID,
		height:   height,
		status:   choices.Processing,
		bytes:    []byte("test block"),
		ts:       ts,
	}
}

func (b *Block) ID() ids.ID              { return b.id }
func (b *Block) Parent() ids.ID          { return b.parentID }
func (b *Block) Height() uint64          { return b.height }
func (b *Block) Timestamp() time.Time    { return b.ts }
func (b *Block) Status() choices.Status  { return b.status }
func (b *Block) Bytes() []byte           { return b.bytes }
func (b *Block) EpochBit() bool          { return false }
func (b *Block) FPCVotes() [][]byte      { return nil }

func (b *Block) Verify(context.Context) error {
	return nil
}

func (b *Block) Accept(context.Context) error {
	b.status = choices.Accepted
	return nil
}

func (b *Block) Reject(context.Context) error {
	b.status = choices.Rejected
	return nil
}

func (b *Block) Options(context.Context) ([2]chain.Block, error) {
	return [2]chain.Block{}, nil
}