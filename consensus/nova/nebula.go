// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nova

import (
	"context"
	"sync"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// Nebula implements the graph-based DAG consensus layer.
// Like an interstellar nebula with its vast web of interconnected gas and dust,
// Nebula enables parallel transaction processing in a non-linear graph structure.
type Nebula struct {
	mu       sync.RWMutex
	params   config.Parameters
	frontier *Frontier
	
	// Consensus thresholds
	beta1 int // Early commit threshold (β₁)
	beta2 int // Final commit threshold (β₂)
}

// NewNebula creates a new Nebula DAG consensus instance.
func NewNebula(params config.Parameters) *Nebula {
	return &Nebula{
		params:   params,
		frontier: NewFrontier(),
		beta1:    params.Beta / 2,      // Early commit at β/2
		beta2:    params.Beta,          // Final commit at β
	}
}

// AddVertex adds a new vertex to the DAG.
func (n *Nebula) AddVertex(ctx context.Context, vertex Vertex) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Verify the vertex
	if err := vertex.Verify(ctx); err != nil {
		return err
	}

	// Add to frontier
	vc := n.frontier.Add(vertex)

	// Start initial polling for this vertex
	n.startPolling(vc)

	return nil
}

// RecordPoll records the results of k-peer sampling.
func (n *Nebula) RecordPoll(ctx context.Context, nodeID ids.NodeID, votes []ids.ID) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Update confidence for voted vertices
	for _, vtxID := range votes {
		if vc, ok := n.frontier.Get(vtxID); ok {
			vc.AddChit()
			
			// Check if confidence crossed thresholds
			confidence := vc.GetConfidence()
			
			if !vc.IsPreferred() && confidence >= n.beta1 {
				// Early commit - mark as preferred
				vc.SetPreferred()
			}
			
			if !vc.IsDecided() && confidence >= n.beta2 {
				// Final commit - mark as decided
				vc.SetDecided()
				n.onVertexDecided(ctx, vc)
			}
		}
	}

	// Update confidence counters based on k-peer responses
	n.updateConfidenceCounters(votes)

	return nil
}

// GetPreferredFrontier returns the highest confidence frontier vertex.
func (n *Nebula) GetPreferredFrontier() (Vertex, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if vc, ok := n.frontier.GetHighestConfidence(); ok {
		return vc.vertex, true
	}
	return nil, false
}

// GetDecidedVertices returns all decided vertices.
func (n *Nebula) GetDecidedVertices() []Vertex {
	n.mu.RLock()
	defer n.mu.RUnlock()

	decided := make([]Vertex, 0)
	for _, vc := range n.frontier.vertices {
		if vc.IsDecided() {
			decided = append(decided, vc.vertex)
		}
	}
	return decided
}

// startPolling initiates k-peer sampling for a vertex.
func (n *Nebula) startPolling(vc *VertexConfidence) {
	// Implementation: trigger k-peer sampling
	// In production, this would initiate network polling for this vertex
}

// updateConfidenceCounters updates d(T) based on sampling results.
func (n *Nebula) updateConfidenceCounters(votes []ids.ID) {
	// Count occurrences of each vertex in votes
	voteCounts := make(map[ids.ID]int)
	for _, vtxID := range votes {
		voteCounts[vtxID]++
	}

	// Update confidence based on vote counts and thresholds
	for vtxID, count := range voteCounts {
		if vc, ok := n.frontier.Get(vtxID); ok {
			if count >= n.params.AlphaPreference {
				// Positive response - increase confidence
				vc.UpdateConfidence(1)
			} else {
				// Not enough support - confidence stays same or decreases
				vc.UpdateConfidence(-1)
			}
		}
	}
}

// onVertexDecided handles a vertex becoming decided.
func (n *Nebula) onVertexDecided(ctx context.Context, vc *VertexConfidence) {
	// Mark all conflicting vertices as rejected
	// Update frontier
	// Trigger any callbacks
}

// NebulaStats provides statistics about the Nebula DAG.
type NebulaStats struct {
	TotalVertices    int
	DecidedVertices  int
	PreferredVertices int
	FrontierSize     int
	AverageConfidence float64
}

// GetStats returns current Nebula statistics.
func (n *Nebula) GetStats() NebulaStats {
	n.mu.RLock()
	defer n.mu.RUnlock()

	stats := NebulaStats{
		TotalVertices: len(n.frontier.vertices),
		FrontierSize:  len(n.frontier.frontier),
	}

	totalConfidence := 0
	for _, vc := range n.frontier.vertices {
		if vc.IsDecided() {
			stats.DecidedVertices++
		}
		if vc.IsPreferred() {
			stats.PreferredVertices++
		}
		totalConfidence += vc.GetConfidence()
	}

	if stats.TotalVertices > 0 {
		stats.AverageConfidence = float64(totalConfidence) / float64(stats.TotalVertices)
	}

	return stats
}