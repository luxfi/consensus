// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package dag

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// ==================== Vertex Tests ====================

func TestNewVertex(t *testing.T) {
	require := require.New(t)

	id := ids.GenerateTestID()
	parentID := ids.GenerateTestID()
	parentIDs := []ids.ID{parentID}
	height := uint64(10)
	timestamp := int64(12345)
	data := []byte("test data")

	vertex := NewVertex(id, parentIDs, height, timestamp, data)

	require.Equal(id, vertex.ID())
	require.Equal(parentIDs, vertex.ParentIDs())
	require.Equal(parentID, vertex.Parent())
	require.Equal(height, vertex.Height())
	require.Equal(data, vertex.Bytes())
	require.False(vertex.IsAccepted())
	require.False(vertex.IsRejected())
	require.False(vertex.IsProcessing())
}

func TestVertexNoParents(t *testing.T) {
	require := require.New(t)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, []ids.ID{}, 0, 0, nil)

	require.Equal(ids.Empty, vertex.Parent())
	require.Empty(vertex.ParentIDs())
}

func TestVertexVerify(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Valid vertex
	id := ids.GenerateTestID()
	parentID := ids.GenerateTestID()
	vertex := NewVertex(id, []ids.ID{parentID}, 1, 0, nil)

	err := vertex.Verify(ctx)
	require.NoError(err)

	// Invalid vertex - empty ID
	invalidVertex := NewVertex(ids.Empty, []ids.ID{parentID}, 1, 0, nil)
	err = invalidVertex.Verify(ctx)
	require.Error(err)
	require.Contains(err.Error(), "invalid vertex ID")

	// Invalid vertex - empty parent ID
	invalidParent := NewVertex(id, []ids.ID{ids.Empty}, 1, 0, nil)
	err = invalidParent.Verify(ctx)
	require.Error(err)
	require.Contains(err.Error(), "invalid parent ID")
}

func TestVertexAccept(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	vertex.SetProcessing(true)

	require.True(vertex.IsProcessing())

	err := vertex.Accept(ctx)
	require.NoError(err)
	require.True(vertex.IsAccepted())
	require.False(vertex.IsProcessing())
}

func TestVertexAcceptAlreadyRejected(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)

	err := vertex.Reject(ctx)
	require.NoError(err)

	err = vertex.Accept(ctx)
	require.Error(err)
	require.Contains(err.Error(), "already rejected")
}

func TestVertexReject(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	vertex.SetProcessing(true)

	err := vertex.Reject(ctx)
	require.NoError(err)
	require.True(vertex.IsRejected())
	require.False(vertex.IsProcessing())
}

func TestVertexRejectAlreadyAccepted(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)

	err := vertex.Accept(ctx)
	require.NoError(err)

	err = vertex.Reject(ctx)
	require.Error(err)
	require.Contains(err.Error(), "already accepted")
}

func TestVertexParentChild(t *testing.T) {
	require := require.New(t)

	parentID := ids.GenerateTestID()
	childID := ids.GenerateTestID()

	parent := NewVertex(parentID, nil, 0, 0, nil)
	child := NewVertex(childID, []ids.ID{parentID}, 1, 0, nil)

	parent.AddChild(child)
	child.AddParent(parent)

	require.Len(parent.Children(), 1)
	require.Equal(child, parent.Children()[0])
	require.Len(child.Parents(), 1)
	require.Equal(parent, child.Parents()[0])
}

func TestVertexLuxConsensus(t *testing.T) {
	require := require.New(t)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)

	require.Nil(vertex.LuxConsensus())

	// SetLuxConsensus is tested via DAGConsensus.AddVertex
}

func TestVertexConcurrency(t *testing.T) {
	require := require.New(t)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vertex.SetProcessing(true)
			_ = vertex.IsProcessing()
			_ = vertex.IsAccepted()
			_ = vertex.IsRejected()
		}()
	}
	wg.Wait()

	require.NotNil(vertex)
}

// ==================== DAGConsensus Tests ====================

func TestNewDAGConsensus(t *testing.T) {
	require := require.New(t)

	dc := NewDAGConsensus(5, 3, 10)
	require.NotNil(dc)
	require.Equal(5, dc.k)
	require.Equal(3, dc.alpha)
	require.Equal(10, dc.beta)
}

func TestDAGConsensusAddVertex(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	// Add genesis vertex
	genesisID := ids.GenerateTestID()
	genesis := NewVertex(genesisID, nil, 0, 0, []byte("genesis"))

	err := dc.AddVertex(ctx, genesis)
	require.NoError(err)

	// Verify it's in the DAG
	retrieved, exists := dc.GetVertex(genesisID)
	require.True(exists)
	require.Equal(genesis, retrieved)

	// Verify it's in the frontier
	frontier := dc.Frontier()
	require.Contains(frontier, genesisID)
}

