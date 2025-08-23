// Package block provides block interfaces for consensus
package block

import (
	"context"
	"net/http"

	consensuscontext "github.com/luxfi/consensus/context"
	"github.com/luxfi/consensus/version"
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
)

// Status represents the status of a block
type Status uint8

const (
	// Unknown status
	Unknown Status = iota
	// Processing status
	Processing
	// Rejected status
	Rejected
	// Accepted status
	Accepted
	// Verified status
	Verified
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
		toEngine chan<- Message,
		fxs []*Fx,
		appSender AppSender,
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
	NetworkID uint32
	ChainID   ids.ID
	NodeID    ids.NodeID
	
	// Additional fields
	XChainID    ids.ID
	CChainID    ids.ID
	AVAXAssetID ids.ID
	
	// Consensus context
	Ctx *consensuscontext.Context
}

// Message represents a consensus message
type Message interface {
	// Get returns the message bytes
	Get() []byte
}

// Fx represents a feature extension
type Fx struct {
	ID   ids.ID
	Name string
}

// AppSender sends application messages
type AppSender interface {
	// SendAppRequest sends an app request
	SendAppRequest(context.Context, ids.NodeID, uint32, []byte) error
	
	// SendAppResponse sends an app response
	SendAppResponse(context.Context, ids.NodeID, uint32, []byte) error
	
	// SendAppGossip sends app gossip
	SendAppGossip(context.Context, []byte) error
}