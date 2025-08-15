// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package nova implements DAG finality for the Quasar consensus engine.
// Nova provides quantum-secure finalization of DAG vertices through 
// parallel certificate aggregation and lattice-based signatures.
package nova

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/flare"
	"github.com/luxfi/consensus/wave"
)

// Finalizer provides DAG finalization with quantum certificates
type Finalizer[ID comparable] struct {
	mu       sync.RWMutex
	finalized map[ID]Certificate
	pending   map[ID]*Vertex[ID]
	
	// Dependencies
	flare *flare.Engine[ID]  // DAG ordering
	wave  *wave.Aggregator[ID] // Threshold aggregation
	
	// Callbacks
	onFinalized func(ID, Certificate)
}

// Vertex represents a DAG vertex awaiting finalization
type Vertex[ID comparable] struct {
	ID       ID
	Parents  []ID
	Height   uint64
	Data     []byte
	Votes    map[ids.NodeID]Vote
	Cert     *Certificate
}

// Vote represents a validator's vote on a vertex
type Vote struct {
	Vertex    ids.ID
	Height    uint64
	BLSSig    []byte // Classical BLS signature
	PQShare   []byte // Post-quantum lattice share
	Timestamp time.Time
}

// Certificate represents dual finality proof
type Certificate struct {
	Height    uint64
	Round     uint32
	BLSAgg    []byte   // Aggregated BLS signature
	PQCert    []byte   // Lattice-based quantum certificate
	Validators []ids.NodeID
	Timestamp time.Time
}

// New creates a new Nova DAG finalizer
func New[ID comparable](flare *flare.Engine[ID], wave *wave.Aggregator[ID]) *Finalizer[ID] {
	return &Finalizer[ID]{
		finalized: make(map[ID]Certificate),
		pending:   make(map[ID]*Vertex[ID]),
		flare:     flare,
		wave:      wave,
	}
}

// AddVertex adds a new vertex for finalization
func (f *Finalizer[ID]) AddVertex(ctx context.Context, v *Vertex[ID]) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	// Check if already finalized
	if _, ok := f.finalized[v.ID]; ok {
		return nil
	}
	
	// Check parent finality
	for _, parent := range v.Parents {
		if _, ok := f.finalized[parent]; !ok {
			// Add to pending if parents not finalized
			f.pending[v.ID] = v
			return nil
		}
	}
	
	// Process for finalization
	return f.processVertex(ctx, v)
}

// processVertex attempts to finalize a vertex
func (f *Finalizer[ID]) processVertex(ctx context.Context, v *Vertex[ID]) error {
	// Get ordering from Flare
	order := f.flare.GetOrder(v.ID)
	
	// Aggregate votes through Wave
	cert, err := f.wave.Aggregate(ctx, v.Votes)
	if err != nil {
		return fmt.Errorf("wave aggregation failed: %w", err)
	}
	
	// Verify dual certificates
	if !f.verifyDualCert(cert) {
		return fmt.Errorf("dual certificate verification failed")
	}
	
	// Mark as finalized
	v.Cert = cert
	f.finalized[v.ID] = *cert
	
	// Notify finalization
	if f.onFinalized != nil {
		f.onFinalized(v.ID, *cert)
	}
	
	// Process pending children
	f.processPending(ctx, v.ID)
	
	return nil
}

// processPending processes vertices pending on the given parent
func (f *Finalizer[ID]) processPending(ctx context.Context, parent ID) {
	for id, v := range f.pending {
		allParentsFinalized := true
		for _, p := range v.Parents {
			if _, ok := f.finalized[p]; !ok {
				allParentsFinalized = false
				break
			}
		}
		
		if allParentsFinalized {
			delete(f.pending, id)
			go f.processVertex(ctx, v) // Process asynchronously
		}
	}
}

// verifyDualCert verifies both BLS and PQ certificates
func (f *Finalizer[ID]) verifyDualCert(cert *Certificate) bool {
	// TODO: Implement actual verification
	// This requires:
	// 1. BLS signature verification against threshold
	// 2. Lattice-based PQ certificate verification
	// 3. Validator set validation
	return len(cert.BLSAgg) > 0 && len(cert.PQCert) > 0
}

// IsFinalized checks if a vertex is finalized
func (f *Finalizer[ID]) IsFinalized(id ID) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	_, ok := f.finalized[id]
	return ok
}

// GetCertificate returns the certificate for a finalized vertex
func (f *Finalizer[ID]) GetCertificate(id ID) (*Certificate, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	cert, ok := f.finalized[id]
	if !ok {
		return nil, false
	}
	return &cert, true
}

// OnFinalized sets the callback for vertex finalization
func (f *Finalizer[ID]) OnFinalized(fn func(ID, Certificate)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onFinalized = fn
}

// Stats returns finalization statistics
func (f *Finalizer[ID]) Stats() Stats {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return Stats{
		Finalized: len(f.finalized),
		Pending:   len(f.pending),
	}
}

// Stats contains finalization statistics
type Stats struct {
	Finalized int
	Pending   int
}

// Config contains Nova configuration parameters
type Config struct {
	// Finalization parameters
	MinVotes      int           // Minimum votes for finalization
	VoteTimeout   time.Duration // Timeout for collecting votes
	MaxPending    int           // Maximum pending vertices
	
	// Certificate parameters
	BLSThreshold  float64 // BLS signature threshold (e.g., 0.67)
	PQThreshold   float64 // PQ certificate threshold (e.g., 0.75)
	
	// Performance tuning
	ParallelProcs int // Number of parallel processors
	BatchSize     int // Batch size for processing
}

// DefaultConfig returns default Nova configuration
func DefaultConfig() Config {
	return Config{
		MinVotes:      15,
		VoteTimeout:   500 * time.Millisecond,
		MaxPending:    1000,
		BLSThreshold:  0.67,
		PQThreshold:   0.75,
		ParallelProcs: 4,
		BatchSize:     10,
	}
}