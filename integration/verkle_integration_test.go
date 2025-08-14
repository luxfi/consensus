package integration

import (
	"crypto/rand"
	"testing"

	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/trie/utils"
	"github.com/luxfi/geth/triedb"
	"github.com/luxfi/consensus/dag/witness"
	"github.com/stretchr/testify/require"
)

func TestVerkleIntegration(t *testing.T) {
	// Create in-memory database
	db := triedb.NewDatabase(triedb.NewMemoryDatabase(), nil)
	pointCache := utils.NewPointCache(1024)
	
	// Create adapter
	adapter, err := NewVerkleAdapter(common.Hash{}, db, pointCache)
	require.NoError(t, err)
	require.NotNil(t, adapter)
	
	// Test witness validation
	key := []byte("testkey")
	value := []byte("testvalue")
	proof := make([]byte, 100)
	_, _ = rand.Read(proof)
	
	// Should fail for non-existent key
	valid := adapter.ValidateWitness(key, value, proof)
	require.False(t, valid)
	
	// Test caching
	nodeKey := witness.NodeKey{
		Stem:  [32]byte{1, 2, 3},
		Index: 42,
	}
	nodeData := []byte("node data")
	
	// Initially not cached
	_, ok := adapter.GetCachedNode(nodeKey)
	require.False(t, ok)
	
	// Cache it
	adapter.cache.PutNode(nodeKey, nodeData)
	
	// Now should be cached
	cached, ok := adapter.GetCachedNode(nodeKey)
	require.True(t, ok)
	require.Equal(t, nodeData, cached)
	
	// Test hash
	hash := adapter.Hash()
	require.NotEqual(t, common.Hash{}, hash)
}

func TestVerkleWitnessPerformance(t *testing.T) {
	db := triedb.NewDatabase(triedb.NewMemoryDatabase(), nil)
	pointCache := utils.NewPointCache(10000)
	
	adapter, err := NewVerkleAdapter(common.Hash{}, db, pointCache)
	require.NoError(t, err)
	
	// Simulate high-throughput witness validation
	const numOps = 10000
	keys := make([]witness.NodeKey, numOps)
	values := make([][]byte, numOps)
	
	for i := 0; i < numOps; i++ {
		keys[i] = witness.NodeKey{
			Stem:  [32]byte{byte(i >> 8), byte(i)},
			Index: uint8(i % 256),
		}
		values[i] = make([]byte, 100)
		_, _ = rand.Read(values[i])
		
		// Cache the node
		adapter.cache.PutNode(keys[i], values[i])
	}
	
	// Verify all cached
	hits := 0
	for i := 0; i < numOps; i++ {
		if cached, ok := adapter.GetCachedNode(keys[i]); ok {
			if len(cached) == len(values[i]) {
				hits++
			}
		}
	}
	
	// Should have high cache hit rate
	hitRate := float64(hits) / float64(numOps)
	require.Greater(t, hitRate, 0.95, "Cache hit rate should be > 95%%")
}

func BenchmarkVerkleIntegration(b *testing.B) {
	db := triedb.NewDatabase(triedb.NewMemoryDatabase(), nil)
	pointCache := utils.NewPointCache(10000)
	
	adapter, err := NewVerkleAdapter(common.Hash{}, db, pointCache)
	require.NoError(b, err)
	
	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		key := witness.NodeKey{
			Stem:  [32]byte{byte(i >> 8), byte(i)},
			Index: uint8(i % 256),
		}
		value := make([]byte, 100)
		adapter.cache.PutNode(key, value)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := witness.NodeKey{
			Stem:  [32]byte{byte(i >> 8), byte(i)},
			Index: uint8(i % 256),
		}
		_, _ = adapter.GetCachedNode(key)
	}
}
