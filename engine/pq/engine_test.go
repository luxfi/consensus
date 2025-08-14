// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quantum

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/engine/dag"
	"github.com/luxfi/consensus/protocols/nova"
	"github.com/luxfi/ids"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		params  Parameters
		wantErr bool
	}{
		{
			name: "valid parameters",
			params: Parameters{
				MaxOrbits:              10,
				MaxGalaxies:            10,
				MaxBridges:             20,
				CrossChainTimeout:      30 * time.Second,
				ConsensusThreshold:     0.67,
				InterEngineBatchSize:   100,
				QuantumSecurityLevel:   128,
				UnifiedConsensusMode:   quasar.EngineMode(1),
			},
			wantErr: false,
		},
		{
			name: "empty parameters",
			params:  Parameters{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			ctx := &interfaces.Context{}
			engine, err := New(ctx, tt.params)
			if tt.wantErr {
				require.Error(err)
				return
			}

			require.NoError(err)
			require.NotNil(engine)
			require.Equal(tt.params, engine.params)
			require.NotNil(engine.state)
			require.NotNil(engine.chains)
			require.NotNil(engine.dags)
			require.NotNil(engine.bridges)
			require.NotNil(engine.coordinator)
			require.NotNil(engine.metrics)
		})
	}
}

func TestStartStop(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		MaxOrbits:            10,
		MaxGalaxies:          10,
		MaxBridges:           20,
		ConsensusThreshold:   0.67,
		InterEngineBatchSize: 100,
		QuantumSecurityLevel: 128,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	// Test Start
	err = engine.Start(context.Background())
	require.NoError(err)
	require.True(engine.IsRunning())

	// Test double start
	err = engine.Start(context.Background())
	require.Error(err)
	require.Contains(err.Error(), "engine already running")

	// Test Stop
	err = engine.Stop(context.Background())
	require.NoError(err)
	require.False(engine.IsRunning())

	// Test double stop
	err = engine.Stop(context.Background())
	require.Error(err)
	require.Contains(err.Error(), "engine not running")
}

func TestCreateChain(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		MaxOrbits:            3,
		MaxGalaxies:          10,
		MaxBridges:           20,
		ConsensusThreshold:   0.67,
		InterEngineBatchSize: 100,
		QuantumSecurityLevel: 128,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	// Create a chain
	err = engine.CreateChain("chain1", Parameters{})
	require.NoError(err)
	require.Equal(1, engine.OrbitCount())

	// Try to create chain with same name
	err = engine.CreateChain("chain1", Parameters{})
	require.Error(err)
	require.Contains(err.Error(), "chain chain1 already exists")

	// Create more chains
	err = engine.CreateChain("chain2", Parameters{})
	require.NoError(err)
	err = engine.CreateChain("chain3", Parameters{})
	require.NoError(err)

	// Try to exceed max chains
	err = engine.CreateChain("chain4", Parameters{})
	require.Error(err)
	require.Contains(err.Error(), "maximum orbits reached")
}

func TestCreateDAG(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		MaxOrbits:            10,
		MaxGalaxies:          3,
		MaxBridges:           20,
		ConsensusThreshold:   0.67,
		InterEngineBatchSize: 100,
		QuantumSecurityLevel: 128,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	// Create a DAG
	dagParams := dag.Parameters{
		K:               21,
		AlphaPreference: 13,
		AlphaConfidence: 18,
		Beta:            8,
		MaxParents:      5,
	}
	err = engine.CreateDAG("dag1", dagParams)
	require.NoError(err)
	require.Equal(1, engine.GalaxyCount())

	// Try to create DAG with same name
	err = engine.CreateDAG("dag1", dagParams)
	require.Error(err)
	require.Contains(err.Error(), "dag dag1 already exists")

	// Create more DAGs
	err = engine.CreateDAG("dag2", dagParams)
	require.NoError(err)
	err = engine.CreateDAG("dag3", dagParams)
	require.NoError(err)

	// Try to exceed max DAGs
	err = engine.CreateDAG("dag4", dagParams)
	require.Error(err)
	require.Contains(err.Error(), "maximum galaxies reached")
}

func TestCreateBridge(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		MaxOrbits:            10,
		MaxGalaxies:          10,
		MaxBridges:           2,
		ConsensusThreshold:   0.67,
		InterEngineBatchSize: 100,
		QuantumSecurityLevel: 128,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	// Create bridges
	bridgeParams1 := BridgeParams{
		SourceEngine:  "chain1",
		DestEngine:    "dag1",
		Bidirectional: true,
		BatchSize:     10,
		Timeout:       5 * time.Second,
	}
	bridgeID1, err := engine.CreateBridge(bridgeParams1)
	require.NoError(err)
	require.Equal("chain1", bridgeID1.Source)
	require.Equal("dag1", bridgeID1.Dest)
	require.Equal(1, engine.BridgeCount())

	// Create another bridge
	bridgeParams2 := BridgeParams{
		SourceEngine:  "chain2",
		DestEngine:    "dag2",
		Bidirectional: false,
		BatchSize:     20,
		Timeout:       10 * time.Second,
	}
	bridgeID2, err := engine.CreateBridge(bridgeParams2)
	require.NoError(err)
	require.NotEqual(bridgeID1, bridgeID2)

	// Try to exceed max bridges
	bridgeParams3 := BridgeParams{
		SourceEngine: "chain3",
		DestEngine:   "dag3",
	}
	_, err = engine.CreateBridge(bridgeParams3)
	require.Error(err)
	require.Contains(err.Error(), "maximum bridges reached")
}

