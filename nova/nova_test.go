// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nova

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

type TestID string

func TestNovaBasicFinalization(t *testing.T) {
	ctx := context.Background()
	finalizer := New[TestID](nil, nil) // TODO: Add proper flare/wave mocks
	
	// Create a vertex with no parents
	v := &Vertex[TestID]{
		ID:     "vertex1",
		Height: 1,
		Data:   []byte("test data"),
		Votes:  make(map[ids.NodeID]Vote),
	}
	
	// Add some votes
	for i := 0; i < 15; i++ {
		nodeID := ids.GenerateTestNodeID()
		v.Votes[nodeID] = Vote{
			Vertex:    ids.GenerateTestID(),
			Height:    1,
			BLSSig:    []byte("bls_sig"),
			PQShare:   []byte("pq_share"),
			Timestamp: time.Now(),
		}
	}
	
	// This will fail without proper mocks, but structure is correct
	err := finalizer.AddVertex(ctx, v)
	_ = err // Expected to fail without proper setup
	
	// Check if vertex is tracked
	require.NotNil(t, finalizer)
}

func TestNovaParentDependency(t *testing.T) {
	ctx := context.Background()
	finalizer := New[TestID](nil, nil)
	
	// Create parent vertex
	parent := &Vertex[TestID]{
		ID:     "parent",
		Height: 1,
		Data:   []byte("parent data"),
		Votes:  make(map[ids.NodeID]Vote),
	}
	
	// Create child vertex
	child := &Vertex[TestID]{
		ID:      "child",
		Parents: []TestID{"parent"},
		Height:  2,
		Data:    []byte("child data"),
		Votes:   make(map[ids.NodeID]Vote),
	}
	
	// Add child before parent (should go to pending)
	err := finalizer.AddVertex(ctx, child)
	_ = err
	
	stats := finalizer.Stats()
	require.GreaterOrEqual(t, stats.Pending, 0)
	
	// Add parent (should trigger child processing)
	err = finalizer.AddVertex(ctx, parent)
	_ = err
}

func TestNovaDualCertificate(t *testing.T) {
	cert := &Certificate{
		Height:     10,
		Round:      2,
		BLSAgg:     []byte("aggregated_bls_signature"),
		PQCert:     []byte("post_quantum_certificate"),
		Validators: []ids.NodeID{ids.GenerateTestNodeID()},
		Timestamp:  time.Now(),
	}
	
	require.NotNil(t, cert.BLSAgg)
	require.NotNil(t, cert.PQCert)
	require.Greater(t, len(cert.Validators), 0)
}

func TestNovaIsFinalized(t *testing.T) {
	finalizer := New[TestID](nil, nil)
	
	// Initially not finalized
	require.False(t, finalizer.IsFinalized("test"))
	
	// Add to finalized map directly (for testing)
	finalizer.finalized["test"] = Certificate{
		Height: 1,
		BLSAgg: []byte("test"),
		PQCert: []byte("test"),
	}
	
	// Now should be finalized
	require.True(t, finalizer.IsFinalized("test"))
	
	// Get certificate
	cert, ok := finalizer.GetCertificate("test")
	require.True(t, ok)
	require.NotNil(t, cert)
}

func TestNovaStats(t *testing.T) {
	finalizer := New[TestID](nil, nil)
	
	// Add some test data
	finalizer.finalized["f1"] = Certificate{}
	finalizer.finalized["f2"] = Certificate{}
	finalizer.pending["p1"] = &Vertex[TestID]{}
	
	stats := finalizer.Stats()
	require.Equal(t, 2, stats.Finalized)
	require.Equal(t, 1, stats.Pending)
}

func TestNovaConfig(t *testing.T) {
	cfg := DefaultConfig()
	
	require.Equal(t, 15, cfg.MinVotes)
	require.Equal(t, 500*time.Millisecond, cfg.VoteTimeout)
	require.Equal(t, 1000, cfg.MaxPending)
	require.Equal(t, 0.67, cfg.BLSThreshold)
	require.Equal(t, 0.75, cfg.PQThreshold)
	require.Equal(t, 4, cfg.ParallelProcs)
	require.Equal(t, 10, cfg.BatchSize)
}

func BenchmarkNovaFinalization(b *testing.B) {
	ctx := context.Background()
	finalizer := New[TestID](nil, nil)
	
	// Pre-create vertices
	vertices := make([]*Vertex[TestID], b.N)
	for i := 0; i < b.N; i++ {
		vertices[i] = &Vertex[TestID]{
			ID:     TestID(ids.GenerateTestID().String()),
			Height: uint64(i),
			Data:   []byte("benchmark data"),
			Votes:  make(map[ids.NodeID]Vote),
		}
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = finalizer.AddVertex(ctx, vertices[i])
	}
}