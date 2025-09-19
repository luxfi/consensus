// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package blocktest

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// VM is a test VM implementation
type VM struct {
	T *testing.T

	InitializeF func(
		context.Context,
		interface{},
		interface{},
		[]byte,
		[]byte,
		[]byte,
		interface{},
		[]interface{},
		interface{},
	) error

	LastAcceptedF func(context.Context) (ids.ID, error)
	GetBlockF     func(context.Context, ids.ID) (block.Block, error)
	ParseBlockF   func(context.Context, []byte) (block.Block, error)
	BuildBlockF   func(context.Context) (block.Block, error)
	GetBlockIDAtHeightF func(context.Context, uint64) (ids.ID, error)
	SetPreferenceF func(context.Context, ids.ID) error
}

func (vm *VM) Initialize(
	ctx context.Context,
	chainCtx interface{},
	db interface{},
	genesisBytes []byte,
	upgradeBytes []byte,
	configBytes []byte,
	msgChan interface{},
	fxs []interface{},
	appSender interface{},
) error {
	if vm.InitializeF != nil {
		return vm.InitializeF(ctx, chainCtx, db, genesisBytes, upgradeBytes, configBytes, msgChan, fxs, appSender)
	}
	return nil
}

func (vm *VM) LastAccepted(ctx context.Context) (ids.ID, error) {
	if vm.LastAcceptedF != nil {
		return vm.LastAcceptedF(ctx)
	}
	return ids.Empty, nil
}

func (vm *VM) GetBlock(ctx context.Context, id ids.ID) (block.Block, error) {
	if vm.GetBlockF != nil {
		return vm.GetBlockF(ctx, id)
	}
	return nil, sql.ErrNoRows
}

func (vm *VM) ParseBlock(ctx context.Context, b []byte) (block.Block, error) {
	if vm.ParseBlockF != nil {
		return vm.ParseBlockF(ctx, b)
	}
	return nil, nil
}

func (vm *VM) BuildBlock(ctx context.Context) (block.Block, error) {
	if vm.BuildBlockF != nil {
		return vm.BuildBlockF(ctx)
	}
	return nil, nil
}

func (vm *VM) GetBlockIDAtHeight(ctx context.Context, height uint64) (ids.ID, error) {
	if vm.GetBlockIDAtHeightF != nil {
		return vm.GetBlockIDAtHeightF(ctx, height)
	}
	return ids.Empty, nil
}

func (vm *VM) SetPreference(ctx context.Context, id ids.ID) error {
	if vm.SetPreferenceF != nil {
		return vm.SetPreferenceF(ctx, id)
	}
	return nil
}

// BatchedVM is a test batched VM implementation
type BatchedVM struct {
	T *testing.T

	GetAncestorsF      func(context.Context, ids.ID, int, time.Duration) ([][]byte, error)
	BatchedParseBlockF func(context.Context, [][]byte) ([]block.Block, error)
}

func (vm *BatchedVM) GetAncestors(ctx context.Context, blkID ids.ID, maxBlocksNum int, maxBlocksSize time.Duration) ([][]byte, error) {
	if vm.GetAncestorsF != nil {
		return vm.GetAncestorsF(ctx, blkID, maxBlocksNum, maxBlocksSize)
	}
	return nil, nil
}

func (vm *BatchedVM) BatchedParseBlock(ctx context.Context, blks [][]byte) ([]block.Block, error) {
	if vm.BatchedParseBlockF != nil {
		return vm.BatchedParseBlockF(ctx, blks)
	}
	return nil, nil
}