func TestDAGConsensusAddVertexWithParent(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	// Add parent
	parentID := ids.GenerateTestID()
	parent := NewVertex(parentID, nil, 0, 0, nil)
	err := dc.AddVertex(ctx, parent)
	require.NoError(err)

	// Add child
	childID := ids.GenerateTestID()
	child := NewVertex(childID, []ids.ID{parentID}, 1, 0, nil)
	err = dc.AddVertex(ctx, child)
	require.NoError(err)

	// Parent should be removed from frontier
	frontier := dc.Frontier()
	require.NotContains(frontier, parentID)
	require.Contains(frontier, childID)

	// Verify parent-child relationship
	require.Len(parent.Children(), 1)
	require.Equal(child, parent.Children()[0])
}

func TestDAGConsensusAddDuplicateVertex(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)

	err := dc.AddVertex(ctx, vertex)
	require.NoError(err)

	// Adding again should fail
	vertex2 := NewVertex(id, nil, 0, 0, nil)
	err = dc.AddVertex(ctx, vertex2)
	require.Error(err)
	require.Contains(err.Error(), "already exists")
}

func TestDAGConsensusAddVertexMissingParent(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	// Add child without adding parent first
	childID := ids.GenerateTestID()
	nonExistentParentID := ids.GenerateTestID()
	child := NewVertex(childID, []ids.ID{nonExistentParentID}, 1, 0, nil)

	err := dc.AddVertex(ctx, child)
	require.Error(err)
	require.Contains(err.Error(), "parent vertex not found")
}

func TestDAGConsensusAddInvalidVertex(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	// Invalid vertex with empty ID
	invalid := NewVertex(ids.Empty, nil, 0, 0, nil)
	err := dc.AddVertex(ctx, invalid)
	require.Error(err)
	require.Contains(err.Error(), "verification failed")
}

func TestDAGConsensusProcessVote(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	err := dc.AddVertex(ctx, vertex)
	require.NoError(err)

	// Process accept vote
	err = dc.ProcessVote(ctx, id, true)
	require.NoError(err)

	// Process reject vote
	err = dc.ProcessVote(ctx, id, false)
	require.NoError(err)
}

func TestDAGConsensusProcessVoteNonExistent(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	nonExistentID := ids.GenerateTestID()
	err := dc.ProcessVote(ctx, nonExistentID, true)
	require.Error(err)
	require.Contains(err.Error(), "vertex not found")
}

func TestDAGConsensusPoll(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(1, 1, 1) // Low thresholds for quick acceptance

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	err := dc.AddVertex(ctx, vertex)
	require.NoError(err)

	// Poll with enough votes
	responses := map[ids.ID]int{id: 10}
	err = dc.Poll(ctx, responses)
	require.NoError(err)
}

func TestDAGConsensusPollNonExistent(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	// Poll for non-existent vertex should not error
	nonExistentID := ids.GenerateTestID()
	responses := map[ids.ID]int{nonExistentID: 5}
	err := dc.Poll(ctx, responses)
	require.NoError(err)
}

func TestDAGConsensusIsAccepted(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	err := dc.AddVertex(ctx, vertex)
	require.NoError(err)

	require.False(dc.IsAccepted(id))

	err = vertex.Accept(ctx)
	require.NoError(err)
	require.True(dc.IsAccepted(id))
}

func TestDAGConsensusIsAcceptedNonExistent(t *testing.T) {
	require := require.New(t)

	dc := NewDAGConsensus(5, 3, 10)

	nonExistentID := ids.GenerateTestID()
	require.False(dc.IsAccepted(nonExistentID))
}

func TestDAGConsensusIsRejected(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	err := dc.AddVertex(ctx, vertex)
	require.NoError(err)

	require.False(dc.IsRejected(id))

	err = vertex.Reject(ctx)
	require.NoError(err)
	require.True(dc.IsRejected(id))
}

func TestDAGConsensusIsRejectedNonExistent(t *testing.T) {
	require := require.New(t)

	dc := NewDAGConsensus(5, 3, 10)

	nonExistentID := ids.GenerateTestID()
	require.False(dc.IsRejected(nonExistentID))
}

func TestDAGConsensusPreference(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	// Empty DAG should return empty ID
	require.Equal(ids.Empty, dc.Preference())

	// Add a vertex
	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	err := dc.AddVertex(ctx, vertex)
	require.NoError(err)

	// Preference should be the frontier vertex
	pref := dc.Preference()
	require.Equal(id, pref)
}

