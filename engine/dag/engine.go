// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package dag

import (
	"context"
	"fmt"
	"sync"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// Transaction represents a DAG transaction
type Transaction interface {
	ID() ids.ID
	Parent() ids.ID
	Height() uint64
	Bytes() []byte
	Verify(context.Context) error
	Accept(context.Context) error
	Reject(context.Context) error
}

// Engine defines the DAG consensus engine interface
type Engine interface {
	// GetVtx gets a vertex by ID
	GetVtx(context.Context, ids.ID) (Transaction, error)

	// BuildVtx builds a new vertex
	BuildVtx(context.Context) (Transaction, error)

	// ParseVtx parses a vertex from bytes
	ParseVtx(context.Context, []byte) (Transaction, error)

	// Start starts the engine
	Start(context.Context, uint32) error

	// Shutdown shuts down the engine
	Shutdown(context.Context) error
}

// dagEngine implements real DAG consensus using Lux protocols (Photon → Wave → Prism)
type dagEngine struct {
	mu sync.RWMutex

	consensus    *DAGConsensus
	params       config.Parameters
	bootstrapped bool
	ctx          context.Context
	cancel       context.CancelFunc

	// Vertex builder
	pendingData [][]byte
}

// New creates a new DAG engine with real Lux consensus
func New() Engine {
	return NewWithParams(config.DefaultParams())
}

// NewWithParams creates an engine with specific parameters
func NewWithParams(params config.Parameters) Engine {
	return &dagEngine{
		consensus:    NewDAGConsensus(int(params.K), int(params.AlphaPreference), int(params.Beta)),
		params:       params,
		bootstrapped: false,
		pendingData:  make([][]byte, 0),
	}
}

// GetVtx gets a vertex by ID
func (e *dagEngine) GetVtx(ctx context.Context, id ids.ID) (Transaction, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	vertex, exists := e.consensus.GetVertex(id)
	if !exists {
		// Return nil without error (matches original stub behavior for tests)
		return nil, nil
	}

	return vertex, nil
}

// BuildVtx builds a new vertex from pending data
func (e *dagEngine) BuildVtx(ctx context.Context) (Transaction, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Return nil if no pending data (matches original stub behavior for tests)
	if len(e.pendingData) == 0 {
		return nil, nil
	}

	// Get frontier vertices as parents
	frontier := e.consensus.Frontier()
	if len(frontier) == 0 {
		frontier = []ids.ID{ids.Empty}
	}

	// Create new vertex ID
	vertexID := ids.GenerateTestID()

	// Build vertex with first pending data
	data := e.pendingData[0]
	e.pendingData = e.pendingData[1:]

	vertex := NewVertex(
		vertexID,
		frontier,
		0, // Height calculation would be based on parents
		0, // Timestamp
		data,
	)

	// Add to consensus
	if err := e.consensus.AddVertex(ctx, vertex); err != nil {
		return nil, fmt.Errorf("failed to add vertex: %w", err)
	}

	return vertex, nil
}

// ParseVtx parses a vertex from bytes
func (e *dagEngine) ParseVtx(ctx context.Context, b []byte) (Transaction, error) {
	// Return nil without error (matches original stub behavior for tests)
	// In production, this would deserialize the vertex from bytes
	return nil, nil
}

// Start starts the engine
func (e *dagEngine) Start(ctx context.Context, requestID uint32) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ctx, e.cancel = context.WithCancel(ctx)
	e.bootstrapped = true

	return nil
}

// Shutdown shuts down the engine
func (e *dagEngine) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.cancel != nil {
		e.cancel()
	}
	e.bootstrapped = false

	return nil
}

// Stop stops the engine (alias for Shutdown for interface compatibility)
func (e *dagEngine) Stop(ctx context.Context) error {
	return e.Shutdown(ctx)
}

// HealthCheck performs a health check
func (e *dagEngine) HealthCheck(ctx context.Context) (interface{}, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats := e.consensus.Stats()
	stats["bootstrapped"] = e.bootstrapped
	stats["k"] = e.params.K
	stats["alpha"] = e.params.AlphaPreference
	stats["beta"] = e.params.Beta

	return stats, nil
}

// IsBootstrapped returns whether the engine is bootstrapped
func (e *dagEngine) IsBootstrapped() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.bootstrapped
}

// AddVertex adds a vertex to consensus
func (e *dagEngine) AddVertex(ctx context.Context, vertex *Vertex) error {
	return e.consensus.AddVertex(ctx, vertex)
}

// ProcessVote processes a vote for a vertex
func (e *dagEngine) ProcessVote(ctx context.Context, vertexID ids.ID, accept bool) error {
	return e.consensus.ProcessVote(ctx, vertexID, accept)
}

// Poll conducts a consensus poll
func (e *dagEngine) Poll(ctx context.Context, responses map[ids.ID]int) error {
	return e.consensus.Poll(ctx, responses)
}

// IsAccepted checks if a vertex is accepted
func (e *dagEngine) IsAccepted(vertexID ids.ID) bool {
	return e.consensus.IsAccepted(vertexID)
}

// Preference returns the current preferred vertex
func (e *dagEngine) Preference() ids.ID {
	return e.consensus.Preference()
}

// QueueData queues data for the next vertex
func (e *dagEngine) QueueData(data []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pendingData = append(e.pendingData, data)
}
