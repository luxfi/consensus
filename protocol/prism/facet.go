// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

// Facet guides how you traverse your decision graph
// Like a prism's facets that bend and refract light, each facet here
// represents a path through the dependency graph that can yield a decision
type Facet struct {
	// The root decision we're evaluating
	root ids.ID
	
	// Dependencies that must be resolved first
	dependencies bag.Bag[ids.ID]
	
	// Current traversal state
	visited map[ids.ID]bool
	
	// Early termination flag
	terminated bool
	
	// Confidence accumulated along this facet
	confidence int
}

// FacetTraverser handles the traversal of vote graphs through multiple facets
type FacetTraverser struct {
	// Dependency graph - maps each decision to its dependencies
	graph map[ids.ID][]ids.ID
	
	// Vote counts for each decision
	votes map[ids.ID]int
	
	// Confidence thresholds
	alphaConfidence int
}

// NewFacetTraverser creates a new traverser for exploring decision facets
func NewFacetTraverser(alphaConfidence int) *FacetTraverser {
	return &FacetTraverser{
		graph:           make(map[ids.ID][]ids.ID),
		votes:           make(map[ids.ID]int),
		alphaConfidence: alphaConfidence,
	}
}

// AddDecision adds a decision and its dependencies to the graph
func (ft *FacetTraverser) AddDecision(decision ids.ID, dependencies []ids.ID) {
	ft.graph[decision] = dependencies
}

// RecordVote records a vote for a decision
func (ft *FacetTraverser) RecordVote(decision ids.ID, weight int) {
	ft.votes[decision] += weight
}

// Traverse walks the dependency graph starting from the given decisions,
// refracting through each facet until we find paths to commitment
func (ft *FacetTraverser) Traverse(decisions []ids.ID) []*Facet {
	facets := make([]*Facet, 0, len(decisions))
	
	for _, decision := range decisions {
		facet := &Facet{
			root:     decision,
			visited:  make(map[ids.ID]bool),
			dependencies: bag.Bag[ids.ID]{},
		}
		
		// Traverse this facet's path through the graph
		ft.traverseFacet(facet, decision)
		facets = append(facets, facet)
	}
	
	return facets
}

// traverseFacet recursively walks a single facet's path
func (ft *FacetTraverser) traverseFacet(facet *Facet, current ids.ID) {
	// Avoid cycles
	if facet.visited[current] {
		return
	}
	facet.visited[current] = true
	
	// Check if this decision has enough votes for early termination
	if votes := ft.votes[current]; votes >= ft.alphaConfidence {
		facet.confidence += votes
		facet.terminated = true
		return
	}
	
	// Otherwise, we need to traverse dependencies
	deps, exists := ft.graph[current]
	if !exists {
		// Leaf node - accumulate any votes
		facet.confidence += ft.votes[current]
		return
	}
	
	// Refract through each dependency
	for _, dep := range deps {
		facet.dependencies.Add(dep)
		ft.traverseFacet(facet, dep)
		
		// Early termination if we've accumulated enough confidence
		if facet.terminated {
			return
		}
	}
}

// CanTerminate checks if a facet has accumulated enough confidence
// to terminate early without exploring the entire graph
func (f *Facet) CanTerminate(threshold int) bool {
	return f.terminated || f.confidence >= threshold
}

// GetDependencies returns all dependencies discovered along this facet
func (f *Facet) GetDependencies() []ids.ID {
	return f.dependencies.List()
}

// GetConfidence returns the total confidence accumulated along this facet
func (f *Facet) GetConfidence() int {
	return f.confidence
}

// MergeFacets combines multiple facets into a single unified view
// This is like combining multiple refracted beams back into one
func MergeFacets(facets []*Facet) *Facet {
	merged := &Facet{
		visited:      make(map[ids.ID]bool),
		dependencies: bag.Bag[ids.ID]{},
	}
	
	for _, facet := range facets {
		// Merge visited nodes
		for node := range facet.visited {
			merged.visited[node] = true
		}
		
		// Merge dependencies
		for _, dep := range facet.dependencies.List() {
			merged.dependencies.Add(dep)
		}
		
		// Sum confidence
		merged.confidence += facet.confidence
		
		// Propagate termination
		if facet.terminated {
			merged.terminated = true
		}
	}
	
	return merged
}

// PrismaticView provides a multi-faceted view of the decision space
type PrismaticView struct {
	facets map[ids.ID]*Facet
}

// NewPrismaticView creates a new multi-faceted view
func NewPrismaticView() *PrismaticView {
	return &PrismaticView{
		facets: make(map[ids.ID]*Facet),
	}
}

// AddFacet adds a facet to the prismatic view
func (pv *PrismaticView) AddFacet(id ids.ID, facet *Facet) {
	pv.facets[id] = facet
}

// GetStrongestFacet returns the facet with the highest confidence
func (pv *PrismaticView) GetStrongestFacet() (ids.ID, *Facet) {
	var strongestID ids.ID
	var strongestFacet *Facet
	maxConfidence := -1
	
	for id, facet := range pv.facets {
		if facet.confidence > maxConfidence {
			strongestID = id
			strongestFacet = facet
			maxConfidence = facet.confidence
		}
	}
	
	return strongestID, strongestFacet
}