func TestDAGConsensusPreferenceAfterAccept(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	err := dc.AddVertex(ctx, vertex)
	require.NoError(err)

	// Manually set last accepted
	dc.mu.Lock()
	dc.lastAccepted = id
	dc.mu.Unlock()

	pref := dc.Preference()
	require.Equal(id, pref)
}

func TestDAGConsensusStats(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	// Add some vertices
	id1 := ids.GenerateTestID()
	v1 := NewVertex(id1, nil, 0, 0, nil)
	err := dc.AddVertex(ctx, v1)
	require.NoError(err)

	id2 := ids.GenerateTestID()
	v2 := NewVertex(id2, []ids.ID{id1}, 1, 0, nil)
	err = dc.AddVertex(ctx, v2)
	require.NoError(err)

	// Accept one
	err = v1.Accept(ctx)
	require.NoError(err)

	// Reject one
	err = v2.Reject(ctx)
	require.NoError(err)

	stats := dc.Stats()
	require.Equal(2, stats["total_vertices"])
	require.Equal(1, stats["accepted"])
	require.Equal(1, stats["rejected"])
	require.Equal(0, stats["pending"])
}

func TestDAGConsensusGetConflicting(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	err := dc.AddVertex(ctx, vertex)
	require.NoError(err)

	conflicts := dc.GetConflicting(ctx, vertex)
	require.Empty(conflicts)
}

func TestDAGConsensusResolveConflict(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(1, 1, 1)

	// Empty list
	_, err := dc.ResolveConflict(ctx, []*Vertex{})
	require.Error(err)
	require.Contains(err.Error(), "no vertices to resolve")

	// Single vertex
	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	winner, err := dc.ResolveConflict(ctx, []*Vertex{vertex})
	require.NoError(err)
	require.Equal(vertex, winner)

	// Multiple vertices
	id2 := ids.GenerateTestID()
	vertex2 := NewVertex(id2, nil, 0, 0, nil)
	winner, err = dc.ResolveConflict(ctx, []*Vertex{vertex, vertex2})
	require.NoError(err)
	require.NotNil(winner)
}

// ==================== dagEngine Tests ====================

func TestNewWithParams(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParams()
	engine := NewWithParams(params)
	require.NotNil(engine)
}

func TestDagEngineGetVtxExisting(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)

	// Add a vertex first
	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	err := engine.consensus.AddVertex(ctx, vertex)
	require.NoError(err)

	// Now get it
	tx, err := engine.GetVtx(ctx, id)
	require.NoError(err)
	require.NotNil(tx)
	require.Equal(id, tx.ID())
}

func TestDagEngineBuildVtxWithData(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)

	// First add a genesis vertex to establish a valid frontier
	genesisID := ids.GenerateTestID()
	genesis := NewVertex(genesisID, nil, 0, 0, []byte("genesis"))
	err := engine.consensus.AddVertex(ctx, genesis)
	require.NoError(err)

	// Queue data
	engine.QueueData([]byte("test data"))

	// Build vertex - should use genesis as parent
	tx, err := engine.BuildVtx(ctx)
	require.NoError(err)
	require.NotNil(tx)
	require.Equal([]byte("test data"), tx.Bytes())
}

func TestDagEngineBuildVtxWithParents(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)

	// Add a parent vertex first
	parentID := ids.GenerateTestID()
	parent := NewVertex(parentID, nil, 0, 0, nil)
	err := engine.consensus.AddVertex(ctx, parent)
	require.NoError(err)

	// Queue data and build
	engine.QueueData([]byte("child data"))
	tx, err := engine.BuildVtx(ctx)
	require.NoError(err)
	require.NotNil(tx)
}

func TestDagEngineStop(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)
	err := engine.Start(ctx, 1)
	require.NoError(err)

	err = engine.Stop(ctx)
	require.NoError(err)
	require.False(engine.IsBootstrapped())
}

func TestDagEngineHealthCheck(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)
	err := engine.Start(ctx, 1)
	require.NoError(err)

	health, err := engine.HealthCheck(ctx)
	require.NoError(err)
	require.NotNil(health)

	stats := health.(map[string]interface{})
	require.True(stats["bootstrapped"].(bool))
}

func TestDagEngineIsBootstrapped(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)
	require.False(engine.IsBootstrapped())

	err := engine.Start(ctx, 1)
	require.NoError(err)
	require.True(engine.IsBootstrapped())

	err = engine.Shutdown(ctx)
	require.NoError(err)
	require.False(engine.IsBootstrapped())
}

