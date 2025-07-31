// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"fmt"

	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

// Spectrum represents the full spectrum of light passing through the prism
// It separates the sampling (splitting) from the traversal (faceting)
type Spectrum struct {
	// The original beam before splitting
	sourceBeam bag.Bag[ids.NodeID]
	
	// The split samples
	rays map[ids.ID][]ids.NodeID // decision -> sampled validators
	
	// The refracted results
	intensities map[ids.ID]bag.Bag[ids.ID] // decision -> vote distribution
}

// NewSpectrum creates a new spectrum analyzer
func NewSpectrum(validators bag.Bag[ids.NodeID]) *Spectrum {
	return &Spectrum{
		sourceBeam:  validators,
		rays:        make(map[ids.ID][]ids.NodeID),
		intensities: make(map[ids.ID]bag.Bag[ids.ID]),
	}
}

// Split performs the initial beam splitting for a decision
// This separates the sampling phase from the traversal phase
func (s *Spectrum) Split(splitter Splitter, decision ids.ID, k int) error {
	sample, err := splitter.Sample(s.sourceBeam, k)
	if err != nil {
		return fmt.Errorf("failed to split beam for decision %s: %w", decision, err)
	}
	
	s.rays[decision] = sample
	return nil
}

// RefractThroughFacets processes the split rays through facets
// This demonstrates how split samples can be processed separately
func (s *Spectrum) RefractThroughFacets(
	decision ids.ID,
	facets []*Facet,
	alphaConfidence int,
) {
	rays, exists := s.rays[decision]
	if !exists {
		return
	}
	
	// Initialize intensity bag for this decision
	intensity := bag.Bag[ids.ID]{}
	
	// Each ray (validator) passes through the facets
	for _, nodeID := range rays {
		weight := s.sourceBeam.Count(nodeID)
		
		// The ray refracts through each facet
		for _, facet := range facets {
			if facet.CanTerminate(alphaConfidence) {
				// This facet allows the ray to pass
				intensity.AddCount(facet.root, weight)
			}
		}
	}
	
	s.intensities[decision] = intensity
}

// GetIntensity returns the vote intensity for a decision
func (s *Spectrum) GetIntensity(decision ids.ID) bag.Bag[ids.ID] {
	return s.intensities[decision]
}

// Analyze returns a spectral analysis of all decisions
func (s *Spectrum) Analyze() map[ids.ID]map[string]interface{} {
	analysis := make(map[ids.ID]map[string]interface{})
	
	for decision, rays := range s.rays {
		intensity := s.intensities[decision]
		
		// Find dominant wavelength (most voted choice)
		var dominant ids.ID
		maxIntensity := 0
		
		for _, choice := range intensity.List() {
			count := intensity.Count(choice)
			if count > maxIntensity {
				dominant = choice
				maxIntensity = count
			}
		}
		
		analysis[decision] = map[string]interface{}{
			"sample_size":      len(rays),
			"dominant_choice":  dominant,
			"max_intensity":    maxIntensity,
			"unique_choices":   len(intensity.List()),
			"total_intensity":  intensity.Len(),
		}
	}
	
	return analysis
}

// ChromaticDispersion represents how different wavelengths
// (validator opinions) separate as they pass through the prism
type ChromaticDispersion struct {
	// Wavelength bands (vote choices)
	bands map[ids.ID]*WavelengthBand
}

// WavelengthBand represents validators that vote for the same choice
type WavelengthBand struct {
	Choice         ids.ID
	Validators     []ids.NodeID
	TotalWeight    uint64
	WeightMultiplier float64 // Multiplier for vote weight (e.g., for prioritization)
}

// NewChromaticDispersion creates a new dispersion analyzer
func NewChromaticDispersion() *ChromaticDispersion {
	return &ChromaticDispersion{
		bands: make(map[ids.ID]*WavelengthBand),
	}
}

// AddPhoton adds a validator's vote to the appropriate wavelength band
func (cd *ChromaticDispersion) AddPhoton(
	validator ids.NodeID,
	choice ids.ID,
	weight uint64,
) {
	band, exists := cd.bands[choice]
	if !exists {
		band = &WavelengthBand{
			Choice:      choice,
			Validators:  make([]ids.NodeID, 0),
			TotalWeight: 0,
			WeightMultiplier: 1.0,
		}
		cd.bands[choice] = band
	}
	
	band.Validators = append(band.Validators, validator)
	band.TotalWeight += weight
}

// SetWeightMultiplier sets the weight multiplier for a choice
// Higher multiplier = higher priority in consensus
func (cd *ChromaticDispersion) SetWeightMultiplier(choice ids.ID, multiplier float64) {
	if band, exists := cd.bands[choice]; exists {
		band.WeightMultiplier = multiplier
	}
}

// GetDominantWavelength returns the strongest wavelength band
func (cd *ChromaticDispersion) GetDominantWavelength() (ids.ID, uint64) {
	var dominant ids.ID
	var maxWeight uint64
	
	for choice, band := range cd.bands {
		// Weight adjusted by multiplier
		adjustedWeight := uint64(float64(band.TotalWeight) * band.WeightMultiplier)
		if adjustedWeight > maxWeight {
			dominant = choice
			maxWeight = adjustedWeight
		}
	}
	
	return dominant, maxWeight
}

// GetSpectralLines returns all wavelength bands
func (cd *ChromaticDispersion) GetSpectralLines() map[ids.ID]*WavelengthBand {
	return cd.bands
}

// OpticalPath represents the complete path light takes through the prism
type OpticalPath struct {
	// Entry point
	entryAngle float64
	
	// Internal reflections
	reflections []Reflection
	
	// Exit point
	exitAngle float64
	exitChoice ids.ID
}

// Reflection represents an internal reflection within the prism
type Reflection struct {
	facetID    ids.ID
	angle      float64
	intensity  float64
	terminated bool
}

// TraceOpticalPath traces how a single validator's opinion
// travels through the prism's optical system
func TraceOpticalPath(
	validator ids.NodeID,
	weight uint64,
	facets []*Facet,
	alphaConfidence int,
) *OpticalPath {
	path := &OpticalPath{
		entryAngle:  float64(weight) / 1000.0, // Normalize weight to angle
		reflections: make([]Reflection, 0),
	}
	
	// Trace through each facet
	for _, facet := range facets {
		reflection := Reflection{
			facetID:    facet.root,
			angle:      float64(facet.confidence) / float64(alphaConfidence),
			intensity:  float64(facet.confidence),
			terminated: facet.terminated,
		}
		
		path.reflections = append(path.reflections, reflection)
		
		if facet.CanTerminate(alphaConfidence) {
			path.exitChoice = facet.root
			path.exitAngle = reflection.angle * path.entryAngle
			break
		}
	}
	
	return path
}