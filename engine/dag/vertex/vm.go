package vertex

import (
	"context"
	"net/http"
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/engine/dag"
)

// LinearizableVM defines the interface for a VM that can be linearized
type LinearizableVM interface {
	// Initialize initializes the VM
	Initialize(
		ctx context.Context,
		chainCtx interface{},
		dbManager interface{},
		genesisBytes []byte,
		upgradeBytes []byte,
		configBytes []byte,
		msgChan chan<- interface{},
		fxs []interface{},
		appSender interface{},
	) error
	
	// Shutdown shuts down the VM
	Shutdown() error
	
	// CreateHandlers creates HTTP handlers
	CreateHandlers(ctx context.Context) (map[string]http.Handler, error)
	
	// Linearize linearizes the DAG from the stop vertex
	Linearize(ctx context.Context, stopVertexID ids.ID) error
}

// LinearizableVMWithEngine combines LinearizableVM with engine and DAG capabilities
type LinearizableVMWithEngine interface {
	LinearizableVM
	
	// GetEngine returns the consensus engine
	GetEngine() interface{}
	
	// SetEngine sets the consensus engine
	SetEngine(engine interface{})
	
	// ParseTx parses transaction bytes
	ParseTx(ctx context.Context, txBytes []byte) (dag.Tx, error)
	
	// BuildVertex builds a new vertex
	BuildVertex(ctx context.Context) (dag.Vertex, error)
	
	// GetVertex gets a vertex by ID
	GetVertex(ctx context.Context, vtxID ids.ID) (dag.Vertex, error)
	
	// ParseVertex parses vertex bytes
	ParseVertex(ctx context.Context, vtxBytes []byte) (dag.Vertex, error)
}