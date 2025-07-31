// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package prism provides an optics-inspired abstraction for consensus voting.
//
// Just as a prism splits white light into its component colors, this package
// splits the consensus process into three key optical components:
//
// 1. Splitter - Takes the full validator set and samples a subset (like AvalancheGo's Sample())
// 2. Facet - Guides traversal through decision graphs with early termination
// 3. Cut - Determines when enough votes have been collected (α and β thresholds)
//
// Together, these components refract validator opinions into clear consensus decisions.
package prism

import (
	"fmt"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/consensus/utils/sampler"
	"github.com/luxfi/ids"
)

// Prism combines all three optical components into a unified consensus mechanism
type Prism struct {
	// Optical components
	splitter Splitter
	refract  func(sample []ids.NodeID, deps DependencyGraph) []Decision
	cut      *Cut
	
	// Configuration
	params config.Parameters
	
	// Metrics
	rounds    int
	startTime time.Time
}

// NewPrism creates a new prism with the given parameters
func NewPrism(params config.Parameters, source sampler.Source) *Prism {
	splitter := NewSplitter(source)
	cut := NewCut(params.AlphaPreference, params.AlphaConfidence, params.Beta)
	
	// Create a refract function placeholder - will be set during Poll
	refract := func(sample []ids.NodeID, deps DependencyGraph) []Decision {
		// This will be overridden in the Refract method
		return nil
	}
	
	return &Prism{
		splitter:  splitter,
		refract:   refract,
		cut:       cut,
		params:    params,
		startTime: time.Now(),
	}
}

// Refract runs the full split→refract→cut pipeline.
// It returns true if the α/β "cut" passes.
func (p *Prism) Refract(
	validators bag.Bag[ids.NodeID],
	deps DependencyGraph,
	params config.Parameters, // holds K, Alpha, Beta, etc.
) (bool, error) {
	// 1) Split off k validators from the full set:
	sample, err := p.splitter.Sample(validators, params.GetK())
	if err != nil {
		return false, fmt.Errorf("splitter failed: %w", err)
	}

	// 2) Refract (traverse with early exit) through the vote-graph:
	// Use existing refract function if set, otherwise use standard
	if p.refract == nil {
		p.refract = StandardRefract(validators, params.AlphaConfidence)
	}
	decisions := p.refract(sample, deps)

	// 3) Apply the α/β cut to decide if consensus is reached:
	// Clear only the votes from previous round, not the confidence state
	p.cut.votes = bag.Bag[ids.ID]{}
	p.cut.totalWeight = 0
	
	// Convert decisions to votes with weights
	for _, decision := range decisions {
		if refracted, ok := decision.(*RefractedDecision); ok {
			p.cut.RecordVote(refracted.Choice, refracted.Weight)
		}
	}
	p.cut.Refract()
	p.rounds++ // Increment rounds counter
	
	return p.cut.IsFinalized(), nil
}

// Poll performs a single consensus round using a simple dependency graph
// Returns whether consensus was reached
func (p *Prism) Poll(validators bag.Bag[ids.NodeID], decisionIDs []ids.ID) (bool, error) {
	p.rounds++
	
	// Create a simple dependency graph from the decision list
	deps := NewSimpleDependencyGraph()
	for _, id := range decisionIDs {
		deps.Add(id, nil) // No dependencies for simple case
	}
	
	// Use Refract to process the entire optical path
	hasQuorum, err := p.Refract(validators, deps, p.params)
	if err != nil {
		return false, err
	}
	
	return hasQuorum, nil
}

// GetPreference returns the current preference
func (p *Prism) GetPreference() ids.ID {
	return p.cut.GetPreference()
}

// GetConfidence returns the current confidence level
func (p *Prism) GetConfidence() int {
	return p.cut.GetConfidence()
}

// GetMetrics returns performance metrics
func (p *Prism) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"rounds":     p.rounds,
		"duration":   time.Since(p.startTime),
		"finalized":  p.cut.IsFinalized(),
		"preference": p.cut.GetPreference(),
		"confidence": p.cut.GetConfidence(),
	}
}

// Reset clears the prism state for a new consensus instance
func (p *Prism) Reset() {
	p.cut.Reset()
	p.rounds = 0
	p.startTime = time.Now()
}

// String returns a string representation of the prism state
func (p *Prism) String() string {
	return fmt.Sprintf("Prism{rounds=%d, %s}", p.rounds, p.cut)
}

// PrismArray manages multiple prisms for parallel consensus instances
type PrismArray struct {
	prisms map[ids.ID]*Prism
	params config.Parameters
	source sampler.Source
}

// NewPrismArray creates a new array of prisms
func NewPrismArray(params config.Parameters, source sampler.Source) *PrismArray {
	return &PrismArray{
		prisms: make(map[ids.ID]*Prism),
		params: params,
		source: source,
	}
}

