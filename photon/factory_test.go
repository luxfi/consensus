// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

import (
	"testing"
	"sync"
	"github.com/stretchr/testify/require"
	"github.com/luxfi/ids"
)

func TestPhotonFactoryBasic(t *testing.T) {
	require := require.New(t)
	
	// Test global factory instance
	require.NotNil(PhotonFactory)
	
	// Test factory interface implementation
	var f Factory = photonFactory{}
	require.NotNil(f)
	
	// Test NewFactory function
	factory := NewFactory()
	require.NotNil(factory)
}

func TestPhotonFactoryNilReturns(t *testing.T) {
	require := require.New(t)
	
	factory := photonFactory{}
	params := DefaultParameters
	
	// Verify photon factory returns nil for full consensus
	// Photon only implements quantum sampling
	polyadic := factory.NewPolyadic(params, ids.GenerateTestID())
	require.Nil(polyadic)
	
	monadic := factory.NewMonadic(params)
	require.Nil(monadic)
}

func TestPhotonFactoryConcurrent(t *testing.T) {
	require := require.New(t)
	
	factory := photonFactory{}
	params := DefaultParameters
	
	// Test concurrent access to factory
	var wg sync.WaitGroup
	numGoroutines := 100
	
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			
			// Concurrent calls should be safe
			polyadic := factory.NewPolyadic(params, ids.GenerateTestID())
			require.Nil(polyadic)
			
			monadic := factory.NewMonadic(params)
			require.Nil(monadic)
		}()
	}
	
	wg.Wait()
}

func BenchmarkPhotonFactory(b *testing.B) {
	factory := photonFactory{}
	params := DefaultParameters
	choice := ids.GenerateTestID()
	
	b.Run("NewPolyadic", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = factory.NewPolyadic(params, choice)
		}
	})
	
	b.Run("NewMonadic", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = factory.NewMonadic(params)
		}
	})
}