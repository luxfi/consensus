// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

// Preference represents a preference choice (e.g., block index or vertex id)
type Preference int

// State represents the current wave state
type State struct {
	AlphaPref int // Current preference strength
	AlphaConf int // Current confidence level
}

// Tally manages preference and confidence thresholds
type Tally interface {
	Record(pref Preference)
	State() State
	Reset()
}
