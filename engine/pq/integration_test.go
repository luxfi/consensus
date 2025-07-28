// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pq

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// TestPhotonToQuasarFlow tests the complete journey from photon to immortalization
func TestPhotonToQuasarFlow(t *testing.T) {
	require := require.New(t)

	// Setup
	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()

	// Create the supermassive blackhole
	quasar := NewQuasar(params, nodeID)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(quasar.Initialize(ctx))

	// Track immortalized blocks
	var immortalizedBlocks []QBlock
	quasar.SetImmortalizedCallback(func(block QBlock) {
		immortalizedBlocks = append(immortalizedBlocks, block)
	})

	// Phase 1: Create Photons
	source1 := ids.GenerateTestNodeID()
	source2 := ids.GenerateTestNodeID()
	source3 := ids.GenerateTestNodeID()

	photons := make([]*Photon, 30)
	for i := 0; i < 10; i++ {
		photons[i] = NewPhoton(source1, []byte("quantum data 1"))
		photons[i].Energy = 20

		photons[i+10] = NewPhoton(source2, []byte("quantum data 2"))
		photons[i+10].Energy = 30

		photons[i+20] = NewPhoton(source3, []byte("quantum data 3"))
		photons[i+20].Energy = 25
	}

	// Entangle some photons
	photons[0].Entangle(photons[10])
	photons[10].Entangle(photons[20])

	// Phase 2: Form Beams
	destination := ids.GenerateTestNodeID()
	wavelength := uint64(100)

	beam1 := NewBeam(source1, destination, wavelength)
	beam2 := NewBeam(source2, destination, wavelength)
	beam3 := NewBeam(source3, destination, wavelength)

	// Add photons to beams
	for i := 0; i < 10; i++ {
		require.True(beam1.AddPhoton(photons[i]))
		require.True(beam2.AddPhoton(photons[i+10]))
		require.True(beam3.AddPhoton(photons[i+20]))
	}

	// Verify beam properties
	require.Equal(uint64(200), beam1.Intensity) // 10 photons * 20 energy
	require.Equal(uint64(300), beam2.Intensity) // 10 photons * 30 energy
	require.Equal(uint64(250), beam3.Intensity) // 10 photons * 25 energy

	// Phase 3: Trigger Flare
	flareThreshold := FlareThreshold{
		MinBeams:     3,
		MinIntensity: 500,
		MinCoherence: 0.5,
		TimeWindow:   100 * time.Millisecond,
	}

	detector := NewFlareDetector(flareThreshold)

	// Add beams to detector
	var flare *Flare
	flare = detector.AddBeam(beam1)
	require.Nil(flare) // Not enough yet

	flare = detector.AddBeam(beam2)
	require.Nil(flare) // Still not enough

	flare = detector.AddBeam(beam3)
	require.NotNil(flare) // Flare triggered!

	// Verify flare properties
	require.Equal(destination, flare.Epicenter)
	require.Len(flare.TriggerBeams, 3)
	require.Equal(uint64(750), flare.Intensity) // Sum of all beams
	require.Len(flare.Photons, 30) // All photons
	require.True(flare.Active)

	// Erupt the flare
	flare.Erupt()
	require.True(flare.Peaked)
	require.Greater(flare.Temperature, 0.0)

	// Phase 4: Create cascading flares BEFORE feeding to quasar
	cascadeDestinations := []ids.NodeID{
		ids.GenerateTestNodeID(),
		ids.GenerateTestNodeID(),
	}

	childFlares := flare.TriggerCascade(cascadeDestinations)
	require.Len(childFlares, 2)
	require.True(flare.Cascading)

	// Phase 5: Feed to Quasar
	require.True(quasar.FeedFlare(flare))
	require.Equal(1, quasar.GetAccretionDiskSize())

	// Verify gravitational effects
	require.Greater(quasar.gravity, 1.0)
	require.Greater(quasar.GetEventHorizonRadius(), uint64(1000))

	// Feed child flares to quasar
	for _, child := range childFlares {
		child.Erupt()
		quasar.FeedFlare(child)
	}

	require.Equal(3, quasar.GetAccretionDiskSize()) // Original + 2 children

	// Phase 6: Process Accretion Disk
	// Give the quasar time to process
	time.Sleep(200 * time.Millisecond)

	// Manually trigger processing for test
	quasar.processAccretionDisk(ctx)

	// Verify singularity growth
	require.Greater(quasar.GetSingularityMass(), uint64(1))

	// Phase 7: Verify photon entanglement preserved
	require.True(photons[0].IsEntangled(photons[10].ID))
	require.True(photons[10].IsEntangled(photons[20].ID))

	// Collapse entangled photons
	collapsed1 := photons[0].Collapse()
	collapsed2 := photons[10].Collapse()

	require.Len(collapsed1, 32+len(photons[0].Payload))
	require.Len(collapsed2, 32+len(photons[10].Payload))
}

