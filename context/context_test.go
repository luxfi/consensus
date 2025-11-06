package context

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// Mock implementations for interfaces
type mockValidatorState struct {
	chainID       ids.ID
	netID         ids.ID
	subnetID      ids.ID
	height        uint64
	minHeight     uint64
	validatorSet  map[ids.NodeID]uint64
	shouldError   bool
}

func (m *mockValidatorState) GetChainID(id ids.ID) (ids.ID, error) {
	if m.shouldError {
		return ids.Empty, errors.New("mock error")
	}
	return m.chainID, nil
}

func (m *mockValidatorState) GetNetID(id ids.ID) (ids.ID, error) {
	if m.shouldError {
		return ids.Empty, errors.New("mock error")
	}
	return m.netID, nil
}

func (m *mockValidatorState) GetSubnetID(chainID ids.ID) (ids.ID, error) {
	if m.shouldError {
		return ids.Empty, errors.New("mock error")
	}
	return m.subnetID, nil
}

func (m *mockValidatorState) GetValidatorSet(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error) {
	if m.shouldError {
		return nil, errors.New("mock error")
	}
	return m.validatorSet, nil
}

func (m *mockValidatorState) GetCurrentHeight(ctx context.Context) (uint64, error) {
	if m.shouldError {
		return 0, errors.New("mock error")
	}
	return m.height, nil
}

func (m *mockValidatorState) GetMinimumHeight(ctx context.Context) (uint64, error) {
	if m.shouldError {
		return 0, errors.New("mock error")
	}
	return m.minHeight, nil
}

type mockKeystore struct {
	shouldError bool
}

func (m *mockKeystore) GetDatabase(username, password string) (interface{}, error) {
	if m.shouldError {
		return nil, errors.New("mock error")
	}
	return "mock database", nil
}

func (m *mockKeystore) NewAccount(username, password string) error {
	if m.shouldError {
		return errors.New("mock error")
	}
	return nil
}

type mockBlockchainIDLookup struct {
	lookupMap   map[string]ids.ID
	shouldError bool
}

func (m *mockBlockchainIDLookup) Lookup(alias string) (ids.ID, error) {
	if m.shouldError {
		return ids.Empty, errors.New("mock error")
	}
	if id, ok := m.lookupMap[alias]; ok {
		return id, nil
	}
	return ids.Empty, errors.New("not found")
}

type mockMetrics struct {
	registered  map[string]interface{}
	shouldError bool
}

func (m *mockMetrics) Register(namespace string, registerer interface{}) error {
	if m.shouldError {
		return errors.New("mock error")
	}
	if m.registered == nil {
		m.registered = make(map[string]interface{})
	}
	m.registered[namespace] = registerer
	return nil
}

func TestContext(t *testing.T) {
	// Create a test context
	testCtx := &Context{
		QuantumID:   1337,
		NetID:       ids.GenerateTestID(),
		ChainID:     ids.GenerateTestID(),
		NodeID:      ids.GenerateTestNodeID(),
		PublicKey:   []byte("test-public-key"),
		XChainID:    ids.GenerateTestID(),
		CChainID:    ids.GenerateTestID(),
		XAssetID: ids.GenerateTestID(),
		StartTime:   time.Now(),
	}

	// Test context fields
	require.Equal(t, uint32(1337), testCtx.QuantumID)
	require.NotEqual(t, ids.Empty, testCtx.NetID)
	require.NotEqual(t, ids.Empty, testCtx.ChainID)
	require.NotEqual(t, ids.EmptyNodeID, testCtx.NodeID)
	require.NotNil(t, testCtx.PublicKey)
}

func TestGetTimestamp(t *testing.T) {
	before := time.Now().Unix()
	timestamp := GetTimestamp()
	after := time.Now().Unix()

	require.GreaterOrEqual(t, timestamp, before)
	require.LessOrEqual(t, timestamp, after)
}

