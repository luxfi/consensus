// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package flare

import (
	"context"
	"testing"
	"sync"
	"time"
	
	"github.com/stretchr/testify/require"
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/choices"
)

func TestFlareBasic(t *testing.T) {
	require := require.New(t)
	
	params := Parameters{
		MaxVertices:      100,
		OrderingInterval: 100 * time.Millisecond,
	}
	
	flare := New(params)
	ctx := context.Background()
	
	// Create test vertices
	tx1 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		BytesV: []byte("tx1"),
	}
	
	tx2 := &TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		BytesV: []byte("tx2"),
	}
	
	// Add vertices
	require.NoError(flare.Add(ctx, tx1))
	require.NoError(flare.Add(ctx, tx2))
	
	// Check they were added
	v1, err := flare.GetVertex(tx1.ID())
	require.NoError(err)
	require.Equal(tx1.ID(), v1.ID())
	
	v2, err := flare.GetVertex(tx2.ID())
	require.NoError(err)
	require.Equal(tx2.ID(), v2.ID())
	
	// Test ordering
	order := []ids.ID{tx1.ID(), tx2.ID()}
	require.NoError(flare.RecordOrder(ctx, order))
	
	currentOrder := flare.Order()
	require.Equal(order, currentOrder)
	
	// Test health check
	health, err := flare.HealthCheck(ctx)
	require.NoError(err)
	healthMap := health.(map[string]interface{})
	require.True(healthMap["healthy"].(bool))
	require.Equal(2, healthMap["vertices"].(int))
	require.Equal(2, healthMap["ordered"].(int))
}

func TestFlareEdgeCases(t *testing.T) {
	require := require.New(t)
	
	params := Parameters{
		MaxVertices:      10,
		OrderingInterval: 10 * time.Millisecond,
	}
	
	flare := New(params)
	ctx := context.Background()
	
	// Test getting non-existent vertex
	_, err := flare.GetVertex(ids.GenerateTestID())
	require.Error(err)
	
	// Test empty ordering
	require.Empty(flare.Order())
	
	// Test ordering with non-existent vertices
	fakeOrder := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}
	require.NoError(flare.RecordOrder(ctx, fakeOrder))
	require.Equal(fakeOrder, flare.Order())
	
	// Test health check with no vertices
	health, err := flare.HealthCheck(ctx)
	require.NoError(err)
	healthMap := health.(map[string]interface{})
	require.True(healthMap["healthy"].(bool))
	require.Equal(0, healthMap["vertices"].(int))
}

func TestFlareConcurrent(t *testing.T) {
	require := require.New(t)
	
	params := Parameters{
		MaxVertices:      1000,
		OrderingInterval: 50 * time.Millisecond,
	}
	
	flare := New(params)
	ctx := context.Background()
	
	var wg sync.WaitGroup
	numGoroutines := 100
	txPerGoroutine := 10
	
	// Concurrently add vertices
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			
			for j := 0; j < txPerGoroutine; j++ {
				tx := &TestTx{
					TestDecidable: choices.TestDecidable{
						IDV:     ids.GenerateTestID(),
						StatusV: choices.Processing,
					},
					BytesV: []byte("concurrent test"),
				}
				
				err := flare.Add(ctx, tx)
				if err != nil {
					t.Errorf("failed to add tx: %v", err)
				}
			}
		}(i)
	}
	
	wg.Wait()
	
	// Check health
	health, err := flare.HealthCheck(ctx)
	require.NoError(err)
	healthMap := health.(map[string]interface{})
	require.Equal(numGoroutines*txPerGoroutine, healthMap["vertices"].(int))
}

func TestFlareOrdering(t *testing.T) {
	require := require.New(t)
	
	params := Parameters{
		MaxVertices:      50,
		OrderingInterval: 10 * time.Millisecond,
	}
	
	flare := New(params)
	ctx := context.Background()
	
	// Create a chain of dependent vertices
	var txs []Tx
	var txIDs []ids.ID
	
	for i := 0; i < 10; i++ {
		tx := &TestTx{
			TestDecidable: choices.TestDecidable{
				IDV:     ids.GenerateTestID(),
				StatusV: choices.Processing,
			},
			BytesV: []byte("ordered tx"),
		}
		
		require.NoError(flare.Add(ctx, tx))
		txs = append(txs, tx)
		txIDs = append(txIDs, tx.ID())
	}
	
	// Record ordering
	require.NoError(flare.RecordOrder(ctx, txIDs))
	
	// Verify ordering is preserved
	currentOrder := flare.Order()
	require.Equal(txIDs, currentOrder)
	
	// Update ordering
	reversedIds := make([]ids.ID, len(txIDs))
	for i, id := range txIDs {
		reversedIds[len(txIDs)-1-i] = id
	}
	
	require.NoError(flare.RecordOrder(ctx, reversedIds))
	require.Equal(reversedIds, flare.Order())
}

func BenchmarkFlareAdd(b *testing.B) {
	params := Parameters{
		MaxVertices:      10000,
		OrderingInterval: 100 * time.Millisecond,
	}
	
	flare := New(params)
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx := &TestTx{
			TestDecidable: choices.TestDecidable{
				IDV:     ids.GenerateTestID(),
				StatusV: choices.Processing,
			},
			BytesV: []byte("benchmark tx"),
		}
		
		flare.Add(ctx, tx)
	}
}

func BenchmarkFlareGetVertex(b *testing.B) {
	params := Parameters{
		MaxVertices:      10000,
		OrderingInterval: 100 * time.Millisecond,
	}
	
	flare := New(params)
	ctx := context.Background()
	
	// Pre-populate vertices
	var txIDs []ids.ID
	for i := 0; i < 1000; i++ {
		tx := &TestTx{
			TestDecidable: choices.TestDecidable{
				IDV:     ids.GenerateTestID(),
				StatusV: choices.Processing,
			},
			BytesV: []byte("benchmark tx"),
		}
		
		flare.Add(ctx, tx)
		txIDs = append(txIDs, tx.ID())
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flare.GetVertex(txIDs[i%len(txIDs)])
	}
}

func BenchmarkFlareOrdering(b *testing.B) {
	params := Parameters{
		MaxVertices:      10000,
		OrderingInterval: 100 * time.Millisecond,
	}
	
	flare := New(params)
	ctx := context.Background()
	
	// Create ordering
	var order []ids.ID
	for i := 0; i < 100; i++ {
		order = append(order, ids.GenerateTestID())
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flare.RecordOrder(ctx, order)
		_ = flare.Order()
	}
}

// TestTx is a test implementation of Tx
type TestTx struct {
	choices.TestDecidable
	
	BytesV []byte
	ParentsV []ids.ID
	ConflictsV   []ids.ID
	DependenciesV []ids.ID
}

func (t *TestTx) Bytes() []byte {
	return t.BytesV
}

func (t *TestTx) Parents() []ids.ID {
	return t.ParentsV
}

func (t *TestTx) Verify(context.Context) error {
	return nil
}

func (t *TestTx) Conflicts() ([]ids.ID, error) {
	return t.ConflictsV, nil
}

func (t *TestTx) Dependencies() ([]ids.ID, error) {
	return t.DependenciesV, nil
}