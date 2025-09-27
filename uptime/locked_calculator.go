// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package uptime

// LockedCalculator is a wrapper for a Calculator that ensures thread-safety
type LockedCalculator interface {
	Calculator
}

// NewLockedCalculator returns a new LockedCalculator
func NewLockedCalculator() LockedCalculator {
	return &lockedCalculator{}
}

type lockedCalculator struct {
	NoOpCalculator
}