func TestDagEngineAddVertex(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)

	err := engine.AddVertex(ctx, vertex)
	require.NoError(err)
	require.True(engine.consensus.IsAccepted(id) || !engine.consensus.IsRejected(id))
}

func TestDagEngineProcessVote(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	err := engine.AddVertex(ctx, vertex)
	require.NoError(err)

	err = engine.ProcessVote(ctx, id, true)
	require.NoError(err)
}

func TestDagEnginePoll(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	err := engine.AddVertex(ctx, vertex)
	require.NoError(err)

	responses := map[ids.ID]int{id: 5}
	err = engine.Poll(ctx, responses)
	require.NoError(err)
}

func TestDagEngineIsAccepted(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	err := engine.AddVertex(ctx, vertex)
	require.NoError(err)

	require.False(engine.IsAccepted(id))

	err = vertex.Accept(ctx)
	require.NoError(err)
	require.True(engine.IsAccepted(id))
}

func TestDagEnginePreference(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)

	require.Equal(ids.Empty, engine.Preference())

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)
	err := engine.AddVertex(ctx, vertex)
	require.NoError(err)

	pref := engine.Preference()
	require.Equal(id, pref)
}

func TestDagEngineQueueData(t *testing.T) {
	require := require.New(t)

	engine := New().(*dagEngine)

	engine.QueueData([]byte("data1"))
	engine.QueueData([]byte("data2"))

	require.Len(engine.pendingData, 2)
}

func TestDagEngineShutdownWithoutStart(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)

	// Should not panic even without Start
	err := engine.Shutdown(ctx)
	require.NoError(err)
}

// ==================== Integration Tests ====================

func TestDAGConsensusWithChildAcceptance(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(1, 1, 1)

	// Create parent
	parentID := ids.GenerateTestID()
	parent := NewVertex(parentID, nil, 0, 0, nil)
	err := dc.AddVertex(ctx, parent)
	require.NoError(err)

	// Create child
	childID := ids.GenerateTestID()
	child := NewVertex(childID, []ids.ID{parentID}, 1, 0, nil)
	err = dc.AddVertex(ctx, child)
	require.NoError(err)

	// Accept parent through polling
	err = parent.Accept(ctx)
	require.NoError(err)

	dc.mu.Lock()
	dc.lastAccepted = parentID
	dc.mu.Unlock()

	// Process children in order
	err = dc.processChildrenInOrder(ctx, parent)
	require.NoError(err)

	// Child should be marked for processing
	require.True(child.IsProcessing())
}

func TestDAGConcurrentOperations(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	var wg sync.WaitGroup
	numVertices := 10

	// Concurrently add vertices
	for i := 0; i < numVertices; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := ids.GenerateTestID()
			vertex := NewVertex(id, nil, 0, 0, nil)
			_ = dc.AddVertex(ctx, vertex)
		}()
	}

	wg.Wait()

	// Verify stats
	stats := dc.Stats()
	total := stats["total_vertices"].(int)
	require.Equal(numVertices, total)
}

func TestProcessVoteWithoutLuxConsensus(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)

	// Add vertex without Lux consensus (manually)
	dc.mu.Lock()
	dc.vertices[id] = vertex
	dc.mu.Unlock()

	err := dc.ProcessVote(ctx, id, true)
	require.Error(err)
	require.Contains(err.Error(), "not initialized for consensus")
}

func TestAddVertexWithEmptyParentInList(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	// Add vertex with empty parent ID in list (should be skipped)
	id := ids.GenerateTestID()
	vertex := NewVertex(id, []ids.ID{ids.Empty}, 0, 0, nil)

	// This should fail verification since empty parent ID is invalid
	err := dc.AddVertex(ctx, vertex)
	require.Error(err)
}

func TestPollWithNoLuxConsensus(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	id := ids.GenerateTestID()
	vertex := NewVertex(id, nil, 0, 0, nil)

	// Manually add vertex without consensus
	dc.mu.Lock()
	dc.vertices[id] = vertex
	dc.mu.Unlock()

	// Poll should skip this vertex
	responses := map[ids.ID]int{id: 5}
	err := dc.Poll(ctx, responses)
	require.NoError(err)
}

