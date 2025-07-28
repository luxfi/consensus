// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package set

import (
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/exp/maps"
)

// The minimum capacity of a set
const minSetSize = 16

var _ json.Marshaler = (*Set[int])(nil)

// Set is a set of elements.
type Set[T comparable] map[T]struct{}

// Of returns a Set initialized with [elts]
func Of[T comparable](elts ...T) Set[T] {
	s := NewSet[T](len(elts))
	s.Add(elts...)
	return s
}

// Return a new set with initial capacity [size].
// More or less than [size] elements can be added to this set.
// Using NewSet() rather than Set[T]{} is just an optimization that can
// be used if you know how many elements will be put in this set.
func NewSet[T comparable](size int) Set[T] {
	if size < 0 {
		return Set[T]{}
	}
	return make(map[T]struct{}, size)
}

func (s *Set[T]) resize(size int) {
	if *s == nil {
		if minSetSize > size {
			size = minSetSize
		}
		*s = make(map[T]struct{}, size)
	}
}

// Add all the elements to this set.
// If the element is already in the set, nothing happens.
func (s *Set[T]) Add(elts ...T) {
	s.resize(2 * len(elts))
	for _, elt := range elts {
		(*s)[elt] = struct{}{}
	}
}

// Union adds all the elements from the provided set to this set.
func (s *Set[T]) Union(set Set[T]) {
	s.resize(2 * set.Len())
	for elt := range set {
		(*s)[elt] = struct{}{}
	}
}

// Difference removes all the elements in [set] from [s].
func (s *Set[T]) Difference(set Set[T]) {
	for elt := range set {
		delete(*s, elt)
	}
}

// Contains returns true iff the set contains this element.
func (s *Set[T]) Contains(elt T) bool {
	_, contains := (*s)[elt]
	return contains
}

// Overlaps returns true if the intersection of the set is non-empty
func (s *Set[T]) Overlaps(big Set[T]) bool {
	small := *s
	if small.Len() > big.Len() {
		small, big = big, small
	}

	for elt := range small {
		if _, ok := big[elt]; ok {
			return true
		}
	}
	return false
}

// Len returns the number of elements in this set.
func (s Set[_]) Len() int {
	return len(s)
}

// Clear empties this set
func (s *Set[_]) Clear() {
	clear(*s)
}

// List converts this set into a list
func (s Set[T]) List() []T {
	return maps.Keys(s)
}

// CappedList returns a list of length at most [size].
// Size should be >= 0. If size < 0, returns nil.
func (s Set[T]) CappedList(size int) []T {
	if size < 0 {
		return nil
	}
	if l := s.Len(); l < size {
		size = l
	}
	elts := make([]T, 0, size)
	for elt := range s {
		if size == 0 {
			break
		}
		size--
		elts = append(elts, elt)
	}
	return elts
}

// Equals returns true if the sets contain the same elements
func (s Set[T]) Equals(other Set[T]) bool {
	return maps.Equal(s, other)
}

// Peek returns a random element. If the set is empty, returns false.
func (s Set[T]) Peek() (T, bool) {
	for elt := range s {
		return elt, true
	}
	var zero T
	return zero, false
}

// Removes [elt] from the set.
func (s *Set[T]) Remove(elts ...T) {
	for _, elt := range elts {
		delete(*s, elt)
	}
}

// Pop returns a random element. If the set is empty returns false.
func (s *Set[T]) Pop() (T, bool) {
	elt, ok := s.Peek()
	if ok {
		s.Remove(elt)
	}
	return elt, ok
}

func (s Set[T]) MarshalJSON() ([]byte, error) {
	lst := s.List()
	// Note: Cannot sort generic type T as it may not be ordered
	// The original implementation likely had constraints on T
	return json.Marshal(lst)
}

func (s *Set[T]) UnmarshalJSON(b []byte) error {
	var slc []T
	if err := json.Unmarshal(b, &slc); err != nil {
		return err
	}
	*s = make(map[T]struct{}, minSetSize)
	for _, elt := range slc {
		(*s)[elt] = struct{}{}
	}
	return nil
}

// String returns the string representation of this set
func (s Set[T]) String() string {
	sb := strings.Builder{}
	sb.WriteString("{")
	first := true
	for elt := range s {
		if !first {
			sb.WriteString(", ")
		}
		first = false
		sb.WriteString(fmt.Sprintf("%v", elt))
	}
	sb.WriteString("}")
	return sb.String()
}