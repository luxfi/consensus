// Package block provides block interfaces for consensus
package block

import (
	"context"
	"net/http"

	"github.com/luxfi/consensus/core/choices"
	consensuscontext "github.com/luxfi/consensus/context"
	"github.com/luxfi/consensus/version"
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
	"github.com/luxfi/warp"
)

// Status re-exports from choices for consistency
type Status = choices.Status

// Status constants re-exported from choices
const (
	Unknown    = choices.Unknown
	Processing = choices.Processing
	Rejected   = choices.Rejected
	Accepted   = choices.Accepted
)

// Block is a block in the chain
type Block interface {
	ID() ids.ID
	Height() uint64
	Timestamp() int64
	Parent() ids.ID
	Bytes() []byte
	Status() Status
	Accept(context.Context) error
	Reject(context.Context) error
	Verify(context.Context) error
}

// ChainVM defines the interface for a blockchain VM
type ChainVM interface {
	// Initialize the VM
	Initialize(
		ctx context.Context,
		chainCtx *ChainContext,
		dbManager database.Database,
		genesisBytes []byte,
		upgradeBytes []byte,
		configBytes []byte,
		toEngine chan<- BlockMessage,
		fxs []*BlockFx,
		sender warp.Sender,
	) error

	// BuildBlock builds a new block
	BuildBlock(context.Context) (Block, error)

	// ParseBlock parses a block from bytes
	ParseBlock(context.Context, []byte) (Block, error)

	// GetBlock gets a block by ID
	GetBlock(context.Context, ids.ID) (Block, error)

	// SetPreference sets the preferred block
	SetPreference(context.Context, ids.ID) error

	// LastAccepted returns the last accepted block
	LastAccepted(context.Context) (ids.ID, error)

	// GetBlockIDAtHeight returns block ID at height
	GetBlockIDAtHeight(context.Context, uint64) (ids.ID, error)

	// Shutdown shuts down the VM
	Shutdown(context.Context) error

	// CreateHandlers creates HTTP handlers
	CreateHandlers(context.Context) (map[string]http.Handler, error)

	// CreateStaticHandlers creates static HTTP handlers
	CreateStaticHandlers(context.Context) (map[string]http.Handler, error)

	// HealthCheck performs a health check
	HealthCheck(context.Context) (interface{}, error)

	// Version returns the version
	Version(context.Context) (string, error)

	// Connected is called when a node connects
	Connected(context.Context, ids.NodeID, *version.Application) error

	// Disconnected is called when a node disconnects
	Disconnected(context.Context, ids.NodeID) error
}

// ChainContext provides context for chain operations
type ChainContext struct {
	// QuantumID is the root quantum network identifier
	QuantumID uint32
	// NetID identifies the network within the quantum network
	NetID ids.ID
	// ChainID identifies the chain within the network
	ChainID ids.ID
	NodeID  ids.NodeID

	// Additional fields
	XChainID ids.ID
	CChainID ids.ID
	XAssetID ids.ID

	// Consensus context
	Ctx *consensuscontext.Context
}

// BlockMessage represents a block-specific consensus message
type BlockMessage interface {
	// Get returns the message bytes
	Get() []byte
}

// BlockFx represents a block-specific feature extension
type BlockFx struct {
	ID   ids.ID
	Name string
}
