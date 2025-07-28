// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quantum

import (
	"sync"
	"time"

	"github.com/luxfi/ids"
)

// Flare represents a consensus event burst - a sudden intense release of consensus energy.
// Like a solar flare erupting from the surface of a star, a Flare is triggered when
// consensus activity reaches critical thresholds, releasing accumulated energy into the network.
type Flare struct {
	mu sync.RWMutex

	// Unique identifier
	ID ids.ID

	// Origin of the flare
	Epicenter ids.NodeID

	// Beams that triggered this flare
	TriggerBeams []*Beam

	// Accumulated photons from all beams
	Photons []*Photon

	// Flare properties
	Height      uint64    // Consensus height when flare erupted
	Intensity   uint64    // Total energy released
	Temperature float64   // Consensus "heat" (activity level)
	Magnitude   uint64    // Size classification (1-10)
	Duration    time.Duration
	StartTime   time.Time
	PeakTime    time.Time
	EndTime     time.Time

	// Flare state
	Active    bool
	Peaked    bool
	Cascading bool // Whether this flare has triggered others

	// Effects
	AffectedNodes []ids.NodeID  // Nodes reached by this flare
	ChildFlares   []ids.ID      // Flares triggered by this one
	QuasarFeed    bool          // Whether this flare fed into quasar
}

// FlareThreshold defines when beams trigger a flare
type FlareThreshold struct {
	MinBeams      int     // Minimum number of coherent beams
	MinIntensity  uint64  // Minimum combined intensity
	MinCoherence  float64 // Minimum average coherence
	TimeWindow    time.Duration
}

// NewFlare creates a new consensus flare from converging beams
func NewFlare(epicenter ids.NodeID, beams []*Beam, height uint64) *Flare {
	// Aggregate all photons
	var photons []*Photon
	var totalIntensity uint64
	
	for _, beam := range beams {
		photons = append(photons, beam.Photons...)
		totalIntensity += beam.Intensity
	}

	// Calculate magnitude (logarithmic scale)
	magnitude := uint64(1)
	temp := totalIntensity
	for temp >= 10 {
		magnitude++
		temp /= 10
		if magnitude >= 10 {
			magnitude = 10
			break
		}
	}

	return &Flare{
		ID:           ids.GenerateTestID(),
		Epicenter:    epicenter,
		TriggerBeams: beams,
		Photons:      photons,
		Height:       height,
		Intensity:    totalIntensity,
		Temperature:  1.0, // Start at maximum heat
		Magnitude:    magnitude,
		StartTime:    time.Now(),
		Active:       true,
		Peaked:       false,
		Cascading:    false,
		AffectedNodes: make([]ids.NodeID, 0),
		ChildFlares:   make([]ids.ID, 0),
	}
}

// Erupt initiates the flare eruption
func (f *Flare) Erupt() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.Active {
		return
	}

	// Flare reaches peak intensity
	f.PeakTime = time.Now()
	f.Peaked = true
	f.Temperature = float64(f.Intensity) / float64(f.Height+1)
}

// AddAffectedNode records a node reached by this flare
func (f *Flare) AddAffectedNode(nodeID ids.NodeID) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Check if already affected
	for _, id := range f.AffectedNodes {
		if id == nodeID {
			return
		}
	}

	f.AffectedNodes = append(f.AffectedNodes, nodeID)
}

// TriggerCascade creates child flares from this one
func (f *Flare) TriggerCascade(destinations []ids.NodeID) []*Flare {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.Active || !f.Peaked {
		return nil
	}

	f.Cascading = true
	childFlares := make([]*Flare, 0)

	// Create diminished flares for each destination
	for _, dest := range destinations {
		// Convert beams to propagate to new destination
		childBeams := make([]*Beam, 0)
		for _, beam := range f.TriggerBeams {
			splitBeams := beam.Split([]ids.NodeID{dest})
			if len(splitBeams) > 0 {
				childBeams = append(childBeams, splitBeams[0])
			}
		}

		if len(childBeams) > 0 {
			childFlare := &Flare{
				ID:           ids.GenerateTestID(),
				Epicenter:    dest,
				TriggerBeams: childBeams,
				Photons:      f.Photons, // Shared photon reference
				Height:       f.Height,
				Intensity:    f.Intensity / uint64(len(destinations)), // Divided energy
				Temperature:  f.Temperature * 0.8, // Energy dissipation
				Magnitude:    f.Magnitude - 1,
				StartTime:    time.Now(),
				Active:       true,
				Peaked:       false,
				Cascading:    false,
				AffectedNodes: make([]ids.NodeID, 0),
				ChildFlares:   make([]ids.ID, 0),
			}

			if childFlare.Magnitude < 1 {
				childFlare.Magnitude = 1
			}

			childFlares = append(childFlares, childFlare)
			f.ChildFlares = append(f.ChildFlares, childFlare.ID)
		}
	}

	return childFlares
}