func TestSubmitCrossEngineTransaction(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		MaxOrbits:            10,
		MaxGalaxies:          10,
		MaxBridges:           20,
		ConsensusThreshold:   0.67,
		InterEngineBatchSize: 100,
		QuantumSecurityLevel: 128,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	// Start engine
	err = engine.Start(context.Background())
	require.NoError(err)

	// Create chains and DAGs
	err = engine.CreateChain("chain1", Parameters{})
	require.NoError(err)
	err = engine.CreateDAG("dag1", dag.Parameters{})
	require.NoError(err)

	// Submit cross-engine transaction
	tx := CrossEngineTx{
		ID:           ids.GenerateTestID(),
		SourceType:   EngineTypeOrbit,
		SourceEngine: "chain1",
		DestType:     EngineTypeGalaxy,
		DestEngine:   "dag1",
		Payload:      []byte("test transaction"),
		Timestamp:    time.Now(),
	}

	err = engine.SubmitCrossEngineTransaction(context.Background(), tx)
	require.NoError(err)

	// Test with non-existent source
	txBadSource := CrossEngineTx{
		ID:           ids.GenerateTestID(),
		SourceType:   EngineTypeOrbit,
		SourceEngine: "nonexistent",
		DestType:     EngineTypeGalaxy,
		DestEngine:   "dag1",
	}
	err = engine.SubmitCrossEngineTransaction(context.Background(), txBadSource)
	require.Error(err)
	require.Contains(err.Error(), "source chain nonexistent not found")

	// Test with non-existent destination
	txBadDest := CrossEngineTx{
		ID:           ids.GenerateTestID(),
		SourceType:   EngineTypeOrbit,
		SourceEngine: "chain1",
		DestType:     EngineTypeGalaxy,
		DestEngine:   "nonexistent",
	}
	err = engine.SubmitCrossEngineTransaction(context.Background(), txBadDest)
	require.Error(err)
	require.Contains(err.Error(), "destination dag nonexistent not found")

	// Test when engine not running
	err = engine.Stop(context.Background())
	require.NoError(err)

	err = engine.SubmitCrossEngineTransaction(context.Background(), tx)
	require.Error(err)
	require.Contains(err.Error(), "engine not running")
}

func TestGetUniversalState(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		MaxOrbits:            10,
		MaxGalaxies:          10,
		MaxBridges:           20,
		ConsensusThreshold:   0.67,
		InterEngineBatchSize: 100,
		QuantumSecurityLevel: 128,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	// Get initial state
	state := engine.GetUniversalState()
	require.Equal(0, state.OrbitCount)
	require.Equal(0, state.GalaxyCount)
	require.Equal(0, state.BridgeCount)
	require.Equal(uint64(0), state.CrossTxProcessed)

	// Start engine
	err = engine.Start(context.Background())
	require.NoError(err)

	// Create some chains and DAGs
	err = engine.CreateChain("chain1", Parameters{})
	require.NoError(err)
	err = engine.CreateDAG("dag1", dag.Parameters{})
	require.NoError(err)

	// Create a bridge
	bridgeParams := BridgeParams{
		SourceEngine: "chain1",
		DestEngine:   "dag1",
	}
	_, err = engine.CreateBridge(bridgeParams)
	require.NoError(err)

	// Get updated state
	state = engine.GetUniversalState()
	require.Equal(1, state.OrbitCount)
	require.Equal(1, state.GalaxyCount)
	require.Equal(1, state.BridgeCount)
	require.NotZero(state.Uptime)
}

func TestMetrics(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		MaxOrbits:            10,
		MaxGalaxies:          10,
		MaxBridges:           20,
		ConsensusThreshold:   0.67,
		InterEngineBatchSize: 100,
		QuantumSecurityLevel: 128,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	metrics := engine.Metrics()
	require.NotNil(metrics)
	require.NotNil(metrics.CrossEngineTx)
	require.NotNil(metrics.BridgeOperations)
}

func TestConsensusBridge(t *testing.T) {
	require := require.New(t)

	params := BridgeParams{
		SourceEngine:  "source",
		DestEngine:    "dest",
		Bidirectional: true,
		BatchSize:     10,
		Timeout:       5 * time.Second,
	}

	bridge := NewConsensusBridge(params)
	require.NotNil(bridge)

	id := bridge.ID()
	require.Equal("source", id.Source)
	require.Equal("dest", id.Dest)
}

