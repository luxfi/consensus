// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package getter

import (
	"context"
	"errors"

	"github.com/luxfi/consensus"
	"github.com/luxfi/ids"
)

var (
	ErrNotFound = errors.New("not found")
)

// Getter fetches vertices for avalanche consensus
type Getter interface {
	// Get a vertex by its ID
	Get(ctx context.Context, vertexID ids.ID) (consensus.Vertex, error)

	// GetAncestors returns the ancestors of a vertex
	GetAncestors(ctx context.Context, vertexID ids.ID, maxContainers int) ([]consensus.Vertex, error)
}

// getter implements the Getter interface
type getter struct {
	storage map[ids.ID]consensus.Vertex
}

// New creates a new getter
func New() Getter {
	return &getter{
		storage: make(map[ids.ID]consensus.Vertex),
	}
}

// Get retrieves a vertex by ID
func (g *getter) Get(ctx context.Context, vertexID ids.ID) (consensus.Vertex, error) {
	vertex, ok := g.storage[vertexID]
	if !ok {
		return nil, ErrNotFound
	}
	return vertex, nil
}

// GetAncestors returns ancestors of a vertex
func (g *getter) GetAncestors(ctx context.Context, vertexID ids.ID, maxContainers int) ([]consensus.Vertex, error) {
	vertex, err := g.Get(ctx, vertexID)
	if err != nil {
		return nil, err
	}

	var ancestors []consensus.Vertex
	visited := make(map[ids.ID]bool)

	// BFS to find ancestors
	queue := []consensus.Vertex{vertex}
	for len(queue) > 0 && len(ancestors) < maxContainers {
		current := queue[0]
		queue = queue[1:]

		if visited[current.ID()] {
			continue
		}
		visited[current.ID()] = true

		parents := current.Parents()
		for _, parentID := range parents {
			parent, err := g.Get(ctx, parentID)
			if err != nil {
				continue
			}
			if !visited[parent.ID()] && len(ancestors) < maxContainers {
				ancestors = append(ancestors, parent)
				queue = append(queue, parent)
			}
		}
	}

	return ancestors, nil
}

// TestGetter is a test implementation
type TestGetter struct {
	GetF          func(ctx context.Context, vertexID ids.ID) (consensus.Vertex, error)
	GetAncestorsF func(ctx context.Context, vertexID ids.ID, maxContainers int) ([]consensus.Vertex, error)
}

// Get calls GetF if set
func (t *TestGetter) Get(ctx context.Context, vertexID ids.ID) (consensus.Vertex, error) {
	if t.GetF != nil {
		return t.GetF(ctx, vertexID)
	}
	return nil, ErrNotFound
}

// GetAncestors calls GetAncestorsF if set
func (t *TestGetter) GetAncestors(ctx context.Context, vertexID ids.ID, maxContainers int) ([]consensus.Vertex, error) {
	if t.GetAncestorsF != nil {
		return t.GetAncestorsF(ctx, vertexID, maxContainers)
	}
	return nil, nil
}
