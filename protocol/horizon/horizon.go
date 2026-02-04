// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package horizon

import (
	"github.com/luxfi/consensus/core/dag"
)

type VertexID [32]byte

type Meta interface {
	ID() VertexID
	Author() string
	Round() uint64
	Parents() []VertexID
}

type View interface {
	Get(VertexID) (Meta, bool)
	ByRound(round uint64) []Meta
	Supports(from VertexID, author string, round uint64) bool
}

type Params struct{ N, F int }

// TransitiveClosure computes all ancestors of a vertex using BFS traversal.
// Returns the vertex itself plus all vertices reachable by following parent edges.
func TransitiveClosure[V comparable](store dag.Store[V], vertex V) []V {
	visited := make(map[any]bool)
	var result []V
	queue := []V{vertex}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Use interface conversion for map key
		key := any(current)
		if visited[key] {
			continue
		}
		visited[key] = true
		result = append(result, current)

		// Add all parents to queue
		if block, exists := store.Get(current); exists {
			for _, parent := range block.Parents() {
				parentKey := any(parent)
				if !visited[parentKey] {
					queue = append(queue, parent)
				}
			}
		}
	}

	return result
}

// Certificate represents a proof that a vertex has achieved consensus.
type Certificate[V comparable] struct {
	Vertex    V
	Proof     []V // Vertices that contributed to the certificate
	Threshold int // Minimum valid proofs required
}

// ValidateCertificate checks if a certificate meets the threshold with valid proofs.
func ValidateCertificate[V comparable](store dag.Store[V], cert Certificate[V], isValid func(V) bool) bool {
	validCount := 0
	for _, proof := range cert.Proof {
		if isValid(proof) {
			validCount++
		}
	}
	return validCount >= cert.Threshold
}

// SkipList provides O(log n) traversal through DAG levels.
// Each vertex has pointers to ancestors at exponentially increasing distances.
type SkipList[V comparable] struct {
	Levels map[any][]V // key is vertex, value is skip pointers
}

// BuildSkipList constructs a skip list with logarithmic skip pointers.
// Level 0: immediate parent, Level 1: 2 steps back, Level 2: 4 steps back, etc.
func BuildSkipList[V comparable](store dag.Store[V], vertices []V) *SkipList[V] {
	sl := &SkipList[V]{
		Levels: make(map[any][]V),
	}

	// Build skip pointers for each vertex
	for _, v := range vertices {
		block, exists := store.Get(v)
		if !exists {
			continue
		}

		parents := block.Parents()
		if len(parents) == 0 {
			sl.Levels[any(v)] = []V{}
			continue
		}

		// Start with first parent as level 0
		skips := []V{parents[0]}

		// Build higher levels by following skip pointers
		current := parents[0]
		for level := 1; level < 8; level++ { // Max 8 levels (256 step jumps)
			// Jump 2^level steps
			for step := 0; step < (1 << level); step++ {
				currentBlock, exists := store.Get(current)
				if !exists {
					break
				}
				currentParents := currentBlock.Parents()
				if len(currentParents) == 0 {
					break
				}
				current = currentParents[0]
			}
			skips = append(skips, current)
		}

		sl.Levels[any(v)] = skips
	}

	return sl
}

// FindPath finds a path from 'from' to 'to' using BFS.
// Returns the path as a slice of vertices and true if found, nil and false otherwise.
func FindPath[V comparable](store dag.Store[V], from, to V) ([]V, bool) {
	// BFS to find path
	type node struct {
		vertex V
		path   []V
	}

	visited := make(map[any]bool)
	queue := []node{{vertex: from, path: []V{from}}}

	toKey := any(to)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		currentKey := any(current.vertex)
		if visited[currentKey] {
			continue
		}
		visited[currentKey] = true

		// Check if we've reached the target
		if currentKey == toKey {
			return current.path, true
		}

		// Add parents to queue
		if block, exists := store.Get(current.vertex); exists {
			for _, parent := range block.Parents() {
				parentKey := any(parent)
				if !visited[parentKey] {
					newPath := make([]V, len(current.path)+1)
					copy(newPath, current.path)
					newPath[len(current.path)] = parent
					queue = append(queue, node{vertex: parent, path: newPath})
				}
			}
		}
	}

	return nil, false
}

// LowestCommonAncestor finds the LCA of two vertices in the DAG.
func LowestCommonAncestor[V comparable](store dag.Store[V], a, b V) (V, bool) {
	// Get ancestors of a
	ancestorsA := make(map[any]bool)
	for _, v := range TransitiveClosure(store, a) {
		ancestorsA[any(v)] = true
	}

	// Find first ancestor of b that is also an ancestor of a (BFS order = closest)
	queue := []V{b}
	visited := make(map[any]bool)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		currentKey := any(current)
		if visited[currentKey] {
			continue
		}
		visited[currentKey] = true

		if ancestorsA[currentKey] {
			return current, true
		}

		if block, exists := store.Get(current); exists {
			for _, parent := range block.Parents() {
				parentKey := any(parent)
				if !visited[parentKey] {
					queue = append(queue, parent)
				}
			}
		}
	}

	var zero V
	return zero, false
}
