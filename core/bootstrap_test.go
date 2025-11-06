package core

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Mock BootstrapTracker implementation
type mockBootstrapTracker struct {
	started       atomic.Bool
	completed     atomic.Bool
	bootstrapped  atomic.Bool
	startErr      error
	completeErr   error
	mu            sync.RWMutex
	startCount    int
	completeCount int
}

func newMockBootstrapTracker() *mockBootstrapTracker {
	return &mockBootstrapTracker{}
}

func (m *mockBootstrapTracker) OnBootstrapStarted() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.startErr != nil {
		return m.startErr
	}

	m.started.Store(true)
	m.startCount++
	return nil
}

func (m *mockBootstrapTracker) OnBootstrapCompleted() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.completeErr != nil {
		return m.completeErr
	}

	m.completed.Store(true)
	m.bootstrapped.Store(true)
	m.completeCount++
	return nil
}

func (m *mockBootstrapTracker) IsBootstrapped() bool {
	return m.bootstrapped.Load()
}

func (m *mockBootstrapTracker) getStartCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.startCount
}

func (m *mockBootstrapTracker) getCompleteCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.completeCount
}

// Tests
func TestBootstrapTracker_BasicFlow(t *testing.T) {
	tracker := newMockBootstrapTracker()

	// Initially not bootstrapped
	require.False(t, tracker.IsBootstrapped())

	// Start bootstrap
	err := tracker.OnBootstrapStarted()
	require.NoError(t, err)
	require.True(t, tracker.started.Load())
	require.False(t, tracker.IsBootstrapped())
	require.Equal(t, 1, tracker.getStartCount())

	// Complete bootstrap
	err = tracker.OnBootstrapCompleted()
	require.NoError(t, err)
	require.True(t, tracker.completed.Load())
	require.True(t, tracker.IsBootstrapped())
	require.Equal(t, 1, tracker.getCompleteCount())
}

func TestBootstrapTracker_MultipleStarts(t *testing.T) {
	tracker := newMockBootstrapTracker()

	// Start multiple times
	for i := 0; i < 5; i++ {
		err := tracker.OnBootstrapStarted()
		require.NoError(t, err)
	}

	require.Equal(t, 5, tracker.getStartCount())
	require.False(t, tracker.IsBootstrapped())

	// Complete once
	err := tracker.OnBootstrapCompleted()
	require.NoError(t, err)
	require.True(t, tracker.IsBootstrapped())
}

func TestBootstrapTracker_StartError(t *testing.T) {
	tracker := newMockBootstrapTracker()
	tracker.startErr = errors.New("start failed")

	err := tracker.OnBootstrapStarted()
	require.Error(t, err)
	require.Equal(t, "start failed", err.Error())
	require.False(t, tracker.started.Load())
	require.False(t, tracker.IsBootstrapped())
	require.Equal(t, 0, tracker.getStartCount())
}

func TestBootstrapTracker_CompleteError(t *testing.T) {
	tracker := newMockBootstrapTracker()
	tracker.completeErr = errors.New("complete failed")

	// Start successfully
	err := tracker.OnBootstrapStarted()
	require.NoError(t, err)

	// Complete with error
	err = tracker.OnBootstrapCompleted()
	require.Error(t, err)
	require.Equal(t, "complete failed", err.Error())
	require.False(t, tracker.completed.Load())
	require.False(t, tracker.IsBootstrapped())
	require.Equal(t, 0, tracker.getCompleteCount())
}

func TestBootstrapTracker_CompleteWithoutStart(t *testing.T) {
	tracker := newMockBootstrapTracker()

	// Complete without starting
	err := tracker.OnBootstrapCompleted()
	require.NoError(t, err)
	require.True(t, tracker.IsBootstrapped())
	require.False(t, tracker.started.Load())
	require.True(t, tracker.completed.Load())
}

func TestBootstrapTracker_Concurrent(t *testing.T) {
	tracker := newMockBootstrapTracker()

	const numGoroutines = 100
	var wg sync.WaitGroup

	// Concurrent starts
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = tracker.OnBootstrapStarted()
		}()
	}
	wg.Wait()

	require.True(t, tracker.started.Load())
	require.Equal(t, numGoroutines, tracker.getStartCount())

	// Concurrent completions
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = tracker.OnBootstrapCompleted()
		}()
	}
	wg.Wait()

	require.True(t, tracker.IsBootstrapped())
	require.Equal(t, numGoroutines, tracker.getCompleteCount())

	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			bootstrapped := tracker.IsBootstrapped()
			require.True(t, bootstrapped)
		}()
	}
	wg.Wait()
}

