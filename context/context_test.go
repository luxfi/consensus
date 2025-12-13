package context

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestContext(t *testing.T) {
	ctx := &Context{}

	// Test that context can be created
	require.NotNil(t, ctx)
}

func TestGetTimestamp(t *testing.T) {
	before := time.Now().Unix()
	ts := GetTimestamp()
	after := time.Now().Unix()

	require.GreaterOrEqual(t, ts, before)
	require.LessOrEqual(t, ts, after)
}

func TestGetChainID(t *testing.T) {
	// Test with nil context
	ctx := context.Background()
	result := GetChainID(ctx)
	require.Equal(t, ids.Empty, result)

	// Test with valid context
	chainID := ids.GenerateTestID()
	cc := &Context{ChainID: chainID}
	ctx = WithContext(ctx, cc)
	result = GetChainID(ctx)
	require.Equal(t, chainID, result)
}

func TestGetNetID(t *testing.T) {
	// Test with nil context
	ctx := context.Background()
	result := GetNetID(ctx)
	require.Equal(t, ids.Empty, result)

	// Test with valid context
	netID := ids.GenerateTestID()
	cc := &Context{NetID: netID}
	ctx = WithContext(ctx, cc)
	result = GetNetID(ctx)
	require.Equal(t, netID, result)
}

func TestGetSubnetID(t *testing.T) {
	// Test with nil context
	ctx := context.Background()
	result := GetSubnetID(ctx)
	require.Equal(t, ids.Empty, result)

	// Test with valid context - should return NetID
	netID := ids.GenerateTestID()
	cc := &Context{NetID: netID}
	ctx = WithContext(ctx, cc)
	result = GetSubnetID(ctx)
	require.Equal(t, netID, result)
}

func TestGetNetworkID(t *testing.T) {
	// Test with nil context
	ctx := context.Background()
	result := GetNetworkID(ctx)
	require.Equal(t, uint32(0), result)

	// Test with valid context
	cc := &Context{QuantumID: 12345}
	ctx = WithContext(ctx, cc)
	result = GetNetworkID(ctx)
	require.Equal(t, uint32(12345), result)
}

// mockValidatorState implements ValidatorState for testing
type mockValidatorState struct{}

func (m *mockValidatorState) GetChainID(ids.ID) (ids.ID, error)                { return ids.Empty, nil }
func (m *mockValidatorState) GetNetID(ids.ID) (ids.ID, error)                  { return ids.Empty, nil }
func (m *mockValidatorState) GetSubnetID(chainID ids.ID) (ids.ID, error)       { return ids.Empty, nil }
func (m *mockValidatorState) GetValidatorSet(uint64, ids.ID) (map[ids.NodeID]uint64, error) {
	return nil, nil
}
func (m *mockValidatorState) GetCurrentHeight(context.Context) (uint64, error) { return 0, nil }
func (m *mockValidatorState) GetMinimumHeight(context.Context) (uint64, error) { return 0, nil }

func TestGetValidatorState(t *testing.T) {
	// Test with nil context
	ctx := context.Background()
	result := GetValidatorState(ctx)
	require.Nil(t, result)

	// Test with valid context
	vs := &mockValidatorState{}
	cc := &Context{ValidatorState: vs}
	ctx = WithContext(ctx, cc)
	result = GetValidatorState(ctx)
	require.Equal(t, vs, result)
}

func TestGetWarpSigner(t *testing.T) {
	// Test with nil context
	ctx := context.Background()
	result := GetWarpSigner(ctx)
	require.Nil(t, result)

	// Test with valid context
	signer := struct{}{}
	cc := &Context{WarpSigner: signer}
	ctx = WithContext(ctx, cc)
	result = GetWarpSigner(ctx)
	require.Equal(t, signer, result)
}

func TestWithContext_FromContext(t *testing.T) {
	// Test round-trip
	cc := &Context{
		QuantumID: 1,
		ChainID:   ids.GenerateTestID(),
		NetID:     ids.GenerateTestID(),
	}

	ctx := context.Background()
	ctx = WithContext(ctx, cc)

	result := FromContext(ctx)
	require.Equal(t, cc, result)
	require.Equal(t, cc.QuantumID, result.QuantumID)
	require.Equal(t, cc.ChainID, result.ChainID)
	require.Equal(t, cc.NetID, result.NetID)
}

func TestFromContext_Nil(t *testing.T) {
	ctx := context.Background()
	result := FromContext(ctx)
	require.Nil(t, result)
}

func TestGetNodeID(t *testing.T) {
	// Test with nil context
	ctx := context.Background()
	result := GetNodeID(ctx)
	require.Equal(t, ids.EmptyNodeID, result)

	// Test with valid context
	nodeID := ids.GenerateTestNodeID()
	cc := &Context{NodeID: nodeID}
	ctx = WithContext(ctx, cc)
	result = GetNodeID(ctx)
	require.Equal(t, nodeID, result)
}

func TestWithIDs_ExistingContext(t *testing.T) {
	// Test with existing context
	existingChainID := ids.GenerateTestID()
	cc := &Context{ChainID: existingChainID}
	ctx := WithContext(context.Background(), cc)

	newIDs := IDs{
		QuantumID: 42,
		NetID:     ids.GenerateTestID(),
		ChainID:   ids.GenerateTestID(),
		NodeID:    ids.GenerateTestNodeID(),
		PublicKey: []byte("test-public-key"),
	}

	ctx = WithIDs(ctx, newIDs)
	result := FromContext(ctx)

	require.Equal(t, newIDs.QuantumID, result.QuantumID)
	require.Equal(t, newIDs.NetID, result.NetID)
	require.Equal(t, newIDs.ChainID, result.ChainID)
	require.Equal(t, newIDs.NodeID, result.NodeID)
	require.Equal(t, newIDs.PublicKey, result.PublicKey)
}

