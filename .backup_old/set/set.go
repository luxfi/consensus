// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package set implements a generic set data structure.
package set

import (
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/exp/maps"
)

// Set is a set of unique elements.
type Set[T comparable] map[T]struct{}

// Of returns a Set initialized with [elts].
func Of[T comparable](elts ...T) Set[T] {
	s := make(Set[T], len(elts))
	s.Add(elts...)
	return s
}

// Add adds elements to the set.
func (s Set[T]) Add(elts ...T) {
	for _, elt := range elts {
		s[elt] = struct{}{}
	}
}

// Contains returns true if the set contains the element.
func (s Set[T]) Contains(elt T) bool {
	_, ok := s[elt]
	return ok
}

// Remove removes elements from the set.
func (s Set[T]) Remove(elts ...T) {
	for _, elt := range elts {
		delete(s, elt)
	}
}

// Clear removes all elements from the set.
func (s Set[T]) Clear() {
	maps.Clear(s)
}

// Len returns the number of elements in the set.
func (s Set[T]) Len() int {
	return len(s)
}

// List returns the elements of the set as a slice.
// The order is non-deterministic.
func (s Set[T]) List() []T {
	return maps.Keys(s)
}

// Equals returns true if the sets contain the same elements.
func (s Set[T]) Equals(other Set[T]) bool {
	return maps.Equal(s, other)
}

// Union returns a new set containing all elements from both sets.
func (s Set[T]) Union(other Set[T]) Set[T] {
	result := make(Set[T], max(s.Len(), other.Len()))
	maps.Copy(result, s)
	maps.Copy(result, other)
	return result
}

// Intersection returns a new set containing only elements present in both sets.
func (s Set[T]) Intersection(other Set[T]) Set[T] {
	result := make(Set[T])

	// Iterate over the smaller set for efficiency
	if s.Len() < other.Len() {
		for elt := range s {
			if other.Contains(elt) {
				result.Add(elt)
			}
		}
	} else {
		for elt := range other {
			if s.Contains(elt) {
				result.Add(elt)
			}
		}
	}

	return result
}

// Difference returns a new set containing elements in s that are not in other.
func (s Set[T]) Difference(other Set[T]) Set[T] {
	result := make(Set[T])
	for elt := range s {
		if !other.Contains(elt) {
			result.Add(elt)
		}
	}
	return result
}

// Overlaps returns true if the sets have any elements in common.
func (s Set[T]) Overlaps(other Set[T]) bool {
	// Check the smaller set for efficiency
	if s.Len() < other.Len() {
		for elt := range s {
			if other.Contains(elt) {
				return true
			}
		}
	} else {
		for elt := range other {
			if s.Contains(elt) {
				return true
			}
		}
	}
	return false
}

// MarshalJSON implements json.Marshaler.
func (s Set[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.List())
}

// UnmarshalJSON implements json.Unmarshaler.
func (s *Set[T]) UnmarshalJSON(data []byte) error {
	var elts []T
	if err := json.Unmarshal(data, &elts); err != nil {
		return err
	}
	*s = Of(elts...)
	return nil
}

// String returns a string representation of the set.
func (s Set[T]) String() string {
	var sb strings.Builder
	sb.WriteString("{")
	first := true
	for elt := range s {
		if !first {
			sb.WriteString(", ")
		}
		first = false
		fmt.Fprintf(&sb, "%v", elt)
	}
	sb.WriteString("}")
	return sb.String()
}

// Clone returns a copy of the set.
func (s Set[T]) Clone() Set[T] {
	result := make(Set[T], s.Len())
	maps.Copy(result, s)
	return result
}
