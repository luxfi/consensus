// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nova

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// MockVertex implements the Vertex interface for testing
type MockVertex struct {
	id           ids.ID
	parents      []ids.ID
	height       uint64
	timestamp    time.Time
	transactions []ids.ID
	valid        bool
}

func (m *MockVertex) ID() ids.ID {
	return m.id
}

func (m *MockVertex) Parents() []ids.ID {
	return m.parents
}

func (m *MockVertex) Height() uint64 {
	return m.height
}

func (m *MockVertex) Timestamp() time.Time {
	return m.timestamp
}

func (m *MockVertex) Transactions() []ids.ID {
	return m.transactions
}

func (m *MockVertex) Verify(ctx context.Context) error {
	if !m.valid {
		return fmt.Errorf("invalid vertex")
	}
	return nil
}

func (m *MockVertex) Bytes() []byte {
	return m.id[:]
}

func TestNewNebula(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nebula := NewNebula(params)

	require.NotNil(nebula)
	require.Equal(params, nebula.params)
	require.NotNil(nebula.frontier)
	require.Equal(params.Beta/2, nebula.beta1)
	require.Equal(params.Beta, nebula.beta2)
}

func TestNebulaAddVertex(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nebula := NewNebula(params)
	ctx := context.Background()

	// Valid vertex
	validVertex := &MockVertex{
		id:        ids.GenerateTestID(),
		parents:   []ids.ID{ids.GenerateTestID()},
		height:    1,
		timestamp: time.Now(),
		valid:     true,
	}

	require.NoError(nebula.AddVertex(ctx, validVertex))
	
	// Check vertex was added to frontier
	vc, ok := nebula.frontier.Get(validVertex.ID())
	require.True(ok)
	require.Equal(validVertex, vc.vertex)

	// Invalid vertex
	invalidVertex := &MockVertex{
		id:    ids.GenerateTestID(),
		valid: false,
	}

	require.Error(nebula.AddVertex(ctx, invalidVertex))
}

func TestNebulaRecordPoll(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	params.Beta = 4
	params.AlphaPreference = 2
	nebula := NewNebula(params)
	ctx := context.Background()

	// Add vertices
	vertex1 := &MockVertex{
		id:    ids.GenerateTestID(),
		valid: true,
	}
	vertex2 := &MockVertex{
		id:    ids.GenerateTestID(),
		valid: true,
	}

	require.NoError(nebula.AddVertex(ctx, vertex1))
	require.NoError(nebula.AddVertex(ctx, vertex2))

	// Record poll with votes
	nodeID := ids.GenerateTestNodeID()
	votes := []ids.ID{vertex1.ID(), vertex1.ID(), vertex2.ID()}

	require.NoError(nebula.RecordPoll(ctx, nodeID, votes))

	// Check confidence updates
	vc1, _ := nebula.frontier.Get(vertex1.ID())
	vc2, _ := nebula.frontier.Get(vertex2.ID())

	// Vertex1 got 2 votes (>= AlphaPreference), so confidence increases
	require.Equal(1, vc1.GetConfidence())
	// Vertex2 got 1 vote (< AlphaPreference), so confidence decreases
	require.Equal(-1, vc2.GetConfidence())
}

func TestNebulaPreferredTransition(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	params.Beta = 4
	nebula := NewNebula(params)
	ctx := context.Background()

	vertex := &MockVertex{
		id:    ids.GenerateTestID(),
		valid: true,
	}
	require.NoError(nebula.AddVertex(ctx, vertex))

	vc, _ := nebula.frontier.Get(vertex.ID())
	require.False(vc.IsPreferred())

	// Add chits until we reach beta1 threshold
	for i := 0; i < nebula.beta1; i++ {
		vc.AddChit()
	}

	// Simulate poll that triggers preference
	vc.UpdateConfidence(nebula.beta1)
	
	votes := []ids.ID{vertex.ID()}
	require.NoError(nebula.RecordPoll(ctx, ids.GenerateTestNodeID(), votes))

	// Should now be preferred
	require.True(vc.IsPreferred())
}

