// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package interfaces

import (
	"sync"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/validator"
	"github.com/luxfi/consensus/utils/upgrade"
	"github.com/luxfi/consensus/utils"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/log"
)

// ContextInitializable represents an object that can be initialized
// given a *Context object
type ContextInitializable interface {
	// InitCtx initializes an object provided a *Context object
	InitCtx(ctx *Context)
}

// Context is information about the current execution.
// [NetworkID] is the ID of the network this context exists within.
// [ChainID] is the ID of the chain this context exists within.
// [NodeID] is the ID of this node
type Context struct {
	NetworkID       uint32
	SubnetID        ids.ID
	ChainID         ids.ID
	NodeID          ids.NodeID
	PublicKey       *bls.PublicKey
	NetworkUpgrades upgrade.Config

	XChainID    ids.ID
	CChainID    ids.ID
	LUXAssetID ids.ID

	Log log.Logger
	// Deprecated: This lock should not be used unless absolutely necessary.
	// This lock will be removed in a future release once it is replaced with
	// more granular locks.
	//
	// Warning: This lock is not correctly implemented over the rpcchainvm.
	Lock         sync.RWMutex
	BCLookup     ids.AliaserReader

	WarpSigner interface{} // TODO: Use proper warp.Signer interface

	// beam++ attributes
	ValidatorState validator.State // interface for P-Chain validators
	// Chain-specific directory where arbitrary data can be written
	ChainDataDir string

	// Registerer is used to register metrics
	Registerer Registerer

	// BlockAcceptor is the callback that will be fired whenever a VM is
	// notified that their block was accepted.
	BlockAcceptor Acceptor
}

// Registerer is a no-op interface for metrics registration
// Real metrics should be handled by the parent system
type Registerer interface {
	Register(interface{}) error
}

type ConsensusContext struct {
	*Context

	// PrimaryAlias is the primary alias of the chain this context exists
	// within.
	PrimaryAlias string

	// Registers all consensus metrics.
	Registerer Registerer

	// BlockAcceptor is the callback that will be fired whenever a VM is
	// notified that their block was accepted.
	BlockAcceptor Acceptor

	// TxAcceptor is the callback that will be fired whenever a VM is notified
	// that their transaction was accepted.
	TxAcceptor Acceptor

	// VertexAcceptor is the callback that will be fired whenever a vertex was
	// accepted.
	VertexAcceptor Acceptor

	// State indicates the current state of this consensus instance.
	State utils.Atomic[EngineState]

	// True iff this chain is executing transactions as part of bootstrapping.
	Executing utils.Atomic[bool]

	// True iff this chain is currently state-syncing
	StateSyncing utils.Atomic[bool]
}
