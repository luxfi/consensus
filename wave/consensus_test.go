// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
	"testing"
	"sync"
	"github.com/stretchr/testify/require"
	"github.com/luxfi/ids"
)

func TestDyadicWave(t *testing.T) {
	require := require.New(t)
	
	// Test parameters
	alphaPreference := 2
	terminationConditions := []TerminationCondition{
		{AlphaConfidence: 2, Beta: 2},
	}
	
	// Create dyadic wave
	dw := NewDyadicWave(alphaPreference, terminationConditions, 0)
	require.NotNil(dw)
	
	// Test initial state
	require.Equal(0, dw.Preference())
	require.False(dw.Finalized())
	
	// Test unsuccessful poll (below alpha threshold)
	dw.RecordPoll(1, 0)
	require.False(dw.Finalized())
	
	// Test successful poll
	dw.RecordPoll(2, 0) // First success with alpha=2
	require.False(dw.Finalized())
	
	// Second successful poll should finalize (beta=2)
	dw.RecordPoll(2, 0)
	require.True(dw.Finalized())
	require.Equal(0, dw.Preference())
}

func TestDyadicWavePreferenceChange(t *testing.T) {
	require := require.New(t)
	
	// Test parameters
	alphaPreference := 2
	terminationConditions := []TerminationCondition{
		{AlphaConfidence: 2, Beta: 3},
	}
	
	// Create dyadic wave
	dw := NewDyadicWave(alphaPreference, terminationConditions, 0)
	
	// Start with preference 0
	require.Equal(0, dw.Preference())
	
	// Vote for 1 with enough support to change preference
	dw.RecordPoll(2, 1)
	require.Equal(1, dw.Preference()) // Preference should change
	require.False(dw.Finalized())
	
	// Continue voting for 1
	dw.RecordPoll(2, 1)
	require.False(dw.Finalized())
	
	// Third vote should finalize (beta=3)
	dw.RecordPoll(2, 1)
	require.True(dw.Finalized())
	require.Equal(1, dw.Preference())
}

func TestDyadicWaveMultipleTerminationConditions(t *testing.T) {
	require := require.New(t)
	
	// Multiple termination conditions
	alphaPreference := 2
	terminationConditions := []TerminationCondition{
		{AlphaConfidence: 2, Beta: 4},
		{AlphaConfidence: 3, Beta: 2},
	}
	
	// Create dyadic wave
	dw := NewDyadicWave(alphaPreference, terminationConditions, 0)
	
	// Vote with 3 votes for 0 (meets higher alpha)
	dw.RecordPoll(3, 0)
	require.False(dw.Finalized())
	
	// Second vote with high confidence should finalize (beta=2 for alpha=3)
	dw.RecordPoll(3, 0)
	require.True(dw.Finalized())
}

func TestPolyadicWave(t *testing.T) {
	require := require.New(t)
	
	// Test parameters
	alphaPreference := 2
	terminationConditions := []TerminationCondition{
		{AlphaConfidence: 2, Beta: 2},
	}
	
	// Create polyadic wave
	choice := ids.GenerateTestID()
	pw := NewPolyadicWave(alphaPreference, terminationConditions, choice)
	require.NotNil(pw)
	
	// Test initial state
	require.Equal(choice, pw.Preference())
	require.False(pw.Finalized())
	
	// Test unsuccessful poll
	pw.RecordPoll(1, choice)
	require.False(pw.Finalized())
	
	// Test successful polls
	pw.RecordPoll(2, choice)
	require.False(pw.Finalized())
	pw.RecordPoll(2, choice)
	require.True(pw.Finalized())
}

func TestWaveFactory(t *testing.T) {
	require := require.New(t)
	
	// Test NewFactory function
	factory := NewFactory()
	require.NotNil(factory)
	
	params := DefaultParameters
	
	// Test polyadic wave creation
	choice := ids.GenerateTestID()
	polyadic := factory.NewPolyadic(params, choice)
	require.NotNil(polyadic)
	require.Equal(choice, polyadic.Preference())
	
	// Test monadic wave creation
	monadic := factory.NewMonadic(params)
	require.NotNil(monadic)
	
	// Test global factory instance
	require.NotNil(WaveFactory)
}

func TestWaveConsensusConcurrent(t *testing.T) {
	require := require.New(t)
	
	// Test concurrent polling
	alphaPreference := 3
	terminationConditions := []TerminationCondition{
		{AlphaConfidence: 3, Beta: 5},
	}
	
	var wg sync.WaitGroup
	numGoroutines := 100
	
	// Create shared wave instance
	dw := NewDyadicWave(alphaPreference, terminationConditions, 0)
	
	// Track successful polls
	successCount := 0
	var mu sync.Mutex
	
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			
			// Record poll
			dw.RecordPoll(3, 0)
			if dw.Finalized() {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}
	
	wg.Wait()
	
	// Should have finalized
	require.True(dw.Finalized())
	// Multiple goroutines may have detected finalization due to race conditions
	require.Greater(successCount, 0)
}

func BenchmarkDyadicWave(b *testing.B) {
	alphaPreference := 3
	terminationConditions := []TerminationCondition{
		{AlphaConfidence: 3, Beta: 3},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dw := NewDyadicWave(alphaPreference, terminationConditions, 0)
		
		for !dw.Finalized() {
			dw.RecordPoll(3, 0)
		}
	}
}

func BenchmarkPolyadicWave(b *testing.B) {
	alphaPreference := 3
	terminationConditions := []TerminationCondition{
		{AlphaConfidence: 3, Beta: 3},
	}
	choice := ids.GenerateTestID()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pw := NewPolyadicWave(alphaPreference, terminationConditions, choice)
		
		for !pw.Finalized() {
			pw.RecordPoll(3, choice)
		}
	}
}