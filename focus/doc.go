// Package focus accumulates confidence by counting β consecutive successes.
//
// If wave reports success (both thresholds cleared), Focus advances a counter.
// Reaching β signals local finality for the choice under consideration.
// This is the constructive-interference analogue in the metaphor: persistence,
// not amplitude, creates a stable signal.
package focus
