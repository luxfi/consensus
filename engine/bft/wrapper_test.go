// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bft

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBFTWrapper(t *testing.T) {
	cfg := Config{
		NodeID:      "test-node",
		Validators:  []string{"val1", "val2", "val3"},
		EpochLength: 100,
	}

	engine, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, engine)

	ctx := context.Background()

	// Test start
	err = engine.Start(ctx, 0)
	require.NoError(t, err)

	// Test health check
	health, err := engine.HealthCheck(ctx)
	require.NoError(t, err)
	require.NotNil(t, health)

	// Test bootstrap status
	require.True(t, engine.IsBootstrapped())

	// Test Simplex access
	simplex := engine.GetSimplex()
	require.NotNil(t, simplex, "Should be able to access underlying Simplex engine")

	// Test stop
	err = engine.Stop(ctx)
	require.NoError(t, err)
}

func TestBFTConfig(t *testing.T) {
	tests := []struct {
		name       string
		config     Config
		shouldWork bool
	}{
		{
			name: "valid config",
			config: Config{
				NodeID:      "node1",
				Validators:  []string{"v1", "v2", "v3", "v4"},
				EpochLength: 100,
			},
			shouldWork: true,
		},
		{
			name: "single validator",
			config: Config{
				NodeID:      "node1",
				Validators:  []string{"v1"},
				EpochLength: 10,
			},
			shouldWork: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := New(tt.config)
			if tt.shouldWork {
				require.NoError(t, err)
				require.NotNil(t, engine)
			}
		})
	}
}