func TestNebulaDecidedTransition(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	params.Beta = 4
	nebula := NewNebula(params)
	ctx := context.Background()

	vertex := &MockVertex{
		id:    ids.GenerateTestID(),
		valid: true,
	}
	require.NoError(nebula.AddVertex(ctx, vertex))

	vc, _ := nebula.frontier.Get(vertex.ID())
	require.False(vc.IsDecided())

	// Add chits until we reach beta2 threshold
	for i := 0; i < nebula.beta2; i++ {
		vc.AddChit()
	}

	// Simulate poll that triggers decision
	vc.UpdateConfidence(nebula.beta2)
	
	votes := []ids.ID{vertex.ID()}
	require.NoError(nebula.RecordPoll(ctx, ids.GenerateTestNodeID(), votes))

	// Should now be decided
	require.True(vc.IsDecided())
}

func TestNebulaGetPreferredFrontier(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nebula := NewNebula(params)
	ctx := context.Background()

	// No vertices
	_, ok := nebula.GetPreferredFrontier()
	require.False(ok)

	// Add vertices with different confidence
	vertex1 := &MockVertex{id: ids.GenerateTestID(), valid: true}
	vertex2 := &MockVertex{id: ids.GenerateTestID(), valid: true}
	vertex3 := &MockVertex{id: ids.GenerateTestID(), valid: true}

	require.NoError(nebula.AddVertex(ctx, vertex1))
	require.NoError(nebula.AddVertex(ctx, vertex2))
	require.NoError(nebula.AddVertex(ctx, vertex3))

	// Set different confidence levels
	vc1, _ := nebula.frontier.Get(vertex1.ID())
	vc2, _ := nebula.frontier.Get(vertex2.ID())
	vc3, _ := nebula.frontier.Get(vertex3.ID())

	vc1.UpdateConfidence(5)
	vc2.UpdateConfidence(10)
	vc3.UpdateConfidence(3)

	// Should return vertex2 (highest confidence)
	preferred, ok := nebula.GetPreferredFrontier()
	require.True(ok)
	require.Equal(vertex2, preferred)
}

func TestNebulaGetDecidedVertices(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nebula := NewNebula(params)
	ctx := context.Background()

	// Add vertices
	vertex1 := &MockVertex{id: ids.GenerateTestID(), valid: true}
	vertex2 := &MockVertex{id: ids.GenerateTestID(), valid: true}
	vertex3 := &MockVertex{id: ids.GenerateTestID(), valid: true}

	require.NoError(nebula.AddVertex(ctx, vertex1))
	require.NoError(nebula.AddVertex(ctx, vertex2))
	require.NoError(nebula.AddVertex(ctx, vertex3))

	// Mark some as decided
	vc1, _ := nebula.frontier.Get(vertex1.ID())
	vc3, _ := nebula.frontier.Get(vertex3.ID())

	vc1.SetDecided()
	vc3.SetDecided()

	// Get decided vertices
	decided := nebula.GetDecidedVertices()
	require.Len(decided, 2)

	// Verify the decided vertices
	decidedIDs := make(map[ids.ID]bool)
	for _, v := range decided {
		decidedIDs[v.ID()] = true
	}
	require.True(decidedIDs[vertex1.ID()])
	require.True(decidedIDs[vertex3.ID()])
	require.False(decidedIDs[vertex2.ID()])
}

func TestNebulaStats(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nebula := NewNebula(params)
	ctx := context.Background()

	// Initial stats
	stats := nebula.GetStats()
	require.Equal(0, stats.TotalVertices)
	require.Equal(0, stats.DecidedVertices)
	require.Equal(0, stats.PreferredVertices)
	require.Equal(0, stats.FrontierSize)
	require.Equal(0.0, stats.AverageConfidence)

	// Add vertices with different states
	for i := 0; i < 5; i++ {
		vertex := &MockVertex{
			id:    ids.GenerateTestID(),
			valid: true,
		}
		require.NoError(nebula.AddVertex(ctx, vertex))

		vc, _ := nebula.frontier.Get(vertex.ID())
		vc.UpdateConfidence(i * 2)

		if i < 2 {
			vc.SetDecided()
		}
		if i < 3 {
			vc.SetPreferred()
		}
	}

	// Check stats
	stats = nebula.GetStats()
	require.Equal(5, stats.TotalVertices)
	require.Equal(2, stats.DecidedVertices)
	require.Equal(3, stats.PreferredVertices)
	require.Greater(stats.AverageConfidence, 0.0)
}

