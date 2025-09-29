// Package chaintest provides test utilities for chain testing
package chaintest

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// VM is a test VM for chain testing
type VM struct {
	T           *testing.T
	ParseBlockF func(context.Context, []byte) (block.Block, error)
}

// ParseBlock parses a block from bytes
func (vm *VM) ParseBlock(ctx context.Context, bytes []byte) (block.Block, error) {
	if vm.ParseBlockF != nil {
		return vm.ParseBlockF(ctx, bytes)
	}
	return &TestBlock{
		bytes: bytes,
	}, nil
}

// Initialize initializes the VM
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
	return nil
}

// BuildBlock builds a new block
func (vm *VM) BuildBlock(ctx context.Context) (block.Block, error) {
	return nil, errors.New("BuildBlock not implemented")
}

// GetBlock gets a block by ID
func (vm *VM) GetBlock(ctx context.Context, id ids.ID) (block.Block, error) {
	return nil, errors.New("GetBlock not implemented")
}

// GetBlockIDAtHeight gets the block ID at a given height
func (vm *VM) GetBlockIDAtHeight(ctx context.Context, height uint64) (ids.ID, error) {
	return ids.Empty, errors.New("GetBlockIDAtHeight not implemented")
}

// LastAccepted returns the last accepted block ID
func (vm *VM) LastAccepted(ctx context.Context) (ids.ID, error) {
	return Genesis.ID(), nil
}

// WaitForEvent waits for a blockchain event
func (vm *VM) WaitForEvent(ctx context.Context) (interface{}, error) {
	return nil, errors.New("WaitForEvent not implemented")
}

// SetPreference sets the preferred block ID
func (vm *VM) SetPreference(ctx context.Context, id ids.ID) error {
	return nil
}