// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"hash/fnv"
	"sync"
)

// ShardedMap provides a sharded map for concurrent access with reduced lock contention
type ShardedMap[K comparable, V any] struct {
	shards    []*shard[K, V]
	numShards int
}

type shard[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]V
}

// NewShardedMap creates a new sharded map with the specified number of shards
func NewShardedMap[K comparable, V any](numShards int) *ShardedMap[K, V] {
	if numShards <= 0 {
		numShards = 16
	}

	sm := &ShardedMap[K, V]{
		shards:    make([]*shard[K, V], numShards),
		numShards: numShards,
	}

	for i := 0; i < numShards; i++ {
		sm.shards[i] = &shard[K, V]{
			items: make(map[K]V),
		}
	}

	return sm
}

// getShard returns the shard for a given key
func (sm *ShardedMap[K, V]) getShard(key K) *shard[K, V] {
	// Use FNV hash for better distribution
	h := fnv.New32a()

	// Hash the key based on its type
	switch k := any(key).(type) {
	case TxRef:
		h.Write(k[:])
	case ObjectID:
		h.Write(k[:])
	case [64]byte:
		h.Write(k[:])
	case string:
		h.Write([]byte(k))
	default:
		// Fallback to simple modulo for other types
		// This isn't ideal but works for basic types
		return sm.shards[0]
	}

	return sm.shards[h.Sum32()%uint32(sm.numShards)]
}

// Get retrieves a value from the map
func (sm *ShardedMap[K, V]) Get(key K) (V, bool) {
	shard := sm.getShard(key)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	val, ok := shard.items[key]
	return val, ok
}

// Set stores a value in the map
func (sm *ShardedMap[K, V]) Set(key K, value V) {
	shard := sm.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	shard.items[key] = value
}

// GetOrCreate gets an existing value or creates a new one atomically
func (sm *ShardedMap[K, V]) GetOrCreate(key K, creator func() V) (V, bool) {
	shard := sm.getShard(key)

	// Try with read lock first
	shard.mu.RLock()
	val, ok := shard.items[key]
	shard.mu.RUnlock()

	if ok {
		return val, false // Existing value
	}

	// Need to create - upgrade to write lock
	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Double-check after acquiring write lock
	val, ok = shard.items[key]
	if ok {
		return val, false
	}

	// Create new value
	val = creator()
	shard.items[key] = val
	return val, true // New value created
}

// Delete removes a key from the map
func (sm *ShardedMap[K, V]) Delete(key K) {
	shard := sm.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	delete(shard.items, key)
}

// Size returns the total number of items across all shards
func (sm *ShardedMap[K, V]) Size() int {
	total := 0
	for _, shard := range sm.shards {
		shard.mu.RLock()
		total += len(shard.items)
		shard.mu.RUnlock()
	}
	return total
}

// Clear removes all items from the map
func (sm *ShardedMap[K, V]) Clear() {
	for _, shard := range sm.shards {
		shard.mu.Lock()
		shard.items = make(map[K]V)
		shard.mu.Unlock()
	}
}

// Range iterates over all key-value pairs
// Note: This locks all shards sequentially, use sparingly
func (sm *ShardedMap[K, V]) Range(fn func(key K, value V) bool) {
	for _, shard := range sm.shards {
		shard.mu.RLock()
		for k, v := range shard.items {
			if !fn(k, v) {
				shard.mu.RUnlock()
				return
			}
		}
		shard.mu.RUnlock()
	}
}
