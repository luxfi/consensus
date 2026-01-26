// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package uptime re-exports github.com/luxfi/validators/uptime for backward compatibility.
package uptime

import (
	"github.com/luxfi/validators/uptime"
)

// LockedCalculator is an alias for uptime.LockedCalculator
type LockedCalculator = uptime.LockedCalculator

// NewLockedCalculator re-exports uptime.NewLockedCalculator
func NewLockedCalculator() LockedCalculator {
	return uptime.NewLockedCalculator()
}

// NewLockedCalculatorWithFallback re-exports uptime.NewLockedCalculatorWithFallback
func NewLockedCalculatorWithFallback(fallback Calculator) LockedCalculator {
	return uptime.NewLockedCalculatorWithFallback(fallback)
}
