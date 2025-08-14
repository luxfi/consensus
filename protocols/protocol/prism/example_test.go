// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism_test

import (
	"fmt"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocols/protocol/prism"
	"github.com/luxfi/consensus/utils/sampler"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

// MockValidatorSet implements prism.ValidatorSet for testing
type MockValidatorSet struct {
	validators map[ids.NodeID]uint64
}

func NewMockValidatorSet() *MockValidatorSet {
	return &MockValidatorSet{
		validators: make(map[ids.NodeID]uint64),
	}
}

func (m *MockValidatorSet) AddValidator(nodeID ids.NodeID, weight uint64) {
	m.validators[nodeID] = weight
}

func (m *MockValidatorSet) GetValidators() map[ids.NodeID]interface{} {
	result := make(map[ids.NodeID]interface{})
	for id := range m.validators {
		result[id] = nil
	}
	return result
}

func (m *MockValidatorSet) GetWeight(nodeID ids.NodeID) uint64 {
	return m.validators[nodeID]
}

// ExamplePrism_Refract demonstrates the optical metaphor in action
func ExamplePrism_Refract() {
	// Create a validator set (the light source)
	validators := bag.Bag[ids.NodeID]{}
	for i := 0; i < 100; i++ {
		nodeID := ids.GenerateTestNodeID()
		weight := i + 1 // Varying weights
		validators.AddCount(nodeID, weight)
	}
	
	// Create the prism with consensus parameters
	params := config.DefaultParameters
	params.K = 20                // Sample 20 validators
	params.AlphaPreference = 15  // Need 15 votes for preference
	params.AlphaConfidence = 18  // Need 18 votes for confidence
	params.Beta = 8              // Need 8 consecutive rounds
	
	p := prism.NewPrism(params, sampler.NewSource(42))
	
	// Define two choices (like two colors of light)
	choiceRed := ids.GenerateTestID()
	choiceBlue := ids.GenerateTestID()
	
	// Create dependency graph with the choices  
	deps := prism.NewSimpleDependencyGraph() 
	deps.Add(choiceRed, nil)
	deps.Add(choiceBlue, nil)
	
	// Run consensus rounds (shine light through the prism)
	for round := 1; round <= 10; round++ {
		
		// Use Refract method which handles the split→refract→cut pipeline
		hasQuorum, err := p.Refract(validators, deps, params)
		if err != nil {
			fmt.Printf("Round %d: Error - %v\n", round, err)
			continue
		}
		
		if hasQuorum {
			fmt.Printf("Round %d: Consensus reached! Preference: %s\n", 
				round, p.GetPreference())
			break
		}
		
		fmt.Printf("Round %d: Preference: %s, Confidence: %d\n", 
			round, p.GetPreference(), p.GetConfidence())
	}
	
}

// TestSpectrum demonstrates the spectrum analyzer
func TestSpectrum(t *testing.T) {
	// Create validators
	validators := bag.Bag[ids.NodeID]{}
	nodeIDs := make([]ids.NodeID, 50)
	for i := 0; i < 50; i++ {
		nodeIDs[i] = ids.GenerateTestNodeID()
		validators.AddCount(nodeIDs[i], i+1)
	}
	
	// Create spectrum analyzer
	spectrum := prism.NewSpectrum(validators)
	
	// Create splitter
	splitter := prism.NewSplitter(sampler.NewSource(123))
	
	// Split beams for different decisions
	decision1 := ids.GenerateTestID()
	decision2 := ids.GenerateTestID()
	
	// Split the validator beam
	if err := spectrum.Split(splitter, decision1, 15); err != nil {
		t.Fatalf("Failed to split for decision1: %v", err)
	}
	if err := spectrum.Split(splitter, decision2, 15); err != nil {
		t.Fatalf("Failed to split for decision2: %v", err)
	}
	
	// Create facets for traversal
	traverser := prism.NewFacetTraverser(10)
	traverser.AddDecision(decision1, []ids.ID{})
	traverser.AddDecision(decision2, []ids.ID{decision1}) // decision2 depends on decision1
	
	// Record some votes
	for i := 0; i < 20; i++ {
		traverser.RecordVote(decision1, i*2)
		if i < 10 {
			traverser.RecordVote(decision2, i)
		}
	}
	
	// Refract through facets
	facets1 := traverser.Traverse([]ids.ID{decision1})
	facets2 := traverser.Traverse([]ids.ID{decision2})
	
	spectrum.RefractThroughFacets(decision1, facets1, 10)
	spectrum.RefractThroughFacets(decision2, facets2, 10)
	
	// Analyze the spectrum
	analysis := spectrum.Analyze()
	
	for decision, stats := range analysis {
		fmt.Printf("Decision %s:\n", decision)
		fmt.Printf("  Sample size: %v\n", stats["sample_size"])
		fmt.Printf("  Dominant choice: %v\n", stats["dominant_choice"])
		fmt.Printf("  Max intensity: %v\n", stats["max_intensity"])
		fmt.Printf("  Total intensity: %v\n", stats["total_intensity"])
	}
}

// TestChromaticDispersion demonstrates wavelength separation
func TestChromaticDispersion(t *testing.T) {
	// Create chromatic dispersion analyzer
	dispersion := prism.NewChromaticDispersion()
	
	// Simulate validators voting (photons entering the prism)
	choiceA := ids.GenerateTestID()
	choiceB := ids.GenerateTestID()
	choiceC := ids.GenerateTestID()
	
	// Add photons to different wavelength bands
	for i := 0; i < 30; i++ {
		validator := ids.GenerateTestNodeID()
		
		if i < 15 {
			// Red wavelength (choice A)
			dispersion.AddPhoton(validator, choiceA, uint64(i+1))
		} else if i < 25 {
			// Blue wavelength (choice B)
			dispersion.AddPhoton(validator, choiceB, uint64(i+1))
		} else {
			// Green wavelength (choice C)
			dispersion.AddPhoton(validator, choiceC, uint64(i+1))
		}
	}
	
	// Set weight multipliers (priority adjustments)
	dispersion.SetWeightMultiplier(choiceA, 1.0)  // Normal weight
	dispersion.SetWeightMultiplier(choiceB, 1.5)  // 50% boost
	dispersion.SetWeightMultiplier(choiceC, 0.8)  // 20% reduction
	
	// Get dominant wavelength
	dominant, weight := dispersion.GetDominantWavelength()
	fmt.Printf("Dominant wavelength: %s with adjusted weight %d\n", dominant, weight)
	
	// Show spectral lines
	lines := dispersion.GetSpectralLines()
	for choice, band := range lines {
		fmt.Printf("Wavelength %s: %d validators, total weight %d, weight multiplier %.1f\n",
			choice, len(band.Validators), band.TotalWeight, band.WeightMultiplier)
	}
}