// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package flare

import "github.com/luxfi/consensus/types"

// Vertex represents a DAG vertex
type Vertex interface {
	ID() types.VertexID
	Parents() []types.VertexID
	Height() uint64
	Bytes() []byte
}

// Graph manages DAG ordering and conflict detection
type Graph interface {
	Add(v Vertex) error
	Parents(id types.VertexID) []types.VertexID
	Conflicts(id types.VertexID) []types.VertexID
	Ancestors(id types.VertexID) []types.VertexID
	Size() int
}

// graph implements DAG ordering
type graph struct {
	vertices  map[types.VertexID]Vertex
	parents   map[types.VertexID][]types.VertexID
	conflicts map[types.VertexID][]types.VertexID
}

// NewGraph creates a new DAG graph
func NewGraph() Graph {
	return &graph{
		vertices:  make(map[types.VertexID]Vertex),
		parents:   make(map[types.VertexID][]types.VertexID),
		conflicts: make(map[types.VertexID][]types.VertexID),
	}
}

// Add adds a vertex to the graph
func (g *graph) Add(v Vertex) error {
	id := v.ID()
	g.vertices[id] = v
	g.parents[id] = v.Parents()
	// Conflict detection would be implemented here
	return nil
}

// Parents returns the parents of a vertex
func (g *graph) Parents(id types.VertexID) []types.VertexID {
	return g.parents[id]
}

// Conflicts returns conflicting vertices
func (g *graph) Conflicts(id types.VertexID) []types.VertexID {
	return g.conflicts[id]
}

// Ancestors returns all ancestors of a vertex
func (g *graph) Ancestors(id types.VertexID) []types.VertexID {
	visited := make(map[types.VertexID]bool)
	var ancestors []types.VertexID
	
	var traverse func(types.VertexID)
	traverse = func(vid types.VertexID) {
		if visited[vid] {
			return
		}
		visited[vid] = true
		ancestors = append(ancestors, vid)
		
		for _, parent := range g.parents[vid] {
			traverse(parent)
		}
	}
	
	for _, parent := range g.parents[id] {
		traverse(parent)
	}
	
	return ancestors
}

// Size returns the number of vertices in the graph
func (g *graph) Size() int {
	return len(g.vertices)
}