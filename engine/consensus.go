// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"context"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// Consensus is the quantum consensus engine interface
type Consensus interface {
	// Initialize the consensus engine
	Initialize(ctx context.Context, params config.Parameters) error

	// Poll queries peers for their preferences
	Poll(ctx context.Context) ([]Vote, error)

	// RecordPoll records the results of a poll
	RecordPoll(votes []Vote) error

	// Finalized returns true if consensus has been reached
	Finalized() bool

	// Decision returns the finalized decision
	Decision() ids.ID

	// Preference returns the current preference
	Preference() ids.ID

	// HealthCheck returns the health status of the consensus engine
	HealthCheck(context.Context) (interface{}, error)

	// Shutdown cleanly shuts down the consensus engine
	Shutdown(context.Context) error
}

// Vote represents a vote from a peer
type Vote struct {
	NodeID     ids.NodeID
	Preference ids.ID
	Confidence int
}

// Block represents a block in the consensus
type Block interface {
	// ID returns the unique identifier of the block
	ID() ids.ID

	// Parent returns the parent block's ID
	Parent() ids.ID

	// Height returns the height of the block
	Height() uint64

	// Verify verifies the block's validity
	Verify(context.Context) error

	// Accept marks the block as accepted
	Accept(context.Context) error

	// Reject marks the block as rejected
	Reject(context.Context) error
}

// ChainConsensus is the interface for linear chain consensus (Quantum Chain)
type ChainConsensus interface {
	Consensus

	// AddBlock adds a new block to consensus consideration
	AddBlock(block Block) error

	// GetBlock retrieves a block by ID
	GetBlock(id ids.ID) (Block, error)
}

// DAGConsensus is the interface for DAG-based consensus (Quantum DAG)
type DAGConsensus interface {
	Consensus

	// AddVertex adds a new vertex to consensus consideration
	AddVertex(vertex Vertex) error

	// GetVertex retrieves a vertex by ID
	GetVertex(id ids.ID) (Vertex, error)
}

// Vertex represents a vertex in DAG consensus
type Vertex interface {
	// ID returns the unique identifier of the vertex
	ID() ids.ID

	// Parents returns the parent vertices' IDs
	Parents() []ids.ID

	// Height returns the height of the vertex
	Height() uint64

	// Verify verifies the vertex's validity
	Verify(context.Context) error

	// Accept marks the vertex as accepted
	Accept(context.Context) error

	// Reject marks the vertex as rejected
	Reject(context.Context) error
}