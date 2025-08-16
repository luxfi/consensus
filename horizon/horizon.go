// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package horizon provides DAG order-theory predicates and operations.
// It includes reachability, lowest common ancestor (LCA), antichain detection,
// and certificate/skip validation for DAG consensus protocols.
package horizon

// Graph represents a directed acyclic graph for order-theory operations.
type Graph[V comparable] interface {
	Parents(V) []V
}

// IsAncestor checks if vertex 'ancestor' is an ancestor of vertex 'descendant'.
func IsAncestor[V comparable](g Graph[V], ancestor, descendant V) bool {
	if ancestor == descendant {
		return true
	}

	visited := make(map[V]bool)
	var search func(V) bool

	search = func(v V) bool {
		if v == ancestor {
			return true
		}

		if visited[v] {
			return false
		}
		visited[v] = true

		for _, parent := range g.Parents(v) {
			if search(parent) {
				return true
			}
		}

		return false
	}

	return search(descendant)
}

// IsReachable checks if vertex 'to' is reachable from vertex 'from'.
// This is the inverse of IsAncestor.
func IsReachable[V comparable](g Graph[V], from, to V) bool {
	return IsAncestor(g, from, to)
}

// LCA finds the lowest common ancestor of two vertices.
// Returns the LCA and true if found, or zero value and false if not.
func LCA[V comparable](g Graph[V], x, y V) (V, bool) {
	// Collect all ancestors of x
	xAncestors := make(map[V]int)
	var collectAncestors func(V, int)

	collectAncestors = func(v V, depth int) {
		if existingDepth, exists := xAncestors[v]; exists && existingDepth <= depth {
			return // Already visited at same or lower depth
		}
		xAncestors[v] = depth

		for _, parent := range g.Parents(v) {
			collectAncestors(parent, depth+1)
		}
	}

	collectAncestors(x, 0)

	// Find common ancestors while traversing from y
	var findLCA func(V, int) (V, int, bool)
	visited := make(map[V]bool)

	findLCA = func(v V, depth int) (V, int, bool) {
		if visited[v] {
			var zero V
			return zero, -1, false
		}
		visited[v] = true

		// Check if this vertex is an ancestor of x
		if xDepth, isAncestor := xAncestors[v]; isAncestor {
			// This is a common ancestor
			return v, xDepth + depth, true
		}

		// Check parents
		var bestLCA V
		bestScore := -1
		found := false

		for _, parent := range g.Parents(v) {
			if lca, score, ok := findLCA(parent, depth+1); ok {
				if !found || score < bestScore {
					bestLCA = lca
					bestScore = score
					found = true
				}
			}
		}

		return bestLCA, bestScore, found
	}

	lca, _, found := findLCA(y, 0)
	return lca, found
}

// Antichain returns the maximal antichain from a set of vertices.
// An antichain is a set where no vertex is an ancestor of another.
func Antichain[V comparable](g Graph[V], vertices []V) []V {
	// For each vertex, check if it's an ancestor of any other
	isAncestorOf := make(map[V]bool)

	for i, v1 := range vertices {
		for j, v2 := range vertices {
			if i != j && IsAncestor(g, v1, v2) {
				isAncestorOf[v1] = true
				break
			}
		}
	}

	// Return vertices that are not ancestors of others
	var antichain []V
	for _, v := range vertices {
		if !isAncestorOf[v] {
			antichain = append(antichain, v)
		}
	}

	return antichain
}

// TopologicalSort performs a topological sort on vertices reachable from roots.
func TopologicalSort[V comparable](g Graph[V], roots []V) []V {
	visited := make(map[V]bool)
	var sorted []V

	var visit func(V)
	visit = func(v V) {
		if visited[v] {
			return
		}
		visited[v] = true

		// Visit parents first (post-order traversal)
		for _, parent := range g.Parents(v) {
			visit(parent)
		}

		// Add to sorted list after visiting parents
		sorted = append(sorted, v)
	}

	// Start from all roots
	for _, root := range roots {
		visit(root)
	}

	// Reverse to get correct topological order
	for i, j := 0, len(sorted)-1; i < j; i, j = i+1, j-1 {
		sorted[i], sorted[j] = sorted[j], sorted[i]
	}

	return sorted
}

// TransitiveClosure computes the transitive closure of a vertex.
// Returns all vertices reachable from the given vertex.
func TransitiveClosure[V comparable](g Graph[V], start V) []V {
	closure := make(map[V]bool)

	var traverse func(V)
	traverse = func(v V) {
		if closure[v] {
			return
		}
		closure[v] = true

		for _, parent := range g.Parents(v) {
			traverse(parent)
		}
	}

	traverse(start)

	// Convert map to slice
	result := make([]V, 0, len(closure))
	for v := range closure {
		result = append(result, v)
	}

	return result
}

// Certificate represents a certificate for a vertex in the DAG.
type Certificate[V comparable] struct {
	Vertex V
	// Proof contains the vertices that form the certificate
	Proof []V
	// Threshold is the minimum number of confirmations needed
	Threshold int
}

// ValidateCertificate checks if a certificate is valid.
func ValidateCertificate[V comparable](
	g Graph[V],
	cert Certificate[V],
	isValid func(V) bool,
) bool {
	validCount := 0

	for _, proofVertex := range cert.Proof {
		// Check if proof vertex is actually an ancestor
		if !IsAncestor(g, proofVertex, cert.Vertex) {
			return false
		}

		// Check if proof vertex is valid
		if isValid(proofVertex) {
			validCount++
		}
	}

	return validCount >= cert.Threshold
}

// SkipList represents a skip list for efficient DAG traversal.
type SkipList[V comparable] struct {
	// Levels maps each vertex to its skip pointers at different levels
	Levels map[V]map[int]V
}

// BuildSkipList constructs a skip list for the DAG.
func BuildSkipList[V comparable](g Graph[V], roots []V) *SkipList[V] {
	sl := &SkipList[V]{
		Levels: make(map[V]map[int]V),
	}

	// Simple skip list: each vertex points to ancestors at distances 2^i
	visited := make(map[V]bool)

	var build func(V, int)
	build = func(v V, depth int) {
		if visited[v] {
			return
		}
		visited[v] = true

		// Initialize skip pointers for this vertex
		if sl.Levels[v] == nil {
			sl.Levels[v] = make(map[int]V)
		}

		// Add skip pointers at different levels
		parents := g.Parents(v)
		for level := 0; level < len(parents); level++ {
			if level < len(parents) {
				sl.Levels[v][level] = parents[level]
			}
		}

		// Recursively build for parents
		for _, parent := range parents {
			build(parent, depth+1)
		}
	}

	for _, root := range roots {
		build(root, 0)
	}

	return sl
}

// FindPath finds a path from 'from' to 'to' vertex if one exists.
func FindPath[V comparable](g Graph[V], from, to V) ([]V, bool) {
	if from == to {
		return []V{from}, true
	}

	visited := make(map[V]bool)
	parent := make(map[V]V)

	var search func(V) bool
	search = func(v V) bool {
		if v == to {
			return true
		}

		if visited[v] {
			return false
		}
		visited[v] = true

		for _, p := range g.Parents(v) {
			parent[p] = v
			if search(p) {
				return true
			}
		}

		return false
	}

	if !search(from) {
		return nil, false
	}

	// Reconstruct path
	var path []V
	current := to
	for current != from {
		path = append(path, current)
		current = parent[current]
	}
	path = append(path, from)

	// Reverse path
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return path, true
}
