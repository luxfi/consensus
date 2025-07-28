// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quantum

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestNewQuasar(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()

	quasar := NewQuasar(params, nodeID)

	require.NotNil(quasar)
	require.Equal(params, quasar.params)
	require.Equal(nodeID, quasar.nodeID)
	require.NotNil(quasar.nebula)
	require.NotNil(quasar.ringtail)
	require.NotNil(quasar.eventHorizon)
	require.NotNil(quasar.singularity)
	require.Empty(quasar.accretionDisk)
	require.Empty(quasar.jets)
	require.Empty(quasar.qBlocks)
	require.Nil(quasar.lastQBlock)
	require.Equal(uint64(100), quasar.schwarzschildRadius)
	require.Equal(1.0, quasar.gravity)
	require.Equal(uint64(1000), quasar.eventHorizon.Radius)
	require.Equal(uint64(1), quasar.singularity.Mass)
	require.Equal(1.0, quasar.singularity.Density)
	require.Equal(1.0, quasar.singularity.Temperature)
}

func TestQuasarFeedFlare(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)

	// Create a flare with enough energy
	source := ids.GenerateTestNodeID()
	epicenter := ids.GenerateTestNodeID()
	beam := NewBeam(source, epicenter, 100)
	beam.Intensity = 200 // Above schwarzschild radius

	flare := NewFlare(epicenter, []*Beam{beam}, 100)
	flare.Erupt() // Must be peaked to feed quasar

	// Feed flare to quasar
	require.True(quasar.FeedFlare(flare))
	require.Len(quasar.accretionDisk, 1)
	require.True(flare.QuasarFeed)

	// Verify gravity and event horizon increased
	require.Greater(quasar.gravity, 1.0)
	require.Greater(quasar.eventHorizon.Radius, uint64(1000))
}

func TestQuasarFeedFlareInsufficientEnergy(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)

	// Create a low-energy flare
	source := ids.GenerateTestNodeID()
	epicenter := ids.GenerateTestNodeID()
	beam := NewBeam(source, epicenter, 100)
	beam.Intensity = 50 // Below schwarzschild radius

	flare := NewFlare(epicenter, []*Beam{beam}, 100)
	flare.Erupt()

	// Should reject low-energy flare
	require.False(quasar.FeedFlare(flare))
	require.Empty(quasar.accretionDisk)
	require.False(flare.QuasarFeed)
}

func TestQuasarProcessAccretionDisk(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)
	ctx := context.Background()

	// Create high-energy flares
	for i := 0; i < 3; i++ {
		source := ids.GenerateTestNodeID()
		epicenter := ids.GenerateTestNodeID()
		beam := NewBeam(source, epicenter, 100)
		beam.Intensity = 2000 // High intensity
		
		// Add photons
		for j := 0; j < 5; j++ {
			photon := NewPhoton(source, []byte("data"))
			beam.AddPhoton(photon)
		}

		flare := NewFlare(epicenter, []*Beam{beam}, 100)
		flare.Erupt()
		flare.Temperature = 10.0 // High temperature
		
		quasar.FeedFlare(flare)
	}

	require.Len(quasar.accretionDisk, 3)
	initialMass := quasar.singularity.Mass

	// Process accretion disk
	quasar.processAccretionDisk(ctx)

	// Some flares should have been absorbed
	require.Greater(quasar.singularity.Mass, initialMass)
	require.NotEmpty(quasar.eventHorizon.CapturedData)
	require.Greater(quasar.singularity.SpinRate, 0.0)
}

func TestQuasarAddVertex(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)
	ctx := context.Background()

	vertex := &MockVertex{
		id:    ids.GenerateTestID(),
		valid: true,
	}

	// Should delegate to nebula
	require.NoError(quasar.AddVertex(ctx, vertex))
}

func TestQuasarRecordPoll(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)
	ctx := context.Background()

	// Add a vertex first
	vertex := &MockVertex{
		id:    ids.GenerateTestID(),
		valid: true,
	}
	require.NoError(quasar.AddVertex(ctx, vertex))

	// Record poll
	votes := []ids.ID{vertex.ID()}
	require.NoError(quasar.RecordPoll(ctx, nodeID, votes))
}

func TestQuasarEventHorizonGrowth(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)

	initialRadius := quasar.eventHorizon.Radius

	// Feed multiple high-energy flares
	for i := 0; i < 10; i++ {
		source := ids.GenerateTestNodeID()
		epicenter := ids.GenerateTestNodeID()
		beam := NewBeam(source, epicenter, 100)
		beam.Intensity = 500

		flare := NewFlare(epicenter, []*Beam{beam}, 100)
		flare.Erupt()
		
		quasar.FeedFlare(flare)
	}

	// Event horizon should have grown
	require.Greater(quasar.eventHorizon.Radius, initialRadius)
	require.Greater(quasar.gravity, 1.0)
}

func TestQuasarQuantumJets(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)

	// Create a Q-block
	qBlock := &QBlock{
		Height:       0,
		PhotonCount:  100,
		FlareEnergy:  1000,
		Immortalized: true,
	}

	// Launch quantum jets
	quasar.launchQuantumJets(qBlock)

	// Should create bipolar jets
	require.Len(quasar.jets, 2)
	
	for _, jet := range quasar.jets {
		require.NotEqual(ids.Empty, jet.ID)
		require.Equal(quasar.singularity.Mass/100, jet.Energy)
		require.NotZero(jet.LaunchTime)
	}
}

