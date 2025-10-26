package consensus

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewChainEngine(t *testing.T) {
	engine := NewChainEngine()
	require.NotNil(t, engine)

	// Test Engine interface methods
	ctx := context.Background()
	err := engine.Start(ctx, 1)
	require.NoError(t, err)

	err = engine.Stop(ctx)
	require.NoError(t, err)
}

func TestNewDAGEngine(t *testing.T) {
	engine := NewDAGEngine()
	require.NotNil(t, engine)

	// Test Engine interface methods
	ctx := context.Background()
	err := engine.Start(ctx, 1)
	require.NoError(t, err)

	err = engine.Stop(ctx)
	require.NoError(t, err)
}

func TestNewPQEngine(t *testing.T) {
	engine := NewPQEngine()
	require.NotNil(t, engine)

	// Test Engine interface methods
	ctx := context.Background()
	err := engine.Start(ctx, 1)
	require.NoError(t, err)

	err = engine.Stop(ctx)
	require.NoError(t, err)
}

func TestEngineHealthCheck(t *testing.T) {
	engines := []Engine{
		NewChainEngine(),
		NewDAGEngine(),
		NewPQEngine(),
	}

	ctx := context.Background()

	for _, engine := range engines {
		// Start the engine
		err := engine.Start(ctx, 1)
		require.NoError(t, err)

		// Test IsBootstrapped after start
		bootstrapped := engine.IsBootstrapped()
		require.True(t, bootstrapped)

		// Test HealthCheck
		health, err := engine.HealthCheck(ctx)
		require.NoError(t, err)
		require.NotNil(t, health)

		// Stop the engine
		err = engine.Stop(ctx)
		require.NoError(t, err)
	}
}

func TestConfig(t *testing.T) {
	tests := []struct {
		name  string
		nodes int
	}{
		{
			name:  "local config (5 nodes)",
			nodes: 5,
		},
		{
			name:  "testnet config (11 nodes)",
			nodes: 11,
		},
		{
			name:  "mainnet config (21 nodes)",
			nodes: 21,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config(tt.nodes)
			require.NotNil(t, cfg)
		})
	}
}
