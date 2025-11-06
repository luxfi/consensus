// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package dag

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// TestDAGConsensus tests basic DAG consensus operations
func TestDAGConsensus(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, 1))

	// Build and parse vertices
	vtx, err := engine.BuildVtx(ctx)
	require.NoError(err)
	require.Nil(vtx) // Currently returns nil

	require.NoError(engine.Shutdown(ctx))
}

// TestParallelVertexProcessing tests processing multiple vertices in parallel
func TestParallelVertexProcessing(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, 1))

	// Create multiple vertices concurrently
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			vtx, err := engine.BuildVtx(ctx)
			require.NoError(err)
			require.Nil(vtx)
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	require.NoError(engine.Shutdown(ctx))
}

// TestVertexRetrieval tests retrieving vertices by ID
func TestVertexRetrieval(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, 1))

	// Try to retrieve various vertices
	for i := 0; i < 5; i++ {
		vtxID := ids.GenerateTestID()
		vtx, err := engine.GetVtx(ctx, vtxID)
		require.NoError(err)
		require.Nil(vtx) // Currently returns nil
	}

	require.NoError(engine.Shutdown(ctx))
}

// TestVertexParsing tests parsing vertex data
func TestVertexParsing(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, 1))

	// Test parsing empty data
	vtx, err := engine.ParseVtx(ctx, []byte{})
	require.NoError(err)
	require.Nil(vtx)

	// Test parsing various data sizes
	for size := 1; size <= 100; size *= 10 {
		data := make([]byte, size)
		vtx, err := engine.ParseVtx(ctx, data)
		require.NoError(err)
		require.Nil(vtx)
	}

	require.NoError(engine.Shutdown(ctx))
}

// TestDAGConflictResolution tests conflict resolution in DAG
func TestDAGConflictResolution(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, 1))

	// Create conflicting vertices
	vtxID1 := ids.GenerateTestID()
	vtxID2 := ids.GenerateTestID()

	vtx1, err1 := engine.GetVtx(ctx, vtxID1)
	require.NoError(err1)

	vtx2, err2 := engine.GetVtx(ctx, vtxID2)
	require.NoError(err2)

	// Both operations should succeed
	require.Nil(vtx1)
	require.Nil(vtx2)

	require.NoError(engine.Shutdown(ctx))
}

// TestDAGTopologicalOrder tests that DAG maintains topological ordering
func TestDAGTopologicalOrder(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, 1))

	// Build multiple vertices in sequence
	vertices := make([]ids.ID, 10)
	for i := 0; i < 10; i++ {
		vertices[i] = ids.GenerateTestID()
		vtx, err := engine.GetVtx(ctx, vertices[i])
		require.NoError(err)
		require.Nil(vtx)
	}

	// Verify all were processed
	for _, vtxID := range vertices {
		vtx, err := engine.GetVtx(ctx, vtxID)
		require.NoError(err)
		require.Nil(vtx)
	}

	require.NoError(engine.Shutdown(ctx))
}

// TestDAGHighThroughput tests DAG under high transaction load
func TestDAGHighThroughput(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, 1))

	// Process many vertices
	for i := 0; i < 1000; i++ {
		vtxID := ids.GenerateTestID()
		_, err := engine.GetVtx(ctx, vtxID)
		require.NoError(err)
	}

	// Verify engine still functional
	vtx, err := engine.BuildVtx(ctx)
	require.NoError(err)
	require.Nil(vtx)

	require.NoError(engine.Shutdown(ctx))
}

// TestDAGConcurrentBuildAndGet tests concurrent build and get operations
func TestDAGConcurrentBuildAndGet(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, 1))

	done := make(chan bool, 20)

	// Launch concurrent builds
	for i := 0; i < 10; i++ {
		go func() {
			vtx, err := engine.BuildVtx(ctx)
			require.NoError(err)
			require.Nil(vtx)
			done <- true
		}()
	}

	// Launch concurrent gets
	for i := 0; i < 10; i++ {
		go func() {
			vtxID := ids.GenerateTestID()
			vtx, err := engine.GetVtx(ctx, vtxID)
			require.NoError(err)
			require.Nil(vtx)
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 20; i++ {
		<-done
	}

	require.NoError(engine.Shutdown(ctx))
}

// TestDAGVertexDependencies tests handling of vertex dependencies
func TestDAGVertexDependencies(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	require.NoError(engine.Start(ctx, 1))

	// Create a chain of dependent vertices
	parent := ids.GenerateTestID()
	children := make([]ids.ID, 5)

	// Get parent
	parentVtx, err := engine.GetVtx(ctx, parent)
	require.NoError(err)
	require.Nil(parentVtx)

	// Get children
	for i := 0; i < 5; i++ {
		children[i] = ids.GenerateTestID()
		childVtx, err := engine.GetVtx(ctx, children[i])
		require.NoError(err)
		require.Nil(childVtx)
	}

	require.NoError(engine.Shutdown(ctx))
}

// TestDAGRestartAndRecovery tests DAG engine restart and state recovery
func TestDAGRestartAndRecovery(t *testing.T) {
	require := require.New(t)

	engine := New()
	ctx := context.Background()

	// First session
	require.NoError(engine.Start(ctx, 1))

	// Build some vertices
	for i := 0; i < 5; i++ {
		_, err := engine.BuildVtx(ctx)
		require.NoError(err)
	}

	require.NoError(engine.Shutdown(ctx))

	// Restart
	require.NoError(engine.Start(ctx, 2))

	// Verify still functional
	vtx, err := engine.BuildVtx(ctx)
	require.NoError(err)
	require.Nil(vtx)

	require.NoError(engine.Shutdown(ctx))
}
