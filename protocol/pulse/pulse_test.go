// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pulse

import (
    "testing"
    
    "github.com/stretchr/testify/require"
    
    "github.com/luxfi/consensus/config"
    "github.com/luxfi/consensus/utils/bag"
    "github.com/luxfi/ids"
)

var (
    Red   = ids.ID{0x01}
    Blue  = ids.ID{0x02}
    Green = ids.ID{0x03}
)

func TestPulseBasic(t *testing.T) {
    require := require.New(t)
    
    params := config.DefaultParameters
    p := NewPulse(params)
    require.NotNil(p)
    require.False(p.Finalized())
}

func TestPulseBinaryChoice(t *testing.T) {
    require := require.New(t)
    
    params := config.TestParameters
    p := NewPulse(params)
    
    // Add two choices
    require.NoError(p.Add(Red))
    require.NoError(p.Add(Blue))
    
    // Initially prefers first added
    require.Equal(Red, p.Preference())
    require.False(p.Finalized())
    
    // Vote for Blue
    votes := bag.Bag[ids.ID]{}
    for i := 0; i < params.AlphaPreference; i++ {
        votes.Add(Blue)
    }
    
    require.NoError(p.RecordVotes(votes))
    require.Equal(Blue, p.Preference())
    
    // Continue voting to finalize
    for i := 1; i < params.Beta; i++ {
        require.NoError(p.RecordVotes(votes))
    }
    
    require.True(p.Finalized())
    require.Equal(Blue, p.Preference())
}

func TestPulsePreferenceSwitching(t *testing.T) {
    require := require.New(t)
    
    params := config.TestParameters
    p := NewPulse(params)
    
    require.NoError(p.Add(Red))
    require.NoError(p.Add(Blue))
    
    // Vote for Red first
    redVotes := bag.Bag[ids.ID]{}
    for i := 0; i < params.AlphaPreference; i++ {
        redVotes.Add(Red)
    }
    
    require.NoError(p.RecordVotes(redVotes))
    require.Equal(Red, p.Preference())
    
    // Switch to Blue
    blueVotes := bag.Bag[ids.ID]{}
    for i := 0; i < params.AlphaPreference; i++ {
        blueVotes.Add(Blue)
    }
    
    require.NoError(p.RecordVotes(blueVotes))
    require.Equal(Blue, p.Preference())
    
    // Confidence should reset when switching
    require.False(p.Finalized())
}

func TestPulseMultipleChoices(t *testing.T) {
    require := require.New(t)
    
    params := config.TestParameters
    p := NewPulse(params)
    
    // Pulse actually supports multiple choices (n-ary like Wave)
    require.NoError(p.Add(Red))
    require.NoError(p.Add(Blue))
    require.NoError(p.Add(Green))
    
    // Vote for Green
    votes := bag.Bag[ids.ID]{}
    for i := 0; i < params.AlphaPreference; i++ {
        votes.Add(Green)
    }
    
    require.NoError(p.RecordVotes(votes))
    require.Equal(Green, p.Preference())
}

func TestPulseConfidenceBuildup(t *testing.T) {
    require := require.New(t)
    
    params := config.TestParameters
    p := NewPulse(params)
    
    require.NoError(p.Add(Red))
    require.NoError(p.Add(Blue))
    
    // Vote consistently for Blue
    votes := bag.Bag[ids.ID]{}
    for i := 0; i < params.AlphaConfidence; i++ {
        votes.Add(Blue)
    }
    
    // Track confidence buildup
    for round := 0; round < params.Beta; round++ {
        require.NoError(p.RecordVotes(votes))
        if round < params.Beta-1 {
            require.False(p.Finalized(), "Should not finalize before Beta rounds")
        }
    }
    
    require.True(p.Finalized())
    require.Equal(Blue, p.Preference())
}

func TestPulseRecordUnsuccessfulPoll(t *testing.T) {
    require := require.New(t)
    
    params := config.TestParameters
    p := NewPulse(params)
    
    require.NoError(p.Add(Red))
    require.NoError(p.Add(Blue))
    
    // Build confidence
    votes := bag.Bag[ids.ID]{}
    for i := 0; i < params.AlphaConfidence; i++ {
        votes.Add(Blue)
    }
    
    require.NoError(p.RecordVotes(votes))
    
    // Record unsuccessful poll
    p.RecordUnsuccessfulPoll()
    
    // Should reset confidence but not finalize
    require.False(p.Finalized())
    
    // Can still build confidence and finalize
    for i := 0; i < params.Beta; i++ {
        require.NoError(p.RecordVotes(votes))
    }
    require.True(p.Finalized())
}

func TestPulseSplitVotes(t *testing.T) {
    require := require.New(t)
    
    params := config.DefaultParameters
    p := NewPulse(params)
    
    require.NoError(p.Add(Red))
    require.NoError(p.Add(Blue))
    
    // Split votes evenly
    votes := bag.Bag[ids.ID]{}
    for i := 0; i < params.K/2; i++ {
        votes.Add(Red)
        votes.Add(Blue)
    }
    
    require.NoError(p.RecordVotes(votes))
    
    // Should not change preference or finalize
    require.Equal(Red, p.Preference()) // Still first added
    require.False(p.Finalized())
}

func TestPulseVoteAfterFinalized(t *testing.T) {
    require := require.New(t)
    
    params := config.TestParameters
    p := NewPulse(params)
    
    require.NoError(p.Add(Red))
    require.NoError(p.Add(Blue))
    
    // Finalize on Blue
    votes := bag.Bag[ids.ID]{}
    for i := 0; i < params.AlphaConfidence; i++ {
        votes.Add(Blue)
    }
    
    for i := 0; i < params.Beta; i++ {
        require.NoError(p.RecordVotes(votes))
    }
    
    require.True(p.Finalized())
    require.Equal(Blue, p.Preference())
    
    // Try to vote for Red after finalized
    redVotes := bag.Bag[ids.ID]{}
    for i := 0; i < params.AlphaConfidence; i++ {
        redVotes.Add(Red)
    }
    
    require.NoError(p.RecordVotes(redVotes))
    
    // Should still be Blue
    require.True(p.Finalized())
    require.Equal(Blue, p.Preference())
}

func TestPulsePreferenceStrength(t *testing.T) {
    require := require.New(t)
    
    params := config.TestParameters
    p := NewPulse(params)
    
    require.NoError(p.Add(Red))
    require.NoError(p.Add(Blue))
    
    // Vote for Blue with enough votes to increment strength
    votes := bag.Bag[ids.ID]{}
    for i := 0; i < params.AlphaConfidence; i++ {
        votes.Add(Blue)
    }
    
    // Record multiple polls
    // With TestParameters.Beta = 2, it will finalize after 2 rounds
    require.NoError(p.RecordVotes(votes))
    require.Equal(1, p.preferenceStrength[Blue])
    
    require.NoError(p.RecordVotes(votes))
    require.Equal(2, p.preferenceStrength[Blue])
    require.Equal(0, p.preferenceStrength[Red])
    
    // Should be finalized now
    require.True(p.Finalized())
}

func TestPulseString(t *testing.T) {
    require := require.New(t)
    
    params := config.TestParameters
    p := NewPulse(params)
    
    require.NoError(p.Add(Red))
    require.NoError(p.Add(Blue))
    
    str := p.String()
    require.Contains(str, "Pulse")
    require.Contains(str, "pref=")
    require.Contains(str, "conf=")
    require.Contains(str, "finalized=")
}
