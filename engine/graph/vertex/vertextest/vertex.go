// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertextest

import (
	"context"
	"errors"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/ids"
)

var (
	ErrNotImplemented = errors.New("not implemented")
)

// Vertex is a test vertex implementation
type Vertex struct {
	IDV      ids.ID
	ParentsV []ids.ID
	HeightV  uint64
	TxsV     [][]byte
	BytesV   []byte
	AcceptV  error
	RejectV  error
	VerifyV  error
	StatusV  choices.Status
}

// ID returns the vertex ID
func (v *Vertex) ID() ids.ID {
	return v.IDV
}

// Parents returns the parent IDs
func (v *Vertex) Parents() []ids.ID {
	return v.ParentsV
}

// Height returns the vertex height
func (v *Vertex) Height() uint64 {
	return v.HeightV
}

// Txs returns the transactions
func (v *Vertex) Txs() [][]byte {
	return v.TxsV
}

// Bytes returns the vertex bytes
func (v *Vertex) Bytes() []byte {
	return v.BytesV
}

// Accept accepts the vertex
func (v *Vertex) Accept(ctx context.Context) error {
	if v.AcceptV != nil {
		return v.AcceptV
	}
	v.StatusV = choices.Accepted
	return nil
}

// Reject rejects the vertex
func (v *Vertex) Reject(ctx context.Context) error {
	if v.RejectV != nil {
		return v.RejectV
	}
	v.StatusV = choices.Rejected
	return nil
}

// Verify verifies the vertex
func (v *Vertex) Verify(ctx context.Context) error {
	if v.VerifyV != nil {
		return v.VerifyV
	}
	return nil
}

// Status returns the vertex status
func (v *Vertex) Status() choices.Status {
	return v.StatusV
}

// Builder builds test vertices
type Builder struct {
	idCounter uint64
}

// NewBuilder creates a new vertex builder
func NewBuilder() *Builder {
	return &Builder{}
}

// Build creates a new vertex with the given parents
func (b *Builder) Build(parents ...ids.ID) *Vertex {
	b.idCounter++
	id := ids.GenerateTestID()
	return &Vertex{
		IDV:      id,
		ParentsV: parents,
		HeightV:  b.idCounter,
		BytesV:   id[:],
		StatusV:  choices.Processing,
	}
}

// BuildWithID creates a new vertex with a specific ID
func (b *Builder) BuildWithID(id ids.ID, parents ...ids.ID) *Vertex {
	b.idCounter++
	return &Vertex{
		IDV:      id,
		ParentsV: parents,
		HeightV:  b.idCounter,
		BytesV:   id[:],
		StatusV:  choices.Processing,
	}
}

// BuildChain builds a chain of vertices
func (b *Builder) BuildChain(length int) []*Vertex {
	vertices := make([]*Vertex, length)

	for i := 0; i < length; i++ {
		if i == 0 {
			vertices[i] = b.Build()
		} else {
			vertices[i] = b.Build(vertices[i-1].ID())
		}
	}

	return vertices
}

// BuildDAG builds a DAG of vertices
func (b *Builder) BuildDAG(width, depth int) [][]*Vertex {
	dag := make([][]*Vertex, depth)

	for d := 0; d < depth; d++ {
		dag[d] = make([]*Vertex, width)

		for w := 0; w < width; w++ {
			if d == 0 {
				dag[d][w] = b.Build()
			} else {
				// Connect to multiple parents from previous level
				var parents []ids.ID
				for p := 0; p < width && p <= w; p++ {
					parents = append(parents, dag[d-1][p].ID())
				}
				dag[d][w] = b.Build(parents...)
			}
		}
	}

	return dag
}

// Storage stores vertices for testing
type Storage struct {
	vertices map[ids.ID]*Vertex
}

// NewStorage creates a new vertex storage
func NewStorage() *Storage {
	return &Storage{
		vertices: make(map[ids.ID]*Vertex),
	}
}

// Add adds a vertex to storage
func (s *Storage) Add(v *Vertex) {
	s.vertices[v.ID()] = v
}

// Get retrieves a vertex from storage
func (s *Storage) Get(id ids.ID) (*Vertex, bool) {
	v, ok := s.vertices[id]
	return v, ok
}

// Remove removes a vertex from storage
func (s *Storage) Remove(id ids.ID) {
	delete(s.vertices, id)
}

// Len returns the number of stored vertices
func (s *Storage) Len() int {
	return len(s.vertices)
}

// Clear removes all vertices
func (s *Storage) Clear() {
	s.vertices = make(map[ids.ID]*Vertex)
}
