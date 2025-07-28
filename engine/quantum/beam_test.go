// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quantum

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestNewBeam(t *testing.T) {
	require := require.New(t)

	source := ids.GenerateTestNodeID()
	dest := ids.GenerateTestNodeID()
	wavelength := uint64(100)

	beam := NewBeam(source, dest, wavelength)

	require.NotNil(beam)
	require.NotEqual(ids.Empty, beam.ID)
	require.Equal(source, beam.Source)
	require.Equal(dest, beam.Destination)
	require.Equal(wavelength, beam.Wavelength)
	require.Equal(uint64(1), beam.Frequency)
	require.Equal(uint64(0), beam.Intensity)
	require.Equal(1.0, beam.Coherence)
	require.True(beam.Active)
	require.False(beam.Focused)
	require.False(beam.Terminated)
	require.Empty(beam.Photons)
}

func TestBeamAddPhoton(t *testing.T) {
	require := require.New(t)

	source := ids.GenerateTestNodeID()
	dest := ids.GenerateTestNodeID()
	beam := NewBeam(source, dest, 100)

	// Create photons from same source
	photon1 := NewPhoton(source, []byte("data1"))
	photon1.Energy = 10
	photon2 := NewPhoton(source, []byte("data2"))
	photon2.Energy = 20

	// Add photons
	require.True(beam.AddPhoton(photon1))
	require.True(beam.AddPhoton(photon2))

	require.Len(beam.Photons, 2)
	require.Equal(uint64(30), beam.Intensity)
}

func TestBeamAddPhotonWrongSource(t *testing.T) {
	require := require.New(t)

	source := ids.GenerateTestNodeID()
	otherSource := ids.GenerateTestNodeID()
	dest := ids.GenerateTestNodeID()
	beam := NewBeam(source, dest, 100)

	// Create photon from different source
	photon := NewPhoton(otherSource, []byte("data"))

	// Should reject photon from wrong source
	require.False(beam.AddPhoton(photon))
	require.Empty(beam.Photons)
	require.Equal(uint64(0), beam.Intensity)
}

func TestBeamAddPhotonTerminated(t *testing.T) {
	require := require.New(t)

	source := ids.GenerateTestNodeID()
	dest := ids.GenerateTestNodeID()
	beam := NewBeam(source, dest, 100)

	// Terminate beam
	beam.Terminate()

	// Try to add photon
	photon := NewPhoton(source, []byte("data"))
	require.False(beam.AddPhoton(photon))
	require.Empty(beam.Photons)
}

func TestBeamFocus(t *testing.T) {
	require := require.New(t)

	source := ids.GenerateTestNodeID()
	dest := ids.GenerateTestNodeID()
	wavelength := uint64(100)
	beam := NewBeam(source, dest, wavelength)

	// Add photons until we reach critical mass
	for i := 0; i < 10; i++ {
		photon := NewPhoton(source, []byte("data"))
		photon.Energy = 15
		beam.AddPhoton(photon)
	}

	// Should be focused now (intensity >= wavelength)
	require.True(beam.Focused)
	require.Equal(uint64(150), beam.Intensity)
}

func TestBeamSplit(t *testing.T) {
	require := require.New(t)

	source := ids.GenerateTestNodeID()
	dest1 := ids.GenerateTestNodeID()
	dest2 := ids.GenerateTestNodeID()
	dest3 := ids.GenerateTestNodeID()
	
	beam := NewBeam(source, dest1, 100)

	// Add some photons
	for i := 0; i < 5; i++ {
		photon := NewPhoton(source, []byte("data"))
		photon.Energy = 10
		beam.AddPhoton(photon)
	}

	// Split beam
	destinations := []ids.NodeID{dest2, dest3}
	splitBeams := beam.Split(destinations)

	require.Len(splitBeams, 2)
	
	for i, newBeam := range splitBeams {
		require.Equal(source, newBeam.Source)
		require.Equal(destinations[i], newBeam.Destination)
		require.Equal(beam.Wavelength, newBeam.Wavelength)
		require.Equal(beam.Intensity, newBeam.Intensity)
		require.Equal(beam.Coherence*0.9, newBeam.Coherence) // Coherence loss
		require.Len(newBeam.Photons, 5)
		require.True(newBeam.Active)
	}
}

