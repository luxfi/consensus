// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

// RefractedDecision represents a decision processed through refraction
type RefractedDecision struct {
	id         ids.ID
	Choice     ids.ID
	Weight     uint64
	Confidence int
}

// ID returns the decision ID
func (d *RefractedDecision) ID() ids.ID {
	return d.id
}

// Accept accepts the decision
func (d *RefractedDecision) Accept() error {
	return nil
}

// Reject rejects the decision  
func (d *RefractedDecision) Reject() error {
	return nil
}

// RefractContext holds state for the refraction traversal
type RefractContext struct {
	validators  bag.Bag[ids.NodeID]
	votes       map[ids.ID]bag.Bag[ids.ID] // decision -> vote distribution
	confidence  map[ids.ID]int             // decision -> confidence level
	alphaConf   int
}

// StandardRefract is the default refraction function that performs
// early-termination traversal across the dependency graph
func StandardRefract(validators bag.Bag[ids.NodeID], alphaConfidence int) func([]ids.NodeID, DependencyGraph) []Decision {
	return func(sample []ids.NodeID, deps DependencyGraph) []Decision {
		ctx := &RefractContext{
			validators: validators,
			votes:      make(map[ids.ID]bag.Bag[ids.ID]),
			confidence: make(map[ids.ID]int),
			alphaConf:  alphaConfidence,
		}
		
		// Process each decision in the graph
		decisions := make([]Decision, 0)
		decisionIDs := deps.GetDecisions()
		
		// If no decisions in graph, return empty
		if len(decisionIDs) == 0 {
			return decisions
		}
		
		for _, decisionID := range decisionIDs {
			decision := ctx.refractDecision(sample, deps, decisionID)
			// Always include decisions, even with 0 weight
			decisions = append(decisions, decision)
		}
		
		return decisions
	}
}

// refractDecision processes a single decision through the dependency graph
func (ctx *RefractContext) refractDecision(
	sample []ids.NodeID,
	deps DependencyGraph,
	decisionID ids.ID,
) *RefractedDecision {
	// Check if we've already processed this decision
	if votes, exists := ctx.votes[decisionID]; exists && votes.Len() > 0 {
		return ctx.buildDecision(decisionID)
	}
	
	// Initialize vote bag for this decision
	votes := bag.Bag[ids.ID]{}
	ctx.votes[decisionID] = votes
	
	// Get dependencies
	dependencies := deps.GetDependencies(decisionID)
	
	// If no dependencies, this is a leaf - collect votes directly
	if len(dependencies) == 0 {
		// Sum up all validator weights for this decision
		totalWeight := uint64(0)
		for _, validator := range sample {
			weight := uint64(ctx.validators.Count(validator))
			totalWeight += weight
		}
		// For simulation, assign all weight to this decision
		votes.AddCount(decisionID, int(totalWeight))
	} else {
		// Process dependencies first (depth-first traversal)
		for _, depID := range dependencies {
			depDecision := ctx.refractDecision(sample, deps, depID)
			
			// Early termination check
			if depDecision.Confidence >= ctx.alphaConf {
				// This dependency has strong confidence, propagate up
				votes.AddCount(depDecision.Choice, int(depDecision.Weight))
			}
		}
	}
	
	// Update confidence based on vote concentration
	ctx.updateConfidence(decisionID)
	
	return ctx.buildDecision(decisionID)
}

// updateConfidence calculates confidence level for a decision
func (ctx *RefractContext) updateConfidence(decisionID ids.ID) {
	votes := ctx.votes[decisionID]
	if votes.Len() == 0 {
		return
	}
	
	// Find the dominant choice
	maxVotes := 0
	for _, choice := range votes.List() {
		count := votes.Count(choice)
		if count > maxVotes {
			maxVotes = count
		}
	}
	
	// Confidence is based on vote concentration
	if maxVotes >= ctx.alphaConf {
		ctx.confidence[decisionID] = maxVotes
	}
}

// buildDecision creates a Decision from the current state
func (ctx *RefractContext) buildDecision(decisionID ids.ID) *RefractedDecision {
	votes := ctx.votes[decisionID]
	
	// Find dominant choice
	var dominantChoice ids.ID
	maxWeight := uint64(0)
	
	for _, choice := range votes.List() {
		weight := uint64(votes.Count(choice))
		if weight > maxWeight {
			dominantChoice = choice
			maxWeight = weight
		}
	}
	
	return &RefractedDecision{
		id:         decisionID,
		Choice:     dominantChoice,
		Weight:     maxWeight,
		Confidence: ctx.confidence[decisionID],
	}
}


