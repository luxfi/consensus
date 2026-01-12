package state

import (
	"context"
	"sync"

	"github.com/luxfi/database"
	"github.com/luxfi/ids"
	log "github.com/luxfi/log"
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

// DAGVM is the interface for a DAG-based virtual machine
type DAGVM interface {
	// ParseVtx parses a vertex from bytes
	ParseVtx(ctx context.Context, bytes []byte) (Vertex, error)
}

// Manager extends State with vertex management capabilities
type Manager interface {
	State

	// ParseVtx parses a vertex from bytes
	ParseVtx(ctx context.Context, bytes []byte) (Vertex, error)

	// GetVtx returns a vertex by ID
	GetVtx(ctx context.Context, vtxID ids.ID) (Vertex, error)

	// Edge returns the current edge of the DAG
	Edge(ctx context.Context) []ids.ID

	// StopVertexAccepted marks a vertex as no longer accepted
	StopVertexAccepted()
}

// SerializerConfig configures the vertex serializer
type SerializerConfig struct {
	ChainID ids.ID
	VM      DAGVM
	DB      database.Database
	Log     log.Logger
}

// serializer implements Manager for vertex serialization
type serializer struct {
	mu         sync.RWMutex
	chainID    ids.ID
	vm         DAGVM
	db         database.Database
	log        log.Logger
	vertices   map[ids.ID]Vertex
	processing map[ids.ID]bool
	edge       []ids.ID
}

// NewSerializer creates a new vertex serializer/manager
func NewSerializer(config SerializerConfig) Manager {
	return &serializer{
		chainID:    config.ChainID,
		vm:         config.VM,
		db:         config.DB,
		log:        config.Log,
		vertices:   make(map[ids.ID]Vertex),
		processing: make(map[ids.ID]bool),
		edge:       make([]ids.ID, 0),
	}
}

// GetVertex gets a vertex by ID
func (s *serializer) GetVertex(id ids.ID) (Vertex, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	vertex, exists := s.vertices[id]
	if !exists {
		return nil, ErrVertexNotFound
	}
	return vertex, nil
}

// AddVertex adds a vertex to the state
func (s *serializer) AddVertex(vertex Vertex) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vertices[vertex.ID()] = vertex
	return nil
}

// VertexIssued checks if a vertex has been issued
func (s *serializer) VertexIssued(vertex Vertex) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.vertices[vertex.ID()]
	return exists
}

// IsProcessing checks if a vertex is being processed
func (s *serializer) IsProcessing(id ids.ID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.processing[id]
}

// ParseVtx parses a vertex from bytes
func (s *serializer) ParseVtx(ctx context.Context, bytes []byte) (Vertex, error) {
	return s.vm.ParseVtx(ctx, bytes)
}

// GetVtx returns a vertex by ID, fetching from VM if necessary
func (s *serializer) GetVtx(ctx context.Context, vtxID ids.ID) (Vertex, error) {
	s.mu.RLock()
	vtx, exists := s.vertices[vtxID]
	s.mu.RUnlock()
	if exists {
		return vtx, nil
	}

	// Try to fetch from database
	bytes, err := s.db.Get(vtxID[:])
	if err != nil {
		return nil, err
	}

	return s.ParseVtx(ctx, bytes)
}

// Edge returns the current edge of the DAG
func (s *serializer) Edge(ctx context.Context) []ids.ID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	edge := make([]ids.ID, len(s.edge))
	copy(edge, s.edge)
	return edge
}

// StopVertexAccepted marks vertices as no longer being accepted
func (s *serializer) StopVertexAccepted() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processing = make(map[ids.ID]bool)
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