func TestBeamMerge(t *testing.T) {
	require := require.New(t)

	source1 := ids.GenerateTestNodeID()
	source2 := ids.GenerateTestNodeID()
	dest := ids.GenerateTestNodeID()
	wavelength := uint64(100)

	beam1 := NewBeam(source1, dest, wavelength)
	beam2 := NewBeam(source2, dest, wavelength)

	// Add photons to both beams
	photon1 := NewPhoton(source1, []byte("data1"))
	photon1.Energy = 30
	beam1.AddPhoton(photon1)
	beam1.Coherence = 0.8

	photon2 := NewPhoton(source2, []byte("data2"))
	photon2.Energy = 40
	beam2.AddPhoton(photon2)
	beam2.Coherence = 0.6

	// Merge beams
	require.True(beam1.Merge(beam2))

	require.Len(beam1.Photons, 2)
	require.Equal(uint64(70), beam1.Intensity)
	require.Equal(0.7, beam1.Coherence) // Average of 0.8 and 0.6
}

func TestBeamMergeIncompatible(t *testing.T) {
	require := require.New(t)

	source := ids.GenerateTestNodeID()
	dest1 := ids.GenerateTestNodeID()
	dest2 := ids.GenerateTestNodeID()

	// Different destinations
	beam1 := NewBeam(source, dest1, 100)
	beam2 := NewBeam(source, dest2, 100)
	require.False(beam1.Merge(beam2))

	// Different wavelengths
	beam3 := NewBeam(source, dest1, 100)
	beam4 := NewBeam(source, dest1, 200)
	require.False(beam3.Merge(beam4))

	// Terminated beam
	beam5 := NewBeam(source, dest1, 100)
	beam6 := NewBeam(source, dest1, 100)
	beam5.Terminate()
	require.False(beam5.Merge(beam6))
	require.False(beam6.Merge(beam5))
}

func TestBeamTerminate(t *testing.T) {
	require := require.New(t)

	source := ids.GenerateTestNodeID()
	dest := ids.GenerateTestNodeID()
	beam := NewBeam(source, dest, 100)

	require.True(beam.Active)
	require.False(beam.Terminated)

	beam.Terminate()

	require.False(beam.Active)
	require.True(beam.Terminated)
}

func TestBeamGetCoherentPhotons(t *testing.T) {
	require := require.New(t)

	source := ids.GenerateTestNodeID()
	dest := ids.GenerateTestNodeID()
	beam := NewBeam(source, dest, 100)

	// Add photons
	for i := 0; i < 5; i++ {
		photon := NewPhoton(source, []byte("data"))
		beam.AddPhoton(photon)
	}

	// Test with high threshold (beam coherence is 1.0)
	coherent := beam.GetCoherentPhotons(0.5)
	require.Len(coherent, 5)

	// Test with threshold above beam coherence
	beam.Coherence = 0.3
	coherent = beam.GetCoherentPhotons(0.5)
	require.Empty(coherent)
}

func TestBeamFrequencyCalculation(t *testing.T) {
	require := require.New(t)

	source := ids.GenerateTestNodeID()
	dest := ids.GenerateTestNodeID()
	beam := NewBeam(source, dest, 100)

	// Add photons over time
	time.Sleep(100 * time.Millisecond)
	
	for i := 0; i < 10; i++ {
		photon := NewPhoton(source, []byte("data"))
		beam.AddPhoton(photon)
	}

	// Frequency should be calculated based on photons/second
	require.Greater(beam.Frequency, uint64(0))
}

func BenchmarkBeamAddPhoton(b *testing.B) {
	source := ids.GenerateTestNodeID()
	dest := ids.GenerateTestNodeID()
	beam := NewBeam(source, dest, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		photon := NewPhoton(source, []byte("data"))
		beam.AddPhoton(photon)
	}
}

func BenchmarkBeamSplit(b *testing.B) {
	source := ids.GenerateTestNodeID()
	destinations := make([]ids.NodeID, 10)
	for i := range destinations {
		destinations[i] = ids.GenerateTestNodeID()
	}

	beam := NewBeam(source, destinations[0], 100)
	for i := 0; i < 100; i++ {
		photon := NewPhoton(source, []byte("data"))
		beam.AddPhoton(photon)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = beam.Split(destinations[1:])
	}
}