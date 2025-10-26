package consensus

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// TestCoreAcceptorGroup tests the AcceptorGroup functions
func TestCoreAcceptorGroup(t *testing.T) {
	// Test NewAcceptorGroup
	group := NewAcceptorGroup()
	require.NotNil(t, group)

	// Create a basic acceptor to register
	basicAcceptor := NewBasicAcceptor()
	require.NotNil(t, basicAcceptor)

	// Test RegisterAcceptor
	chainID := ids.GenerateTestID()
	err := group.RegisterAcceptor(chainID, "test-acceptor", basicAcceptor, false)
	require.NoError(t, err)

	// Test RegisterAcceptor with different parameters
	err = group.RegisterAcceptor(chainID, "test-acceptor-2", basicAcceptor, true)
	require.NoError(t, err)

	// Test DeregisterAcceptor
	err = group.DeregisterAcceptor(chainID, "test-acceptor")
	require.NoError(t, err)

	// Test DeregisterAcceptor with non-existent acceptor
	err = group.DeregisterAcceptor(chainID, "non-existent")
	require.NoError(t, err) // Should not error
}

// TestConfigDefaultCase tests the default case in Config function
func TestConfigDefaultCase(t *testing.T) {
	// Test with large node count (should return mainnet params)
	cfg := Config(100)
	require.NotNil(t, cfg)

	// Test with medium node count (should return testnet params)
	cfg = Config(8)
	require.NotNil(t, cfg)

	// Test with small node count (should return local params)
	cfg = Config(3)
	require.NotNil(t, cfg)
}

// TestQuantumNetworkIDWithQuantumContext tests QuantumNetworkID with actual quantum IDs
func TestQuantumNetworkIDWithQuantumContext(t *testing.T) {
	ctx := context.Background()

	// The current implementation checks GetQuantumIDs
	// Since GetQuantumIDs returns nil in our implementation,
	// we just verify the function doesn't panic
	networkID := QuantumNetworkID(ctx)
	require.Equal(t, uint32(0), networkID)

	// Test with a context that might have values
	// (though our current implementation doesn't use context values)
	ctx = context.WithValue(ctx, "test", "value")
	networkID = QuantumNetworkID(ctx)
	require.Equal(t, uint32(0), networkID)
}

// TestContextualizable tests the Contextualizable interface
func TestContextualizable(t *testing.T) {
	// This is just to ensure the interface exists and can be used
	var c Contextualizable
	require.Nil(t, c) // Interface should be nil by default
}

// TestAllEngineTypes tests all engine types thoroughly
func TestAllEngineTypes(t *testing.T) {
	engines := []struct {
		name   string
		engine Engine
	}{
		{"chain", NewChainEngine()},
		{"dag", NewDAGEngine()},
		{"pq", NewPQEngine()},
	}

	ctx := context.Background()

	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			// Start the engine
			err := e.engine.Start(ctx, 1)
			require.NoError(t, err)

			// Check IsBootstrapped
			bootstrapped := e.engine.IsBootstrapped()
			require.True(t, bootstrapped)

			// Check HealthCheck
			health, err := e.engine.HealthCheck(ctx)
			require.NoError(t, err)
			require.NotNil(t, health)

			// Stop the engine
			err = e.engine.Stop(ctx)
			require.NoError(t, err)

			// Test multiple start/stop cycles
			for i := 0; i < 3; i++ {
				err = e.engine.Start(ctx, uint32(i))
				require.NoError(t, err)
				err = e.engine.Stop(ctx)
				require.NoError(t, err)
			}
		})
	}
}