func TestUniversalCoordinator(t *testing.T) {
	require := require.New(t)

	params := CoordinatorParams{
		ConsensusThreshold: 0.67,
		BatchSize:          100,
	}

	coordinator := NewUniversalCoordinator(params)
	require.NotNil(coordinator)

	// Test Start/Stop
	err := coordinator.Start(context.Background())
	require.NoError(err)

	err = coordinator.Stop(context.Background())
	require.NoError(err)

	// Test RegisterBridge
	bridge := NewConsensusBridge(BridgeParams{
		SourceEngine: "test1",
		DestEngine:   "test2",
	})
	coordinator.RegisterBridge(bridge)

	// Test ProcessTransaction
	tx := CrossEngineTx{
		ID: ids.GenerateTestID(),
	}
	err = coordinator.ProcessTransaction(context.Background(), tx)
	require.NoError(err)
}

func TestEngineTypes(t *testing.T) {
	require := require.New(t)

	// Test EngineType constants
	require.Equal(EngineType(0), EngineTypeOrbit)
	require.Equal(EngineType(1), EngineTypeGalaxy)

	// Test CrossEngineTx Verify
	tx := CrossEngineTx{
		ID:           ids.GenerateTestID(),
		SourceType:   EngineTypeOrbit,
		SourceEngine: "chain1",
		DestType:     EngineTypeGalaxy,
		DestEngine:   "dag1",
		Payload:      []byte("test"),
		Timestamp:    time.Now(),
	}
	err := tx.Verify()
	require.NoError(err)
}

func TestStopWithSubEngines(t *testing.T) {
	require := require.New(t)

	params := Parameters{
		MaxOrbits:            10,
		MaxGalaxies:          10,
		MaxBridges:           20,
		ConsensusThreshold:   0.67,
		InterEngineBatchSize: 100,
		QuantumSecurityLevel: 128,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(err)

	// Start engine
	err = engine.Start(context.Background())
	require.NoError(err)

	// Create sub-engines
	err = engine.CreateChain("chain1", Parameters{})
	require.NoError(err)
	err = engine.CreateDAG("dag1", dag.Parameters{})
	require.NoError(err)

	// Stop should stop all sub-engines
	err = engine.Stop(context.Background())
	require.NoError(err)
}


func BenchmarkCreateChain(b *testing.B) {
	params := Parameters{
		MaxOrbits:            1000,
		MaxGalaxies:          1000,
		MaxBridges:           2000,
		ConsensusThreshold:   0.67,
		InterEngineBatchSize: 100,
		QuantumSecurityLevel: 128,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("chain%d", i)
		_ = engine.CreateChain(name, Parameters{})
	}
}

func BenchmarkSubmitCrossEngineTransaction(b *testing.B) {
	params := Parameters{
		MaxOrbits:            10,
		MaxGalaxies:          10,
		MaxBridges:           20,
		ConsensusThreshold:   0.67,
		InterEngineBatchSize: 100,
		QuantumSecurityLevel: 128,
	}

	ctx := &interfaces.Context{}
	engine, err := New(ctx, params)
	require.NoError(b, err)

	_ = engine.Start(context.Background())
	_ = engine.CreateChain("chain1", Parameters{})
	_ = engine.CreateDAG("dag1", dag.Parameters{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx := CrossEngineTx{
			ID:           ids.GenerateTestID(),
			SourceType:   EngineTypeOrbit,
			SourceEngine: "chain1",
			DestType:     EngineTypeGalaxy,
			DestEngine:   "dag1",
			Payload:      []byte("benchmark transaction"),
			Timestamp:    time.Now(),
		}
		_ = engine.SubmitCrossEngineTransaction(context.Background(), tx)
	}
}

func TestEngineState(t *testing.T) {
	require := require.New(t)

	state := newEngineState()
	require.NotNil(state)
	require.False(state.running)
	require.Zero(state.orbitCount)
	require.Zero(state.galaxyCount)
	require.Zero(state.bridgeCount)
	require.Zero(state.crossTxProcessed)

	// Test state mutations
	state.running = true
	state.startTime = time.Now()
	state.orbitCount = 5
	state.galaxyCount = 3
	state.bridgeCount = 8
	state.crossTxProcessed = 100

	require.True(state.running)
	require.NotZero(state.startTime)
	require.Equal(5, state.orbitCount)
	require.Equal(3, state.galaxyCount)
	require.Equal(8, state.bridgeCount)
	require.Equal(uint64(100), state.crossTxProcessed)
}

func TestNewMetrics(t *testing.T) {
	require := require.New(t)

	metrics := NewMetrics()
	require.NotNil(metrics)

	// Test counter increments
	initialCount := metrics.CrossEngineTx.count
	metrics.CrossEngineTx.Inc()
	require.Equal(initialCount+1, metrics.CrossEngineTx.count)

	metrics.BridgeOperations.Inc()
	metrics.BridgeOperations.Inc()
	require.Equal(int64(2), metrics.BridgeOperations.count)
}