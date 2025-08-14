// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/utils/sampler"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// TestPrismRefract tests the complete Refract pipeline
func TestPrismRefract(t *testing.T) {
	require := require.New(t)
	
	// Create validators bag
	validators := bag.Bag[ids.NodeID]{}
	for i := 0; i < 100; i++ {
		nodeID := ids.GenerateTestNodeID()
		weight := i + 1
		validators.AddCount(nodeID, weight)
	}
	
	// Create parameters
	params := config.Parameters{
		K:               20,
		AlphaPreference: 15,
		AlphaConfidence: 18,
		Beta:            3,
	}
	
	// Create prism
	prism := NewPrism(params, sampler.NewSource(42))
	
	// Create dependency graph with two choices
	deps := NewSimpleDependencyGraph()
	choiceA := ids.GenerateTestID()
	choiceB := ids.GenerateTestID()
	deps.Add(choiceA, nil)
	deps.Add(choiceB, nil)
	
	// Override refract function to simulate voting
	prism.refract = func(sample []ids.NodeID, dg DependencyGraph) []Decision {
		decisions := make([]Decision, 0)
		
		// Simulate 80% voting for choiceA
		weightA := uint64(0)
		weightB := uint64(0)
		
		for i, validator := range sample {
			weight := uint64(validators.Count(validator))
			if i < len(sample)*8/10 {
				weightA += weight
			} else {
				weightB += weight
			}
		}
		
		// Create decisions with proper confidence values
		decisions = append(decisions, &RefractedDecision{
			id:         choiceA,
			Choice:     choiceA,
			Weight:     weightA,
			Confidence: 20, // High confidence for testing
		})
		
		decisions = append(decisions, &RefractedDecision{
			id:         choiceB,
			Choice:     choiceB,
			Weight:     weightB,
			Confidence: 5, // Low confidence
		})
		
		t.Logf("Refract called with %d validators, produced: choiceA weight=%d, choiceB weight=%d", len(sample), weightA, weightB)
		
		return decisions
	}
	
	// Run multiple rounds to build confidence
	for i := 0; i < params.Beta+1; i++ {
		hasQuorum, err := prism.Refract(validators, deps, params)
		require.NoError(err)
		
		t.Logf("Round %d: hasQuorum=%v, preference=%s, confidence=%d, finalized=%v", 
			i+1, hasQuorum, prism.cut.GetPreference(), prism.cut.GetConfidence(), prism.cut.IsFinalized())
		
		// Should achieve quorum after Beta rounds
		if i >= params.Beta-1 {
			require.True(hasQuorum, "Should have quorum after %d rounds", i+1)
			break
		}
	}
	
	// Verify finalization
	require.True(prism.cut.IsFinalized())
	require.Equal(choiceA, prism.cut.GetPreference())
}

// TestSplitter tests the splitter component
func TestSplitter(t *testing.T) {
	require := require.New(t)
	
	// Create validators bag
	validators := bag.Bag[ids.NodeID]{}
	nodeIDs := make([]ids.NodeID, 50)
	for i := 0; i < 50; i++ {
		nodeIDs[i] = ids.GenerateTestNodeID()
		validators.AddCount(nodeIDs[i], i+1)
	}
	
	// Create splitter
	splitter := NewSplitter(sampler.NewSource(123))
	
	// Test sampling
	k := 10
	sample, err := splitter.Sample(validators, k)
	require.NoError(err)
	require.Len(sample, k)
	
	// Verify all sampled nodes are valid
	for _, nodeID := range sample {
		count := validators.Count(nodeID)
		require.Greater(count, 0)
	}
	
	// Test sampling with k > validator count
	sample, err = splitter.Sample(validators, 100)
	require.NoError(err)
	require.Len(sample, len(nodeIDs))
}

// TestCut tests the cut component
func TestCut(t *testing.T) {
	require := require.New(t)
	
	// Create cut
	cut := NewCut(15, 18, 3)
	
	// Create decisions
	choiceA := ids.GenerateTestID()
	choiceB := ids.GenerateTestID()
	
	decisions := []Decision{
		&RefractedDecision{
			id:         ids.GenerateTestID(),
			Choice:     choiceA,
			Weight:     20,
			Confidence: 20,
		},
		&RefractedDecision{
			id:         ids.GenerateTestID(),
			Choice:     choiceB,
			Weight:     10,
			Confidence: 10,
		},
	}
	
	// Process decisions by recording votes
	for _, decision := range decisions {
		if refracted, ok := decision.(*RefractedDecision); ok {
			cut.RecordVote(refracted.Choice, refracted.Weight)
		}
	}
	
	// First refraction
	cut.Refract()
	require.False(cut.IsFinalized()) // Need 3 consecutive rounds
	
	// Second refraction with same votes
	cut.Refract()
	require.False(cut.IsFinalized())
	
	// Third refraction
	cut.Refract()
	require.True(cut.IsFinalized())
	require.Equal(choiceA, cut.GetPreference())
}

// TestRefractFunction tests the standard refract function
func TestRefractFunction(t *testing.T) {
	require := require.New(t)
	
	// Create validators bag
	validators := bag.Bag[ids.NodeID]{}
	sample := make([]ids.NodeID, 20)
	for i := 0; i < 20; i++ {
		nodeID := ids.GenerateTestNodeID()
		sample[i] = nodeID
		validators.AddCount(nodeID, i+1)
	}
	
	// Create dependency graph
	deps := NewSimpleDependencyGraph()
	root := ids.GenerateTestID()
	child1 := ids.GenerateTestID()
	child2 := ids.GenerateTestID()
	
	deps.Add(root, []ids.ID{child1, child2})
	deps.Add(child1, nil)
	deps.Add(child2, nil)
	
	// Create refract function
	refractFunc := StandardRefract(validators, 15)
	
	// Sample is already created above, just use first 10 elements
	sample = sample[:10]
	
	// Run refract
	decisions := refractFunc(sample, deps)
	require.NotEmpty(decisions)
	
	// Verify we got decisions for each node in the graph
	require.Len(decisions, 3) // root, child1, child2
	
	// Verify decisions have been created
	hasRoot := false
	hasChild1 := false
	hasChild2 := false
	
	for _, decision := range decisions {
		if decision.ID() == root {
			hasRoot = true
		} else if decision.ID() == child1 {
			hasChild1 = true
		} else if decision.ID() == child2 {
			hasChild2 = true
		}
	}
	
	require.True(hasRoot, "Should have decision for root")
	require.True(hasChild1, "Should have decision for child1")
	require.True(hasChild2, "Should have decision for child2")
}