func TestWithIDs_NewContext(t *testing.T) {
	// Test without existing context
	ctx := context.Background()

	newIDs := IDs{
		QuantumID: 99,
		NetID:     ids.GenerateTestID(),
		ChainID:   ids.GenerateTestID(),
		NodeID:    ids.GenerateTestNodeID(),
		PublicKey: []byte("another-key"),
	}

	ctx = WithIDs(ctx, newIDs)
	result := FromContext(ctx)

	require.NotNil(t, result)
	require.Equal(t, newIDs.QuantumID, result.QuantumID)
	require.Equal(t, newIDs.NetID, result.NetID)
	require.Equal(t, newIDs.ChainID, result.ChainID)
	require.Equal(t, newIDs.NodeID, result.NodeID)
	require.Equal(t, newIDs.PublicKey, result.PublicKey)
}

func TestWithValidatorState_ExistingContext(t *testing.T) {
	// Test with existing context
	cc := &Context{QuantumID: 1}
	ctx := WithContext(context.Background(), cc)

	vs := &mockValidatorState{}
	ctx = WithValidatorState(ctx, vs)
	result := FromContext(ctx)

	require.Equal(t, vs, result.ValidatorState)
	require.Equal(t, uint32(1), result.QuantumID) // Original value preserved
}

func TestWithValidatorState_NewContext(t *testing.T) {
	// Test without existing context
	ctx := context.Background()

	vs := &mockValidatorState{}
	ctx = WithValidatorState(ctx, vs)
	result := FromContext(ctx)

	require.NotNil(t, result)
	require.Equal(t, vs, result.ValidatorState)
}

func TestContextLock(t *testing.T) {
	cc := &Context{}

	// Test that lock works
	cc.Lock.Lock()
	cc.QuantumID = 123
	cc.Lock.Unlock()

	cc.Lock.RLock()
	require.Equal(t, uint32(123), cc.QuantumID)
	cc.Lock.RUnlock()
}

func TestContextFields(t *testing.T) {
	now := time.Now()
	chainID := ids.GenerateTestID()
	netID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()
	xChainID := ids.GenerateTestID()
	cChainID := ids.GenerateTestID()
	xAssetID := ids.GenerateTestID()
	luxAssetID := ids.GenerateTestID()

	cc := &Context{
		QuantumID:    1,
		NetworkID:    1,
		NetID:        netID,
		SubnetID:     netID,
		ChainID:      chainID,
		NodeID:       nodeID,
		PublicKey:    []byte("test-key"),
		XChainID:     xChainID,
		CChainID:     cChainID,
		XAssetID:     xAssetID,
		LUXAssetID:   luxAssetID,
		ChainDataDir: "/data/chain",
		StartTime:    now,
	}

	require.Equal(t, uint32(1), cc.QuantumID)
	require.Equal(t, uint32(1), cc.NetworkID)
	require.Equal(t, netID, cc.NetID)
	require.Equal(t, netID, cc.SubnetID)
	require.Equal(t, chainID, cc.ChainID)
	require.Equal(t, nodeID, cc.NodeID)
	require.Equal(t, []byte("test-key"), cc.PublicKey)
	require.Equal(t, xChainID, cc.XChainID)
	require.Equal(t, cChainID, cc.CChainID)
	require.Equal(t, xAssetID, cc.XAssetID)
	require.Equal(t, luxAssetID, cc.LUXAssetID)
	require.Equal(t, "/data/chain", cc.ChainDataDir)
	require.Equal(t, now, cc.StartTime)
}

func TestIDsStruct(t *testing.T) {
	netID := ids.GenerateTestID()
	chainID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()
	xAssetID := ids.GenerateTestID()
	luxAssetID := ids.GenerateTestID()

	i := IDs{
		NetworkID:    1,
		QuantumID:    2,
		NetID:        netID,
		ChainID:      chainID,
		NodeID:       nodeID,
		PublicKey:    []byte("key"),
		XAssetID:     xAssetID,
		LUXAssetID:   luxAssetID,
		ChainDataDir: "/data",
	}

	require.Equal(t, uint32(1), i.NetworkID)
	require.Equal(t, uint32(2), i.QuantumID)
	require.Equal(t, netID, i.NetID)
	require.Equal(t, chainID, i.ChainID)
	require.Equal(t, nodeID, i.NodeID)
	require.Equal(t, []byte("key"), i.PublicKey)
	require.Equal(t, xAssetID, i.XAssetID)
	require.Equal(t, luxAssetID, i.LUXAssetID)
	require.Equal(t, "/data", i.ChainDataDir)
}

func TestGetValidatorOutput(t *testing.T) {
	nodeID := ids.GenerateTestNodeID()
	out := GetValidatorOutput{
		NodeID:    nodeID,
		PublicKey: []byte("pub-key"),
		Weight:    1000,
	}

	require.Equal(t, nodeID, out.NodeID)
	require.Equal(t, []byte("pub-key"), out.PublicKey)
	require.Equal(t, uint64(1000), out.Weight)
}

// Test interface types exist
func TestInterfaceTypes(t *testing.T) {
	// Ensure these types can be used as interface types
	var _ BCLookup = nil
	var _ ValidatorState = nil
	var _ Keystore = nil
	var _ Metrics = nil
	var _ Logger = nil
	var _ SharedMemory = nil
	var _ WarpSigner = nil
	var _ NetworkUpgrades = nil

	// BlockchainIDLookup should be alias for BCLookup
	var lookup BCLookup
	var alias BlockchainIDLookup = lookup
	require.Equal(t, lookup, alias)
}
