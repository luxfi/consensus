// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package math

import (
	"errors"
	"math"
)

var (
	ErrOverflow  = errors.New("overflow")
	ErrUnderflow = errors.New("underflow")
)

// Add64 returns a + b with overflow detection
func Add64(a, b uint64) (uint64, error) {
	if a > math.MaxUint64-b {
		return 0, ErrOverflow
	}
	return a + b, nil
}

// Sub64 returns a - b with underflow detection
func Sub64(a, b uint64) (uint64, error) {
	if a < b {
		return 0, ErrUnderflow
	}
	return a - b, nil
}

// Mul64 returns a * b with overflow detection
func Mul64(a, b uint64) (uint64, error) {
	if b != 0 && a > math.MaxUint64/b {
		return 0, ErrOverflow
	}
	return a * b, nil
}

// Min returns the minimum of two values
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Max returns the maximum of two values
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Min64 returns the minimum of two uint64 values
func Min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// Max64 returns the maximum of two uint64 values
func Max64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

// AbsDiff returns |a - b|
func AbsDiff(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return b - a
}

// Add is an alias for Add64
func Add(a, b uint64) (uint64, error) {
	return Add64(a, b)
}

// Sub is an alias for Sub64
func Sub(a, b uint64) (uint64, error) {
	return Sub64(a, b)
}

// Mul is an alias for Mul64
func Mul(a, b uint64) (uint64, error) {
	return Mul64(a, b)
}