func TestBootstrapTracker_RaceCondition(t *testing.T) {
	// Test for race conditions with parallel read/write
	tracker := newMockBootstrapTracker()

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			_ = tracker.OnBootstrapStarted()
			_ = tracker.OnBootstrapCompleted()
		}
		done <- true
	}()

	// Reader goroutines
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 1000; j++ {
				_ = tracker.IsBootstrapped()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 11; i++ {
		<-done
	}

	// Final state should be bootstrapped
	require.True(t, tracker.IsBootstrapped())
}

func TestBootstrapTracker_StateTransitions(t *testing.T) {
	tracker := newMockBootstrapTracker()

	// Test all possible state transitions
	states := []struct {
		name      string
		action    func() error
		checkFunc func() bool
		expected  bool
	}{
		{
			name:      "initial state",
			action:    func() error { return nil },
			checkFunc: tracker.IsBootstrapped,
			expected:  false,
		},
		{
			name:      "after start",
			action:    tracker.OnBootstrapStarted,
			checkFunc: tracker.IsBootstrapped,
			expected:  false,
		},
		{
			name:      "after complete",
			action:    tracker.OnBootstrapCompleted,
			checkFunc: tracker.IsBootstrapped,
			expected:  true,
		},
		{
			name:      "start after complete",
			action:    tracker.OnBootstrapStarted,
			checkFunc: tracker.IsBootstrapped,
			expected:  true, // Should remain bootstrapped
		},
		{
			name:      "complete after complete",
			action:    tracker.OnBootstrapCompleted,
			checkFunc: tracker.IsBootstrapped,
			expected:  true,
		},
	}

	for _, st := range states {
		t.Run(st.name, func(t *testing.T) {
			_ = st.action()
			result := st.checkFunc()
			require.Equal(t, st.expected, result)
		})
	}
}

func TestBootstrapTracker_InterfaceCompliance(t *testing.T) {
	// Verify mock implements interface
	var _ BootstrapTracker = (*mockBootstrapTracker)(nil)

	// Test with interface type
	var tracker BootstrapTracker = newMockBootstrapTracker()

	err := tracker.OnBootstrapStarted()
	require.NoError(t, err)

	err = tracker.OnBootstrapCompleted()
	require.NoError(t, err)

	bootstrapped := tracker.IsBootstrapped()
	require.True(t, bootstrapped)
}

func TestBootstrapTracker_Timeout(t *testing.T) {
	tracker := newMockBootstrapTracker()

	// Start bootstrap
	err := tracker.OnBootstrapStarted()
	require.NoError(t, err)

	// Simulate timeout scenario
	done := make(chan bool, 1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		_ = tracker.OnBootstrapCompleted()
		done <- true
	}()

	// Check bootstrapped status while waiting
	startTime := time.Now()
	for !tracker.IsBootstrapped() && time.Since(startTime) < 100*time.Millisecond {
		time.Sleep(1 * time.Millisecond)
	}

	require.True(t, tracker.IsBootstrapped())
	<-done
}

// Benchmarks
func BenchmarkBootstrapTracker_OnBootstrapStarted(b *testing.B) {
	tracker := newMockBootstrapTracker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tracker.OnBootstrapStarted()
	}
}

func BenchmarkBootstrapTracker_OnBootstrapCompleted(b *testing.B) {
	tracker := newMockBootstrapTracker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tracker.OnBootstrapCompleted()
	}
}

func BenchmarkBootstrapTracker_IsBootstrapped(b *testing.B) {
	tracker := newMockBootstrapTracker()
	_ = tracker.OnBootstrapStarted()
	_ = tracker.OnBootstrapCompleted()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tracker.IsBootstrapped()
	}
}

func BenchmarkBootstrapTracker_ConcurrentReads(b *testing.B) {
	tracker := newMockBootstrapTracker()
	_ = tracker.OnBootstrapCompleted()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = tracker.IsBootstrapped()
		}
	})
}

func BenchmarkBootstrapTracker_FullCycle(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker := newMockBootstrapTracker()
		_ = tracker.OnBootstrapStarted()
		_ = tracker.OnBootstrapCompleted()
		_ = tracker.IsBootstrapped()
	}
}
