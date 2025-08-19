// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import (
	"sync/atomic"
)

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

// Zero returns the zero value of type T.
func Zero[T any]() T {
	var zero T
	return zero
}