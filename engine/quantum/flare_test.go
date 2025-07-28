// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quantum

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestNewFlare(t *testing.T) {
	require := require.New(t)

	epicenter := ids.GenerateTestNodeID()
	source := ids.GenerateTestNodeID()
	height := uint64(1000)

	// Create beams
	beams := make([]*Beam, 3)
	for i := range beams {
		beams[i] = NewBeam(source, epicenter, 100)
		// Add photons to each beam
		for j := 0; j < 5; j++ {
			photon := NewPhoton(source, []byte("data"))
			photon.Energy = 10
			beams[i].AddPhoton(photon)
		}
		beams[i].Intensity = 50
	}

	flare := NewFlare(epicenter, beams, height)

	require.NotNil(flare)
	require.NotEqual(ids.Empty, flare.ID)
	require.Equal(epicenter, flare.Epicenter)
	require.Len(flare.TriggerBeams, 3)
	require.Len(flare.Photons, 15) // 5 photons * 3 beams
	require.Equal(height, flare.Height)
	require.Equal(uint64(150), flare.Intensity) // 50 * 3 beams
	require.Equal(1.0, flare.Temperature)
	require.True(flare.Active)
	require.False(flare.Peaked)
	require.False(flare.Cascading)
	require.Empty(flare.AffectedNodes)
	require.Empty(flare.ChildFlares)
}

func TestFlareMagnitude(t *testing.T) {
	require := require.New(t)

	epicenter := ids.GenerateTestNodeID()
	source := ids.GenerateTestNodeID()

	testCases := []struct {
		intensity uint64
		expected  uint64
	}{
		{1, 1},
		{10, 2},
		{100, 3},
		{1000, 4},
		{10000, 5},
		{100000, 6},
		{1000000, 7},
		{10000000, 8},
		{100000000, 9},
		{1000000000, 10},
		{10000000000, 10}, // Capped at 10
	}

	for _, tc := range testCases {
		beam := NewBeam(source, epicenter, 100)
		beam.Intensity = tc.intensity
		flare := NewFlare(epicenter, []*Beam{beam}, 100)
		require.Equal(tc.expected, flare.Magnitude, "Intensity %d should give magnitude %d", tc.intensity, tc.expected)
	}
}

func TestFlareErupt(t *testing.T) {
	require := require.New(t)

	epicenter := ids.GenerateTestNodeID()
	source := ids.GenerateTestNodeID()
	beam := NewBeam(source, epicenter, 100)
	beam.Intensity = 1000

	flare := NewFlare(epicenter, []*Beam{beam}, 100)

	require.False(flare.Peaked)
	require.Zero(flare.PeakTime)

	flare.Erupt()

	require.True(flare.Peaked)
	require.NotZero(flare.PeakTime)
	require.InDelta(10.0, flare.Temperature, 0.1) // intensity/height = 1000/100
}

func TestFlareAddAffectedNode(t *testing.T) {
	require := require.New(t)

	epicenter := ids.GenerateTestNodeID()
	source := ids.GenerateTestNodeID()
	beam := NewBeam(source, epicenter, 100)
	flare := NewFlare(epicenter, []*Beam{beam}, 100)

	node1 := ids.GenerateTestNodeID()
	node2 := ids.GenerateTestNodeID()

	// Add nodes
	flare.AddAffectedNode(node1)
	flare.AddAffectedNode(node2)

	require.Len(flare.AffectedNodes, 2)
	require.Contains(flare.AffectedNodes, node1)
	require.Contains(flare.AffectedNodes, node2)

	// Try to add duplicate
	flare.AddAffectedNode(node1)
	require.Len(flare.AffectedNodes, 2) // Should still be 2
}

func TestFlareTriggerCascade(t *testing.T) {
	require := require.New(t)

	epicenter := ids.GenerateTestNodeID()
	source := ids.GenerateTestNodeID()
	
	// Create parent flare with high intensity
	beam := NewBeam(source, epicenter, 100)
	beam.Intensity = 1000
	// Add photons
	for i := 0; i < 10; i++ {
		photon := NewPhoton(source, []byte("data"))
		beam.AddPhoton(photon)
	}
	
	flare := NewFlare(epicenter, []*Beam{beam}, 100)
	flare.Erupt() // Must be peaked to cascade

	// Create cascade destinations
	destinations := []ids.NodeID{
		ids.GenerateTestNodeID(),
		ids.GenerateTestNodeID(),
		ids.GenerateTestNodeID(),
	}

	childFlares := flare.TriggerCascade(destinations)

	require.Len(childFlares, 3)
	require.True(flare.Cascading)
	require.Len(flare.ChildFlares, 3)

	for i, child := range childFlares {
		require.Equal(destinations[i], child.Epicenter)
		require.Equal(flare.Height, child.Height)
		require.Equal(flare.Intensity/3, child.Intensity) // Energy divided
		require.Equal(flare.Temperature*0.8, child.Temperature) // Energy dissipation
		require.Equal(flare.Magnitude-1, child.Magnitude) // Reduced magnitude
		require.True(child.Active)
		require.False(child.Peaked)
		require.Len(child.Photons, 10) // Shared photon reference
	}
}

func TestFlareTriggerCascadeNotPeaked(t *testing.T) {
	require := require.New(t)

	epicenter := ids.GenerateTestNodeID()
	source := ids.GenerateTestNodeID()
	beam := NewBeam(source, epicenter, 100)
	flare := NewFlare(epicenter, []*Beam{beam}, 100)

	// Try to cascade without erupting
	destinations := []ids.NodeID{ids.GenerateTestNodeID()}
	childFlares := flare.TriggerCascade(destinations)

	require.Nil(childFlares)
	require.False(flare.Cascading)
}

