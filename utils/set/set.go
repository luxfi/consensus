package set

import "github.com/luxfi/math/set"

// Re-export set types from math package
// Generic type aliases work natively in Go 1.23+
type Set[T comparable] = set.Set[T]

// Of creates a new set
func Of[T comparable](elements ...T) Set[T] {
	return set.Of(elements...)
}

// NewSet creates a new empty set
func NewSet[T comparable](size int) Set[T] {
	return make(Set[T], size)
}