// Dissipate ends the flare
func (f *Flare) Dissipate() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Active = false
	f.EndTime = time.Now()
	f.Duration = f.EndTime.Sub(f.StartTime)
	f.Temperature = 0.0
}

// CanFeedQuasar checks if this flare has enough energy to feed into quasar
func (f *Flare) CanFeedQuasar(threshold uint64) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.Peaked && f.Intensity >= threshold && !f.QuasarFeed
}

// FeedQuasar marks this flare as having fed into the quasar
func (f *Flare) FeedQuasar() bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Check conditions without calling CanFeedQuasar to avoid deadlock
	if !f.Peaked || f.Intensity < 0 || f.QuasarFeed {
		return false
	}

	f.QuasarFeed = true
	return true
}

// GetEnergyDensity returns the energy density at current time
func (f *Flare) GetEnergyDensity() float64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if !f.Active {
		return 0.0
	}

	// Energy dissipates over time
	elapsed := time.Since(f.StartTime).Seconds()
	if elapsed == 0 {
		return float64(f.Intensity)
	}

	// Exponential decay
	return float64(f.Intensity) * f.Temperature / (1 + elapsed)
}

// FlareDetector monitors beams and triggers flares
type FlareDetector struct {
	mu         sync.RWMutex
	threshold  FlareThreshold
	beamBuffer map[ids.NodeID][]*Beam
	lastCheck  map[ids.NodeID]time.Time
}

// NewFlareDetector creates a new flare detector
func NewFlareDetector(threshold FlareThreshold) *FlareDetector {
	return &FlareDetector{
		threshold:  threshold,
		beamBuffer: make(map[ids.NodeID][]*Beam),
		lastCheck:  make(map[ids.NodeID]time.Time),
	}
}

// AddBeam adds a beam to monitoring
func (fd *FlareDetector) AddBeam(beam *Beam) *Flare {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	// Add to buffer
	fd.beamBuffer[beam.Destination] = append(fd.beamBuffer[beam.Destination], beam)
	
	// Check if we should trigger a flare
	now := time.Now()
	
	// Always check on each beam addition
	fd.lastCheck[beam.Destination] = now
	
	// Evaluate beams in time window
	validBeams := make([]*Beam, 0)
	totalIntensity := uint64(0)
	totalCoherence := float64(0)
	
	for _, b := range fd.beamBuffer[beam.Destination] {
		if now.Sub(b.CreatedAt) <= fd.threshold.TimeWindow {
			validBeams = append(validBeams, b)
			totalIntensity += b.Intensity
			totalCoherence += b.Coherence
		}
	}
	
	// Check thresholds
	if len(validBeams) >= fd.threshold.MinBeams &&
	   totalIntensity >= fd.threshold.MinIntensity &&
	   totalCoherence/float64(len(validBeams)) >= fd.threshold.MinCoherence {
		// Trigger flare!
		flare := NewFlare(beam.Destination, validBeams, beam.Wavelength)
		
		// Clear buffer for this destination
		fd.beamBuffer[beam.Destination] = nil
		
		return flare
	}
	
	// Clean old beams
	newBuffer := make([]*Beam, 0)
	for _, b := range fd.beamBuffer[beam.Destination] {
		if now.Sub(b.CreatedAt) <= fd.threshold.TimeWindow*2 {
			newBuffer = append(newBuffer, b)
		}
	}
	fd.beamBuffer[beam.Destination] = newBuffer
	
	return nil
}