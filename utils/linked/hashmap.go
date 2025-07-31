// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package linked

// Hashmap is a linked hashmap that maintains insertion order
type Hashmap[K comparable, V any] struct {
	m     map[K]*hashmapEntry[K, V]
	list  *List[*hashmapEntry[K, V]]
}

type hashmapEntry[K comparable, V any] struct {
	key   K
	value V
	node  *ListNode[*hashmapEntry[K, V]]
}

// NewHashmap creates a new linked hashmap
func NewHashmap[K comparable, V any]() *Hashmap[K, V] {
	return &Hashmap[K, V]{
		m:    make(map[K]*hashmapEntry[K, V]),
		list: NewList[*hashmapEntry[K, V]](),
	}
}

// Put adds or updates a key-value pair
func (h *Hashmap[K, V]) Put(key K, value V) {
	if entry, exists := h.m[key]; exists {
		entry.value = value
		return
	}
	
	entry := &hashmapEntry[K, V]{
		key:   key,
		value: value,
	}
	entry.node = h.list.PushBack(entry)
	h.m[key] = entry
}

// Get retrieves a value by key
func (h *Hashmap[K, V]) Get(key K) (V, bool) {
	if entry, exists := h.m[key]; exists {
		return entry.value, true
	}
	var zero V
	return zero, false
}

// Delete removes a key-value pair
func (h *Hashmap[K, V]) Delete(key K) {
	if entry, exists := h.m[key]; exists {
		h.list.Remove(entry.node)
		delete(h.m, key)
	}
}

// Len returns the number of entries
func (h *Hashmap[K, V]) Len() int {
	return h.list.Len()
}

// Clear removes all entries
func (h *Hashmap[K, V]) Clear() {
	h.m = make(map[K]*hashmapEntry[K, V])
	h.list.Clear()
}

// Iterate calls f for each entry in insertion order
func (h *Hashmap[K, V]) Iterate(f func(K, V) bool) {
	for node := h.list.Front(); node != nil; node = node.Next {
		entry := node.Value
		if !f(entry.key, entry.value) {
			break
		}
	}
}

// NewestEntry returns the most recently added entry
func (h *Hashmap[K, V]) NewestEntry() (K, V, bool) {
	node := h.list.Back()
	if node == nil {
		var zeroK K
		var zeroV V
		return zeroK, zeroV, false
	}
	entry := node.Value
	return entry.key, entry.value, true
}

// OldestEntry returns the oldest entry
func (h *Hashmap[K, V]) OldestEntry() (K, V, bool) {
	node := h.list.Front()
	if node == nil {
		var zeroK K
		var zeroV V
		return zeroK, zeroV, false
	}
	entry := node.Value
	return entry.key, entry.value, true
}

// Iterator for the hashmap
type HashmapIterator[K comparable, V any] struct {
	current *ListNode[*hashmapEntry[K, V]]
	key     K
	value   V
}

// NewIterator returns a new iterator for the hashmap
func (h *Hashmap[K, V]) NewIterator() *HashmapIterator[K, V] {
	return &HashmapIterator[K, V]{
		current: h.list.Front(),
	}
}

// Next advances the iterator and returns true if there's a next element
func (it *HashmapIterator[K, V]) Next() bool {
	if it.current == nil {
		return false
	}
	entry := it.current.Value
	it.key = entry.key
	it.value = entry.value
	it.current = it.current.Next
	return true
}

// Key returns the current key
func (it *HashmapIterator[K, V]) Key() K {
	return it.key
}

// Value returns the current value
func (it *HashmapIterator[K, V]) Value() V {
	return it.value
}