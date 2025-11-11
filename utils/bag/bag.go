// Package bag provides utilities for vote collection and counting
package bag

// Bag tracks counts of IDs (votes)
type Bag[T comparable] struct {
	counts map[T]int
	size   int
}

// Of creates a bag from elements
func Of[T comparable](elements ...T) Bag[T] {
	b := Bag[T]{
		counts: make(map[T]int),
	}
	for _, e := range elements {
		b.Add(e)
	}
	return b
}

// New creates an empty bag
func New[T comparable]() Bag[T] {
	return Bag[T]{
		counts: make(map[T]int),
	}
}

// Add increments the count for an element
func (b *Bag[T]) Add(element T) {
	b.counts[element]++
	b.size++
}

// AddCount adds multiple counts for an element
func (b *Bag[T]) AddCount(element T, count int) {
	if count <= 0 {
		return
	}
	b.counts[element] += count
	b.size += count
}

// Count returns the count for an element
func (b *Bag[T]) Count(element T) int {
	return b.counts[element]
}

// Len returns total number of elements (with duplicates)
func (b *Bag[T]) Len() int {
	return b.size
}

// Mode returns the element with highest count and its count
func (b *Bag[T]) Mode() (mode T, count int) {
	for element, elementCount := range b.counts {
		if elementCount > count {
			mode = element
			count = elementCount
		}
	}
	return mode, count
}

// List returns all unique elements
func (b *Bag[T]) List() []T {
	list := make([]T, 0, len(b.counts))
	for element := range b.counts {
		list = append(list, element)
	}
	return list
}

// Filter returns a new bag with only elements passing the filter
func (b *Bag[T]) Filter(filter func(T) bool) Bag[T] {
	result := New[T]()
	for element, count := range b.counts {
		if filter(element) {
			result.AddCount(element, count)
		}
	}
	return result
}

// Equals checks if two bags have the same contents
func (b *Bag[T]) Equals(other Bag[T]) bool {
	if b.size != other.size || len(b.counts) != len(other.counts) {
		return false
	}
	for element, count := range b.counts {
		if other.counts[element] != count {
			return false
		}
	}
	return true
}