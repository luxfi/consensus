// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import (
	"sort"
	"sync/atomic"
)

// Atomic provides atomic operations
type Atomic[T any] struct {
	value atomic.Value
}

// NewAtomic creates a new atomic value
func NewAtomic[T any](value T) *Atomic[T] {
	a := &Atomic[T]{}
	a.Set(value)
	return a
}

// Get returns the current value
func (a *Atomic[T]) Get() T {
	v := a.value.Load()
	if v == nil {
		var zero T
		return zero
	}
	return v.(T)
}

// Set sets the value
func (a *Atomic[T]) Set(value T) {
	a.value.Store(value)
}

// AtomicBool provides atomic bool operations
type AtomicBool struct {
	value atomic.Bool
}

// NewAtomicBool creates a new atomic bool
func NewAtomicBool(value bool) *AtomicBool {
	a := &AtomicBool{}
	a.Set(value)
	return a
}

// Get returns the current value
func (a *AtomicBool) Get() bool {
	return a.value.Load()
}

// Set sets the value
func (a *AtomicBool) Set(value bool) {
	a.value.Store(value)
}

// AtomicInt provides atomic int64 operations
type AtomicInt struct {
	value atomic.Int64
}

// NewAtomicInt creates a new atomic int
func NewAtomicInt(value int64) *AtomicInt {
	a := &AtomicInt{}
	a.Set(value)
	return a
}

// Get returns the current value
func (a *AtomicInt) Get() int64 {
	return a.value.Load()
}

// Set sets the value
func (a *AtomicInt) Set(value int64) {
	a.value.Store(value)
}

// Add atomically adds delta to the value
func (a *AtomicInt) Add(delta int64) int64 {
	return a.value.Add(delta)
}

// Inc atomically increments the value
func (a *AtomicInt) Inc() int64 {
	return a.Add(1)
}

// Dec atomically decrements the value
func (a *AtomicInt) Dec() int64 {
	return a.Add(-1)
}

// Sortable represents types that can be sorted
type Sortable[T any] interface {
	Compare(T) int
}

// Sort sorts a slice using the provided less function or natural ordering
func Sort[T any](slice []T, less ...func(i, j int) bool) {
	if len(less) > 0 {
		// Use the provided less function
		sort.Slice(slice, less[0])
		return
	}

	// Try to use natural ordering for types with Compare method (like ids.ID)
	sort.Slice(slice, func(i, j int) bool {
		if v1, ok := any(slice[i]).(interface{ Compare(T) int }); ok {
			if result := v1.Compare(slice[j]); result < 0 {
				return true
			}
		}
		return false
	})
}

// Zero returns the zero value of type T.
func Zero[T any]() T {
	var zero T
	return zero
}
