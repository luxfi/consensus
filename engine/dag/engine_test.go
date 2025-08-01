// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package dag

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/ids"
)

// MockVertex implements the Vertex interface for testing
type MockVertex struct {
	id        ids.ID
	parents   []ids.ID
	height    uint64
	timestamp time.Time
	bytes     []byte
	verified  bool
}

func (v *MockVertex) ID() ids.ID          { return v.id }
func (v *MockVertex) Parents() []ids.ID   { return v.parents }
func (v *MockVertex) Height() uint64      { return v.height }
func (v *MockVertex) Timestamp() time.Time { return v.timestamp }
func (v *MockVertex) Bytes() []byte       { return v.bytes }
func (v *MockVertex) Verify() error {
	if !v.verified {
		return nil
	}
	return nil
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		params  Parameters
		wantErr bool
	}{
		{
			name: "valid parameters",
			params: Parameters{
				K:                     21,
				AlphaPreference:       13,
				AlphaConfidence:       18,
				Beta:                  8,
				MaxParents:            5,
				MaxVerticesPerRound:   100,
				ConflictSetSize:       10,
				MaxConcurrentVertices: 50,
				VertexTimeout:         5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "empty parameters",
			params:  Parameters{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			ctx := &interfaces.Context{}
			engine, err := New(ctx, tt.params)
			if tt.wantErr {
				require.Error(err)
				return
			}

			require.NoError(err)
			require.NotNil(engine)
			require.Equal(tt.params, engine.params)
			require.NotNil(engine.state)
			require.NotNil(engine.vertices)
			require.NotNil(engine.frontier)
			require.NotNil(engine.metrics)
		})
	}
}

func TestStartStop(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		MaxParents:      5,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	// Test Start
	err = engine.Start(context.Background())
	require.NoError(err)
	require.True(engine.state.running)
	require.NotZero(engine.state.startTime)

	// Test double start
	err = engine.Start(context.Background())
	require.Error(err)
	require.Contains(err.Error(), "engine already running")

	// Test Stop
	err = engine.Stop(context.Background())
	require.NoError(err)
	require.False(engine.state.running)

	// Test double stop
	err = engine.Stop(context.Background())
	require.Error(err)
	require.Contains(err.Error(), "engine not running")
}

func TestSubmitVertex(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		MaxParents:      5,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	err = engine.Start(context.Background())
	require.NoError(err)

	// Create and submit a genesis vertex
	genesis := &MockVertex{
		id:        ids.GenerateTestID(),
		parents:   []ids.ID{},
		height:    0,
		timestamp: time.Now(),
		bytes:     []byte("genesis"),
		verified:  true,
	}

	err = engine.SubmitVertex(context.Background(), genesis)
	require.NoError(err)

	// Verify vertex was stored
	stored, err := engine.GetVertex(genesis.ID())
	require.NoError(err)
	require.Equal(genesis.ID(), stored.ID())

	// Verify frontier was updated
	frontier := engine.GetFrontier()
	require.Len(frontier, 1)
	require.Equal(genesis.ID(), frontier[0])

	// Submit a child vertex
	child := &MockVertex{
		id:        ids.GenerateTestID(),
		parents:   []ids.ID{genesis.ID()},
		height:    1,
		timestamp: time.Now(),
		bytes:     []byte("child"),
		verified:  true,
	}

	err = engine.SubmitVertex(context.Background(), child)
	require.NoError(err)

	// Verify frontier was updated
	frontier = engine.GetFrontier()
	require.Len(frontier, 1)
	require.Equal(child.ID(), frontier[0])
}

func TestSubmitVertexValidation(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		MaxParents:      2, // Small limit for testing
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	err = engine.Start(context.Background())
	require.NoError(err)

	// Test submitting vertex when engine not running
	err = engine.Stop(context.Background())
	require.NoError(err)

	vertex := &MockVertex{
		id: ids.GenerateTestID(),
	}
	err = engine.SubmitVertex(context.Background(), vertex)
	require.Error(err)
	require.Contains(err.Error(), "engine not running")

	// Start engine again
	err = engine.Start(context.Background())
	require.NoError(err)

	// Test vertex with too many parents
	tooManyParents := &MockVertex{
		id:      ids.GenerateTestID(),
		parents: []ids.ID{ids.GenerateTestID(), ids.GenerateTestID(), ids.GenerateTestID()},
	}
	err = engine.SubmitVertex(context.Background(), tooManyParents)
	require.Error(err)
	require.Contains(err.Error(), "too many parents")

	// Test vertex with non-existent parent
	missingParent := &MockVertex{
		id:      ids.GenerateTestID(),
		parents: []ids.ID{ids.GenerateTestID()},
	}
	err = engine.SubmitVertex(context.Background(), missingParent)
	require.Error(err)
	require.Contains(err.Error(), "does not exist")
}

