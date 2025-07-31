// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/core/utils"
	"github.com/luxfi/consensus/protocol/photon"
	"github.com/luxfi/consensus/protocol/pulse"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

var (
	Red   = ids.Empty.Prefix(0)
	Blue  = ids.Empty.Prefix(1)
	Green = ids.Empty.Prefix(2)
)

// Byzantine is a naive implementation of a multi-choice consensus instance
type Byzantine struct {
	// Hardcode the preference
	preference ids.ID
	finalized  bool
	rounds     int
}

func NewByzantine(_ *utils.Factory, _ config.Parameters, choice ids.ID) interfaces.Consensus {
	return &Byzantine{
		preference: choice,
	}
}

func (*Byzantine) Add(ids.ID) error { return nil }

func (b *Byzantine) Preference() ids.ID {
	return b.preference
}

func (b *Byzantine) RecordVotes(bag.Bag[ids.ID]) error {
	b.rounds++
	// Finalize after many rounds to allow network to progress
	if b.rounds > 50 {
		b.finalized = true
	}
	return nil
}

func (*Byzantine) RecordPrism(bag.Bag[ids.ID]) error {
	return nil
}

func (b *Byzantine) Finalized() bool {
	return b.finalized
}

func (b *Byzantine) String() string {
	return b.preference.String()
}

// TestPhotonConsensus tests basic photon consensus
func TestPhotonConsensus(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters

	photonInstance := photon.NewPhoton(params)
	
	// Photon only accepts one choice - let's test with Blue
	require.NoError(photonInstance.Add(Blue))
	require.Equal(Blue, photonInstance.Preference())

	// Create votes with super majority for Blue
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votes.Add(Blue)
	}

	// Should finalize after Beta consecutive successful polls
	for i := 0; i < params.Beta; i++ {
		require.NoError(photonInstance.RecordVotes(votes))
		t.Logf("Round %d: finalized=%v, confidence=%v", i+1, photonInstance.Finalized(), photonInstance)
	}

	require.True(photonInstance.Finalized())
	require.Equal(Blue, photonInstance.Preference())
}

// TestPulseConsensus tests basic pulse consensus
func TestPulseConsensus(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters

	pulseInstance := pulse.NewPulse(params)
	
	// Add choices
	require.NoError(pulseInstance.Add(Red))
	require.NoError(pulseInstance.Add(Blue))
	require.NoError(pulseInstance.Add(Green))

	// Create votes - majority for Red
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		votes.Add(Red)
	}
	for i := 0; i < 2; i++ {
		votes.Add(Blue)
		votes.Add(Green)
	}

	// Record votes until finalized
	for round := 0; round < params.Beta+1 && !pulseInstance.Finalized(); round++ {
		require.NoError(pulseInstance.RecordVotes(votes))
	}

	require.True(pulseInstance.Finalized())
	require.Equal(Red, pulseInstance.Preference())
}

// TestWaveConsensus tests basic wave consensus
func TestWaveConsensus(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters

	waveInstance := wave.NewWave(params)
	
	// Add decisions
	require.NoError(waveInstance.Add(Red))
	require.NoError(waveInstance.Add(Blue))
	require.NoError(waveInstance.Add(Green))

	// Create wave pattern - strong for Blue
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votes.Add(Blue)
	}
	for i := 0; i < 3; i++ {
		votes.Add(Red)
		votes.Add(Green)
	}

	// Wave through rounds
	for round := 0; round < params.Beta+1 && !waveInstance.Finalized(); round++ {
		require.NoError(waveInstance.RecordVotes(votes))
	}

	require.True(waveInstance.Finalized())
	require.Equal(Blue, waveInstance.Preference())
}

// TestByzantineNode tests byzantine behavior
func TestByzantineNode(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	// Create a minimal context for testing
	ctx := &interfaces.Context{
		NetworkID: 1,
		ChainID:   ids.GenerateTestID(),
		NodeID:    ids.GenerateTestNodeID(),
	}
	factory := utils.NewFactory(ctx)

	// Byzantine nodes don't change preference
	byzNode := NewByzantine(factory, params, Red)
	require.Equal(Red, byzNode.Preference())
	require.False(byzNode.Finalized()) // Initially not finalized

	// Try to influence it
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.K; i++ {
		votes.Add(Blue)
	}

	require.NoError(byzNode.RecordVotes(votes))
	require.Equal(Red, byzNode.Preference()) // Still Red
	require.False(byzNode.Finalized()) // Not yet finalized
}

