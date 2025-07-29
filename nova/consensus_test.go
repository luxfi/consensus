// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nova

import (
	"context"
	"sync"
	"testing"
	
	"github.com/stretchr/testify/require"
	"github.com/luxfi/ids"
)

func TestNovaConsensusBasic(t *testing.T) {
	require := require.New(t)
	
	params := Parameters{
		FinalizationDepth: 3,
		MaxPending:       100,
	}
	
	nova := New(params)
	ctx := context.Background()
	
	// Test initial state
	require.Empty(nova.GetFinalized())
	
	// Generate test IDs
	id1 := ids.GenerateTestID()
	id2 := ids.GenerateTestID()
	id3 := ids.GenerateTestID()
	
	// Test finalization
	require.False(nova.IsFinalized(id1))
	require.NoError(nova.Finalize(ctx, id1))
	require.True(nova.IsFinalized(id1))
	
	// Test multiple finalizations
	require.NoError(nova.Finalize(ctx, id2))
	require.NoError(nova.Finalize(ctx, id3))
	
	// Verify ordering
	finalized := nova.GetFinalized()
	require.Len(finalized, 3)
	require.Equal(id1, finalized[0])
	require.Equal(id2, finalized[1])
	require.Equal(id3, finalized[2])
	
	// Test health check
	health, err := nova.HealthCheck(ctx)
	require.NoError(err)
	require.NotNil(health)
	
	healthMap, ok := health.(map[string]interface{})
	require.True(ok)
	require.True(healthMap["healthy"].(bool))
	require.Equal(3, healthMap["finalized"].(int))
}

func TestNovaConsensusEdgeCases(t *testing.T) {
	require := require.New(t)
	
	params := Parameters{
		FinalizationDepth: 1,
		MaxPending:       10,
	}
	
	nova := New(params)
	ctx := context.Background()
	
	// Test empty ID
	emptyID := ids.Empty
	require.False(nova.IsFinalized(emptyID))
	require.NoError(nova.Finalize(ctx, emptyID))
	require.True(nova.IsFinalized(emptyID))
	
	// Test duplicate finalization
	id := ids.GenerateTestID()
	require.NoError(nova.Finalize(ctx, id))
	require.NoError(nova.Finalize(ctx, id)) // Should not error
	
	// Verify only added once
	finalized := nova.GetFinalized()
	count := 0
	for _, fid := range finalized {
		if fid == id {
			count++
		}
	}
	require.Equal(1, count)
	
	// Test RecordFinalization (same as Finalize)
	newID := ids.GenerateTestID()
	require.NoError(nova.RecordFinalization(ctx, newID))
	require.True(nova.IsFinalized(newID))
}

func TestNovaConsensusConcurrent(t *testing.T) {
	require := require.New(t)
	
	params := Parameters{
		FinalizationDepth: 5,
		MaxPending:       1000,
	}
	
	nova := New(params)
	ctx := context.Background()
	
	// Generate IDs
	numIDs := 100
	testIDs := make([]ids.ID, numIDs)
	for i := 0; i < numIDs; i++ {
		testIDs[i] = ids.GenerateTestID()
	}
	
	// Concurrent finalization
	var wg sync.WaitGroup
	wg.Add(numIDs)
	
	for i := 0; i < numIDs; i++ {
		go func(idx int) {
			defer wg.Done()
			require.NoError(nova.Finalize(ctx, testIDs[idx]))
		}(i)
	}
	
	wg.Wait()
	
	// Verify all finalized
	for _, id := range testIDs {
		require.True(nova.IsFinalized(id))
	}
	
	// Check total count
	finalized := nova.GetFinalized()
	require.Len(finalized, numIDs)
}

func TestNovaHealthCheck(t *testing.T) {
	require := require.New(t)
	
	params := Parameters{
		FinalizationDepth: 3,
		MaxPending:       50,
	}
	
	nova := New(params)
	ctx := context.Background()
	
	// Initial health check
	health, err := nova.HealthCheck(ctx)
	require.NoError(err)
	
	healthMap := health.(map[string]interface{})
	require.True(healthMap["healthy"].(bool))
	require.Equal(0, healthMap["finalized"].(int))
	
	// Add some finalizations
	for i := 0; i < 5; i++ {
		require.NoError(nova.Finalize(ctx, ids.GenerateTestID()))
	}
	
	// Check health again
	health, err = nova.HealthCheck(ctx)
	require.NoError(err)
	
	healthMap = health.(map[string]interface{})
	require.True(healthMap["healthy"].(bool))
	require.Equal(5, healthMap["finalized"].(int))
}

func BenchmarkNovaConsensus(b *testing.B) {
	params := Parameters{
		FinalizationDepth: 3,
		MaxPending:       100,
	}
	
	nova := New(params)
	ctx := context.Background()
	
	// Pre-generate IDs
	testIDs := make([]ids.ID, b.N)
	for i := 0; i < b.N; i++ {
		testIDs[i] = ids.GenerateTestID()
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		nova.Finalize(ctx, testIDs[i])
	}
}

func BenchmarkNovaIsFinalized(b *testing.B) {
	params := Parameters{
		FinalizationDepth: 3,
		MaxPending:       100,
	}
	
	nova := New(params)
	ctx := context.Background()
	
	// Pre-finalize some IDs
	numFinalized := 1000
	for i := 0; i < numFinalized; i++ {
		nova.Finalize(ctx, ids.GenerateTestID())
	}
	
	// Generate test ID
	testID := ids.GenerateTestID()
	nova.Finalize(ctx, testID)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		nova.IsFinalized(testID)
	}
}
