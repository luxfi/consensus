// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package focus

// Confidence tracks consecutive successful polls for finalization
type Confidence interface {
	Record(success bool)
	Finalized() bool
	Reset()
	Count() int
}

// betaCounter implements the Focus confidence counter
type betaCounter struct {
	beta    int // Target consecutive successes
	current int // Current consecutive count
}

// New creates a new Focus confidence tracker
func New(beta int) Confidence {
	return &betaCounter{beta: beta}
}

// Record updates the confidence counter based on poll success
func (b *betaCounter) Record(success bool) {
	if success {
		b.current++
	} else {
		b.current = 0 // Reset on failure
	}
}

// Finalized returns true if we've reached beta consecutive successes
func (b *betaCounter) Finalized() bool {
	return b.current >= b.beta
}

// Reset resets the confidence counter
func (b *betaCounter) Reset() {
	b.current = 0
}

// Count returns the current consecutive success count
func (b *betaCounter) Count() int {
	return b.current
}