func TestGetVertex(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		MaxParents:      5,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	err = engine.Start(context.Background())
	require.NoError(err)

	// Test getting non-existent vertex
	_, err = engine.GetVertex(ids.GenerateTestID())
	require.Error(err)
	require.Contains(err.Error(), "not found")

	// Submit a vertex
	vertex := &MockVertex{
		id:        ids.GenerateTestID(),
		parents:   []ids.ID{},
		height:    0,
		timestamp: time.Now(),
		bytes:     []byte("test"),
		verified:  true,
	}

	err = engine.SubmitVertex(context.Background(), vertex)
	require.NoError(err)

	// Get the vertex
	retrieved, err := engine.GetVertex(vertex.ID())
	require.NoError(err)
	require.Equal(vertex.ID(), retrieved.ID())
}

func TestFrontierManagement(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		MaxParents:      5,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	err = engine.Start(context.Background())
	require.NoError(err)

	// Initial frontier should be empty
	frontier := engine.GetFrontier()
	require.Empty(frontier)

	// Add vertices to create a DAG structure
	v1 := &MockVertex{
		id:       ids.GenerateTestID(),
		parents:  []ids.ID{},
		verified: true,
	}
	v2 := &MockVertex{
		id:       ids.GenerateTestID(),
		parents:  []ids.ID{},
		verified: true,
	}
	v3 := &MockVertex{
		id:       ids.GenerateTestID(),
		parents:  []ids.ID{v1.ID(), v2.ID()},
		verified: true,
	}

	// Submit v1 and v2
	err = engine.SubmitVertex(context.Background(), v1)
	require.NoError(err)
	err = engine.SubmitVertex(context.Background(), v2)
	require.NoError(err)

	// Frontier should contain both v1 and v2
	frontier = engine.GetFrontier()
	require.Len(frontier, 2)
	frontierSet := make(map[ids.ID]bool)
	for _, id := range frontier {
		frontierSet[id] = true
	}
	require.True(frontierSet[v1.ID()])
	require.True(frontierSet[v2.ID()])

	// Submit v3
	err = engine.SubmitVertex(context.Background(), v3)
	require.NoError(err)

	// Frontier should now only contain v3
	frontier = engine.GetFrontier()
	require.Len(frontier, 1)
	require.Equal(v3.ID(), frontier[0])
}

func TestMetrics(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		MaxParents:      5,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	metrics := engine.Metrics()
	require.NotNil(metrics)

	// Submit vertices and check metrics
	err = engine.Start(context.Background())
	require.NoError(err)

	initialCount := metrics.ProcessedVertices.count

	vertex := &MockVertex{
		id:       ids.GenerateTestID(),
		parents:  []ids.ID{},
		verified: true,
	}

	err = engine.SubmitVertex(context.Background(), vertex)
	require.NoError(err)

	require.Equal(initialCount+1, metrics.ProcessedVertices.count)
}

func TestConcurrentVertexSubmission(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:                     21,
		AlphaPreference:       13,
		AlphaConfidence:       18,
		Beta:                  8,
		MaxParents:            5,
		MaxConcurrentVertices: 50,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	err = engine.Start(context.Background())
	require.NoError(err)

	// Submit genesis vertex
	genesis := &MockVertex{
		id:       ids.GenerateTestID(),
		parents:  []ids.ID{},
		verified: true,
	}
	err = engine.SubmitVertex(context.Background(), genesis)
	require.NoError(err)

	// Submit multiple vertices concurrently
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			vertex := &MockVertex{
				id:        ids.GenerateTestID(),
				parents:   []ids.ID{genesis.ID()},
				height:    uint64(idx + 1),
				timestamp: time.Now(),
				bytes:     []byte("concurrent vertex"),
				verified:  true,
			}
			done <- engine.SubmitVertex(context.Background(), vertex)
		}(i)
	}

	// Wait for all submissions
	for i := 0; i < 10; i++ {
		err := <-done
		require.NoError(err)
	}

	// Verify all vertices were processed
	require.Equal(int64(11), engine.metrics.ProcessedVertices.count) // genesis + 10
}

