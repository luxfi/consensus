package bootstrap

import (
	"context"
	"fmt"
	"sync"

	"github.com/luxfi/consensus/engine/dag/state"
	"github.com/luxfi/ids"
)

// BootstrapableEngine is an engine that can be bootstrapped
type BootstrapableEngine interface {
	// Start starts the engine
	Start(ctx context.Context, startReqID uint32) error

	// HealthCheck checks engine health
	HealthCheck(ctx context.Context) (interface{}, error)

	// Shutdown shuts down the engine
	Shutdown(ctx context.Context) error

	// IsBootstrapped returns whether bootstrapping is complete
	IsBootstrapped() bool
}

// Bootstrapper bootstraps a DAG chain
type Bootstrapper struct {
	mu             sync.RWMutex
	config         Config
	onFinished     func(ctx context.Context, lastReqID uint32) error
	requestID      uint32
	bootstrapped   bool
	pendingFetches map[ids.ID]struct{}
}

// New creates a new DAG bootstrapper
func New(config Config, onFinished func(ctx context.Context, lastReqID uint32) error) (*Bootstrapper, error) {
	if config.Manager == nil {
		return nil, fmt.Errorf("manager is required")
	}
	if config.VM == nil {
		return nil, fmt.Errorf("VM is required")
	}

	return &Bootstrapper{
		config:         config,
		onFinished:     onFinished,
		pendingFetches: make(map[ids.ID]struct{}),
	}, nil
}

// Start starts the bootstrapping process
func (b *Bootstrapper) Start(ctx context.Context, startReqID uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.requestID = startReqID

	// Check if we should start
	if b.config.StartupTracker != nil && !b.config.StartupTracker.ShouldStart() {
		return nil
	}

	// If we have a stop vertex, linearize to it
	if b.config.StopVertexID != ids.Empty {
		if err := b.config.VM.Linearize(ctx, b.config.StopVertexID); err != nil {
			return fmt.Errorf("failed to linearize: %w", err)
		}
	}

	// Mark as bootstrapped
	b.bootstrapped = true

	// Call the finished callback
	if b.onFinished != nil {
		return b.onFinished(ctx, b.requestID)
	}

	return nil
}

// HealthCheck returns health information
func (b *Bootstrapper) HealthCheck(ctx context.Context) (interface{}, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return map[string]interface{}{
		"bootstrapped":   b.bootstrapped,
		"pendingFetches": len(b.pendingFetches),
	}, nil
}

// Shutdown shuts down the bootstrapper
func (b *Bootstrapper) Shutdown(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Clear pending fetches
	b.pendingFetches = make(map[ids.ID]struct{})

	// Clear blocked jobs
	if b.config.VtxBlocked != nil {
		b.config.VtxBlocked.Clear()
	}
	if b.config.TxBlocked != nil {
		b.config.TxBlocked.Clear()
	}

	return nil
}

// IsBootstrapped returns whether bootstrapping is complete
func (b *Bootstrapper) IsBootstrapped() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.bootstrapped
}

// GetAncestors handles ancestor requests
func (b *Bootstrapper) GetAncestors(ctx context.Context, nodeID ids.NodeID, requestID uint32, vtxID ids.ID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Delegate to the AllGetsServer if available
	if b.config.AllGetsServer != nil {
		return b.config.AllGetsServer.GetAncestors(ctx, nodeID, requestID, vtxID, b.config.AncestorsMaxContainersReceived)
	}

	return nil
}

// Ancestors handles received ancestors
func (b *Bootstrapper) Ancestors(ctx context.Context, nodeID ids.NodeID, requestID uint32, containers [][]byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Parse and add each vertex
	for _, container := range containers {
		vtx, err := b.config.Manager.ParseVtx(ctx, container)
		if err != nil {
			continue // Skip invalid vertices
		}

		if err := b.config.Manager.AddVertex(vtx); err != nil {
			continue // Skip if already exists
		}
	}

	return nil
}

// Connected handles peer connection
func (b *Bootstrapper) Connected(ctx context.Context, nodeID ids.NodeID, nodeVersion string) error {
	return nil
}

// Disconnected handles peer disconnection
func (b *Bootstrapper) Disconnected(ctx context.Context, nodeID ids.NodeID) error {
	return nil
}

// Put handles vertex put
func (b *Bootstrapper) Put(ctx context.Context, nodeID ids.NodeID, requestID uint32, vtxBytes []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	vtx, err := b.config.Manager.ParseVtx(ctx, vtxBytes)
	if err != nil {
		return fmt.Errorf("failed to parse vertex: %w", err)
	}

	return b.config.Manager.AddVertex(vtx)
}

// GetFailed handles a failed get request
func (b *Bootstrapper) GetFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Remove from pending fetches if tracking
	return nil
}

// PushQuery handles push query
func (b *Bootstrapper) PushQuery(ctx context.Context, nodeID ids.NodeID, requestID uint32, vtxBytes []byte) error {
	return b.Put(ctx, nodeID, requestID, vtxBytes)
}

// PullQuery handles pull query
func (b *Bootstrapper) PullQuery(ctx context.Context, nodeID ids.NodeID, requestID uint32, vtxID ids.ID) error {
	return nil
}

// Chits handles chit messages
func (b *Bootstrapper) Chits(ctx context.Context, nodeID ids.NodeID, requestID uint32, preferredID ids.ID, preferredIDAtHeight ids.ID, acceptedID ids.ID) error {
	return nil
}

// QueryFailed handles query failure
func (b *Bootstrapper) QueryFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32) error {
	return nil
}

// Trace wraps a bootstrapper with tracing
func Trace(b *Bootstrapper, tracer interface{}) *Bootstrapper {
	// Tracing wrapper - to be implemented
	return b
}

// Ensure Bootstrapper implements BootstrapableEngine
var _ BootstrapableEngine = (*Bootstrapper)(nil)

// fetchVertex fetches a vertex from the network
func (b *Bootstrapper) fetchVertex(ctx context.Context, vtxID ids.ID) error {
	if b.config.Sender == nil {
		return nil
	}

	// Track pending fetch
	b.pendingFetches[vtxID] = struct{}{}
	b.requestID++

	// Request from network (would normally iterate beacons)
	return nil
}

// processVertex processes a received vertex
func (b *Bootstrapper) processVertex(ctx context.Context, vtx state.Vertex) error {
	// Check if we already have this vertex
	if b.config.Manager.VertexIssued(vtx) {
		return nil
	}

	// Add vertex to state
	if err := b.config.Manager.AddVertex(vtx); err != nil {
		return fmt.Errorf("failed to add vertex: %w", err)
	}

	// Fetch missing parents
	for _, parentID := range vtx.ParentIDs() {
		if _, err := b.config.Manager.GetVertex(parentID); err != nil {
			if err := b.fetchVertex(ctx, parentID); err != nil {
				return fmt.Errorf("failed to fetch parent: %w", err)
			}
		}
	}

	return nil
}