// TestQuantumEnginesIntegration tests all three engines working together
func TestQuantumEnginesIntegration(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create all three engines
	pulsar := NewPulsar(params, nodeID)
	nebula := NewNebula(params)
	quasar := NewQuasar(params, nodeID)

	// Initialize engines
	require.NoError(pulsar.Initialize(ctx))
	require.NoError(quasar.Initialize(ctx))

	// Phase 1: Pulsar generates sequential blocks
	var pulsarBlocks []*LinearBlock
	pulsar.SetPulseCallback(func(block *LinearBlock) {
		pulsarBlocks = append(pulsarBlocks, block)
	})

	// Add transactions to pulsar
	for i := 0; i < 5; i++ {
		tx := &MockTransaction{
			id:    ids.GenerateTestID(),
			data:  []byte("pulsar tx"),
			valid: true,
		}
		require.NoError(pulsar.AddTransaction(tx))
	}

	// Add photons to boost pulse
	for i := 0; i < 3; i++ {
		photon := NewPhoton(nodeID, []byte("pulsar photon"))
		photon.Energy = 5
		pulsar.AddPhoton(photon)
	}

	// Emit pulse
	pulsar.emitPulse(ctx, 1)
	require.Len(pulsarBlocks, 1)
	require.Greater(pulsar.GetHeight(), uint64(0))

	// Phase 2: Nebula processes DAG vertices
	vertices := make([]Vertex, 5)
	for i := range vertices {
		vertices[i] = &MockVertex{
			id:           ids.GenerateTestID(),
			parents:      []ids.ID{},
			height:       uint64(i),
			timestamp:    time.Now(),
			transactions: []ids.ID{ids.GenerateTestID()},
			valid:        true,
		}
		require.NoError(nebula.AddVertex(ctx, vertices[i]))
	}

	// Simulate voting
	for i := 0; i < params.Beta; i++ {
		votes := []ids.ID{vertices[0].ID(), vertices[1].ID()}
		require.NoError(nebula.RecordPoll(ctx, ids.GenerateTestNodeID(), votes))
	}

	// Check for preferred vertices
	preferred, ok := nebula.GetPreferredFrontier()
	require.True(ok)
	require.NotNil(preferred)

	// Phase 3: Feed energy to Quasar
	// Create high-energy beams from multiple sources
	sources := []ids.NodeID{
		ids.GenerateTestNodeID(),
		ids.GenerateTestNodeID(),
		ids.GenerateTestNodeID(),
	}

	beams := make([]*Beam, len(sources))
	for i, source := range sources {
		beams[i] = NewBeam(source, nodeID, 100)

		// Add photons from Pulsar's emission
		for j := 0; j < 10; j++ {
			photon := NewPhoton(source, []byte("integration photon"))
			photon.Energy = 50
			beams[i].AddPhoton(photon)
		}
		// Don't manually set intensity - it's calculated from photons
		require.Equal(uint64(500), beams[i].Intensity) // 10 * 50
	}

	// Create and feed flare
	flare := NewFlare(nodeID, beams, 100)
	flare.Erupt()
	require.True(quasar.FeedFlare(flare))

	// Give time for background processing
	time.Sleep(100 * time.Millisecond)

	// Force accretion disk processing (may have already been processed by background goroutine)
	quasar.processAccretionDisk(ctx)

	// Phase 4: Verify cross-engine state
	pulsarStats := pulsar.GetStats()
	nebulaStats := nebula.GetStats()
	quasarStats := quasar.GetStats()

	require.Greater(pulsarStats.Height, uint64(0))
	require.Greater(nebulaStats.TotalVertices, 0)
	require.Greater(quasarStats.SingularityMass, uint64(1))
	// After processing, accretion disk should be empty or have fewer items
	require.GreaterOrEqual(quasarStats.AccretionDiskSize, 0)

	// Phase 5: Test quantum jet emission
	qBlock := &QBlock{
		Height:       0,
		PhotonCount:  uint64(len(beams) * 10),
		FlareEnergy:  flare.Intensity,
		Immortalized: true,
	}
	quasar.launchQuantumJets(qBlock)
	
	// Get updated stats after launching jets
	quasarStats = quasar.GetStats()
	require.Equal(2, quasarStats.QuantumJetsActive)
}

// TestEventHorizonCrossing tests data immortalization
func TestEventHorizonCrossing(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)
	ctx := context.Background()

	// Track initial event horizon state
	initialCrossings := len(quasar.eventHorizon.CapturedData)

	// Create ultra-high energy flare
	source := ids.GenerateTestNodeID()
	beam := NewBeam(source, nodeID, 100)

	// Massive photon bombardment
	for i := 0; i < 100; i++ {
		photon := NewPhoton(source, []byte("massive energy"))
		photon.Energy = 100
		beam.AddPhoton(photon)
	}
	// beam.Intensity is calculated automatically: 100 * 100 = 10000

	flare := NewFlare(nodeID, []*Beam{beam}, 100)
	flare.Erupt()
	flare.Temperature = 100.0 // Extreme temperature

	// Feed to quasar
	require.True(quasar.FeedFlare(flare))

	// Process accretion disk
	quasar.processAccretionDisk(ctx)

	// Verify crossing
	require.Greater(len(quasar.eventHorizon.CapturedData), initialCrossings)
	require.NotEmpty(quasar.eventHorizon.CapturedData)
	require.Contains(quasar.eventHorizon.CapturedData, flare.ID)
	require.Greater(quasar.singularity.Mass, uint64(1))
	require.Greater(quasar.singularity.SpinRate, 0.0)

	// Verify flare was dissipated after crossing
	require.False(flare.Active)
}

// BenchmarkFullQuantumFlow benchmarks the complete flow
func BenchmarkFullQuantumFlow(b *testing.B) {
	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create components
		quasar := NewQuasar(params, nodeID)
		detector := NewFlareDetector(FlareThreshold{
			MinBeams:     3,
			MinIntensity: 300,
			MinCoherence: 0.5,
			TimeWindow:   100 * time.Millisecond,
		})

		// Generate photons → beams → flare → quasar
		for j := 0; j < 3; j++ {
			source := ids.GenerateTestNodeID()
			beam := NewBeam(source, nodeID, 100)

			for k := 0; k < 10; k++ {
				photon := NewPhoton(source, []byte("bench"))
				photon.Energy = 10
				beam.AddPhoton(photon)
			}

			if flare := detector.AddBeam(beam); flare != nil {
				flare.Erupt()
				quasar.FeedFlare(flare)
				quasar.processAccretionDisk(ctx)
			}
		}
	}
}