// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package witness

import (
	"container/list"
	"sync"
	"time"

	"github.com/luxfi/ids"
)

// Mode represents the witness validation mode
type Mode int

const (
	// Soft allows commit even if witness is large; execution may lag
	Soft Mode = iota
	// RequireFull requires full witness to be under MaxBytes
	RequireFull
	// DeltaOnly requires delta witness to be under MaxDelta
	DeltaOnly
)

// Policy defines witness size constraints
type Policy struct {
	Mode     Mode
	MaxBytes uint64 // Maximum full witness size
	MaxDelta uint64 // Maximum delta witness size
}

// Header interface for block headers
type Header interface {
	GetID() ids.ID
	GetParent() ids.ID
	GetStateRoot() []byte
	GetHeight() uint64
}

// WitnessData represents witness information
type WitnessData struct {
	FullSize  uint64
	DeltaSize uint64
	Nodes     [][]byte
	Delta     []byte
}

// simpleLRU is a basic LRU cache implementation
type simpleLRU struct {
	capacity int
	items    map[ids.ID]*list.Element
	list     *list.List
}

type lruEntry struct {
	key   ids.ID
	value []byte
}

func newSimpleLRU(capacity int) *simpleLRU {
	return &simpleLRU{
		capacity: capacity,
		items:    make(map[ids.ID]*list.Element),
		list:     list.New(),
	}
}

func (l *simpleLRU) Get(key ids.ID) ([]byte, bool) {
	if elem, ok := l.items[key]; ok {
		l.list.MoveToFront(elem)
		return elem.Value.(*lruEntry).value, true
	}
	return nil, false
}

func (l *simpleLRU) Put(key ids.ID, value []byte) {
	if elem, ok := l.items[key]; ok {
		l.list.MoveToFront(elem)
		elem.Value.(*lruEntry).value = value
		return
	}
	
	if l.list.Len() >= l.capacity {
		back := l.list.Back()
		if back != nil {
			l.list.Remove(back)
			delete(l.items, back.Value.(*lruEntry).key)
		}
	}
	
	entry := &lruEntry{key: key, value: value}
	elem := l.list.PushFront(entry)
	l.items[key] = elem
}

func (l *simpleLRU) Len() int {
	return l.list.Len()
}

func (l *simpleLRU) Purge() {
	l.items = make(map[ids.ID]*list.Element)
	l.list.Init()
}

// Cache manages witness node caching for Verkle trees
type Cache struct {
	mu           sync.RWMutex
	policy       Policy
	nodeCache    *simpleLRU // Node ID -> node data
	rootCache    *simpleLRU // Block ID -> state root
	nodeLimit    int        // Max cached nodes
	nodeBudget   uint64     // Max memory for nodes
	currentUsage uint64
}

// NewCache creates a new witness cache
func NewCache(policy Policy, nodeLimit int, nodeBudget uint64) *Cache {
	return &Cache{
		policy:     policy,
		nodeCache:  newSimpleLRU(nodeLimit),
		rootCache:  newSimpleLRU(100), // Keep last 100 block roots
		nodeLimit:  nodeLimit,
		nodeBudget: nodeBudget,
	}
}

// Validate checks if witness size is acceptable under current policy
func (c *Cache) Validate(header Header, witnessBytes []byte) (bool, uint64, []byte) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	witnessSize := uint64(len(witnessBytes))
	
	// Try to compute delta from parent if available
	parentRoot, hasParent := c.rootCache.Get(header.GetParent())
	var deltaSize uint64
	var delta []byte
	
	if hasParent {
		// Compute delta witness (simplified - in production would use actual Verkle diff)
		delta = computeDelta(parentRoot, header.GetStateRoot(), witnessBytes)
		deltaSize = uint64(len(delta))
	} else {
		// No parent, full witness required
		deltaSize = witnessSize
		delta = witnessBytes
	}
	
	// Apply policy
	switch c.policy.Mode {
	case RequireFull:
		return witnessSize <= c.policy.MaxBytes, witnessSize, delta
		
	case DeltaOnly:
		if !hasParent {
			// Need parent for delta mode
			return false, witnessSize, delta
		}
		return deltaSize <= c.policy.MaxDelta, witnessSize, delta
		
	case Soft:
		// Always allow in soft mode, execution will catch up
		return true, witnessSize, delta
		
	default:
		return false, witnessSize, delta
	}
}

// PutCommittedRoot stores the state root for a committed block
func (c *Cache) PutCommittedRoot(blockID ids.ID, stateRoot []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	rootCopy := make([]byte, len(stateRoot))
	copy(rootCopy, stateRoot)
	c.rootCache.Put(blockID, rootCopy)
}

// PutNode caches a Verkle tree node
func (c *Cache) PutNode(nodeID ids.ID, nodeData []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	nodeSize := uint64(len(nodeData))
	
	// Check budget
	if c.currentUsage+nodeSize > c.nodeBudget {
		// Try to evict old nodes
		if !c.evictNodes(nodeSize) {
			return false // Can't fit within budget
		}
	}
	
	nodeCopy := make([]byte, len(nodeData))
	copy(nodeCopy, nodeData)
	
	c.nodeCache.Put(nodeID, nodeCopy)
	c.currentUsage += nodeSize
	
	return true
}

// GetNode retrieves a cached Verkle tree node
func (c *Cache) GetNode(nodeID ids.ID) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nodeCache.Get(nodeID)
}

// evictNodes tries to free up space by evicting LRU nodes
func (c *Cache) evictNodes(needed uint64) bool {
	freed := uint64(0)
	
	for freed < needed {
		// Get LRU item (simplified - actual LRU would track this)
		// For now, just clear if over budget
		if c.currentUsage > c.nodeBudget/2 {
			c.nodeCache.Purge()
			c.currentUsage = 0
			return true
		}
		break
	}
	
	return freed >= needed
}

// SetPolicy updates the witness validation policy
func (c *Cache) SetPolicy(policy Policy) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.policy = policy
}

// GetStats returns cache statistics
func (c *Cache) GetStats() (nodes int, memory uint64, roots int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nodeCache.Len(), c.currentUsage, c.rootCache.Len()
}

// computeDelta computes the delta witness between two state roots
// This is a simplified stub - actual implementation would use Verkle tree diffing
func computeDelta(parentRoot, currentRoot, fullWitness []byte) []byte {
	// In production: compute actual Verkle tree delta
	// For now, return a fraction of full witness to simulate compression
	if len(fullWitness) > 1024 {
		return fullWitness[:len(fullWitness)/4] // Simulate 75% reduction
	}
	return fullWitness
}

// Manager coordinates witness validation across the consensus engine
type Manager struct {
	cache     *Cache
	startTime time.Time
	softUntil time.Duration
}

// NewManager creates a witness manager with gradual policy hardening
func NewManager(initialPolicy Policy, nodeLimit int, nodeBudget uint64, softDuration time.Duration) *Manager {
	return &Manager{
		cache:     NewCache(initialPolicy, nodeLimit, nodeBudget),
		startTime: time.Now(),
		softUntil: softDuration,
	}
}

// CheckPolicy upgrades from Soft to stricter modes after warm-up
func (m *Manager) CheckPolicy() {
	elapsed := time.Since(m.startTime)
	
	if elapsed > m.softUntil {
		// Graduate from Soft mode after warm-up period
		currentPolicy := m.cache.policy
		if currentPolicy.Mode == Soft {
			currentPolicy.Mode = DeltaOnly
			m.cache.SetPolicy(currentPolicy)
		}
	}
}