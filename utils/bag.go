// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import (
	"github.com/luxfi/ids"
)

// Bag is a multiset of IDs
type Bag struct {
	counts map[ids.ID]int
}

// NewBag creates a new empty bag
func NewBag() *Bag {
	return &Bag{
		counts: make(map[ids.ID]int),
	}
}

// Add adds an ID to the bag
func (b *Bag) Add(id ids.ID) {
	b.counts[id]++
}

// AddCount adds multiple instances of an ID
func (b *Bag) AddCount(id ids.ID, count int) {
	if count <= 0 {
		return
	}
	b.counts[id] += count
}

// Count returns the number of times an ID appears in the bag
func (b *Bag) Count(id ids.ID) int {
	return b.counts[id]
}

// Remove removes one instance of an ID from the bag
func (b *Bag) Remove(id ids.ID) {
	count := b.counts[id]
	if count <= 1 {
		delete(b.counts, id)
	} else {
		b.counts[id]--
	}
}

// Len returns the total number of items in the bag
func (b *Bag) Len() int {
	total := 0
	for _, count := range b.counts {
		total += count
	}
	return total
}

// List returns all unique IDs in the bag
func (b *Bag) List() []ids.ID {
	list := make([]ids.ID, 0, len(b.counts))
	for id := range b.counts {
		list = append(list, id)
	}
	return list
}

// Mode returns the ID with the highest count and its count
func (b *Bag) Mode() (ids.ID, int) {
	var mode ids.ID
	maxCount := 0
	for id, count := range b.counts {
		if count > maxCount {
			mode = id
			maxCount = count
		}
	}
	return mode, maxCount
}

// Clear removes all items from the bag
func (b *Bag) Clear() {
	b.counts = make(map[ids.ID]int)
}

// Equals returns true if this bag has the same contents as another
func (b *Bag) Equals(other *Bag) bool {
	if len(b.counts) != len(other.counts) {
		return false
	}
	for id, count := range b.counts {
		if other.counts[id] != count {
			return false
		}
	}
	return true
}