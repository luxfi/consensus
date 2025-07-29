// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package galaxy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/consensus/engine/nebula"
	"github.com/luxfi/consensus/flare"
	"github.com/luxfi/consensus/focus"
	"github.com/luxfi/consensus/nova"
	"github.com/luxfi/consensus/photon"
	"github.com/luxfi/consensus/wave"
	"github.com/luxfi/ids"
)

// Runtime implements the Galaxy runtime for DAG consensus
// This operates at stellar cluster scale, handling multiple parallel chains
type Runtime struct {
	// Core engine
	engine *nebula.Engine
	
	// Consensus stages
	photonStage photon.Photon   // Quantum sampling
	waveStage   wave.Wave       // Propagation
	focusStage  focus.Focus     // Confidence aggregation
	flareStage  flare.Flare     // Rapid vertex ordering
	novaStage   nova.Nova       // DAG finalization
	
	// Runtime state
	params    Parameters
	state     *runtimeState
	mu        sync.RWMutex
	
	// DAG structure
	vertices  map[ids.ID]*Vertex
	frontier  map[ids.ID]bool
	
	// Metrics
	metrics   *Metrics
}

// Parameters for Galaxy runtime
type Parameters struct {
	// Network parameters
	K               int
	AlphaPreference int
	AlphaConfidence int
	Beta            int
	
	// DAG parameters
	MaxParents         int
	MaxVerticesPerRound int
	ConflictSetSize    int
	
	// Performance tuning
	MaxConcurrentVertices int
	VertexTimeout         time.Duration
}

// New creates a new Galaxy runtime
func New(params Parameters) (*Runtime, error) {
	// Create consensus stages
	photonFactory := photon.NewFactory()
	waveFactory := wave.NewFactory()
	focusFactory := focus.NewFactory()
	
	photonStage := photonFactory.New()
	waveStage := waveFactory.New()
	focusStage := focusFactory.New()
	
	// Create DAG-specific stages
	flareStage := flare.New(flare.Parameters{
		MaxParents:      params.MaxParents,
		ConflictSetSize: params.ConflictSetSize,
	})
	
	novaStage := nova.New(nova.Parameters{
		K:               params.K,
		AlphaPreference: params.AlphaPreference,
		AlphaConfidence: params.AlphaConfidence,
		Beta:            params.Beta,
	})
	
	// Create nebula engine
	engine := nebula.New(nebula.Parameters{
		K:               params.K,
		AlphaPreference: params.AlphaPreference,
		AlphaConfidence: params.AlphaConfidence,
		Beta:            params.Beta,
	})
	
	return &Runtime{
		engine:      engine,
		photonStage: photonStage,
		waveStage:   waveStage,
		focusStage:  focusStage,
		flareStage:  flareStage,
		novaStage:   novaStage,
		params:      params,
		state:       newRuntimeState(),
		vertices:    make(map[ids.ID]*Vertex),
		frontier:    make(map[ids.ID]bool),
		metrics:     NewMetrics(),
	}, nil
}

// Start begins the Galaxy runtime
func (r *Runtime) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if r.state.running {
		return fmt.Errorf("runtime already running")
	}
	
	// Initialize stages
	r.photonStage.Initialize(r.params.K, r.params.AlphaPreference)
	r.waveStage.Initialize(r.params.AlphaPreference, r.params.AlphaConfidence)
	r.focusStage.Initialize(r.params.AlphaConfidence, r.params.Beta)
	
	// Start engine
	if err := r.engine.Start(ctx); err != nil {
		return fmt.Errorf("failed to start nebula engine: %w", err)
	}
	
	r.state.running = true
	r.state.startTime = time.Now()
	
	return nil
}

// Stop halts the Galaxy runtime
func (r *Runtime) Stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if !r.state.running {
		return fmt.Errorf("runtime not running")
	}
	
	// Stop engine
	if err := r.engine.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop nebula engine: %w", err)
	}
	
	r.state.running = false
	return nil
}