func TestComplexDAGStructure(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		MaxParents:      3,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	err = engine.Start(context.Background())
	require.NoError(err)

	// Create a complex DAG structure
	//     v1
	//    / \
	//   v2  v3
	//   |\ /|
	//   | X |
	//   |/ \|
	//   v4  v5
	//    \ /
	//     v6

	v1 := &MockVertex{id: ids.GenerateTestID(), parents: []ids.ID{}, verified: true}
	v2 := &MockVertex{id: ids.GenerateTestID(), parents: []ids.ID{v1.ID()}, verified: true}
	v3 := &MockVertex{id: ids.GenerateTestID(), parents: []ids.ID{v1.ID()}, verified: true}
	v4 := &MockVertex{id: ids.GenerateTestID(), parents: []ids.ID{v2.ID(), v3.ID()}, verified: true}
	v5 := &MockVertex{id: ids.GenerateTestID(), parents: []ids.ID{v2.ID(), v3.ID()}, verified: true}
	v6 := &MockVertex{id: ids.GenerateTestID(), parents: []ids.ID{v4.ID(), v5.ID()}, verified: true}

	// Submit all vertices in order
	vertices := []*MockVertex{v1, v2, v3, v4, v5, v6}
	for _, v := range vertices {
		err = engine.SubmitVertex(context.Background(), v)
		require.NoError(err)
	}

	// Final frontier should only contain v6
	frontier := engine.GetFrontier()
	require.Len(frontier, 1)
	require.Equal(v6.ID(), frontier[0])

	// Verify all vertices are stored
	for _, v := range vertices {
		stored, err := engine.GetVertex(v.ID())
		require.NoError(err)
		require.Equal(v.ID(), stored.ID())
	}
}

func TestEngineState(t *testing.T) {
	require := require.New(t)

	state := newEngineState()
	require.NotNil(state)
	require.False(state.running)
	require.Zero(state.vertexCount)
	require.Zero(state.frontierSize)

	// Test state mutations
	state.running = true
	state.startTime = time.Now()
	state.vertexCount = 10
	state.frontierSize = 3

	require.True(state.running)
	require.NotZero(state.startTime)
	require.Equal(uint64(10), state.vertexCount)
	require.Equal(3, state.frontierSize)
}

func BenchmarkSubmitVertex(b *testing.B) {
	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		MaxParents:      5,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(b, err)

	err = engine.Start(context.Background())
	require.NoError(b, err)

	// Create genesis
	genesis := &MockVertex{
		id:       ids.GenerateTestID(),
		parents:  []ids.ID{},
		verified: true,
	}
	_ = engine.SubmitVertex(context.Background(), genesis)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vertex := &MockVertex{
			id:        ids.GenerateTestID(),
			parents:   []ids.ID{genesis.ID()},
			height:    uint64(i + 1),
			timestamp: time.Now(),
			bytes:     []byte("bench vertex"),
			verified:  true,
		}
		_ = engine.SubmitVertex(context.Background(), vertex)
	}
}

func BenchmarkGetVertex(b *testing.B) {
	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		MaxParents:      5,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(b, err)

	err = engine.Start(context.Background())
	require.NoError(b, err)

	// Submit a vertex
	vertex := &MockVertex{
		id:       ids.GenerateTestID(),
		parents:  []ids.ID{},
		verified: true,
	}
	_ = engine.SubmitVertex(context.Background(), vertex)
	vertexID := vertex.ID()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.GetVertex(vertexID)
	}
}

func TestProcessingStages(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		MaxParents:      5,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	// Test individual stage processing (currently stubs)
	vertex := &MockVertex{
		id:       ids.GenerateTestID(),
		parents:  []ids.ID{},
		verified: true,
	}

	// All stage processing should succeed (they're stubs)
	err = engine.processPhoton(context.Background(), vertex)
	require.NoError(err)

	err = engine.processWave(context.Background(), vertex)
	require.NoError(err)

	err = engine.processFocus(context.Background(), vertex)
	require.NoError(err)

	err = engine.processFlare(context.Background(), vertex)
	require.NoError(err)

	err = engine.processNova(context.Background(), vertex)
	require.NoError(err)
}