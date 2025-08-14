// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package witness

import (
	"crypto/rand"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockHeader struct {
	id          BlockID
	round       uint64
	parents     []BlockID
	witnessRoot [32]byte
}

func (h mockHeader) ID() BlockID           { return h.id }
func (h mockHeader) Round() uint64         { return h.round }
func (h mockHeader) Parents() []BlockID    { return h.parents }
func (h mockHeader) WitnessRoot() [32]byte { return h.witnessRoot }

func makePayload(txLen int, witnessLen int) []byte {
	// varint(txLen) | tx | witness
	tx := make([]byte, txLen)
	witness := make([]byte, witnessLen)
	rand.Read(tx)
	rand.Read(witness)
	
	buf := make([]byte, 0, 10+txLen+witnessLen)
	tmp := make([]byte, 10)
	n := binary.PutUvarint(tmp, uint64(txLen))
	buf = append(buf, tmp[:n]...)
	buf = append(buf, tx...)
	buf = append(buf, witness...)
	return buf
}

func TestCacheBasic(t *testing.T) {
	cache := NewCache(Policy{
		Mode:     RequireFull,
		MaxBytes: 1024,
	}, 100, 10000)
	
	// Create header
	h := mockHeader{
		id:    BlockID{1},
		round: 1,
	}
	
	// Test with valid witness
	payload := makePayload(100, 500)
	ok, size, deltaRoot := cache.Validate(h, payload)
	
	require.True(t, ok)
	require.Equal(t, 500, size)
	require.NotEqual(t, [32]byte{}, deltaRoot)
}

func TestCachePolicy(t *testing.T) {
	t.Run("RequireFull", func(t *testing.T) {
		cache := NewCache(Policy{
			Mode:     RequireFull,
			MaxBytes: 100,
		}, 10, 1000)
		
		h := mockHeader{id: BlockID{1}}
		
		// Too large witness
		payload := makePayload(50, 200)
		ok, _, _ := cache.Validate(h, payload)
		require.False(t, ok)
		
		// Within budget
		payload = makePayload(50, 50)
		ok, _, _ = cache.Validate(h, payload)
		require.True(t, ok)
	})
	
	t.Run("Soft", func(t *testing.T) {
		cache := NewCache(Policy{
			Mode: Soft,
		}, 10, 1000)
		
		h := mockHeader{id: BlockID{2}}
		
		// Any size is ok in Soft mode
		payload := makePayload(1000, 5000)
		ok, size, _ := cache.Validate(h, payload)
		require.True(t, ok)
		require.Equal(t, 5000, size)
	})
	
	t.Run("DeltaOnly", func(t *testing.T) {
		cache := NewCache(Policy{
			Mode:     DeltaOnly,
			MaxDelta: 100,
		}, 10, 1000)
		
		// First block with no parent - should fail
		h1 := mockHeader{
			id:      BlockID{1},
			parents: []BlockID{},
		}
		payload := makePayload(50, 50)
		ok, _, _ := cache.Validate(h1, payload)
		require.False(t, ok) // No parent for delta
		
		// Add a committed root
		parentID := BlockID{10}
		parentRoot := [32]byte{99}
		cache.PutCommittedRoot(parentID, parentRoot)
		
		// Now with parent
		h2 := mockHeader{
			id:      BlockID{2},
			parents: []BlockID{parentID},
		}
		ok, _, deltaRoot := cache.Validate(h2, payload)
		require.True(t, ok)
		require.NotEqual(t, [32]byte{}, deltaRoot)
	})
}

func TestCacheHint(t *testing.T) {
	cache := NewCache(Policy{Mode: Soft}, 10, 1000)
	
	// Initially no hint
	id := BlockID{1}
	_, ok := cache.CacheHint(id)
	require.False(t, ok)
	
	// Add root
	root := [32]byte{42}
	cache.PutCommittedRoot(id, root)
	
	// Now should have hint
	gotRoot, ok := cache.CacheHint(id)
	require.True(t, ok)
	require.Equal(t, root, gotRoot)
}

func TestNodeCache(t *testing.T) {
	cache := NewCache(Policy{Mode: Soft}, 10, 1000)
	
	key := NodeKey{
		Stem:  [32]byte{1, 2, 3},
		Index: 42,
	}
	blob := []byte("node data")
	
	// Initially not cached
	_, ok := cache.GetNode(key)
	require.False(t, ok)
	
	// Put node
	cache.PutNode(key, blob)
	
	// Now should be cached
	gotBlob, ok := cache.GetNode(key)
	require.True(t, ok)
	require.Equal(t, blob, gotBlob)
}

func TestLRU(t *testing.T) {
	lru := NewLRU[int, string](3, 100, func(s string) int { return len(s) })
	
	// Add items
	lru.Put(1, "one")
	lru.Put(2, "two")
	lru.Put(3, "three")
	
	// All should be present
	v, ok := lru.Get(1)
	require.True(t, ok)
	require.Equal(t, "one", v)
	
	// Add 4th item, should evict oldest (2)
	lru.Put(4, "four")
	
	_, ok = lru.Get(2) // Was evicted
	require.False(t, ok)
	
	_, ok = lru.Get(1) // Still there (was accessed)
	require.True(t, ok)
}

func TestLRUByteLimit(t *testing.T) {
	// Limit to 20 bytes
	lru := NewLRU[int, string](100, 20, func(s string) int { return len(s) })
	
	lru.Put(1, "hello")     // 5 bytes
	lru.Put(2, "world")     // 5 bytes
	lru.Put(3, "foo")       // 3 bytes
	lru.Put(4, "bar")       // 3 bytes, total 16
	
	// All should fit
	_, ok := lru.Get(1)
	require.True(t, ok)
	
	// Adding large item should trigger eviction
	lru.Put(5, "verylongstring") // 14 bytes
	
	// Item 2 should be evicted (oldest after we accessed item 1)
	_, ok = lru.Get(2)
	require.False(t, ok) // Evicted
	
	_, ok = lru.Get(5)
	require.True(t, ok) // New item present
}

func BenchmarkValidate(b *testing.B) {
	cache := NewCache(Policy{
		Mode:     RequireFull,
		MaxBytes: 10000,
	}, 1000, 1<<20)
	
	h := mockHeader{
		id:    BlockID{1},
		round: 1,
	}
	payload := makePayload(1000, 5000)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Validate(h, payload)
	}
}

func BenchmarkLRU(b *testing.B) {
	lru := NewLRU[int, []byte](1000, 1<<20, func(b []byte) int { return len(b) })
	
	data := make([]byte, 1024)
	rand.Read(data)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lru.Put(i%1000, data)
		lru.Get(i % 1000)
	}
}