func TestQuasarGetters(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)

	// Test getters
	require.Equal(quasar.eventHorizon.Radius, quasar.GetEventHorizonRadius())
	require.Equal(quasar.singularity.Mass, quasar.GetSingularityMass())
	require.Equal(len(quasar.accretionDisk), quasar.GetAccretionDiskSize())

	// Modify state and verify
	quasar.singularity.Mass = 5000
	require.Equal(uint64(5000), quasar.GetSingularityMass())

	// Add flares to accretion disk
	source := ids.GenerateTestNodeID()
	beam := NewBeam(source, nodeID, 100)
	flare := NewFlare(nodeID, []*Beam{beam}, 100)
	quasar.accretionDisk = append(quasar.accretionDisk, flare)
	require.Equal(1, quasar.GetAccretionDiskSize())
}

func TestQuasarStats(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)

	// Set up some state
	quasar.singularity.Mass = 10000
	quasar.eventHorizon.Radius = 2000
	quasar.gravity = 2.5

	// Add Q-blocks
	qBlock1 := &QBlock{Height: 0, PhotonCount: 50}
	qBlock2 := &QBlock{Height: 1, PhotonCount: 100}
	quasar.qBlocks = []*QBlock{qBlock1, qBlock2}
	quasar.lastQBlock = qBlock2

	// Add flares
	for i := 0; i < 3; i++ {
		source := ids.GenerateTestNodeID()
		beam := NewBeam(source, nodeID, 100)
		flare := NewFlare(nodeID, []*Beam{beam}, 100)
		quasar.accretionDisk = append(quasar.accretionDisk, flare)
	}

	// Add jets
	jet := &QuantumJet{ID: ids.GenerateTestID()}
	quasar.jets = append(quasar.jets, jet)

	stats := quasar.GetStats()

	require.Equal(uint64(1), stats.QBlockHeight)
	require.Equal(2, stats.QBlockCount)
	require.Equal(uint64(2000), stats.EventHorizonRadius)
	require.Equal(uint64(10000), stats.SingularityMass)
	require.Equal(3, stats.AccretionDiskSize)
	require.Equal(1, stats.QuantumJetsActive)
	require.Equal(uint64(100), stats.TotalPhotonsAbsorbed)
	require.Equal(2.5, stats.GravitationalField)
}

func TestQuasarQBlockComputation(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)

	qBlock := &QBlock{
		Height:         0,
		ProposalDigest: [32]byte{1, 2, 3},
		CommitDigest:   [32]byte{4, 5, 6},
		EventHorizon:   [32]byte{7, 8, 9},
		PhotonCount:    100,
		FlareEnergy:    1000,
	}

	// Compute Q-block ID
	id := quasar.computeQBlockID(qBlock)
	require.NotEqual(ids.Empty, id)

	// Same block should produce same ID
	id2 := quasar.computeQBlockID(qBlock)
	require.Equal(id, id2)

	// Different block should produce different ID
	qBlock2 := &QBlock{
		Height:      1,
		PhotonCount: 200,
		FlareEnergy: 2000,
	}
	id3 := quasar.computeQBlockID(qBlock2)
	require.NotEqual(id, id3)
}

func TestQuasarCallbacks(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)

	var immortalizedBlock *QBlock
	quasar.SetImmortalizedCallback(func(block QBlock) {
		immortalizedBlock = &block
	})

	// Simulate block immortalization
	testBlock := QBlock{
		Height:       1,
		Immortalized: true,
	}
	
	if quasar.onImmortalized != nil {
		quasar.onImmortalized(testBlock)
	}

	require.NotNil(immortalizedBlock)
	require.Equal(uint64(1), immortalizedBlock.Height)
	require.True(immortalizedBlock.Immortalized)
}

func TestQuasarIntegration(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize quasar
	require.NoError(quasar.Initialize(ctx))

	// Create and feed high-energy flare
	source := ids.GenerateTestNodeID()
	epicenter := ids.GenerateTestNodeID()
	
	// Create multiple beams
	beams := make([]*Beam, 5)
	for i := range beams {
		beams[i] = NewBeam(source, epicenter, 100)
		beams[i].Intensity = 500
		
		// Add photons
		for j := 0; j < 10; j++ {
			photon := NewPhoton(source, []byte("quantum data"))
			photon.Energy = 10
			beams[i].AddPhoton(photon)
		}
	}

	// Create high-energy flare
	flare := NewFlare(epicenter, beams, 100)
	flare.Erupt()
	
	// Feed to quasar
	require.True(quasar.FeedFlare(flare))

	// Give time for processing
	time.Sleep(100 * time.Millisecond)

	// Verify state changes
	require.Greater(quasar.GetSingularityMass(), uint64(1))
	require.Greater(quasar.GetEventHorizonRadius(), uint64(1000))
}

func BenchmarkQuasarFeedFlare(b *testing.B) {
	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)

	source := ids.GenerateTestNodeID()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		beam := NewBeam(source, nodeID, 100)
		beam.Intensity = 200
		flare := NewFlare(nodeID, []*Beam{beam}, 100)
		flare.Erupt()
		quasar.FeedFlare(flare)
	}
}

func BenchmarkQuasarProcessAccretionDisk(b *testing.B) {
	params := config.DefaultParameters
	nodeID := ids.GenerateTestNodeID()
	quasar := NewQuasar(params, nodeID)
	ctx := context.Background()

	// Pre-populate accretion disk
	for i := 0; i < 100; i++ {
		source := ids.GenerateTestNodeID()
		beam := NewBeam(source, nodeID, 100)
		beam.Intensity = 1000
		flare := NewFlare(nodeID, []*Beam{beam}, 100)
		flare.Erupt()
		quasar.accretionDisk = append(quasar.accretionDisk, flare)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		quasar.processAccretionDisk(ctx)
	}
}