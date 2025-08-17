// Package enginetest provides test utilities for consensus engine operations
package enginetest

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
	"github.com/luxfi/node/version"
)

var (
	errInitialize = errors.New("unexpectedly called Initialize")
	errShutdown   = errors.New("unexpectedly called Shutdown")
	errHealthCheck = errors.New("unexpectedly called HealthCheck")
	errVersion = errors.New("unexpectedly called Version")
)

// TestEngine represents a test consensus engine
type TestEngine struct {
	ID      string
	Running bool
}

// NewTestEngine creates a new test engine
func NewTestEngine(id string) *TestEngine {
	return &TestEngine{
		ID:      id,
		Running: false,
	}
}

// Helper provides test helper functions
func Helper(t *testing.T) {
	t.Helper()
}

// VM is a test VM that is useful for testing consensus.
type VM struct {
	T *testing.T

	CantInitialize,
	CantShutdown,
	CantCreateHandlers,
	CantHealthCheck,
	CantVersion,
	CantConnected,
	CantDisconnected bool

	InitializeF       func(context.Context, *block.ChainContext, block.DBManager, []byte, []byte, []byte, chan<- block.Message, []*block.Fx, block.AppSender) error
	ShutdownF         func(context.Context) error
	CreateHandlersF   func(context.Context) (map[string]interface{}, error)
	HealthCheckF      func(context.Context) (interface{}, error)
	VersionF          func(context.Context) (string, error)
	ConnectedF        func(context.Context, ids.NodeID, *version.Application) error
	DisconnectedF     func(context.Context, ids.NodeID) error
}

func (vm *VM) Default(cant bool) {
	vm.CantInitialize = cant
	vm.CantShutdown = cant
	vm.CantCreateHandlers = cant
	vm.CantHealthCheck = cant
	vm.CantVersion = cant
	vm.CantConnected = cant
	vm.CantDisconnected = cant
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

func (vm *VM) Shutdown(ctx context.Context) error {
	if vm.ShutdownF != nil {
		return vm.ShutdownF(ctx)
	}
	if !vm.CantShutdown {
		return nil
	}
	if vm.T != nil {
		require.FailNow(vm.T, errShutdown.Error())
	}
	return errShutdown
}

func (vm *VM) CreateHandlers(ctx context.Context) (map[string]interface{}, error) {
	if vm.CreateHandlersF != nil {
		return vm.CreateHandlersF(ctx)
	}
	if !vm.CantCreateHandlers {
		return nil, nil
	}
	if vm.T != nil {
		require.FailNow(vm.T, "unexpectedly called CreateHandlers")
	}
	return nil, errors.New("unexpectedly called CreateHandlers")
}

func (vm *VM) HealthCheck(ctx context.Context) (interface{}, error) {
	if vm.HealthCheckF != nil {
		return vm.HealthCheckF(ctx)
	}
	if !vm.CantHealthCheck {
		return nil, nil
	}
	if vm.T != nil {
		require.FailNow(vm.T, errHealthCheck.Error())
	}
	return nil, errHealthCheck
}

func (vm *VM) Version(ctx context.Context) (string, error) {
	if vm.VersionF != nil {
		return vm.VersionF(ctx)
	}
	if !vm.CantVersion {
		return "", nil
	}
	if vm.T != nil {
		require.FailNow(vm.T, errVersion.Error())
	}
	return "", errVersion
}

func (vm *VM) Connected(ctx context.Context, nodeID ids.NodeID, v *version.Application) error {
	if vm.ConnectedF != nil {
		return vm.ConnectedF(ctx, nodeID, v)
	}
	if !vm.CantConnected {
		return nil
	}
	if vm.T != nil {
		require.FailNow(vm.T, "unexpectedly called Connected")
	}
	return errors.New("unexpectedly called Connected")
}

func (vm *VM) Disconnected(ctx context.Context, nodeID ids.NodeID) error {
	if vm.DisconnectedF != nil {
		return vm.DisconnectedF(ctx, nodeID)
	}
	if !vm.CantDisconnected {
		return nil
	}
	if vm.T != nil {
		require.FailNow(vm.T, "unexpectedly called Disconnected")
	}
	return errors.New("unexpectedly called Disconnected")
}