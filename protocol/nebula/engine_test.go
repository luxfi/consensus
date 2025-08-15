// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nebula

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/ids"
)

// Mock types for testing
type mockDecision struct {
	id    ids.ID
	bytes []byte
}

func (m *mockDecision) ID() ids.ID {
	return m.id
}

func (m *mockDecision) Bytes() []byte {
	return m.bytes
}

func (m *mockDecision) Verify() error {
	return nil
}

func TestNew(t *testing.T) {
	require := require.New(t)

	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	require.NotNil(engine)
	require.Equal(ctx, engine.ctx)
	require.Equal(StateInitializing, engine.state)
}

func TestInitialize(t *testing.T) {
	require := require.New(t)

	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	err := engine.Initialize(context.Background())
	require.NoError(err)
	require.Equal(StateRunning, engine.state)
}

func TestStart(t *testing.T) {
	tests := []struct {
		name      string
		state     State
		wantError bool
	}{
		{
			name:      "start when running",
			state:     StateRunning,
			wantError: false,
		},
		{
			name:      "start when not running",
			state:     StateInitializing,
			wantError: true,
		},
		{
			name:      "start when stopped",
			state:     StateStopped,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			ctx := &interfaces.Runtime{}
			engine := New(ctx)
			engine.state = tt.state

			err := engine.Start(context.Background())
			if tt.wantError {
				require.Error(err)
			} else {
				require.NoError(err)
			}
		})
	}
}

func TestStop(t *testing.T) {
	require := require.New(t)

	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	// Initialize and start
	err := engine.Initialize(context.Background())
	require.NoError(err)

	err = engine.Start(context.Background())
	require.NoError(err)

	// Stop
	err = engine.Stop(context.Background())
	require.NoError(err)
	require.Equal(StateStopped, engine.state)
}

func TestSubmit(t *testing.T) {
	require := require.New(t)

	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	decision := &mockDecision{
		id: ids.GenerateTestID(),
	}

	err := engine.Submit(context.Background(), decision)
	require.NoError(err)
}

func TestGetVertex(t *testing.T) {
	require := require.New(t)

	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	vtxID := ids.GenerateTestID()
	vtx, err := engine.GetVertex(context.Background(), vtxID)
	require.Error(err)
	require.Nil(vtx)
}

func TestPutVertex(t *testing.T) {
	require := require.New(t)

	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	// PutVertex expects (context.Context, types.Vertex)
	err := engine.PutVertex(context.Background(), nil)
	require.Error(err) // Should error with nil vertex
}

func TestBuildVertex(t *testing.T) {
	require := require.New(t)

	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	vtx, err := engine.BuildVertex(context.Background())
	require.Error(err) // Returns ErrNotImplemented
	require.Nil(vtx)
}

func TestLastAccepted(t *testing.T) {
	require := require.New(t)

	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	last, err := engine.LastAccepted(context.Background())
	require.Error(err) // Returns ErrNotImplemented
	require.Nil(last)
}

func TestHealth(t *testing.T) {
	require := require.New(t)

	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	health, err := engine.Health(context.Background())
	require.NoError(err)
	require.NotNil(health)
	
	// Check health map contains expected fields
	healthMap, ok := health.(map[string]interface{})
	require.True(ok)
	require.Equal(StateInitializing, healthMap["state"])
	require.Equal("nebula", healthMap["engine"])
}

func TestTimeout(t *testing.T) {
	require := require.New(t)

	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	timeout := engine.Timeout()
	require.Equal(30*time.Second, timeout)
}