// SubmitVertex submits a DAG vertex for consensus
func (r *Runtime) SubmitVertex(ctx context.Context, vertex Vertex) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if !r.state.running {
		return fmt.Errorf("runtime not running")
	}
	
	// Validate vertex
	if err := r.validateVertex(vertex); err != nil {
		return fmt.Errorf("invalid vertex: %w", err)
	}
	
	// Store vertex
	r.vertices[vertex.ID()] = &vertex
	
	// Process through consensus stages
	// 1. Photon stage - quantum sampling
	if err := r.processPhoton(ctx, vertex); err != nil {
		return fmt.Errorf("photon stage failed: %w", err)
	}
	
	// 2. Wave stage - propagation
	if err := r.processWave(ctx, vertex); err != nil {
		return fmt.Errorf("wave stage failed: %w", err)
	}
	
	// 3. Focus stage - confidence aggregation
	if err := r.processFocus(ctx, vertex); err != nil {
		return fmt.Errorf("focus stage failed: %w", err)
	}
	
	// 4. Flare stage - rapid ordering
	if err := r.processFlare(ctx, vertex); err != nil {
		return fmt.Errorf("flare stage failed: %w", err)
	}
	
	// 5. Nova stage - DAG finalization
	if err := r.processNova(ctx, vertex); err != nil {
		return fmt.Errorf("nova stage failed: %w", err)
	}
	
	// Update frontier
	r.updateFrontier(vertex)
	
	r.metrics.ProcessedVertices.Inc()
	return nil
}

// validateVertex ensures the vertex is valid
func (r *Runtime) validateVertex(vertex Vertex) error {
	// Check parent count
	if len(vertex.Parents()) > r.params.MaxParents {
		return fmt.Errorf("too many parents: %d > %d", len(vertex.Parents()), r.params.MaxParents)
	}
	
	// Verify parents exist
	for _, parentID := range vertex.Parents() {
		if _, exists := r.vertices[parentID]; !exists {
			return fmt.Errorf("parent %s does not exist", parentID)
		}
	}
	
	// Verify vertex signature
	return vertex.Verify()
}

// processPhoton handles quantum sampling stage
func (r *Runtime) processPhoton(ctx context.Context, vertex Vertex) error {
	r.photonStage.RecordPoll(vertex.ID())
	return nil
}

// processWave handles propagation stage
func (r *Runtime) processWave(ctx context.Context, vertex Vertex) error {
	r.waveStage.RecordPoll(vertex.ID())
	return nil
}

// processFocus handles confidence aggregation
func (r *Runtime) processFocus(ctx context.Context, vertex Vertex) error {
	r.focusStage.RecordPoll(vertex.ID())
	return nil
}

// processFlare handles rapid ordering
func (r *Runtime) processFlare(ctx context.Context, vertex Vertex) error {
	// Flare stage determines vertex ordering within conflict sets
	return r.flareStage.Order(ctx, vertex)
}

// processNova handles DAG finalization
func (r *Runtime) processNova(ctx context.Context, vertex Vertex) error {
	// Nova stage finalizes the vertex in the DAG
	return r.novaStage.Finalize(ctx, vertex)
}

// updateFrontier updates the DAG frontier
func (r *Runtime) updateFrontier(vertex Vertex) {
	// Add vertex to frontier
	r.frontier[vertex.ID()] = true
	
	// Remove parents from frontier
	for _, parentID := range vertex.Parents() {
		delete(r.frontier, parentID)
	}
	
	r.state.frontierSize = len(r.frontier)
}

// GetFrontier returns the current DAG frontier
func (r *Runtime) GetFrontier() []ids.ID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	frontier := make([]ids.ID, 0, len(r.frontier))
	for id := range r.frontier {
		frontier = append(frontier, id)
	}
	return frontier
}

// GetVertex returns a vertex by ID
func (r *Runtime) GetVertex(id ids.ID) (Vertex, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	vertex, exists := r.vertices[id]
	if !exists {
		return nil, fmt.Errorf("vertex %s not found", id)
	}
	return *vertex, nil
}

// Metrics returns runtime metrics
func (r *Runtime) Metrics() *Metrics {
	return r.metrics
}

// Vertex interface for Galaxy runtime
type Vertex interface {
	ID() ids.ID
	Parents() []ids.ID
	Height() uint64
	Timestamp() time.Time
	Bytes() []byte
	Verify() error
}

// Runtime state
type runtimeState struct {
	running      bool
	startTime    time.Time
	vertexCount  uint64
	frontierSize int
}

func newRuntimeState() *runtimeState {
	return &runtimeState{}
}

// Metrics tracking
type Metrics struct {
	ProcessedVertices  Counter
	FinalizedVertices  Counter
	ConsensusLatency   Histogram
	VerticesPerSecond  Gauge
	FrontierSize       Gauge
}

// Stub types for metrics
type Counter struct{ count int64 }
func (c *Counter) Inc() { c.count++ }

type Histogram struct{}
type Gauge struct{ value float64 }

func NewMetrics() *Metrics {
	return &Metrics{}
}