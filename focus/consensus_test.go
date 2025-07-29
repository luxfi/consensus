// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package focus

import (
	"testing"
	"sync"
	"github.com/stretchr/testify/require"
	"github.com/luxfi/ids"
)

func TestDyadicFocus(t *testing.T) {
	require := require.New(t)
	
	// Test parameters
	alphaPreference := 2
	terminationConditions := []terminationCondition{
		{alphaConfidence: 2, beta: 2},
	}
	
	// Create dyadic focus
	df := newDyadicFocus(alphaPreference, terminationConditions, 0)
	
	// Test initial state
	require.Equal(0, df.Preference())
	require.False(df.Finalized())
	
	// Test preference strength tracking
	df.RecordPoll(2, 0)
	require.Equal(0, df.Preference())
	require.Equal([2]int{1, 0}, df.preferenceStrength)
	
	// Switch preference with enough votes
	df.RecordPoll(3, 1)
	require.Equal(0, df.Preference()) // Still 0 because 1 < 1
	require.Equal([2]int{1, 1}, df.preferenceStrength)
	
	// Continue with new preference
	df.RecordPoll(2, 1)
	require.Equal(1, df.Preference())
	require.True(df.Finalized())
}

func TestDyadicFocusPreferenceStrength(t *testing.T) {
	require := require.New(t)
	
	alphaPreference := 3
	terminationConditions := []terminationCondition{
		{alphaConfidence: 3, beta: 3},
	}
	
	df := newDyadicFocus(alphaPreference, terminationConditions, 0)
	
	// Build up preference strength for 0
	df.RecordPoll(3, 0)
	require.Equal([2]int{1, 0}, df.preferenceStrength)
	
	df.RecordPoll(3, 0)
	require.Equal([2]int{2, 0}, df.preferenceStrength)
	
	// Try to switch but not enough strength (count < alphaPreference)
	df.RecordPoll(2, 1)
	require.Equal(0, df.Preference()) // Still 0
	require.Equal([2]int{2, 0}, df.preferenceStrength)
	
	// Build up strength for 1
	df.RecordPoll(3, 1)
	require.Equal([2]int{2, 1}, df.preferenceStrength)
	
	// Now 1 should win with more strength
	df.RecordPoll(3, 1)
	require.Equal([2]int{2, 2}, df.preferenceStrength)
	df.RecordPoll(3, 1)
	require.Equal(1, df.Preference()) // Switched to 1
	require.Equal([2]int{2, 3}, df.preferenceStrength)
}

func TestPolyadicFocus(t *testing.T) {
	require := require.New(t)
	
	alphaPreference := 2
	terminationConditions := []terminationCondition{
		{alphaConfidence: 2, beta: 2},
	}
	
	choice := ids.GenerateTestID()
	pf := newPolyadicFocus(alphaPreference, terminationConditions, choice)
	
	// Test initial state
	require.Equal(choice, pf.Preference())
	require.False(pf.Finalized())
	
	// Add another choice
	newChoice := ids.GenerateTestID()
	pf.Add(newChoice)
	
	// Vote for new choice with strength
	pf.RecordPoll(3, newChoice)
	require.Equal(newChoice, pf.Preference())
	
	// Continue voting to finalize
	pf.RecordPoll(2, newChoice)
	require.Equal(newChoice, pf.Preference())
	require.True(pf.Finalized())
}

func TestMonadicFocus(t *testing.T) {
	require := require.New(t)
	
	alphaPreference := 2
	terminationConditions := []terminationCondition{
		{alphaConfidence: 2, beta: 2},
	}
	
	mf := newMonadicFocus(alphaPreference, terminationConditions)
	
	// Test initial state
	require.False(mf.Finalized())
	
	// Record polls
	mf.RecordPoll(2)
	require.False(mf.Finalized())
	
	mf.RecordPoll(2)
	require.True(mf.Finalized())
	
	// Test extend
	df := mf.Extend(0)
	require.NotNil(df)
	require.Equal(0, df.Preference())
	
	// Test clone
	clone := mf.Clone()
	require.NotNil(clone)
	require.True(clone.Finalized())
}

func TestFocusFactory(t *testing.T) {
	require := require.New(t)
	
	factory := &focusFactory{}
	params := DefaultParameters
	
	// Test dyadic creation
	dyadic := factory.NewDyadic(params, 1)
	require.NotNil(dyadic)
	require.Equal(1, dyadic.Preference())
	
	// Test polyadic creation
	choice := ids.GenerateTestID()
	polyadic := factory.NewPolyadic(params, choice)
	require.NotNil(polyadic)
	require.Equal(choice, polyadic.Preference())
	
	// Test monadic creation
	monadic := factory.NewMonadic(params)
	require.NotNil(monadic)
	require.False(monadic.Finalized())
}

func TestFocusConsensusConcurrent(t *testing.T) {
	require := require.New(t)
	
	alphaPreference := 3
	terminationConditions := []terminationCondition{
		{alphaConfidence: 3, beta: 5},
	}
	
	var wg sync.WaitGroup
	var mu sync.Mutex
	numGoroutines := 100
	
	// Create shared focus instance
	df := newDyadicFocus(alphaPreference, terminationConditions, 0)
	
	// Track finalization
	finalized := 0
	
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			
			// Vote with bias towards 0 (70% for 0, 30% for 1)
			choice := 0
			if idx%10 < 3 {
				choice = 1
			}
			
			mu.Lock()
			df.RecordPoll(3, choice)
			isFinalized := df.Finalized()
			mu.Unlock()
			
			if isFinalized {
				mu.Lock()
				finalized++
				mu.Unlock()
			}
		}(i)
	}
	
	wg.Wait()
	
	// Check final state
	mu.Lock()
	require.True(df.Finalized())
	require.Greater(finalized, 0)
	mu.Unlock()
}

func BenchmarkDyadicFocus(b *testing.B) {
	alphaPreference := 3
	terminationConditions := []terminationCondition{
		{alphaConfidence: 3, beta: 3},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		df := newDyadicFocus(alphaPreference, terminationConditions, 0)
		
		for !df.Finalized() {
			df.RecordPoll(3, 0)
		}
	}
}

func BenchmarkPolyadicFocus(b *testing.B) {
	alphaPreference := 3
	terminationConditions := []terminationCondition{
		{alphaConfidence: 3, beta: 3},
	}
	choice := ids.GenerateTestID()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pf := newPolyadicFocus(alphaPreference, terminationConditions, choice)
		
		for !pf.Finalized() {
			pf.RecordPoll(3, choice)
		}
	}
}