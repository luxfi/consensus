// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	. "github.com/luxfi/consensus"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/photon"
	"github.com/luxfi/consensus/protocol/pulse"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/luxfi/consensus/utils/bag"
	"github.com/luxfi/ids"
)

// TestErrorAddAfterFinalized tests adding choices after finalization
func TestErrorAddAfterFinalized(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	// Test each protocol
	testProtocols := []struct {
		name    string
		factory func() interface {
			Add(ids.ID) error
			RecordVotes(bag.Bag[ids.ID]) error
			Finalized() bool
		}
	}{
		{
			name: "Photon",
			factory: func() interface {
				Add(ids.ID) error
				RecordVotes(bag.Bag[ids.ID]) error
				Finalized() bool
			} {
				return photon.NewPhoton(params)
			},
		},
		{
			name: "Pulse",
			factory: func() interface {
				Add(ids.ID) error
				RecordVotes(bag.Bag[ids.ID]) error
				Finalized() bool
			} {
				return pulse.NewPulse(params)
			},
		},
		{
			name: "Wave",
			factory: func() interface {
				Add(ids.ID) error
				RecordVotes(bag.Bag[ids.ID]) error
				Finalized() bool
			} {
				return wave.NewWave(params)
			},
		},
	}
	
	for _, tt := range testProtocols {
		t.Run(tt.name, func(t *testing.T) {
			consensus := tt.factory()
			
			// Add initial choice
			require.NoError(consensus.Add(Red))
			
			// Finalize
			votes := bag.Bag[ids.ID]{}
			for i := 0; i < params.AlphaConfidence; i++ {
				votes.Add(Red)
			}
			
			for i := 0; i < params.Beta; i++ {
				require.NoError(consensus.RecordVotes(votes))
			}
			
			require.True(consensus.Finalized())
			
			// Should not be able to add after finalized
			err := consensus.Add(Blue)
			require.Error(err)
		})
	}
}

// TestErrorVoteAfterFinalized tests voting after finalization
func TestErrorVoteAfterFinalized(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	p := pulse.NewPulse(params)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Blue))
	
	// Finalize on Red
	redVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		redVotes.Add(Red)
	}
	
	for i := 0; i < params.Beta; i++ {
		require.NoError(p.RecordVotes(redVotes))
	}
	
	require.True(p.Finalized())
	
	// Should still accept votes after finalized (no-op)
	blueVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		blueVotes.Add(Blue)
	}
	
	require.NoError(p.RecordVotes(blueVotes))
	require.True(p.Finalized())
	require.Equal(Red, p.Preference()) // Should not change
}

// TestErrorInvalidParameters tests invalid parameter configurations
func TestErrorInvalidParameters(t *testing.T) {
	require := require.New(t)

	// Test various invalid parameter combinations
	invalidParams := []struct {
		name   string
		params config.Parameters
		valid  bool
	}{
		{
			name: "Zero K",
			params: config.Parameters{
				K:               0,
				AlphaPreference: 1,
				AlphaConfidence: 1,
				Beta:            1,
			},
			valid: false,
		},
		{
			name: "AlphaPreference > K",
			params: config.Parameters{
				K:               5,
				AlphaPreference: 6,
				AlphaConfidence: 4,
				Beta:            2,
			},
			valid: false,
		},
		{
			name: "AlphaConfidence > K",
			params: config.Parameters{
				K:               5,
				AlphaPreference: 3,
				AlphaConfidence: 6,
				Beta:            2,
			},
			valid: false,
		},
		{
			name: "Zero Beta",
			params: config.Parameters{
				K:               5,
				AlphaPreference: 3,
				AlphaConfidence: 4,
				Beta:            0,
			},
			valid: false,
		},
		{
			name: "Valid params",
			params: config.Parameters{
				K:               5,
				AlphaPreference: 3,
				AlphaConfidence: 4,
				Beta:            2,
			},
			valid: true,
		},
	}
	
	for _, tt := range invalidParams {
		t.Run(tt.name, func(t *testing.T) {
			// Validate parameters
			err := validateParameters(tt.params)
			if tt.valid {
				require.NoError(err)
			} else {
				require.Error(err)
			}
		})
	}
}

// validateParameters checks if parameters are valid
func validateParameters(p config.Parameters) error {
	if p.K <= 0 {
		return errors.New("K must be positive")
	}
	if p.AlphaPreference > p.K {
		return errors.New("AlphaPreference cannot exceed K")
	}
	if p.AlphaConfidence > p.K {
		return errors.New("AlphaConfidence cannot exceed K")
	}
	if p.Beta <= 0 {
		return errors.New("Beta must be positive")
	}
	return nil
}

// TestErrorEmptyBag tests handling of empty vote bags
func TestErrorEmptyBag(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	w := wave.NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))
	require.NoError(w.Add(Green))
	
	// Empty bag should not crash or progress
	emptyBag := bag.Bag[ids.ID]{}
	
	initialPref := w.Preference()
	require.NoError(w.RecordVotes(emptyBag))
	require.Equal(initialPref, w.Preference())
	require.False(w.Finalized())
}

// TestErrorNilHandling tests nil parameter handling
func TestErrorNilHandling(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	// Create instances
	p := photon.NewPhoton(params)
	require.NotNil(p)
	
	pu := pulse.NewPulse(params)
	require.NotNil(pu)
	
	w := wave.NewWave(params)
	require.NotNil(w)
}

// TestErrorDuplicateVotes tests handling of duplicate votes in bag
func TestErrorDuplicateVotes(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	p := pulse.NewPulse(params)
	require.NoError(p.Add(Red))
	require.NoError(p.Add(Blue))
	
	// Create bag with many duplicate votes
	dupVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence*2; i++ {
		dupVotes.Add(Blue)
	}
	
	// Should handle gracefully
	require.NoError(p.RecordVotes(dupVotes))
	require.Equal(Blue, p.Preference())
	
	// With TestParameters Beta=1, should be finalized
	require.True(p.Finalized())
	require.Equal(Blue, p.Preference())
}

// TestErrorMixedVoteBag tests bags with mixed valid/invalid votes
func TestErrorMixedVoteBag(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	w := wave.NewWave(params)
	require.NoError(w.Add(Red))
	require.NoError(w.Add(Blue))
	// Don't add Green
	
	// Bag with mix of valid and invalid choices
	mixedVotes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaPreference; i++ {
		mixedVotes.Add(Blue)
		mixedVotes.Add(Green) // Not added to consensus
	}
	
	require.NoError(w.RecordVotes(mixedVotes))
	require.Equal(Blue, w.Preference()) // Should only count Blue votes
}

// TestErrorRecordUnsuccessfulPollOnFinalized tests unsuccessful poll after finalized
func TestErrorRecordUnsuccessfulPollOnFinalized(t *testing.T) {
	require := require.New(t)

	params := config.TestParameters
	
	p := photon.NewPhoton(params)
	require.NoError(p.Add(Red))
	
	// Finalize
	votes := bag.Bag[ids.ID]{}
	for i := 0; i < params.AlphaConfidence; i++ {
		votes.Add(Red)
	}
	
	for i := 0; i < params.Beta; i++ {
		require.NoError(p.RecordVotes(votes))
	}
	
	require.True(p.Finalized())
	
	// Record unsuccessful poll after finalized
	p.RecordUnsuccessfulPoll()
	require.True(p.Finalized()) // Should still be finalized
	require.Equal(Red, p.Preference())
}