// Test Parameters validation
func TestParametersValid(t *testing.T) {
	tests := []struct {
		name      string
		params    Parameters
		wantError bool
		errMsg    string
	}{
		{
			name: "valid parameters",
			params: Parameters{
				K:                     21,
				AlphaPreference:       13,
				AlphaConfidence:       18,
				Beta:                  8,
				ConcurrentPolls:     4,
				OptimalProcessing:     10,
				MaxOutstandingItems:   256,
				MaxItemProcessingTime: 30 * time.Second,
				MaxParents:            5,
				ConflictSetSize:       10,
			},
			wantError: false,
		},
		{
			name: "invalid K",
			params: Parameters{
				K: 0,
			},
			wantError: true,
			errMsg:    "k must be positive",
		},
		{
			name: "alpha preference too low",
			params: Parameters{
				K:               10,
				AlphaPreference: 5,
			},
			wantError: true,
			errMsg:    "alpha preference must be greater than k/2",
		},
		{
			name: "alpha preference too high",
			params: Parameters{
				K:               10,
				AlphaPreference: 11,
			},
			wantError: true,
			errMsg:    "alpha preference must be less than or equal to k",
		},
		{
			name: "alpha confidence too low",
			params: Parameters{
				K:               10,
				AlphaPreference: 7,
				AlphaConfidence: 6,
			},
			wantError: true,
			errMsg:    "alpha confidence must be greater than or equal to alpha preference",
		},
		{
			name: "alpha confidence too high",
			params: Parameters{
				K:               10,
				AlphaPreference: 7,
				AlphaConfidence: 11,
			},
			wantError: true,
			errMsg:    "alpha confidence must be less than or equal to k",
		},
		{
			name: "invalid beta",
			params: Parameters{
				K:               10,
				AlphaPreference: 7,
				AlphaConfidence: 8,
				Beta:            0,
			},
			wantError: true,
			errMsg:    "beta must be positive",
		},
		{
			name: "invalid concurrent polls",
			params: Parameters{
				K:                 10,
				AlphaPreference:   7,
				AlphaConfidence:   8,
				Beta:              5,
				ConcurrentPolls: 0,
			},
			wantError: true,
			errMsg:    "concurrent polls must be positive",
		},
		{
			name: "invalid optimal processing",
			params: Parameters{
				K:                 10,
				AlphaPreference:   7,
				AlphaConfidence:   8,
				Beta:              5,
				ConcurrentPolls: 2,
				OptimalProcessing: 0,
			},
			wantError: true,
			errMsg:    "optimal processing must be positive",
		},
		{
			name: "invalid max outstanding items",
			params: Parameters{
				K:                   10,
				AlphaPreference:     7,
				AlphaConfidence:     8,
				Beta:                5,
				ConcurrentPolls:   2,
				OptimalProcessing:   5,
				MaxOutstandingItems: 0,
			},
			wantError: true,
			errMsg:    "max outstanding items must be positive",
		},
		{
			name: "invalid max item processing time",
			params: Parameters{
				K:                     10,
				AlphaPreference:       7,
				AlphaConfidence:       8,
				Beta:                  5,
				ConcurrentPolls:     2,
				OptimalProcessing:     5,
				MaxOutstandingItems:   10,
				MaxItemProcessingTime: 0,
			},
			wantError: true,
			errMsg:    "max item processing time must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			err := tt.params.Valid()
			if tt.wantError {
				require.Error(err)
				if tt.errMsg != "" {
					require.Contains(err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(err)
			}
		})
	}
}

func TestEngineLifecycle(t *testing.T) {
	require := require.New(t)

	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	// Initialize
	err := engine.Initialize(context.Background())
	require.NoError(err)
	require.Equal(StateRunning, engine.state)

	// Start
	err = engine.Start(context.Background())
	require.NoError(err)

	// Submit a decision
	decision := &mockDecision{
		id: ids.GenerateTestID(),
	}
	err = engine.Submit(context.Background(), decision)
	require.NoError(err)

	// Stop
	err = engine.Stop(context.Background())
	require.NoError(err)
	require.Equal(StateStopped, engine.state)

	// Try to start when stopped
	err = engine.Start(context.Background())
	require.Error(err)
}

func TestConcurrentOperations(t *testing.T) {
	require := require.New(t)

	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	// Initialize
	err := engine.Initialize(context.Background())
	require.NoError(err)

	// Start
	err = engine.Start(context.Background())
	require.NoError(err)

	// Concurrent submissions
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			decision := &mockDecision{
				id: ids.GenerateTestID(),
			}
			done <- engine.Submit(context.Background(), decision)
		}(i)
	}

	// Wait for all submissions
	for i := 0; i < 10; i++ {
		err := <-done
		require.NoError(err)
	}

	// Stop
	err = engine.Stop(context.Background())
	require.NoError(err)
}

func BenchmarkSubmit(b *testing.B) {
	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	decision := &mockDecision{
		id: ids.GenerateTestID(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.Submit(context.Background(), decision)
	}
}

func BenchmarkGetVertex(b *testing.B) {
	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	vtxID := ids.GenerateTestID()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.GetVertex(context.Background(), vtxID)
	}
}

func BenchmarkHealth(b *testing.B) {
	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Health(context.Background())
	}
}

func BenchmarkTimeout(b *testing.B) {
	ctx := &interfaces.Runtime{}
	engine := New(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.Timeout()
	}
}