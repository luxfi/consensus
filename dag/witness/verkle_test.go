// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package witness

import (
	"crypto/rand"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVerkleWitnessBasic tests basic Verkle witness operations
func TestVerkleWitnessBasic(t *testing.T) {
	cache := NewCache(Policy{
		Mode:     RequireFull,
		MaxBytes: 10 * 1024 * 1024, // 10MB
	}, 1000, 100*1024*1024) // 100MB node cache

	// Create a mock Verkle witness
	witness := makeVerkleWitness(32, 256) // 32 stems, 256 bytes each

	h := mockHeader{
		id:          BlockID{1},
		round:       1,
		witnessRoot: hashWitness(witness),
	}

	payload := makePayload(1024, len(witness))
	ok, size, deltaRoot := cache.Validate(h, payload)

	require.True(t, ok)
	require.Equal(t, len(witness), size)
	require.NotEqual(t, [32]byte{}, deltaRoot)
}

// TestVerkleDeltaWitness tests delta witness functionality
func TestVerkleDeltaWitness(t *testing.T) {
	cache := NewCache(Policy{
		Mode:     DeltaOnly,
		MaxDelta: 4096, // 4KB max delta
	}, 1000, 100*1024*1024)

	// Create parent block with witness
	parentID := BlockID{10}
	parentRoot := [32]byte{0xAA}
	cache.PutCommittedRoot(parentID, parentRoot)

	// Create child block with delta witness
	deltaWitness := makeVerkleDelta(8, 128) // 8 stems, 128 bytes each
	h := mockHeader{
		id:      BlockID{11},
		round:   2,
		parents: []BlockID{parentID},
	}

	payload := makePayload(512, len(deltaWitness))
	ok, size, deltaRoot := cache.Validate(h, payload)

	require.True(t, ok)
	require.Equal(t, len(deltaWitness), size)
	require.NotEqual(t, [32]byte{}, deltaRoot)
	require.NotEqual(t, parentRoot, deltaRoot) // Should be different from parent
}

// TestVerkleNodeCache tests Verkle node caching
func TestVerkleNodeCache(t *testing.T) {
	cache := NewCache(Policy{Mode: Soft}, 100, 10*1024) // 10KB node cache

	// Generate Verkle nodes
	nodes := make(map[NodeKey][]byte)
	for i := 0; i < 50; i++ {
		stem := [32]byte{}
		binary.BigEndian.PutUint32(stem[:], uint32(i))
		key := NodeKey{
			Stem:  stem,
			Index: uint16(i % 256),
		}
		blob := makeVerkleNode(128) // 128 bytes per node
		nodes[key] = blob
		cache.PutNode(key, blob)
	}

	// Verify recently added nodes are cached
	for i := 40; i < 50; i++ {
		stem := [32]byte{}
		binary.BigEndian.PutUint32(stem[:], uint32(i))
		key := NodeKey{
			Stem:  stem,
			Index: uint16(i % 256),
		}
		blob, ok := cache.GetNode(key)
		require.True(t, ok, "Node %d should be cached", i)
		require.Equal(t, nodes[key], blob)
	}

	// Older nodes might be evicted due to size limit
	stem := [32]byte{}
	binary.BigEndian.PutUint32(stem[:], uint32(0))
	key := NodeKey{Stem: stem, Index: 0}
	_, ok := cache.GetNode(key)
	// May or may not be cached depending on LRU eviction
	_ = ok
}

// TestVerkleWitnessSize tests witness size enforcement
func TestVerkleWitnessSize(t *testing.T) {
	testCases := []struct {
		name       string
		policy     Policy
		witnessLen int
		expectOK   bool
	}{
		{
			name: "small_witness_accepted",
			policy: Policy{
				Mode:     RequireFull,
				MaxBytes: 10000,
			},
			witnessLen: 5000,
			expectOK:   true,
		},
		{
			name: "large_witness_rejected",
			policy: Policy{
				Mode:     RequireFull,
				MaxBytes: 1000,
			},
			witnessLen: 2000,
			expectOK:   false,
		},
		{
			name: "soft_mode_accepts_any",
			policy: Policy{
				Mode: Soft,
			},
			witnessLen: 100000,
			expectOK:   true,
		},
		{
			name: "delta_within_limit",
			policy: Policy{
				Mode:     DeltaOnly,
				MaxDelta: 2048,
			},
			witnessLen: 1024,
			expectOK:   false, // No parent, so fails
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cache := NewCache(tc.policy, 100, 10000)

			h := mockHeader{
				id:    BlockID{1},
				round: 1,
			}

			witness := makeVerkleWitness(tc.witnessLen/32, 32)
			payload := makePayload(100, len(witness))

			ok, _, _ := cache.Validate(h, payload)
			require.Equal(t, tc.expectOK, ok)
		})
	}
}

