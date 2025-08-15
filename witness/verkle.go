// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package witness implements Verkle witness policy for consensus state proofs
package witness

import (
	"sync"
	"time"
)

// Policy defines admission control for witness data
type Policy interface {
	Admit(height uint64, bytes int) bool
}

// Cache stores witness data with LRU eviction
type Cache interface {
	Put(id []byte, blob []byte)
	Get(id []byte) ([]byte, bool)
}

// DefaultPolicy implements a simple rate-limited admission policy
type DefaultPolicy struct {
	mu           sync.RWMutex
	maxBytesPerHeight uint64
	bytesUsed    map[uint64]uint64
	windowSize   uint64
}

// NewDefaultPolicy creates a new default admission policy
func NewDefaultPolicy(maxBytesPerHeight uint64, windowSize uint64) *DefaultPolicy {
	return &DefaultPolicy{
		maxBytesPerHeight: maxBytesPerHeight,
		bytesUsed:        make(map[uint64]uint64),
		windowSize:       windowSize,
	}
}

// Admit checks if witness data should be admitted
func (p *DefaultPolicy) Admit(height uint64, bytes int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Clean old heights outside window
	if height > p.windowSize {
		minHeight := height - p.windowSize
		for h := range p.bytesUsed {
			if h < minHeight {
				delete(p.bytesUsed, h)
			}
		}
	}

	// Check if adding these bytes would exceed limit
	currentUsed := p.bytesUsed[height]
	if currentUsed+uint64(bytes) > p.maxBytesPerHeight {
		return false
	}

	// Update usage
	p.bytesUsed[height] = currentUsed + uint64(bytes)
	return true
}

// LRUCache implements a simple LRU cache for witness data
type LRUCache struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*cacheItem
	head     *cacheItem
	tail     *cacheItem
}

type cacheItem struct {
	key   string
	value []byte
	prev  *cacheItem
	next  *cacheItem
	timestamp time.Time
}

// NewLRUCache creates a new LRU cache
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		items:    make(map[string]*cacheItem),
	}
}

// Put adds or updates an item in the cache
func (c *LRUCache) Put(id []byte, blob []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := string(id)
	
	// Check if item exists
	if item, exists := c.items[key]; exists {
		// Update value and move to front
		item.value = blob
		item.timestamp = time.Now()
		c.moveToFront(item)
		return
	}

	// Create new item
	item := &cacheItem{
		key:       key,
		value:     blob,
		timestamp: time.Now(),
	}

	// Add to front
	c.addToFront(item)
	c.items[key] = item

	// Evict if over capacity
	if len(c.items) > c.capacity {
		c.evictOldest()
	}
}

// Get retrieves an item from the cache
func (c *LRUCache) Get(id []byte) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := string(id)
	item, exists := c.items[key]
	if !exists {
		return nil, false
	}

	// Move to front (LRU)
	c.moveToFront(item)
	return item.value, true
}

// moveToFront moves an item to the front of the list
func (c *LRUCache) moveToFront(item *cacheItem) {
	if item == c.head {
		return
	}

	// Remove from current position
	if item.prev != nil {
		item.prev.next = item.next
	}
	if item.next != nil {
		item.next.prev = item.prev
	}
	if item == c.tail {
		c.tail = item.prev
	}

	// Add to front
	item.prev = nil
	item.next = c.head
	if c.head != nil {
		c.head.prev = item
	}
	c.head = item
	if c.tail == nil {
		c.tail = item
	}
}

// addToFront adds a new item to the front
func (c *LRUCache) addToFront(item *cacheItem) {
	item.next = c.head
	item.prev = nil
	if c.head != nil {
		c.head.prev = item
	}
	c.head = item
	if c.tail == nil {
		c.tail = item
	}
}

// evictOldest removes the least recently used item
func (c *LRUCache) evictOldest() {
	if c.tail == nil {
		return
	}

	delete(c.items, c.tail.key)
	
	if c.tail.prev != nil {
		c.tail.prev.next = nil
		c.tail = c.tail.prev
	} else {
		c.head = nil
		c.tail = nil
	}
}

// NewCache creates a cache with the specified policy
func NewCache(policy Policy) Cache {
	return NewLRUCache(1000) // Default capacity
}