// TestConsensusComparison tests multiple consensus protocols side by side
func TestConsensusComparison(t *testing.T) {
	require := require.New(t)

	params := config.Parameters{
		K:               21,
		AlphaPreference: 14,
		AlphaConfidence: 18,
		Beta:            8,
	}

	// Create different consensus instances
	photonInstance := photon.NewPhoton(params)
	pulseInstance := pulse.NewPulse(params)
	waveInstance := wave.NewWave(params)

	// Photon only accepts one choice, so we'll only test Green for it
	require.NoError(photonInstance.Add(Green))
	
	// Pulse and wave can handle multiple choices
	protocols := []interfaces.Consensus{
		pulseInstance,
		waveInstance,
	}

	// Add same choices to pulse and wave
	for _, protocol := range protocols {
		require.NoError(protocol.Add(Red))
		require.NoError(protocol.Add(Blue))
		require.NoError(protocol.Add(Green))
	}

	// Create consistent voting pattern
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votes.Add(Green)
	}
	for i := 0; i < 2; i++ {
		votes.Add(Red)
		votes.Add(Blue)
	}

	// Run consensus rounds
	maxRounds := params.Beta * 2
	photonFinalized := false
	protocolsFinalized := 0

	for round := 0; round < maxRounds && (!photonFinalized || protocolsFinalized < len(protocols)); round++ {
		// Handle photon separately
		if !photonFinalized && !photonInstance.Finalized() {
			require.NoError(photonInstance.RecordVotes(votes))
			if photonInstance.Finalized() {
				photonFinalized = true
				t.Logf("Photon finalized at round %d with preference %s", round+1, photonInstance.Preference())
			}
		}
		
		// Handle pulse and wave
		for _, protocol := range protocols {
			if !protocol.Finalized() {
				require.NoError(protocol.RecordVotes(votes))
				if protocol.Finalized() {
					protocolsFinalized++
					t.Logf("Protocol finalized at round %d with preference %s", round+1, protocol.Preference())
				}
			}
		}
	}

	// All should finalize
	require.True(photonInstance.Finalized())
	require.Equal(Green, photonInstance.Preference())
	for _, protocol := range protocols {
		require.True(protocol.Finalized())
		require.Equal(Green, protocol.Preference())
	}
}

// TestConsensusLiveness tests that consensus makes progress
func TestConsensusLiveness(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters

	// Create protocols
	photonInstance := photon.NewPhoton(params)
	pulseInstance := pulse.NewPulse(params)
	waveInstance := wave.NewWave(params)
	
	// Photon only accepts one choice
	require.NoError(photonInstance.Add(Red))
	
	// Pulse and wave accept multiple choices
	protocols := map[string]interfaces.Consensus{
		"pulse": pulseInstance,
		"wave":  waveInstance,
	}

	// Add choices to pulse and wave
	for _, protocol := range protocols {
		require.NoError(protocol.Add(Red))
		require.NoError(protocol.Add(Blue))
	}

	// Simulate changing votes over time
	for round := 0; round < 100; round++ {
		votes := bag.Bag[ids.ID]{}
		
		// Gradually shift from Red to Blue
		redVotes := params.K - (round * params.K / 100)
		blueVotes := params.K - redVotes
		
		for i := 0; i < redVotes; i++ {
			votes.Add(Red)
		}
		for i := 0; i < blueVotes; i++ {
			votes.Add(Blue)
		}

		// Handle photon separately (only votes for Red)
		if !photonInstance.Finalized() {
			require.NoError(photonInstance.RecordVotes(votes))
			if photonInstance.Finalized() {
				t.Logf("photon finalized at round %d with preference %s", round+1, photonInstance.Preference())
			}
		}
		
		// Handle pulse and wave
		for name, protocol := range protocols {
			if !protocol.Finalized() {
				require.NoError(protocol.RecordVotes(votes))
				if protocol.Finalized() {
					t.Logf("%s finalized at round %d with preference %s", name, round+1, protocol.Preference())
				}
			}
		}

		// Check if all finalized
		allFinalized := photonInstance.Finalized()
		for _, protocol := range protocols {
			if !protocol.Finalized() {
				allFinalized = false
				break
			}
		}
		if allFinalized {
			break
		}
	}

	// Verify all protocols eventually finalized
	require.True(photonInstance.Finalized(), "photon should have finalized")
	for name, protocol := range protocols {
		require.True(protocol.Finalized(), "%s should have finalized", name)
	}
}