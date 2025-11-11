// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package dag

import (
	"context"
	"fmt"
	"sync"

	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/ids"
)

// DAGConsensus implements real Lux consensus for DAG structures using Photon → Wave → Prism
type DAGConsensus struct {
	mu sync.RWMutex

	// Parameters
	k     int // Sample size
	alpha int // Quorum size
	beta  int // Decision threshold

	// State
	vertices   map[ids.ID]*Vertex
	frontier   map[ids.ID]bool // Current frontier (vertices with no unprocessed children)
	processing map[ids.ID]bool // Vertices currently being processed

	// Consensus tracking
	bootstrapped bool
	lastAccepted ids.ID
}

// NewDAGConsensus creates a real consensus engine for DAG
func NewDAGConsensus(k, alpha, beta int) *DAGConsensus {
	return &DAGConsensus{
		k:          k,
		alpha:      alpha,
		beta:       beta,
		vertices:   make(map[ids.ID]*Vertex),
		frontier:   make(map[ids.ID]bool),
		processing: make(map[ids.ID]bool),
	}
}

// AddVertex adds a vertex to the DAG
func (d *DAGConsensus) AddVertex(ctx context.Context, vertex *Vertex) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if vertex already exists
	if _, exists := d.vertices[vertex.ID()]; exists {
		return fmt.Errorf("vertex already exists: %s", vertex.ID())
	}

	// Verify the vertex
	if err := vertex.Verify(ctx); err != nil {
		return fmt.Errorf("vertex verification failed: %w", err)
	}

	// Initialize Lux consensus for this vertex using Photon → Wave → Prism (DAG refraction)
	vertex.SetLuxConsensus(engine.NewLuxConsensus(d.k, d.alpha, d.beta))

	// Add to vertices map
	d.vertices[vertex.ID()] = vertex

	// Link with parent vertices
	for _, parentID := range vertex.ParentIDs() {
		if parentID == ids.Empty {
			continue
		}

		parent, exists := d.vertices[parentID]
		if !exists {
			return fmt.Errorf("parent vertex not found: %s", parentID)
		}

		// Link parent-child relationship
		parent.AddChild(vertex)
		vertex.AddParent(parent)

		// Remove parent from frontier (it now has children)
		delete(d.frontier, parentID)
	}

	// Add vertex to frontier (it has no children yet)
	d.frontier[vertex.ID()] = true

	return nil
}

// ProcessVote processes a vote for a vertex
func (d *DAGConsensus) ProcessVote(ctx context.Context, vertexID ids.ID, accept bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	vertex, exists := d.vertices[vertexID]
	if !exists {
		return fmt.Errorf("vertex not found: %s", vertexID)
	}

	luxConsensus := vertex.LuxConsensus()
	if luxConsensus == nil {
		return fmt.Errorf("vertex not initialized for consensus")
	}

	if accept {
		luxConsensus.RecordVote(vertexID)
	}

	return nil
}

// Poll conducts a consensus poll
func (d *DAGConsensus) Poll(ctx context.Context, responses map[ids.ID]int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Poll each vertex's Lux consensus instance using Wave → Prism (DAG) protocols
	for vertexID, votes := range responses {
		vertex, exists := d.vertices[vertexID]
		if !exists {
			continue
		}

		luxConsensus := vertex.LuxConsensus()
		if luxConsensus == nil {
			continue
		}

		vertexResponses := map[ids.ID]int{vertexID: votes}
		shouldContinue := luxConsensus.Poll(vertexResponses)

		// Check if vertex reached finality through Prism DAG refraction
		if !shouldContinue && luxConsensus.Decided() {
			if err := vertex.Accept(ctx); err != nil {
				return fmt.Errorf("failed to accept vertex: %w", err)
			}
			d.lastAccepted = vertexID

			// Process children in topological order
			if err := d.processChildrenInOrder(ctx, vertex); err != nil {
				return fmt.Errorf("failed to process children: %w", err)
			}
		}
	}

	return nil
}

