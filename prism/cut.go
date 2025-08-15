// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import "github.com/luxfi/consensus/types"

// Frontier returns the frontier vertices from a DAG
func Frontier[V comparable](parents func(V) []V, tips []V) []V {
	visited := make(map[V]bool)
	frontier := []V{}
	
	for _, tip := range tips {
		if !visited[tip] {
			frontier = append(frontier, tip)
			visited[tip] = true
		}
	}
	
	return frontier
}

// Cut returns vertices matching a predicate
func Cut[V comparable](vertices []V, predicate func(V) bool) []V {
	result := []V{}
	for _, v := range vertices {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}

// CutByHeight returns vertices at a specific height
func CutByHeight(vertices []types.VertexID, heights map[types.VertexID]uint64, targetHeight uint64) []types.VertexID {
	return Cut(vertices, func(v types.VertexID) bool {
		return heights[v] == targetHeight
	})
}