func TestContextFunctions(t *testing.T) {
	testCtx := &Context{
		QuantumID: 1337,
		NetID:     ids.GenerateTestID(),
		ChainID:   ids.GenerateTestID(),
		NodeID:    ids.GenerateTestNodeID(),
		PublicKey: []byte("test-key"),
		ValidatorState: &mockValidatorState{
			height: 100,
		},
	}

	t.Run("WithContext and FromContext", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithContext(ctx, testCtx)

		retrieved := FromContext(ctx)
		require.NotNil(t, retrieved)
		require.Equal(t, testCtx.QuantumID, retrieved.QuantumID)
		require.Equal(t, testCtx.ChainID, retrieved.ChainID)
		require.Equal(t, testCtx.NetID, retrieved.NetID)
	})

	t.Run("GetChainID", func(t *testing.T) {
		ctx := context.Background()

		// Without context
		chainID := GetChainID(ctx)
		require.Equal(t, ids.Empty, chainID)

		// With context
		ctx = WithContext(ctx, testCtx)
		chainID = GetChainID(ctx)
		require.Equal(t, testCtx.ChainID, chainID)
	})

	t.Run("GetNetID", func(t *testing.T) {
		ctx := context.Background()

		// Without context
		netID := GetNetID(ctx)
		require.Equal(t, ids.Empty, netID)

		// With context
		ctx = WithContext(ctx, testCtx)
		netID = GetNetID(ctx)
		require.Equal(t, testCtx.NetID, netID)
	})

	t.Run("GetSubnetID", func(t *testing.T) {
		ctx := context.Background()

		// Without context - should return empty
		subnetID := GetSubnetID(ctx)
		require.Equal(t, ids.Empty, subnetID)

		// With context - should return NetID (deprecated function)
		ctx = WithContext(ctx, testCtx)
		subnetID = GetSubnetID(ctx)
		require.Equal(t, testCtx.NetID, subnetID)
	})

	t.Run("GetNetworkID", func(t *testing.T) {
		ctx := context.Background()

		// Without context
		networkID := GetNetworkID(ctx)
		require.Equal(t, uint32(0), networkID)

		// With context
		ctx = WithContext(ctx, testCtx)
		networkID = GetNetworkID(ctx)
		require.Equal(t, testCtx.QuantumID, networkID)
	})

	t.Run("GetNodeID", func(t *testing.T) {
		ctx := context.Background()

		// Without context
		nodeID := GetNodeID(ctx)
		require.Equal(t, ids.EmptyNodeID, nodeID)

		// With context
		ctx = WithContext(ctx, testCtx)
		nodeID = GetNodeID(ctx)
		require.Equal(t, testCtx.NodeID, nodeID)
	})

	t.Run("GetValidatorState", func(t *testing.T) {
		ctx := context.Background()

		// Without context
		vs := GetValidatorState(ctx)
		require.Nil(t, vs)

		// With context
		ctx = WithContext(ctx, testCtx)
		vs = GetValidatorState(ctx)
		require.NotNil(t, vs)
		require.Equal(t, testCtx.ValidatorState, vs)
	})
}

func TestWithIDs(t *testing.T) {
	ctx := context.Background()

	testIDs := IDs{
		NetworkID:  1,
		QuantumID:  1337,
		NetID:      ids.GenerateTestID(),
		ChainID:    ids.GenerateTestID(),
		NodeID:     ids.GenerateTestNodeID(),
		PublicKey:  []byte("test-key"),
		XAssetID: ids.GenerateTestID(),
	}

	t.Run("WithIDs on empty context", func(t *testing.T) {
		newCtx := WithIDs(ctx, testIDs)

		c := FromContext(newCtx)
		require.NotNil(t, c)
		require.Equal(t, testIDs.QuantumID, c.QuantumID)
		require.Equal(t, testIDs.NetID, c.NetID)
		require.Equal(t, testIDs.ChainID, c.ChainID)
		require.Equal(t, testIDs.NodeID, c.NodeID)
		require.Equal(t, testIDs.PublicKey, c.PublicKey)
	})

	t.Run("WithIDs on existing context", func(t *testing.T) {
		// Start with a context that has some values
		initialCtx := &Context{
			QuantumID: 999,
			NetID:     ids.GenerateTestID(),
		}
		ctx = WithContext(ctx, initialCtx)

		// Update with new IDs
		newCtx := WithIDs(ctx, testIDs)

		c := FromContext(newCtx)
		require.NotNil(t, c)
		// Values should be updated
		require.Equal(t, testIDs.QuantumID, c.QuantumID)
		require.Equal(t, testIDs.NetID, c.NetID)
		require.Equal(t, testIDs.ChainID, c.ChainID)
	})
}