func TestVertexConfidence(t *testing.T) {
	require := require.New(t)

	vertex := &MockVertex{
		id:    ids.GenerateTestID(),
		valid: true,
	}

	vc := NewVertexConfidence(vertex)

	require.Equal(vertex, vc.vertex)
	require.Equal(0, vc.confidence)
	require.Equal(0, vc.chits)
	require.False(vc.preferred)
	require.False(vc.decided)

	// Test chit addition
	vc.AddChit()
	vc.AddChit()
	require.Equal(2, vc.chits)

	// Test confidence update
	vc.UpdateConfidence(5)
	require.Equal(5, vc.GetConfidence())

	// Test state transitions
	vc.SetPreferred()
	require.True(vc.IsPreferred())

	vc.SetDecided()
	require.True(vc.IsDecided())
}

func TestFrontier(t *testing.T) {
	require := require.New(t)

	frontier := NewFrontier()
	require.NotNil(frontier)
	require.Empty(frontier.vertices)
	require.Empty(frontier.frontier)

	// Add vertices
	vertex1 := &MockVertex{id: ids.GenerateTestID(), valid: true}
	vertex2 := &MockVertex{id: ids.GenerateTestID(), valid: true}

	vc1 := frontier.Add(vertex1)
	vc2 := frontier.Add(vertex2)

	require.NotNil(vc1)
	require.NotNil(vc2)
	require.Len(frontier.vertices, 2)

	// Get vertices
	gotVC1, ok := frontier.Get(vertex1.ID())
	require.True(ok)
	require.Equal(vc1, gotVC1)

	gotVC2, ok := frontier.Get(vertex2.ID())
	require.True(ok)
	require.Equal(vc2, gotVC2)

	// Non-existent vertex
	_, ok = frontier.Get(ids.GenerateTestID())
	require.False(ok)
}

func TestFrontierUpdateLogic(t *testing.T) {
	require := require.New(t)

	frontier := NewFrontier()

	// Create vertices with parent relationships
	parent := &MockVertex{
		id:      ids.GenerateTestID(),
		parents: []ids.ID{},
		valid:   true,
	}
	child1 := &MockVertex{
		id:      ids.GenerateTestID(),
		parents: []ids.ID{parent.ID()},
		valid:   true,
	}
	child2 := &MockVertex{
		id:      ids.GenerateTestID(),
		parents: []ids.ID{parent.ID()},
		valid:   true,
	}

	// Add vertices
	frontier.Add(parent)
	frontier.Add(child1)
	frontier.Add(child2)

	// Initially all should be in frontier (simplified logic)
	require.Len(frontier.frontier, 2) // Only children should be in frontier

	// Mark child1 as decided
	vc1, _ := frontier.Get(child1.ID())
	vc1.SetDecided()

	// Update frontier - child2 should remain
	frontier.updateFrontier()
	require.Contains(frontier.frontier, child2.ID())
}

func BenchmarkNebulaAddVertex(b *testing.B) {
	params := config.DefaultParameters
	nebula := NewNebula(params)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vertex := &MockVertex{
			id:    ids.GenerateTestID(),
			valid: true,
		}
		nebula.AddVertex(ctx, vertex)
	}
}

func BenchmarkNebulaRecordPoll(b *testing.B) {
	params := config.DefaultParameters
	nebula := NewNebula(params)
	ctx := context.Background()

	// Pre-populate with vertices
	vertexIDs := make([]ids.ID, 100)
	for i := range vertexIDs {
		vertex := &MockVertex{
			id:    ids.GenerateTestID(),
			valid: true,
		}
		nebula.AddVertex(ctx, vertex)
		vertexIDs[i] = vertex.ID()
	}

	nodeID := ids.GenerateTestNodeID()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Vote for random subset
		votes := vertexIDs[:10]
		nebula.RecordPoll(ctx, nodeID, votes)
	}
}