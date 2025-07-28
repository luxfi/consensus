// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quantum

import (
	"sync"
	"time"

	"github.com/luxfi/ids"
)

// Beam represents a coherent stream of photons forming a message propagation path.
// Like a laser beam cutting through space, a Beam carries consensus messages
// with focused intensity from source to destination across the network.
type Beam struct {
	mu sync.RWMutex

	// Unique identifier
	ID ids.ID

	// Source and destination
	Source      ids.NodeID
	Destination ids.NodeID

	// Photons in this beam
	Photons []*Photon

	// Beam properties
	Wavelength  uint64    // Consensus round/height
	Frequency   uint64    // Messages per second
	Intensity   uint64    // Aggregated energy
	Coherence   float64   // Alignment of photons (0.0-1.0)
	CreatedAt   time.Time
	LastUpdated time.Time

	// Beam state
	Active     bool
	Focused    bool // Whether beam has reached critical mass
	Terminated bool
}

// NewBeam creates a new message propagation beam
func NewBeam(source, destination ids.NodeID, wavelength uint64) *Beam {
	return &Beam{
		ID:          ids.GenerateTestID(),
		Source:      source,
		Destination: destination,
		Wavelength:  wavelength,
		Photons:     make([]*Photon, 0),
		Frequency:   1,
		Intensity:   0,
		Coherence:   1.0,
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
		Active:      true,
		Focused:     false,
		Terminated:  false,
	}
}

// AddPhoton adds a photon to the beam
func (b *Beam) AddPhoton(p *Photon) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.Terminated {
		return false
	}

	// Only accept photons from the same source
	if p.Source != b.Source {
		return false
	}

	b.Photons = append(b.Photons, p)
	b.Intensity += p.Energy
	b.LastUpdated = time.Now()

	// Update frequency
	duration := b.LastUpdated.Sub(b.CreatedAt).Seconds()
	if duration > 0 {
		b.Frequency = uint64(float64(len(b.Photons)) / duration)
	}

	// Check if beam has reached critical mass
	if !b.Focused && b.Intensity >= b.Wavelength {
		b.Focused = true
	}

	return true
}

// Split creates multiple beams from this one (multicast)
func (b *Beam) Split(destinations []ids.NodeID) []*Beam {
	b.mu.RLock()
	defer b.mu.RUnlock()

	beams := make([]*Beam, len(destinations))
	for i, dest := range destinations {
		newBeam := &Beam{
			ID:          ids.GenerateTestID(),
			Source:      b.Source,
			Destination: dest,
			Wavelength:  b.Wavelength,
			Photons:     make([]*Photon, len(b.Photons)),
			Frequency:   b.Frequency,
			Intensity:   b.Intensity,
			Coherence:   b.Coherence * 0.9, // Slight coherence loss in splitting
			CreatedAt:   time.Now(),
			LastUpdated: time.Now(),
			Active:      true,
			Focused:     b.Focused,
			Terminated:  false,
		}
		// Deep copy photons
		copy(newBeam.Photons, b.Photons)
		beams[i] = newBeam
	}
	return beams
}

// Merge combines multiple beams into one (convergence)
func (b *Beam) Merge(other *Beam) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.Terminated || other.Terminated {
		return false
	}

	// Only merge beams with same destination and wavelength
	if b.Destination != other.Destination || b.Wavelength != other.Wavelength {
		return false
	}

	// Combine photons
	b.Photons = append(b.Photons, other.Photons...)
	b.Intensity += other.Intensity

	// Update coherence (average)
	b.Coherence = (b.Coherence + other.Coherence) / 2

	// Update frequency and timestamps
	b.LastUpdated = time.Now()
	duration := b.LastUpdated.Sub(b.CreatedAt).Seconds()
	if duration > 0 {
		b.Frequency = uint64(float64(len(b.Photons)) / duration)
	}

	// Check focus
	if !b.Focused && b.Intensity >= b.Wavelength {
		b.Focused = true
	}

	return true
}

// Terminate ends the beam transmission
func (b *Beam) Terminate() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.Active = false
	b.Terminated = true
	b.LastUpdated = time.Now()
}

// GetCoherentPhotons returns photons above coherence threshold
func (b *Beam) GetCoherentPhotons(threshold float64) []*Photon {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.Coherence < threshold {
		return []*Photon{}
	}

	return b.Photons
}