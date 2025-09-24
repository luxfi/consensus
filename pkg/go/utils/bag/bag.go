package bag

import "github.com/luxfi/ids"

// Bag is a collection of IDs with counts
type Bag struct {
	counts map[ids.ID]int
}

// New creates a new bag
func New() *Bag {
	return &Bag{
		counts: make(map[ids.ID]int),
	}
}

// Add adds an ID to the bag
func (b *Bag) Add(id ids.ID) {
	b.counts[id]++
}

// Count returns the count for an ID
func (b *Bag) Count(id ids.ID) int {
	return b.counts[id]
}

// Len returns the number of unique IDs
func (b *Bag) Len() int {
	return len(b.counts)
}

// List returns all IDs in the bag
func (b *Bag) List() []ids.ID {
	list := make([]ids.ID, 0, len(b.counts))
	for id := range b.counts {
		list = append(list, id)
	}
	return list
}
