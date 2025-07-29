// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

import (
	"testing"
	"time"
	"github.com/stretchr/testify/require"
	"github.com/luxfi/ids"
)

func TestDyadicPhoton(t *testing.T) {
	require := require.New(t)
	
	// Test initialization
	dp := NewDyadicPhoton(0)
	require.Equal(0, dp.Preference())
	
	// Test preference update
	dp.RecordSuccessfulPoll(1)
	require.Equal(1, dp.Preference())
	
	// Test string representation
	require.Equal("DyadicPhoton(Preference = 1)", dp.String())
}

func TestPolyadicPhoton(t *testing.T) {
	require := require.New(t)
	
	// Create test choice
	choice := ids.GenerateTestID()
	
	// Test initialization
	pp := NewPolyadicPhoton(choice)
	require.Equal(choice, pp.Preference())
	
	// Test preference update
	newChoice := ids.GenerateTestID()
	pp.RecordSuccessfulPoll(newChoice)
	require.Equal(newChoice, pp.Preference())
	
	// Test string representation
	expected := "PolyadicPhoton(Preference = " + newChoice.String() + ")"
	require.Equal(expected, pp.String())
}

func TestFactory(t *testing.T) {
	require := require.New(t)
	
	factory := NewFactory()
	require.NotNil(factory)
	
	// Test polyadic creation (returns nil for photon)
	params := DefaultParameters
	choice := ids.GenerateTestID()
	polyadic := factory.NewPolyadic(params, choice)
	require.Nil(polyadic)
	
	// Test monadic creation (returns nil for photon)
	monadic := factory.NewMonadic(params)
	require.Nil(monadic)
}

func TestParameters(t *testing.T) {
	require := require.New(t)
	
	// Test default parameters
	params := DefaultParameters
	require.Equal(20, params.K)
	require.Equal(15, params.AlphaPreference)
	require.Equal(15, params.AlphaConfidence)
	require.Equal(20, params.Beta)
	
	// Test custom parameters
	customParams := Parameters{
		K:                   11,
		AlphaPreference:     7,
		AlphaConfidence:     9,
		Beta:                1,
		ConcurrentRepolls:   1,
		OptimalProcessing:   5,
		MaxOutstandingItems: 128,
		MaxItemProcessingTime: 15 * time.Second,
	}
	require.Equal(11, customParams.K)
	require.Equal(7, customParams.AlphaPreference)
	require.Equal(9, customParams.AlphaConfidence)
	require.Equal(1, customParams.Beta)
	
	// Test parameters validation
	err := params.Verify()
	require.NoError(err)
	
	// Test invalid parameters
	invalidParams := Parameters{K: 0}
	err = invalidParams.Verify()
	require.Error(err)
}

func BenchmarkDyadicPhoton(b *testing.B) {
	dp := NewDyadicPhoton(0)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dp.RecordSuccessfulPoll(i % 2)
		_ = dp.Preference()
	}
}

func BenchmarkPolyadicPhoton(b *testing.B) {
	choice := ids.GenerateTestID()
	pp := NewPolyadicPhoton(choice)
	
	choices := make([]ids.ID, 10)
	for i := 0; i < 10; i++ {
		choices[i] = ids.GenerateTestID()
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pp.RecordSuccessfulPoll(choices[i%10])
		_ = pp.Preference()
	}
}
