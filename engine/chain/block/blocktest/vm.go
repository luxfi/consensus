// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package blocktest

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

var (
	errInitialize         = errors.New("unexpectedly called Initialize")
	errBuildBlock         = errors.New("unexpectedly called BuildBlock")
	errParseBlock         = errors.New("unexpectedly called ParseBlock")
	errGetBlock           = errors.New("unexpectedly called GetBlock")
	errLastAccepted       = errors.New("unexpectedly called LastAccepted")
	errGetBlockIDAtHeight = errors.New("unexpectedly called GetBlockIDAtHeight")

	_ block.ChainVM = (*VM)(nil)
)

// VM is a ChainVM that is useful for testing.
type VM struct {
	T *testing.T

	CantInitialize,
	CantBuildBlock,
	CantParseBlock,
	CantGetBlock,
	CantSetPreference,
	CantLastAccepted,
	CantGetBlockIDAtHeight bool

	InitializeF         func(context.Context, *block.ChainContext, block.DBManager, []byte, []byte, []byte, chan<- block.Message, []*block.Fx, block.AppSender) error
	BuildBlockF         func(context.Context) (block.Block, error)
	ParseBlockF         func(context.Context, []byte) (block.Block, error)
	GetBlockF           func(context.Context, ids.ID) (block.Block, error)
	SetPreferenceF      func(context.Context, ids.ID) error
	LastAcceptedF       func(context.Context) (ids.ID, error)
	GetBlockIDAtHeightF func(ctx context.Context, height uint64) (ids.ID, error)
}

func (vm *VM) Default(cant bool) {
	vm.CantInitialize = cant
	vm.CantBuildBlock = cant
	vm.CantParseBlock = cant
	vm.CantGetBlock = cant
	vm.CantSetPreference = cant
	vm.CantLastAccepted = cant
	vm.CantGetBlockIDAtHeight = cant
}

func (vm *VM) Initialize(
	ctx context.Context,
	chainCtx *block.ChainContext,
	dbManager block.DBManager,
	genesisBytes []byte,
	upgradeBytes []byte,
	configBytes []byte,
	toEngine chan<- block.Message,
	fxs []*block.Fx,
	appSender block.AppSender,
) error {
	if vm.InitializeF != nil {
		return vm.InitializeF(ctx, chainCtx, dbManager, genesisBytes, upgradeBytes, configBytes, toEngine, fxs, appSender)
	}
	if !vm.CantInitialize {
		return nil
	}
	if vm.T != nil {
		require.FailNow(vm.T, errInitialize.Error())
	}
	return errInitialize
}

func (vm *VM) BuildBlock(ctx context.Context) (block.Block, error) {
	if vm.BuildBlockF != nil {
		return vm.BuildBlockF(ctx)
	}
	if vm.CantBuildBlock && vm.T != nil {
		require.FailNow(vm.T, errBuildBlock.Error())
	}
	return nil, errBuildBlock
}

func (vm *VM) ParseBlock(ctx context.Context, b []byte) (block.Block, error) {
	if vm.ParseBlockF != nil {
		return vm.ParseBlockF(ctx, b)
	}
	if vm.CantParseBlock && vm.T != nil {
		require.FailNow(vm.T, errParseBlock.Error())
	}
	return nil, errParseBlock
}

func (vm *VM) GetBlock(ctx context.Context, id ids.ID) (block.Block, error) {
	if vm.GetBlockF != nil {
		return vm.GetBlockF(ctx, id)
	}
	if vm.CantGetBlock && vm.T != nil {
		require.FailNow(vm.T, errGetBlock.Error())
	}
	return nil, errGetBlock
}

func (vm *VM) SetPreference(ctx context.Context, id ids.ID) error {
	if vm.SetPreferenceF != nil {
		return vm.SetPreferenceF(ctx, id)
	}
	if vm.CantSetPreference && vm.T != nil {
		require.FailNow(vm.T, "Unexpectedly called SetPreference")
	}
	return nil
}

func (vm *VM) LastAccepted(ctx context.Context) (ids.ID, error) {
	if vm.LastAcceptedF != nil {
		return vm.LastAcceptedF(ctx)
	}
	if vm.CantLastAccepted && vm.T != nil {
		require.FailNow(vm.T, errLastAccepted.Error())
	}
	return ids.Empty, errLastAccepted
}

func (vm *VM) GetBlockIDAtHeight(ctx context.Context, height uint64) (ids.ID, error) {
	if vm.GetBlockIDAtHeightF != nil {
		return vm.GetBlockIDAtHeightF(ctx, height)
	}
	if vm.CantGetBlockIDAtHeight && vm.T != nil {
		require.FailNow(vm.T, errGetBlockIDAtHeight.Error())
	}
	return ids.Empty, errGetBlockIDAtHeight
}