// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pq

import (
	"testing"

	"github.com/luxfi/consensus/protocol/quasar"
	"github.com/stretchr/testify/require"
)

func TestMemoryVertex(t *testing.T) {
	vertexID := quasar.VertexID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	parentID := quasar.VertexID{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	v := &memoryVertex{
		id:       vertexID,
		parents:  []quasar.VertexID{parentID},
		author:   "test-author",
		round:    42,
		children: []quasar.VertexID{},
	}

	// Test ID()
	require.Equal(t, vertexID, v.ID())

	// Test Parents()
	parents := v.Parents()
	require.Len(t, parents, 1)
	require.Equal(t, parentID, parents[0])

	// Test Author()
	require.Equal(t, "test-author", v.Author())

	// Test Round()
	require.Equal(t, uint64(42), v.Round())
}

func TestMemoryVertexEmpty(t *testing.T) {
	v := &memoryVertex{}

	// Test empty vertex
	require.Equal(t, quasar.VertexID{}, v.ID())
	require.Nil(t, v.Parents())
	require.Empty(t, v.Author())
	require.Equal(t, uint64(0), v.Round())
}

func TestMemoryVertexMultipleParents(t *testing.T) {
	parent1 := quasar.VertexID{1}
	parent2 := quasar.VertexID{2}
	parent3 := quasar.VertexID{3}

	v := &memoryVertex{
		parents: []quasar.VertexID{parent1, parent2, parent3},
	}

	parents := v.Parents()
	require.Len(t, parents, 3)
	require.Equal(t, parent1, parents[0])
	require.Equal(t, parent2, parents[1])
	require.Equal(t, parent3, parents[2])
}

func TestMemoryStoreHead(t *testing.T) {
	store := &memoryStore{}

	// Test empty store
	heads := store.Head()
	require.NotNil(t, heads)
	require.Empty(t, heads)

	// Add heads
	head1 := quasar.VertexID{1}
	head2 := quasar.VertexID{2}
	store.heads = []quasar.VertexID{head1, head2}

	heads = store.Head()
	require.Len(t, heads, 2)
	require.Equal(t, head1, heads[0])
	require.Equal(t, head2, heads[1])
}

func TestMemoryStoreHeadCopy(t *testing.T) {
	store := &memoryStore{}
	head1 := quasar.VertexID{1}
	store.heads = []quasar.VertexID{head1}

	// Get heads and modify the copy
	heads := store.Head()
	heads[0] = quasar.VertexID{99}

	// Original should be unchanged
	require.Equal(t, head1, store.heads[0])
}

func TestMemoryStoreGet(t *testing.T) {
	store := &memoryStore{}

	vertexID := quasar.VertexID{1, 2, 3}

	// Test not found on empty store
	v, exists := store.Get(vertexID)
	require.False(t, exists)
	require.Nil(t, v)

	// Add a vertex
	vertex := &memoryVertex{
		id:     vertexID,
		author: "author1",
		round:  10,
	}
	store.vertices[vertexID] = vertex

	// Test found
	v, exists = store.Get(vertexID)
	require.True(t, exists)
	require.NotNil(t, v)
	require.Equal(t, vertexID, v.ID())
	require.Equal(t, "author1", v.Author())
	require.Equal(t, uint64(10), v.Round())
}

func TestMemoryStoreGetNotFound(t *testing.T) {
	store := &memoryStore{
		vertices: make(map[quasar.VertexID]*memoryVertex),
	}

	// Add one vertex
	existingID := quasar.VertexID{1}
	store.vertices[existingID] = &memoryVertex{id: existingID}

	// Try to get a different vertex
	nonExistentID := quasar.VertexID{2}
	v, exists := store.Get(nonExistentID)
	require.False(t, exists)
	require.Nil(t, v)
}

func TestMemoryStoreChildren(t *testing.T) {
	store := &memoryStore{}

	parentID := quasar.VertexID{1}

	// Test empty store
	children := store.Children(parentID)
	require.NotNil(t, children)
	require.Empty(t, children)

	// Add a vertex with children
	child1 := quasar.VertexID{10}
	child2 := quasar.VertexID{20}
	vertex := &memoryVertex{
		id:       parentID,
		children: []quasar.VertexID{child1, child2},
	}
	store.vertices[parentID] = vertex

	// Test found
	children = store.Children(parentID)
	require.Len(t, children, 2)
	require.Equal(t, child1, children[0])
	require.Equal(t, child2, children[1])
}

func TestMemoryStoreChildrenNotFound(t *testing.T) {
	store := &memoryStore{
		vertices: make(map[quasar.VertexID]*memoryVertex),
	}

	// Add one vertex
	existingID := quasar.VertexID{1}
	store.vertices[existingID] = &memoryVertex{
		id:       existingID,
		children: []quasar.VertexID{{10}},
	}

	// Try to get children of a non-existent vertex
	nonExistentID := quasar.VertexID{2}
	children := store.Children(nonExistentID)
	require.NotNil(t, children)
	require.Empty(t, children)
}

func TestMemoryStoreChildrenCopy(t *testing.T) {
	store := &memoryStore{
		vertices: make(map[quasar.VertexID]*memoryVertex),
	}

	parentID := quasar.VertexID{1}
	child1 := quasar.VertexID{10}
	store.vertices[parentID] = &memoryVertex{
		id:       parentID,
		children: []quasar.VertexID{child1},
	}

	// Get children and modify the copy
	children := store.Children(parentID)
	children[0] = quasar.VertexID{99}

	// Original should be unchanged
	require.Equal(t, child1, store.vertices[parentID].children[0])
}

func TestMemoryStoreNilVerticesInit(t *testing.T) {
	// Test that nil vertices map is initialized on first access
	store := &memoryStore{}
	require.Nil(t, store.vertices)

	// Head() should initialize it
	_ = store.Head()
	require.NotNil(t, store.vertices)

	// Get() should also initialize if nil
	store2 := &memoryStore{}
	require.Nil(t, store2.vertices)
	_, _ = store2.Get(quasar.VertexID{})
	require.NotNil(t, store2.vertices)

	// Children() should also initialize if nil
	store3 := &memoryStore{}
	require.Nil(t, store3.vertices)
	_ = store3.Children(quasar.VertexID{})
	require.NotNil(t, store3.vertices)
}

func TestMemoryStoreConcurrency(t *testing.T) {
	store := &memoryStore{
		vertices: make(map[quasar.VertexID]*memoryVertex),
	}

	// Add some vertices
	for i := 0; i < 100; i++ {
		id := quasar.VertexID{byte(i)}
		store.vertices[id] = &memoryVertex{id: id, round: uint64(i)}
	}

	// Concurrent reads should be safe
	done := make(chan bool, 3)

	go func() {
		for i := 0; i < 1000; i++ {
			_ = store.Head()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			_, _ = store.Get(quasar.VertexID{byte(i % 100)})
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			_ = store.Children(quasar.VertexID{byte(i % 100)})
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		<-done
	}
}
