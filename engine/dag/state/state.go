package state

import (
	"github.com/luxfi/ids"
)

// State represents DAG state
type State interface {
	// GetVertex gets a vertex
	GetVertex(ids.ID) (Vertex, error)

	// AddVertex adds a vertex
	AddVertex(Vertex) error

	// VertexIssued checks if vertex issued
	VertexIssued(Vertex) bool

	// IsProcessing checks if processing
	IsProcessing(ids.ID) bool
}

// Vertex represents a DAG vertex
type Vertex interface {
	ID() ids.ID
	ParentIDs() []ids.ID
	Height() uint64
	Bytes() []byte
}

// state implementation
type state struct {
	vertices   map[ids.ID]Vertex
	processing map[ids.ID]bool
}

// New creates a new state
func New() State {
	return &state{
		vertices:   make(map[ids.ID]Vertex),
		processing: make(map[ids.ID]bool),
	}
}

// GetVertex gets a vertex
func (s *state) GetVertex(id ids.ID) (Vertex, error) {
	vertex, exists := s.vertices[id]
	if !exists {
		return nil, ErrVertexNotFound
	}
	return vertex, nil
}

// AddVertex adds a vertex
func (s *state) AddVertex(vertex Vertex) error {
	s.vertices[vertex.ID()] = vertex
	return nil
}

// VertexIssued checks if vertex issued
func (s *state) VertexIssued(vertex Vertex) bool {
	_, exists := s.vertices[vertex.ID()]
	return exists
}

// IsProcessing checks if processing
func (s *state) IsProcessing(id ids.ID) bool {
	return s.processing[id]
}

// ErrVertexNotFound is returned when vertex not found
var ErrVertexNotFound = &vertexNotFoundError{}

type vertexNotFoundError struct{}

func (e *vertexNotFoundError) Error() string {
	return "vertex not found"
}
