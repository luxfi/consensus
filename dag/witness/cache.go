// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package witness implements witness validation and caching for DAG
package witness

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"time"
)

// ==== public API =============================================================

type Mode uint8

const (
	// RequireFull: a block can flip Undecided -> ToCommit only if its witness
	// verifies locally and is within Policy.MaxBytes.
	RequireFull Mode = iota

	// Soft: allow commit of header now; execution can be deferred until witness arrives.
	Soft

	// DeltaOnly: accept only delta witnesses relative to at least one parent witness root.
	DeltaOnly
)

type Policy struct {
	Mode     Mode
	MaxBytes int // max witness bytes allowed per block during Validate
	MaxDelta int // max delta bytes allowed (used when Mode == DeltaOnly)
}

// BlockID is a consensus header identifier (32 bytes recommended).
type BlockID = [32]byte

// Header is the minimal adapter your dag.Header should satisfy.
// Provide a shim type if needed.
type Header interface {
	ID() BlockID
	Round() uint64
	Parents() []BlockID
	WitnessRoot() [32]byte
}

// Manager gates witness validation/delta hinting for the decider.
type Manager interface {
	// Validate parses the payload, extracts witness bytes, applies the Policy,
	// and (optionally) computes a deltaRoot using the first parent as base.
	// This stub does not do crypto verification; it enforces byte budgets and
	// provides a deterministic deltaRoot you can bind to for experiments.
	Validate(h Header, payload []byte) (ok bool, size int, deltaRoot [32]byte)

	// CacheHint returns a parent's witness root if the cache has seen/recorded it.
	CacheHint(parent BlockID) ([32]byte, bool)

	// PutCommittedRoot lets consensus record the witness root for a committed block.
	PutCommittedRoot(id BlockID, root [32]byte)

	// PutNode/GetNode expose a tiny LRU for Verkle node blobs keyed by (stem,index).
	PutNode(key NodeKey, blob []byte)
	GetNode(key NodeKey) (blob []byte, ok bool)
}

// NewCache creates a witness Manager with an internal node LRU and a small
// map of blockID -> witnessRoot (for delta bases). Size budgets are best-effort.
func NewCache(policy Policy, nodeEntriesCap int, nodeBudgetBytes int) *Cache {
	return &Cache{
		policy: policy,
		nodes:  NewLRU[NodeKey, []byte](nodeEntriesCap, nodeBudgetBytes, func(v []byte) int { return len(v) }),
		roots:  make(map[BlockID][32]byte),
		added:  make(map[BlockID]time.Time),
	}
}

// ==== concrete implementation ===============================================

type Cache struct {
	mu     sync.RWMutex
	policy Policy

	// blockID -> witnessRoot (used as delta bases)
	roots map[BlockID][32]byte
	added map[BlockID]time.Time

	// tiny node cache for Verkle nodes
	nodes *LRU[NodeKey, []byte]
}

func (c *Cache) Validate(h Header, payload []byte) (bool, int, [32]byte) {
	// Layout: varint txLen | txBytes | witnessBytes
	tx, wit := splitPayload(payload)
	witLen := len(wit)

	// Budget checks (no cryptographic verification in this stub).
	switch c.policy.Mode {
	case RequireFull:
		if witLen == 0 || (c.policy.MaxBytes > 0 && witLen > c.policy.MaxBytes) {
			return false, witLen, [32]byte{}
		}
	case DeltaOnly:
		if witLen == 0 || (c.policy.MaxDelta > 0 && witLen > c.policy.MaxDelta) {
			return false, witLen, [32]byte{}
		}
		// Must have at least one parent base.
		if parents := h.Parents(); len(parents) == 0 {
			return false, witLen, [32]byte{}
		} else if _, ok := c.CacheHint(parents[0]); !ok {
			return false, witLen, [32]byte{}
		}
	case Soft:
		// Always ok; execution can be deferred.
	}

	// Compute a deterministic deltaRoot against first parent (if available).
	var base [32]byte
	if ps := h.Parents(); len(ps) > 0 {
		if r, ok := c.CacheHint(ps[0]); ok {
			base = r
		}
	}
	deltaRoot := fold(base, wit)

	// Optionally record the header's own WitnessRoot (if non-zero).
	wr := h.WitnessRoot()
	if wr != ([32]byte{}) {
		c.PutCommittedRoot(h.ID(), wr)
	}

	// Tiny heuristic: cache the first ~64 KiB of witness as "hot" nodes (fake).
	if len(wit) > 0 {
		max := 64 << 10
		chunk := wit
		if len(chunk) > max {
			chunk = chunk[:max]
		}
		// Pretend split into fixed "node" pieces so LRU gets some churn.
		const piece = 2048
		for i := 0; i < len(chunk); i += piece {
			j := i + piece
			if j > len(chunk) {
				j = len(chunk)
			}
			key := NodeKey{Stem: deltaRoot, Index: uint16(i / piece)}
			c.PutNode(key, append([]byte(nil), chunk[i:j]...))
		}
	}

	_ = tx // tx bytes are ignored here; execution pipeline will use them.
	return true, witLen, deltaRoot
}