// TestVerkleMultiParent tests witness validation with multiple parents
func TestVerkleMultiParent(t *testing.T) {
	cache := NewCache(Policy{
		Mode:     DeltaOnly,
		MaxDelta: 4096,
	}, 100, 10000)

	// Add multiple parent roots
	parent1 := BlockID{1}
	parent2 := BlockID{2}
	parent3 := BlockID{3}

	cache.PutCommittedRoot(parent1, [32]byte{0x11})
	cache.PutCommittedRoot(parent2, [32]byte{0x22})
	cache.PutCommittedRoot(parent3, [32]byte{0x33})

	// Create block with all parents
	h := mockHeader{
		id:      BlockID{4},
		round:   10,
		parents: []BlockID{parent1, parent2, parent3},
	}

	witness := makeVerkleDelta(10, 100)
	payload := makePayload(500, len(witness))

	ok, size, deltaRoot := cache.Validate(h, payload)

	require.True(t, ok)
	require.Equal(t, len(witness), size)
	require.NotEqual(t, [32]byte{}, deltaRoot)

	// Verify it used first parent as base
	root1, ok1 := cache.CacheHint(parent1)
	require.True(t, ok1)
	require.NotEqual(t, root1, deltaRoot) // Delta should differ from base
}

// Helper functions for testing

func makeVerkleWitness(stems, bytesPerStem int) []byte {
	witness := make([]byte, stems*bytesPerStem)
	_, _ = rand.Read(witness)
	return witness
}

func makeVerkleDelta(stems, bytesPerStem int) []byte {
	// Verkle delta witness is typically smaller
	delta := make([]byte, stems*bytesPerStem)
	// Add some structure to simulate real delta
	for i := 0; i < len(delta); i += bytesPerStem {
		delta[i] = 0xDE   // Delta marker
		delta[i+1] = 0x17 // Delta type
		binary.BigEndian.PutUint32(delta[i+2:], uint32(i/bytesPerStem))
	}
	return delta
}

func makeVerkleNode(size int) []byte {
	node := make([]byte, size)
	_, _ = rand.Read(node)
	// Add Verkle node structure
	node[0] = 0xFE // Verkle marker
	node[1] = 0x01 // Version
	return node
}

func hashWitness(witness []byte) [32]byte {
	// Simple hash for testing
	var h [32]byte
	if len(witness) > 0 {
		copy(h[:], witness[:min(32, len(witness))])
	}
	return h
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Benchmarks

func BenchmarkVerkleValidate(b *testing.B) {
	cache := NewCache(Policy{
		Mode:     RequireFull,
		MaxBytes: 100000,
	}, 10000, 100*1024*1024)

	h := mockHeader{
		id:    BlockID{1},
		round: 1,
	}

	witness := makeVerkleWitness(100, 256) // 100 stems, 256 bytes each
	payload := makePayload(10000, len(witness))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Validate(h, payload)
	}
}

func BenchmarkVerkleNodeCache(b *testing.B) {
	cache := NewCache(Policy{Mode: Soft}, 10000, 100*1024*1024)

	// Pre-populate cache
	stems := make([][32]byte, 1000)
	for i := range stems {
		_, _ = rand.Read(stems[i][:])
		key := NodeKey{Stem: stems[i], Index: uint16(i % 256)}
		cache.PutNode(key, makeVerkleNode(256))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := NodeKey{
			Stem:  stems[i%len(stems)],
			Index: uint16(i % 256),
		}
		cache.GetNode(key)
	}
}
