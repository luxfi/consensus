// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package flare

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type TestTx string

func TestFlareBasic(t *testing.T) {
	fl := New[TestTx](7) // f=7, need 2f+1 = 15 votes

	tx := TestTx("tx1")
	
	// Initially pending
	require.Equal(t, StatusPending, fl.Status(tx))
	
	// Vote 14 times (not enough)
	for i := 0; i < 14; i++ {
		fl.Propose(tx)
	}
	require.Equal(t, StatusPending, fl.Status(tx))
	
	// 15th vote makes it executable
	fl.Propose(tx)
	require.Equal(t, StatusExecutable, fl.Status(tx))
	
	// Check executable list
	exec := fl.Executable()
	require.Len(t, exec, 1)
	require.Equal(t, tx, exec[0])
}

func TestFlareConcurrent(t *testing.T) {
	fl := New[TestTx](10) // f=10, need 21 votes
	
	tx1 := TestTx("tx1")
	tx2 := TestTx("tx2")
	
	var wg sync.WaitGroup
	
	// Concurrent voting for tx1
	for i := 0; i < 21; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fl.Propose(tx1)
		}()
	}
	
	// Concurrent voting for tx2 (only 15 votes)
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fl.Propose(tx2)
		}()
	}
	
	wg.Wait()
	
	// tx1 should be executable, tx2 should not
	require.Equal(t, StatusExecutable, fl.Status(tx1))
	require.Equal(t, StatusPending, fl.Status(tx2))
	
	exec := fl.Executable()
	require.Len(t, exec, 1)
	require.Equal(t, tx1, exec[0])
}

func TestFlareThreshold(t *testing.T) {
	testCases := []struct {
		f         int
		votes     int
		shouldExec bool
	}{
		{1, 2, false},
		{1, 3, true},
		{3, 6, false},
		{3, 7, true},
		{10, 20, false},
		{10, 21, true},
		{33, 66, false},
		{33, 67, true},
	}
	
	for _, tc := range testCases {
		fl := New[TestTx](tc.f)
		tx := TestTx("test")
		
		for i := 0; i < tc.votes; i++ {
			fl.Propose(tx)
		}
		
		status := fl.Status(tx)
		if tc.shouldExec {
			require.Equal(t, StatusExecutable, status, "f=%d, votes=%d should be executable", tc.f, tc.votes)
		} else {
			require.Equal(t, StatusPending, status, "f=%d, votes=%d should be pending", tc.f, tc.votes)
		}
	}
}

func BenchmarkFlarePropose(b *testing.B) {
	fl := New[TestTx](10)
	tx := TestTx("bench")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fl.Propose(tx)
	}
}

func BenchmarkFlareConcurrentPropose(b *testing.B) {
	fl := New[TestTx](10)
	
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tx := TestTx(string(rune(i)))
			fl.Propose(tx)
			i++
		}
	})
}