// GetOrCreatePrism gets an existing prism or creates a new one
func (pa *PrismArray) GetOrCreatePrism(id ids.ID) *Prism {
	if prism, exists := pa.prisms[id]; exists {
		return prism
	}
	
	prism := NewPrism(pa.params, pa.source)
	pa.prisms[id] = prism
	return prism
}

// Poll runs consensus on a specific decision
func (pa *PrismArray) Poll(id ids.ID, validators bag.Bag[ids.NodeID], decisions []ids.ID) (bool, error) {
	prism := pa.GetOrCreatePrism(id)
	return prism.Poll(validators, decisions)
}

// GetFinalized returns all finalized decisions
func (pa *PrismArray) GetFinalized() []ids.ID {
	var finalized []ids.ID
	
	for id, prism := range pa.prisms {
		if prism.cut.IsFinalized() {
			finalized = append(finalized, id)
		}
	}
	
	return finalized
}

// RemoveFinalized removes all finalized prisms
func (pa *PrismArray) RemoveFinalized() int {
	finalized := pa.GetFinalized()
	
	for _, id := range finalized {
		delete(pa.prisms, id)
	}
	
	return len(finalized)
}

// OpticalBench provides a testing harness for prism components
type OpticalBench struct {
	// Components under test
	splitter  Splitter
	traverser *FacetTraverser
	cuts      map[ids.ID]Cutter
	
	// Test configuration
	validators bag.Bag[ids.NodeID]
	decisions  []ids.ID
	
	// Metrics
	samples    [][]ids.NodeID
	facets     [][]*Facet
	voteCounts []bag.Bag[ids.ID]
}

// NewOpticalBench creates a new test bench
func NewOpticalBench(validators bag.Bag[ids.NodeID], decisions []ids.ID) *OpticalBench {
	return &OpticalBench{
		splitter:   NewSplitter(sampler.NewSource(0)),
		traverser:  NewFacetTraverser(15), // Default alpha confidence
		cuts:       make(map[ids.ID]Cutter),
		validators: validators,
		decisions:  decisions,
		samples:    make([][]ids.NodeID, 0),
		facets:     make([][]*Facet, 0),
		voteCounts: make([]bag.Bag[ids.ID], 0),
	}
}

// RunExperiment runs a full consensus experiment
func (ob *OpticalBench) RunExperiment(rounds int, k int) {
	for round := 0; round < rounds; round++ {
		// Sample validators
		sample, _ := ob.splitter.Sample(ob.validators, k)
		ob.samples = append(ob.samples, sample)
		
		// Traverse decision graph
		facets := ob.traverser.Traverse(ob.decisions)
		ob.facets = append(ob.facets, facets)
		
		// Process votes through cuts
		votes := bag.Bag[ids.ID]{}
		for _, nodeID := range sample {
			// Simulate vote collection
			weight := uint64(ob.validators.Count(nodeID))
			// In real scenario, we'd get actual vote from validator
			// For testing, we'll simulate based on facet termination
			for _, facet := range facets {
				if facet.CanTerminate(15) {
					votes.AddCount(facet.root, int(weight))
				}
			}
		}
		ob.voteCounts = append(ob.voteCounts, votes)
	}
}

// GetResults returns experiment results
func (ob *OpticalBench) GetResults() map[string]interface{} {
	return map[string]interface{}{
		"total_rounds":   len(ob.samples),
		"avg_sample_size": ob.avgSampleSize(),
		"facet_stats":     ob.facetStats(),
		"vote_distribution": ob.voteDistribution(),
	}
}

func (ob *OpticalBench) avgSampleSize() float64 {
	if len(ob.samples) == 0 {
		return 0
	}
	
	total := 0
	for _, sample := range ob.samples {
		total += len(sample)
	}
	
	return float64(total) / float64(len(ob.samples))
}

func (ob *OpticalBench) facetStats() map[string]interface{} {
	totalFacets := 0
	totalTerminated := 0
	
	for _, roundFacets := range ob.facets {
		totalFacets += len(roundFacets)
		for _, facet := range roundFacets {
			if facet.terminated {
				totalTerminated++
			}
		}
	}
	
	return map[string]interface{}{
		"total_facets":      totalFacets,
		"terminated_facets": totalTerminated,
		"termination_rate":  float64(totalTerminated) / float64(totalFacets),
	}
}

func (ob *OpticalBench) voteDistribution() map[ids.ID]int {
	distribution := make(map[ids.ID]int)
	
	for _, votes := range ob.voteCounts {
		for _, id := range votes.List() {
			count := votes.Count(id)
			distribution[id] += count
		}
	}
	
	return distribution
}