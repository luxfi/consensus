// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"github.com/luxfi/ids"
)

// SimpleDependencyGraph implements DependencyGraph interface
type SimpleDependencyGraph struct {
	dependencies map[ids.ID][]ids.ID
	decisions    []ids.ID
}

// NewSimpleDependencyGraph creates a new dependency graph
func NewSimpleDependencyGraph() *SimpleDependencyGraph {
	return &SimpleDependencyGraph{
		dependencies: make(map[ids.ID][]ids.ID),
		decisions:    make([]ids.ID, 0),
	}
}

// Add adds a decision with its dependencies
func (g *SimpleDependencyGraph) Add(decisionID ids.ID, deps []ids.ID) {
	g.dependencies[decisionID] = deps
	g.decisions = append(g.decisions, decisionID)
}

// GetDependencies returns the dependencies of a decision
func (g *SimpleDependencyGraph) GetDependencies(decisionID ids.ID) []ids.ID {
	deps, ok := g.dependencies[decisionID]
	if !ok {
		return []ids.ID{}
	}
	return deps
}

// GetDecisions returns all decisions in the graph
func (g *SimpleDependencyGraph) GetDecisions() []ids.ID {
	return g.decisions
}