// processChildrenInOrder processes children in topological order
func (d *DAGConsensus) processChildrenInOrder(ctx context.Context, parent *Vertex) error {
	// Get all children that are ready to be processed
	children := parent.Children()

	for _, child := range children {
		// Check if all parents are accepted
		allParentsAccepted := true
		for _, p := range child.Parents() {
			if !p.IsAccepted() {
				allParentsAccepted = false
				break
			}
		}

		// If all parents accepted, mark child as ready for processing
		if allParentsAccepted && !child.IsProcessing() {
			child.SetProcessing(true)
			d.processing[child.ID()] = true
		}
	}

	return nil
}

// IsAccepted checks if a vertex is accepted
func (d *DAGConsensus) IsAccepted(vertexID ids.ID) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	vertex, exists := d.vertices[vertexID]
	if !exists {
		return false
	}

	return vertex.IsAccepted()
}

// IsRejected checks if a vertex is rejected
func (d *DAGConsensus) IsRejected(vertexID ids.ID) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	vertex, exists := d.vertices[vertexID]
	if !exists {
		return false
	}

	return vertex.IsRejected()
}

// Preference returns current preferred vertex
func (d *DAGConsensus) Preference() ids.ID {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Return the last accepted vertex if available
	if d.lastAccepted != ids.Empty {
		return d.lastAccepted
	}

	// Otherwise return latest vertex in frontier
	for vertexID := range d.frontier {
		return vertexID
	}

	return ids.Empty
}

// GetVertex returns a vertex by ID
func (d *DAGConsensus) GetVertex(vertexID ids.ID) (*Vertex, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	vertex, exists := d.vertices[vertexID]
	return vertex, exists
}

// Frontier returns the current frontier vertices
func (d *DAGConsensus) Frontier() []ids.ID {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]ids.ID, 0, len(d.frontier))
	for vertexID := range d.frontier {
		result = append(result, vertexID)
	}

	return result
}

// Stats returns consensus statistics
func (d *DAGConsensus) Stats() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	accepted := 0
	rejected := 0
	pending := 0

	for _, vertex := range d.vertices {
		if vertex.IsAccepted() {
			accepted++
		} else if vertex.IsRejected() {
			rejected++
		} else {
			pending++
		}
	}

	return map[string]interface{}{
		"total_vertices": len(d.vertices),
		"accepted":       accepted,
		"rejected":       rejected,
		"pending":        pending,
		"frontier":       len(d.frontier),
		"processing":     len(d.processing),
		"last_accepted":  d.lastAccepted.String(),
	}
}

// GetConflicting returns vertices that conflict with the given vertex
func (d *DAGConsensus) GetConflicting(ctx context.Context, vertex *Vertex) []*Vertex {
	d.mu.RLock()
	defer d.mu.RUnlock()

	conflicting := make([]*Vertex, 0)

	// In a real implementation, this would check for actual conflicts
	// (e.g., double-spends, incompatible states)
	// For now, we return an empty list
	// TODO: Implement conflict detection logic

	return conflicting
}

// ResolveConflict resolves conflicts between vertices using Lux consensus with Prism DAG refraction
func (d *DAGConsensus) ResolveConflict(ctx context.Context, vertices []*Vertex) (*Vertex, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(vertices) == 0 {
		return nil, fmt.Errorf("no vertices to resolve")
	}

	if len(vertices) == 1 {
		return vertices[0], nil
	}

	// Create a temporary Lux consensus instance for conflict resolution using Prism protocol
	conflictResolver := engine.NewLuxConsensus(d.k, d.alpha, d.beta)

	// Build responses map for conflict resolution
	responses := make(map[ids.ID]int)
	for _, v := range vertices {
		// In real implementation, this would gather actual votes from network
		responses[v.ID()] = 1
	}

	// Poll until decision reached
	for conflictResolver.Poll(responses) {
		// Continue polling until consensus reached
	}

	// Return the vertex that was accepted
	for _, v := range vertices {
		decision, exists := conflictResolver.Decision(v.ID())
		if exists && decision == 2 { // DecideAccept
			return v, nil
		}
	}

	// Fallback to first vertex if no clear winner
	return vertices[0], nil
}