func TestFlareDissipate(t *testing.T) {
	require := require.New(t)

	epicenter := ids.GenerateTestNodeID()
	source := ids.GenerateTestNodeID()
	beam := NewBeam(source, epicenter, 100)
	flare := NewFlare(epicenter, []*Beam{beam}, 100)

	require.True(flare.Active)
	require.Zero(flare.EndTime)
	require.Zero(flare.Duration)

	time.Sleep(10 * time.Millisecond)
	flare.Dissipate()

	require.False(flare.Active)
	require.NotZero(flare.EndTime)
	require.Greater(flare.Duration, time.Duration(0))
	require.Equal(0.0, flare.Temperature)
}

func TestFlareCanFeedQuasar(t *testing.T) {
	require := require.New(t)

	epicenter := ids.GenerateTestNodeID()
	source := ids.GenerateTestNodeID()
	beam := NewBeam(source, epicenter, 100)
	beam.Intensity = 1000
	flare := NewFlare(epicenter, []*Beam{beam}, 100)

	// Not peaked yet
	require.False(flare.CanFeedQuasar(500))

	// Peak the flare
	flare.Erupt()

	// Below threshold
	require.False(flare.CanFeedQuasar(2000))

	// Above threshold
	require.True(flare.CanFeedQuasar(500))

	// Feed to quasar
	require.True(flare.FeedQuasar())
	require.True(flare.QuasarFeed)

	// Can't feed twice
	require.False(flare.CanFeedQuasar(500))
	require.False(flare.FeedQuasar())
}

func TestFlareGetEnergyDensity(t *testing.T) {
	require := require.New(t)

	epicenter := ids.GenerateTestNodeID()
	source := ids.GenerateTestNodeID()
	beam := NewBeam(source, epicenter, 100)
	beam.Intensity = 1000
	flare := NewFlare(epicenter, []*Beam{beam}, 100)

	// Active flare has energy
	density := flare.GetEnergyDensity()
	require.Greater(density, 0.0)

	// Dissipated flare has no energy
	flare.Dissipate()
	density = flare.GetEnergyDensity()
	require.Equal(0.0, density)
}

func TestFlareDetector(t *testing.T) {
	require := require.New(t)

	threshold := FlareThreshold{
		MinBeams:     3,
		MinIntensity: 100,
		MinCoherence: 0.5,
		TimeWindow:   100 * time.Millisecond,
	}

	detector := NewFlareDetector(threshold)
	require.NotNil(detector)

	source := ids.GenerateTestNodeID()
	dest := ids.GenerateTestNodeID()

	// Add beams - they need to be created at slightly different times
	beam1 := NewBeam(source, dest, 100)
	beam1.Intensity = 40
	beam1.Coherence = 0.8
	beam1.CreatedAt = time.Now()
	flare := detector.AddBeam(beam1)
	require.Nil(flare)

	time.Sleep(10 * time.Millisecond)
	
	beam2 := NewBeam(source, dest, 100)
	beam2.Intensity = 40
	beam2.Coherence = 0.8
	beam2.CreatedAt = time.Now()
	flare = detector.AddBeam(beam2)
	require.Nil(flare)

	time.Sleep(10 * time.Millisecond)
	
	// Third beam triggers flare
	beam3 := NewBeam(source, dest, 100)
	beam3.Intensity = 40
	beam3.Coherence = 0.8
	beam3.CreatedAt = time.Now()
	flare = detector.AddBeam(beam3)
	
	// Flare should be triggered since we have 3 beams with total intensity 120 > 100
	require.NotNil(flare)
	require.Equal(dest, flare.Epicenter)
	require.Len(flare.TriggerBeams, 3)
	require.Equal(uint64(120), flare.Intensity)
}

func TestFlareDetectorTimeWindow(t *testing.T) {
	require := require.New(t)

	threshold := FlareThreshold{
		MinBeams:     2,
		MinIntensity: 50,
		MinCoherence: 0.5,
		TimeWindow:   50 * time.Millisecond,
	}

	detector := NewFlareDetector(threshold)
	source := ids.GenerateTestNodeID()
	dest := ids.GenerateTestNodeID()

	// Add first beam
	beam1 := NewBeam(source, dest, 100)
	beam1.Intensity = 30
	beam1.Coherence = 0.8
	detector.AddBeam(beam1)

	// Wait beyond time window
	time.Sleep(100 * time.Millisecond)

	// Add second beam - should not trigger flare
	beam2 := NewBeam(source, dest, 100)
	beam2.Intensity = 30
	beam2.Coherence = 0.8
	flare := detector.AddBeam(beam2)

	require.Nil(flare) // Beams too far apart in time
}

func BenchmarkNewFlare(b *testing.B) {
	epicenter := ids.GenerateTestNodeID()
	source := ids.GenerateTestNodeID()
	
	beams := make([]*Beam, 10)
	for i := range beams {
		beams[i] = NewBeam(source, epicenter, 100)
		beams[i].Intensity = 100
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewFlare(epicenter, beams, 1000)
	}
}

func BenchmarkFlareDetector(b *testing.B) {
	threshold := FlareThreshold{
		MinBeams:     5,
		MinIntensity: 500,
		MinCoherence: 0.7,
		TimeWindow:   100 * time.Millisecond,
	}
	detector := NewFlareDetector(threshold)
	
	source := ids.GenerateTestNodeID()
	dest := ids.GenerateTestNodeID()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		beam := NewBeam(source, dest, 100)
		beam.Intensity = 100
		beam.Coherence = 0.8
		detector.AddBeam(beam)
	}
}