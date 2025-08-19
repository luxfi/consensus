package utils

import (
	nodeutils "github.com/luxfi/node/utils"
)

// Atomic wraps an atomic value
type Atomic[T any] struct {
	*nodeutils.Atomic[T]
}

// NewAtomic creates a new atomic value
func NewAtomic[T any](val T) *Atomic[T] {
	return &Atomic[T]{
		Atomic: nodeutils.NewAtomic(val),
	}
}

// Get returns the value
func (a *Atomic[T]) Get() T {
	if a.Atomic != nil {
		return a.Atomic.Get()
	}
	var zero T
	return zero
}

// Set sets the value
func (a *Atomic[T]) Set(val T) {
	if a.Atomic != nil {
		a.Atomic.Set(val)
	}
}