func TestWithValidatorState(t *testing.T) {
	ctx := context.Background()

	mockVS := &mockValidatorState{
		height:    100,
		minHeight: 10,
		validatorSet: map[ids.NodeID]uint64{
			ids.GenerateTestNodeID(): 1000,
		},
	}

	t.Run("WithValidatorState on empty context", func(t *testing.T) {
		newCtx := WithValidatorState(ctx, mockVS)

		vs := GetValidatorState(newCtx)
		require.NotNil(t, vs)
		require.Equal(t, mockVS, vs)
	})

	t.Run("WithValidatorState on existing context", func(t *testing.T) {
		// Start with a context that has some values
		initialCtx := &Context{
			QuantumID: 1337,
			ChainID:   ids.GenerateTestID(),
		}
		ctx = WithContext(ctx, initialCtx)

		// Add validator state
		newCtx := WithValidatorState(ctx, mockVS)

		c := FromContext(newCtx)
		require.NotNil(t, c)
		// Original values should be preserved
		require.Equal(t, uint32(1337), c.QuantumID)
		require.NotEqual(t, ids.Empty, c.ChainID)
		// Validator state should be added
		require.Equal(t, mockVS, c.ValidatorState)
	})
}

func TestValidatorStateInterface(t *testing.T) {
	mockVS := &mockValidatorState{
		chainID:      ids.GenerateTestID(),
		netID:        ids.GenerateTestID(),
		subnetID:     ids.GenerateTestID(),
		height:       100,
		minHeight:    10,
		validatorSet: map[ids.NodeID]uint64{
			ids.GenerateTestNodeID(): 1000,
			ids.GenerateTestNodeID(): 2000,
		},
	}

	t.Run("GetChainID", func(t *testing.T) {
		id, err := mockVS.GetChainID(ids.GenerateTestID())
		require.NoError(t, err)
		require.Equal(t, mockVS.chainID, id)

		// Test error case
		mockVS.shouldError = true
		_, err = mockVS.GetChainID(ids.GenerateTestID())
		require.Error(t, err)
		mockVS.shouldError = false
	})

	t.Run("GetNetID", func(t *testing.T) {
		id, err := mockVS.GetNetID(ids.GenerateTestID())
		require.NoError(t, err)
		require.Equal(t, mockVS.netID, id)
	})

	t.Run("GetSubnetID", func(t *testing.T) {
		id, err := mockVS.GetSubnetID(ids.GenerateTestID())
		require.NoError(t, err)
		require.Equal(t, mockVS.subnetID, id)
	})

	t.Run("GetValidatorSet", func(t *testing.T) {
		set, err := mockVS.GetValidatorSet(100, ids.GenerateTestID())
		require.NoError(t, err)
		require.Len(t, set, 2)
	})

	t.Run("GetCurrentHeight", func(t *testing.T) {
		ctx := context.Background()
		height, err := mockVS.GetCurrentHeight(ctx)
		require.NoError(t, err)
		require.Equal(t, uint64(100), height)
	})

	t.Run("GetMinimumHeight", func(t *testing.T) {
		height, err := mockVS.GetMinimumHeight(context.Background())
		require.NoError(t, err)
		require.Equal(t, uint64(10), height)
	})
}

func TestKeystoreInterface(t *testing.T) {
	mockKS := &mockKeystore{}

	t.Run("GetDatabase", func(t *testing.T) {
		db, err := mockKS.GetDatabase("user", "pass")
		require.NoError(t, err)
		require.Equal(t, "mock database", db)

		// Test error case
		mockKS.shouldError = true
		_, err = mockKS.GetDatabase("user", "pass")
		require.Error(t, err)
		mockKS.shouldError = false
	})

	t.Run("NewAccount", func(t *testing.T) {
		err := mockKS.NewAccount("user", "pass")
		require.NoError(t, err)

		// Test error case
		mockKS.shouldError = true
		err = mockKS.NewAccount("user", "pass")
		require.Error(t, err)
		mockKS.shouldError = false
	})
}

