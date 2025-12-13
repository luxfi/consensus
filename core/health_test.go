package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Mock HealthCheckable implementation
type mockHealthCheckable struct {
	health     interface{}
	healthErr  error
	checkCount atomic.Int32
	mu         sync.RWMutex
	checkDelay time.Duration
}

func (m *mockHealthCheckable) HealthCheck(ctx context.Context) (interface{}, error) {
	m.checkCount.Add(1)

	// Simulate delay if configured
	if m.checkDelay > 0 {
		select {
		case <-time.After(m.checkDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.healthErr != nil {
		return nil, m.healthErr
	}
	return m.health, nil
}

func (m *mockHealthCheckable) setHealth(health interface{}, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.health = health
	m.healthErr = err
}

// Tests for HealthStatus
func TestHealthStatus_String(t *testing.T) {
	tests := []struct {
		name     string
		status   HealthStatus
		expected string
	}{
		{
			name:     "healthy",
			status:   HealthHealthy,
			expected: "healthy",
		},
		{
			name:     "unhealthy",
			status:   HealthUnhealthy,
			expected: "unhealthy",
		},
		{
			name:     "unknown",
			status:   HealthUnknown,
			expected: "unknown",
		},
		{
			name:     "invalid status",
			status:   HealthStatus(999),
			expected: "unknown",
		},
		{
			name:     "negative status",
			status:   HealthStatus(-1),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.String()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestHealthStatus_Values(t *testing.T) {
	// Verify constant values
	require.Equal(t, HealthStatus(0), HealthUnknown)
	require.Equal(t, HealthStatus(1), HealthHealthy)
	require.Equal(t, HealthStatus(2), HealthUnhealthy)

	// Verify ordering
	require.Less(t, int(HealthUnknown), int(HealthHealthy))
	require.Less(t, int(HealthHealthy), int(HealthUnhealthy))
}

func TestHealthStatus_Comparison(t *testing.T) {
	// Test equality (using variables to avoid trivial comparison warnings)
	healthy := HealthHealthy
	unhealthy := HealthUnhealthy
	unknown := HealthUnknown
	require.Equal(t, HealthHealthy, healthy)
	require.Equal(t, HealthUnhealthy, unhealthy)
	require.Equal(t, HealthUnknown, unknown)

	// Test inequality
	require.True(t, HealthHealthy != HealthUnhealthy)
	require.True(t, HealthHealthy != HealthUnknown)
	require.True(t, HealthUnhealthy != HealthUnknown)
}

func TestHealthStatus_Switch(t *testing.T) {
	statuses := []HealthStatus{HealthUnknown, HealthHealthy, HealthUnhealthy, HealthStatus(100)}

	for _, status := range statuses {
		var result string
		switch status {
		case HealthUnknown:
			result = "u"
		case HealthHealthy:
			result = "h"
		case HealthUnhealthy:
			result = "uh"
		default:
			result = "d"
		}

		switch status {
		case HealthUnknown:
			require.Equal(t, "u", result)
		case HealthHealthy:
			require.Equal(t, "h", result)
		case HealthUnhealthy:
			require.Equal(t, "uh", result)
		default:
			require.Equal(t, "d", result)
		}
	}
}

// Tests for HealthCheckable
func TestHealthCheckable_Basic(t *testing.T) {
	ctx := context.Background()
	mock := &mockHealthCheckable{
		health: map[string]string{"status": "ok"},
	}

	health, err := mock.HealthCheck(ctx)
	require.NoError(t, err)
	require.NotNil(t, health)
	require.Equal(t, map[string]string{"status": "ok"}, health)
	require.Equal(t, int32(1), mock.checkCount.Load())
}

func TestHealthCheckable_Error(t *testing.T) {
	ctx := context.Background()
	mock := &mockHealthCheckable{
		healthErr: errors.New("health check failed"),
	}

	health, err := mock.HealthCheck(ctx)
	require.Error(t, err)
	require.Nil(t, health)
	require.Equal(t, "health check failed", err.Error())
}

func TestHealthCheckable_ContextCancellation(t *testing.T) {
	mock := &mockHealthCheckable{
		health:     "healthy",
		checkDelay: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start health check
	done := make(chan struct{})
	var health interface{}
	var err error

	go func() {
		health, err = mock.HealthCheck(ctx)
		close(done)
	}()

	// Cancel context before check completes
	time.Sleep(10 * time.Millisecond)
	cancel()

	<-done

	require.Error(t, err)
	require.Equal(t, context.Canceled, err)
	require.Nil(t, health)
}

func TestHealthCheckable_ContextTimeout(t *testing.T) {
	mock := &mockHealthCheckable{
		health:     "healthy",
		checkDelay: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	health, err := mock.HealthCheck(ctx)
	require.Error(t, err)
	require.Equal(t, context.DeadlineExceeded, err)
	require.Nil(t, health)
}

func TestHealthCheckable_DynamicHealth(t *testing.T) {
	ctx := context.Background()
	mock := &mockHealthCheckable{}

	// Initially healthy
	mock.setHealth(HealthHealthy, nil)
	health, err := mock.HealthCheck(ctx)
	require.NoError(t, err)
	require.Equal(t, HealthHealthy, health)

	// Change to unhealthy
	mock.setHealth(HealthUnhealthy, nil)
	health, err = mock.HealthCheck(ctx)
	require.NoError(t, err)
	require.Equal(t, HealthUnhealthy, health)

	// Change to error
	mock.setHealth(nil, errors.New("system down"))
	health, err = mock.HealthCheck(ctx)
	require.Error(t, err)
	require.Nil(t, health)
}

func TestHealthCheckable_ConcurrentChecks(t *testing.T) {
	ctx := context.Background()
	mock := &mockHealthCheckable{
		health: map[string]bool{"healthy": true},
	}

	const numGoroutines = 100
	var wg sync.WaitGroup

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			health, err := mock.HealthCheck(ctx)
			require.NoError(t, err)
			require.NotNil(t, health)
		}()
	}

	wg.Wait()
	require.Equal(t, int32(numGoroutines), mock.checkCount.Load())
}

func TestHealthCheckable_RaceCondition(t *testing.T) {
	ctx := context.Background()
	mock := &mockHealthCheckable{}

	done := make(chan bool)

	// Writer goroutine - changes health status
	go func() {
		for i := 0; i < 1000; i++ {
			if i%2 == 0 {
				mock.setHealth(HealthHealthy, nil)
			} else {
				mock.setHealth(HealthUnhealthy, nil)
			}
		}
		done <- true
	}()

	// Reader goroutines - check health
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, _ = mock.HealthCheck(ctx)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 11; i++ {
		<-done
	}
}

func TestHealthCheckable_ComplexHealthData(t *testing.T) {
	ctx := context.Background()

	type HealthData struct {
		Status     HealthStatus
		Components map[string]HealthStatus
		Timestamp  time.Time
		Metrics    map[string]float64
		Messages   []string
	}

	healthData := &HealthData{
		Status: HealthHealthy,
		Components: map[string]HealthStatus{
			"database":   HealthHealthy,
			"cache":      HealthHealthy,
			"networking": HealthUnhealthy,
		},
		Timestamp: time.Now(),
		Metrics: map[string]float64{
			"cpu_usage":    45.5,
			"memory_usage": 67.2,
			"disk_usage":   23.1,
		},
		Messages: []string{
			"System operational",
			"Network degradation detected",
		},
	}

	mock := &mockHealthCheckable{
		health: healthData,
	}

	health, err := mock.HealthCheck(ctx)
	require.NoError(t, err)

	result, ok := health.(*HealthData)
	require.True(t, ok)
	require.Equal(t, HealthHealthy, result.Status)
	require.Equal(t, HealthUnhealthy, result.Components["networking"])
	require.Len(t, result.Messages, 2)
}

func TestHealthCheckable_InterfaceCompliance(t *testing.T) {
	// Verify mock implements interface
	var _ HealthCheckable = (*mockHealthCheckable)(nil)

	// Test with interface type
	var checker HealthCheckable = &mockHealthCheckable{
		health: "healthy",
	}

	ctx := context.Background()
	health, err := checker.HealthCheck(ctx)
	require.NoError(t, err)
	require.Equal(t, "healthy", health)
}

// Integration test
func TestHealthSystem_Integration(t *testing.T) {
	// Simulate a system with multiple health checkable components
	components := map[string]HealthCheckable{
		"component1": &mockHealthCheckable{health: HealthHealthy},
		"component2": &mockHealthCheckable{health: HealthHealthy},
		"component3": &mockHealthCheckable{health: HealthUnhealthy},
	}

	ctx := context.Background()
	overallHealth := HealthHealthy

	// Check all components
	for name, component := range components {
		health, err := component.HealthCheck(ctx)
		if err != nil {
			overallHealth = HealthUnhealthy
			t.Logf("Component %s error: %v", name, err)
		} else if status, ok := health.(HealthStatus); ok && status == HealthUnhealthy {
			overallHealth = HealthUnhealthy
			t.Logf("Component %s unhealthy", name)
		}
	}

	require.Equal(t, HealthUnhealthy, overallHealth)
}

// Benchmarks
func BenchmarkHealthStatus_String(b *testing.B) {
	status := HealthHealthy

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = status.String()
	}
}

func BenchmarkHealthCheckable_HealthCheck(b *testing.B) {
	ctx := context.Background()
	mock := &mockHealthCheckable{
		health: map[string]string{"status": "ok"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mock.HealthCheck(ctx)
	}
}

func BenchmarkHealthCheckable_ConcurrentHealthCheck(b *testing.B) {
	ctx := context.Background()
	mock := &mockHealthCheckable{
		health: HealthHealthy,
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = mock.HealthCheck(ctx)
		}
	})
}

// Examples
func ExampleHealthStatus_String() {
	status := HealthHealthy
	fmt.Println(status.String())
	// Output: healthy
}

func ExampleHealthCheckable() {
	ctx := context.Background()
	checker := &mockHealthCheckable{
		health: map[string]string{"status": "operational"},
	}

	health, err := checker.HealthCheck(ctx)
	if err != nil {
		fmt.Printf("Health check failed: %v\n", err)
	} else {
		fmt.Printf("Health: %v\n", health)
	}
	// Output: Health: map[status:operational]
}