func (c *Cache) CacheHint(parent BlockID) ([32]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.roots[parent]
	return r, ok
}

func (c *Cache) PutCommittedRoot(id BlockID, root [32]byte) {
	if id == ([32]byte{}) || root == ([32]byte{}) {
		return
	}
	c.mu.Lock()
	c.roots[id] = root
	c.added[id] = time.Now()
	c.mu.Unlock()
}

type NodeKey struct {
	Stem  [32]byte // e.g., stem commitment for a Verkle subtree
	Index uint16   // arbitrary node index within that stem
}

func (c *Cache) PutNode(key NodeKey, blob []byte) {
	c.mu.Lock()
	c.nodes.Put(key, blob)
	c.mu.Unlock()
}

func (c *Cache) GetNode(key NodeKey) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nodes.Get(key)
}

// ==== helpers ===============================================================

func splitPayload(payload []byte) (tx, witness []byte) {
	if len(payload) == 0 {
		return nil, nil
	}
	r := bytes.NewReader(payload)
	txLen, _ := binary.ReadUvarint(r)
	off := varintLen(uint64(txLen))
	if int(off) > len(payload) {
		return nil, nil
	}
	start := int(off)
	end := start + int(txLen)
	if end > len(payload) {
		end = len(payload)
	}
	tx = payload[start:end]
	witness = payload[end:]
	return
}

func varintLen(x uint64) int64 {
	switch {
	case x < 1<<7:
		return 1
	case x < 1<<14:
		return 2
	case x < 1<<21:
		return 3
	case x < 1<<28:
		return 4
	case x < 1<<35:
		return 5
	case x < 1<<42:
		return 6
	case x < 1<<49:
		return 7
	case x < 1<<56:
		return 8
	default:
		return 9
	}
}

// fold deterministically mixes a base root and witness bytes to derive a delta root.
// This is NOT a cryptographic proof; it's a stable placeholder for consensus wiring.
func fold(base [32]byte, wit []byte) [32]byte {
	h := sha256.New()
	_, _ = h.Write(base[:])
	_, _ = h.Write(wit)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// ==== generic LRU (bytes + entry caps) ======================================

type LRU[K comparable, V any] struct {
	mu          sync.Mutex
	ll          *list.List
	entries     map[K]*list.Element
	capEntries  int
	capBytes    int
	curBytes    int
	sizeOfValue func(V) int
}

type lruEntry[K comparable, V any] struct {
	key   K
	value V
	size  int
}

func NewLRU[K comparable, V any](capEntries, capBytes int, sizeOfValue func(V) int) *LRU[K, V] {
	if capEntries <= 0 {
		capEntries = 1
	}
	if capBytes < 0 {
		capBytes = 0
	}
	return &LRU[K, V]{
		ll:          list.New(),
		entries:     make(map[K]*list.Element, capEntries),
		capEntries:  capEntries,
		capBytes:    capBytes,
		sizeOfValue: sizeOfValue,
	}
}

func (l *LRU[K, V]) Get(k K) (V, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if el, ok := l.entries[k]; ok {
		l.ll.MoveToFront(el)
		en := el.Value.(lruEntry[K, V])
		return en.value, true
	}
	var zero V
	return zero, false
}

func (l *LRU[K, V]) Put(k K, v V) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if el, ok := l.entries[k]; ok {
		en := el.Value.(lruEntry[K, V])
		l.curBytes -= en.size
		en.value = v
		en.size = l.sizeOfValue(v)
		el.Value = en
		l.curBytes += en.size
		l.ll.MoveToFront(el)
		l.evict()
		return
	}

	en := lruEntry[K, V]{key: k, value: v, size: l.sizeOfValue(v)}
	el := l.ll.PushFront(en)
	l.entries[k] = el
	l.curBytes += en.size
	l.evict()
}

func (l *LRU[K, V]) evict() {
	for (l.capEntries > 0 && l.ll.Len() > l.capEntries) || (l.capBytes > 0 && l.curBytes > l.capBytes) {
		el := l.ll.Back()
		if el == nil {
			return
		}
		en := el.Value.(lruEntry[K, V])
		delete(l.entries, en.key)
		l.curBytes -= en.size
		l.ll.Remove(el)
	}
}