func TestBlockchainIDLookupInterface(t *testing.T) {
	mockLookup := &mockBlockchainIDLookup{
		lookupMap: map[string]ids.ID{
			"chain1": ids.GenerateTestID(),
			"chain2": ids.GenerateTestID(),
		},
	}

	t.Run("Lookup success", func(t *testing.T) {
		id, err := mockLookup.Lookup("chain1")
		require.NoError(t, err)
		require.NotEqual(t, ids.Empty, id)
	})

	t.Run("Lookup not found", func(t *testing.T) {
		_, err := mockLookup.Lookup("unknown")
		require.Error(t, err)
	})

	t.Run("Lookup error", func(t *testing.T) {
		mockLookup.shouldError = true
		_, err := mockLookup.Lookup("chain1")
		require.Error(t, err)
		mockLookup.shouldError = false
	})
}

func TestMetricsInterface(t *testing.T) {
	mockM := &mockMetrics{}

	t.Run("Register", func(t *testing.T) {
		err := mockM.Register("namespace1", "registerer1")
		require.NoError(t, err)
		require.Equal(t, "registerer1", mockM.registered["namespace1"])

		// Register another
		err = mockM.Register("namespace2", "registerer2")
		require.NoError(t, err)
		require.Len(t, mockM.registered, 2)

		// Test error case
		mockM.shouldError = true
		err = mockM.Register("namespace3", "registerer3")
		require.Error(t, err)
		mockM.shouldError = false
	})
}

func TestGetValidatorOutput(t *testing.T) {
	output := GetValidatorOutput{
		NodeID:    ids.GenerateTestNodeID(),
		PublicKey: []byte("public-key"),
		Weight:    1000,
	}

	require.NotEqual(t, ids.EmptyNodeID, output.NodeID)
	require.NotNil(t, output.PublicKey)
	require.Equal(t, uint64(1000), output.Weight)
}

func TestContextWithAllFields(t *testing.T) {
	// Create a fully populated context
	fullCtx := &Context{
		QuantumID:   1337,
		NetID:       ids.GenerateTestID(),
		ChainID:     ids.GenerateTestID(),
		NodeID:      ids.GenerateTestNodeID(),
		PublicKey:   []byte("test-public-key"),
		XChainID:    ids.GenerateTestID(),
		CChainID:    ids.GenerateTestID(),
		XAssetID: ids.GenerateTestID(),
		StartTime:   time.Now(),
		ValidatorState: &mockValidatorState{
			height: 100,
		},
		Keystore: &mockKeystore{},
		Metrics: &mockMetrics{},
	}

	ctx := context.Background()
	ctx = WithContext(ctx, fullCtx)

	// Retrieve and verify all fields
	retrieved := FromContext(ctx)
	require.NotNil(t, retrieved)
	require.Equal(t, fullCtx.QuantumID, retrieved.QuantumID)
	require.Equal(t, fullCtx.NetID, retrieved.NetID)
	require.Equal(t, fullCtx.ChainID, retrieved.ChainID)
	require.Equal(t, fullCtx.NodeID, retrieved.NodeID)
	require.Equal(t, fullCtx.PublicKey, retrieved.PublicKey)
	require.Equal(t, fullCtx.XChainID, retrieved.XChainID)
	require.Equal(t, fullCtx.CChainID, retrieved.CChainID)
	require.Equal(t, fullCtx.XAssetID, retrieved.XAssetID)
	require.Equal(t, fullCtx.StartTime, retrieved.StartTime)
	require.NotNil(t, retrieved.ValidatorState)
	require.NotNil(t, retrieved.Keystore)
	require.NotNil(t, retrieved.Metrics)
}

func BenchmarkGetChainID(b *testing.B) {
	ctx := context.Background()
	testCtx := &Context{
		ChainID: ids.GenerateTestID(),
	}
	ctx = WithContext(ctx, testCtx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetChainID(ctx)
	}
}

func BenchmarkWithContext(b *testing.B) {
	ctx := context.Background()
	testCtx := &Context{
		QuantumID: 1337,
		ChainID:   ids.GenerateTestID(),
		NetID:     ids.GenerateTestID(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WithContext(ctx, testCtx)
	}
}

func BenchmarkFromContext(b *testing.B) {
	ctx := context.Background()
	testCtx := &Context{
		QuantumID: 1337,
		ChainID:   ids.GenerateTestID(),
	}
	ctx = WithContext(ctx, testCtx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FromContext(ctx)
	}
}