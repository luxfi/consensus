package bootstrap

import (
	"context"

	"github.com/luxfi/consensus/engine/dag/getter"
	"github.com/luxfi/consensus/engine/dag/state"
	"github.com/luxfi/ids"
)

// Config configures the DAG bootstrapper
type Config struct {
	// AllGetsServer handles ancestor fetching
	AllGetsServer getter.Handler

	// Ctx is the consensus context
	Ctx context.Context

	// Beacons are the validators used for bootstrapping
	Beacons interface {
		TotalWeight(netID ids.ID) (uint64, error)
	}

	// StartupTracker tracks startup progress
	StartupTracker interface {
		ShouldStart() bool
	}

	// Sender sends messages to peers
	Sender interface {
		SendGetAncestors(ctx context.Context, nodeID ids.NodeID, requestID uint32, vtxID ids.ID) error
	}

	// AncestorsMaxContainersReceived limits the number of containers received
	AncestorsMaxContainersReceived int

	// VtxBlocked is the blocker for vertex processing
	VtxBlocked Blocker

	// TxBlocked is the blocker for transaction processing
	TxBlocked Blocker

	// Manager manages vertex state
	Manager state.Manager

	// VM is the linearizable DAG VM
	VM LinearizableVM

	// Haltable provides halt functionality
	Haltable Halter

	// StopVertexID is an optional vertex ID to stop at (for upgrades)
	StopVertexID ids.ID
}

// Blocker blocks/unblocks processing
type Blocker interface {
	// Register registers a job to be unblocked
	Register(ctx context.Context, id ids.ID) (bool, error)

	// PushJob pushes a job to be processed
	PushJob(ctx context.Context, id ids.ID) error

	// Unblock marks a job as unblocked
	Unblock(ctx context.Context, id ids.ID) error

	// Clear clears all blocked jobs
	Clear()
}

// Halter can halt execution
type Halter interface {
	// Halt stops execution
	Halt(ctx context.Context)

	// Halted returns whether halted
	Halted() bool
}

// LinearizableVM is a DAG VM that can be linearized
type LinearizableVM interface {
	// Linearize converts DAG to linear chain
	Linearize(ctx context.Context, stopVertexID ids.ID) error

	// ParseVtx parses a vertex from bytes
	ParseVtx(ctx context.Context, bytes []byte) (state.Vertex, error)
}