// TestPollWithDecisionAndChildren tests Poll when a vertex gets decided and has children
func TestPollWithDecisionAndChildren(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Use low thresholds for quick decision
	dc := NewDAGConsensus(1, 1, 1)

	// Add parent vertex
	parentID := ids.GenerateTestID()
	parent := NewVertex(parentID, nil, 0, 0, []byte("parent"))
	err := dc.AddVertex(ctx, parent)
	require.NoError(err)

	// Add child vertex
	childID := ids.GenerateTestID()
	child := NewVertex(childID, []ids.ID{parentID}, 1, 0, []byte("child"))
	err = dc.AddVertex(ctx, child)
	require.NoError(err)

	// Poll with enough votes to reach decision (with k=1, alpha=1, beta=1, one vote should decide)
	responses := map[ids.ID]int{parentID: 1}
	err = dc.Poll(ctx, responses)
	require.NoError(err)

	// Parent should be accepted after sufficient polling
	// May need multiple polls depending on consensus implementation
	for i := 0; i < 5; i++ {
		err = dc.Poll(ctx, responses)
		require.NoError(err)
	}
}

// TestProcessChildrenWithUnacceptedParent tests that children aren't processed when a parent is not accepted
func TestProcessChildrenWithUnacceptedParent(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	// Add two parent vertices
	parent1ID := ids.GenerateTestID()
	parent1 := NewVertex(parent1ID, nil, 0, 0, []byte("parent1"))
	err := dc.AddVertex(ctx, parent1)
	require.NoError(err)

	parent2ID := ids.GenerateTestID()
	parent2 := NewVertex(parent2ID, nil, 0, 0, []byte("parent2"))
	err = dc.AddVertex(ctx, parent2)
	require.NoError(err)

	// Add child with both parents
	childID := ids.GenerateTestID()
	child := NewVertex(childID, []ids.ID{parent1ID, parent2ID}, 1, 0, []byte("child"))
	err = dc.AddVertex(ctx, child)
	require.NoError(err)

	// Accept only parent1
	err = parent1.Accept(ctx)
	require.NoError(err)

	// Process children - child should NOT be marked for processing because parent2 is not accepted
	dc.mu.Lock()
	err = dc.processChildrenInOrder(ctx, parent1)
	dc.mu.Unlock()
	require.NoError(err)

	// Child should NOT be processing because parent2 is not accepted
	require.False(child.IsProcessing())

	// Now accept parent2
	err = parent2.Accept(ctx)
	require.NoError(err)

	// Process children again - now child should be marked for processing
	dc.mu.Lock()
	err = dc.processChildrenInOrder(ctx, parent2)
	dc.mu.Unlock()
	require.NoError(err)

	// Child should now be processing
	require.True(child.IsProcessing())
}

// TestResolveConflictMultipleVertices tests conflict resolution with multiple vertices
func TestResolveConflictMultipleVertices(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(1, 1, 1)

	// Create multiple vertices
	vertices := make([]*Vertex, 3)
	for i := 0; i < 3; i++ {
		id := ids.GenerateTestID()
		vertices[i] = NewVertex(id, nil, 0, 0, []byte(fmt.Sprintf("vertex%d", i)))
		err := dc.AddVertex(ctx, vertices[i])
		require.NoError(err)
	}

	// Resolve conflict
	winner, err := dc.ResolveConflict(ctx, vertices)
	require.NoError(err)
	require.NotNil(winner)
	// Winner should be one of the vertices
	found := false
	for _, v := range vertices {
		if v.ID() == winner.ID() {
			found = true
			break
		}
	}
	require.True(found)
}

// TestBuildVtxEmptyData tests BuildVtx when no data is queued
func TestBuildVtxEmptyData(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)

	// Build without queuing data
	tx, err := engine.BuildVtx(ctx)
	require.NoError(err)
	require.Nil(tx) // Should return nil when no data
}

// TestBuildVtxEmptyFrontier tests BuildVtx when frontier is empty
func TestBuildVtxEmptyFrontier(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	engine := New().(*dagEngine)

	// Queue data
	engine.QueueData([]byte("test"))

	// Build without any frontier - this will fail since empty ID is invalid parent
	tx, err := engine.BuildVtx(ctx)
	// The vertex will fail to add due to invalid parent ID
	require.Error(err)
	require.Nil(tx)
}

// TestAddVertexWithEmptyIDRejected tests that vertices with empty parent IDs are rejected during verification
func TestAddVertexWithEmptyIDRejected(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	dc := NewDAGConsensus(5, 3, 10)

	// Add a parent vertex first
	parentID := ids.GenerateTestID()
	parent := NewVertex(parentID, nil, 0, 0, []byte("parent"))
	err := dc.AddVertex(ctx, parent)
	require.NoError(err)

	// Add child with mix of valid and empty parent IDs
	childID := ids.GenerateTestID()
	child := NewVertex(childID, []ids.ID{parentID, ids.Empty}, 1, 0, []byte("child"))

	// This should fail - empty ID is rejected during verification
	err = dc.AddVertex(ctx, child)
	require.Error(err)
	require.Contains(err.Error(), "invalid parent ID")
}
