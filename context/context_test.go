package context

import (
	"testing"

	consensusctx "github.com/luxfi/consensus/context"
	"github.com/stretchr/testify/require"
)

func TestXAssetID(t *testing.T) {
	ctx := &consensusctx.Context{}

	// Test that XAssetID returns the XAssetID from context
	assetID := XAssetID(ctx)
	require.NotNil(t, assetID) // Returns ctx.XAssetID

	// Test that it returns the same value each time (should be deterministic)
	assetID2 := XAssetID(ctx)
	require.Equal(t, assetID, assetID2)

	// Test with nil context
	assetIDNil := XAssetID(nil)
	require.Nil(t, assetIDNil)
}

func TestQuantumNetworkID(t *testing.T) {
	ctx := &consensusctx.Context{}

	// Test with context that doesn't have quantum IDs
	networkID := QuantumNetworkID(ctx)
	require.Equal(t, uint32(0), networkID)

	// Test with quantum ID set
	ctx.QuantumID = 12345
	networkID = QuantumNetworkID(ctx)
	require.Equal(t, uint32(12345), networkID)
}

func TestGetQuantumIDs(t *testing.T) {
	ctx := &consensusctx.Context{}

	// Test that GetQuantumIDs returns something
	qIDs := GetQuantumIDs(ctx)

	// Should return a QuantumIDs struct with values from ctx
	require.NotNil(t, qIDs)
	require.Equal(t, ctx.QuantumID, qIDs.QuantumID)
}
